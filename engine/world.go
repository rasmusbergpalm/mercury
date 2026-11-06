package engine

import "math"

// World is a set of regions linked by traders that pay transport costs, and
// optionally nations that own the regions.
type World struct {
	Rows, Cols int
	Regions    []*Region
	Traders    []*Trader
	Links      []Link
	Nations    []*Nation
	Tick       int
	// Game rules, off in the classic world: firms invest in building levels,
	// people migrate between neighbouring tiles, and populations grow.
	Investment, Migration, Demography bool
	// Spec is the scenario a game world was built from (nil for the classic world).
	Spec *GameSpec
	// Ports lists tiles with ports in the order they opened (which fixes the
	// order of sea traders).
	Ports []int
	// TradePrices are the prices trade happened at in the last tick, per region.
	TradePrices [][]float64

	Armies     []*Army
	NextArmyID int
	Events     []Event

	adj [][][2]int // per tile: (neighbour, link index)
}

// Link is a pair of neighbouring regions and the cost of one hop between them:
// the average of the two tiles' movement costs.
type Link struct {
	A, B int
	Hop  float64
}

// NewWorld wires regions and traders together.
func NewWorld(rows, cols int, regions []*Region, traders []*Trader, links []Link) *World {
	for _, r := range regions {
		r.connect(traders)
	}
	w := &World{Rows: rows, Cols: cols, Regions: regions, Traders: traders, Links: links}
	for _, r := range regions {
		w.TradePrices = append(w.TradePrices, r.Prices())
	}
	return w
}

// Money is all cash in the world; it is conserved.
func (w *World) Money() float64 {
	s := 0.0
	for _, r := range w.Regions {
		s += r.Money()
	}
	for _, t := range w.Traders {
		s += t.Cash
	}
	for _, n := range w.Nations {
		s += n.Treasury
	}
	return s
}

// Soldiers is how many people serve in armies.
func (w *World) Soldiers() float64 {
	s := 0.0
	for _, a := range w.Armies {
		s += a.Men
	}
	return s
}

// Step advances the world one tick.
func (w *World) Step() {
	w.runAI()
	for i, r := range w.Regions {
		copy(w.TradePrices[i], r.refreshPrices())
	}
	for _, r := range w.Regions {
		r.Plan()
		if w.Investment {
			r.planInvestment()
		}
	}
	for _, t := range w.Traders {
		t.Plan()
	}
	for _, n := range w.Nations {
		n.plan(w)
	}
	for _, r := range w.Regions {
		r.LaborMarket()
	}
	for _, r := range w.Regions {
		r.GoodsMarkets()
	}
	for _, t := range w.Traders {
		t.Ship()
	}
	w.trackTrade()
	for _, r := range w.Regions {
		r.Produce()
		for _, f := range r.Firms {
			r.PayDividends(&f.Cash, f.CashBuffer, f.DesiredSpend)
		}
	}
	for _, t := range w.Traders {
		t.Origin.PayDividends(&t.Cash, t.CashBuffer, t.DesiredSpend)
	}
	changed := false
	for _, n := range w.Nations {
		changed = n.settle(w) || changed
		n.rebate()
	}
	w.openNewPorts()
	if changed {
		w.refreshAllHops()
	}
	if len(w.Nations) > 0 {
		w.armies()
	}
	if w.Investment {
		for _, r := range w.Regions {
			r.settleFirms()
		}
	}
	if w.Migration {
		w.migrate()
	}
	if w.Demography {
		for _, r := range w.Regions {
			r.grow()
		}
	}
	w.Tick++
}

// trackTrade updates each nation's moving average of trade with every other.
func (w *World) trackTrade() {
	if len(w.Nations) == 0 {
		return
	}
	k := len(w.Nations)
	flow := make([]float64, k*k)
	for _, t := range w.Traders {
		on, dn := t.Origin.Nation, t.Dest.Nation
		if on != nil && dn != nil && on != dn && t.Shipped > 0 {
			v := t.Shipped * t.Origin.Markets[t.Good].Price
			flow[on.ID*k+dn.ID] += v
			flow[dn.ID*k+on.ID] += v
		}
	}
	for _, n := range w.Nations {
		for j := range n.TradeWith {
			n.TradeWith[j] = 0.98*n.TradeWith[j] + 0.02*flow[n.ID*k+j]
		}
	}
}

// Build starts the next infrastructure level on a tile, paid for by its nation.
func (w *World) Build(region int) error {
	r := w.Regions[region]
	switch {
	case r.Nation == nil:
		return ErrNoNation
	case r.Project != nil:
		return ErrBuilding
	case r.Infra+1 >= len(InfraLevels):
		return ErrMaxLevel
	case !r.Nation.Has(InfraLevels[r.Infra+1].Tech):
		return ErrNeedTech
	}
	r.Project = &Project{Target: r.Infra + 1, Materials: map[GoodID]float64{}}
	return nil
}

// Expand has a tile's nation pay for one more building level of a firm.
func (w *World) Expand(region, firm int) error {
	r := w.Regions[region]
	f := r.Firms[firm]
	switch {
	case r.Nation == nil:
		return ErrNoNation
	case f.Expansion != nil:
		return ErrExpanding
	case !f.Enabled:
		return ErrFirmLocked
	case f.Levels >= f.MaxLevels:
		return ErrFirmMax
	}
	f.Expansion = &Expansion{Public: true}
	return nil
}

// BuildPort starts a port on a coast tile, paid for by its nation. The nation
// needs a tech that allows ports (Ocean sailing).
func (w *World) BuildPort(region int) error {
	r := w.Regions[region]
	switch {
	case r.Nation == nil:
		return ErrNoNation
	case r.Terrain != "coast":
		return ErrNotCoast
	case r.Port:
		return ErrHasPort
	case r.Project != nil:
		return ErrBuilding
	case !r.Nation.CanBuildPorts():
		return ErrNeedTech
	}
	r.Project = &Project{Port: true, Materials: map[GoodID]float64{}}
	return nil
}

// Sea routes: a fixed cost for loading and unloading, plus distance at sea,
// cheaper with the owners' shipping technology.
const (
	portHandling = 0.2
	seaPerHex    = 0.12
)

// HexDistance is the number of hops between two tiles of an odd-r hex grid.
func HexDistance(r1, c1, r2, c2 int) int {
	cube := func(r, c int) (int, int, int) {
		x := c - (r-(r&1))/2
		return x, -x - r, r
	}
	x1, y1, z1 := cube(r1, c1)
	x2, y2, z2 := cube(r2, c2)
	abs := func(v int) int {
		if v < 0 {
			return -v
		}
		return v
	}
	return max(abs(x1-x2), abs(y1-y2), abs(z1-z2))
}

func (w *World) seaHop(a, b *Region) float64 {
	mult := func(r *Region) float64 {
		if r.Nation == nil {
			return 1
		}
		return r.Nation.coastMult()
	}
	d := float64(HexDistance(a.Row, a.Col, b.Row, b.Col))
	return portHandling + seaPerHex*d*(mult(a)+mult(b))/2
}

// openNewPorts opens sea routes for ports finished this tick.
func (w *World) openNewPorts() {
	for _, r := range w.Regions {
		if r.Port && !w.hasPortRoutes(r.ID) {
			w.openPort(r)
		}
	}
}

func (w *World) hasPortRoutes(id int) bool {
	for _, p := range w.Ports {
		if p == id {
			return true
		}
	}
	return false
}

// openPort adds traders for every good both ways between r and every port
// already open.
func (w *World) openPort(r *Region) {
	if !math.IsInf(w.Spec.TransportCost, 1) {
		var added []*Trader
		for _, id := range w.Ports {
			p := w.Regions[id]
			hop := w.seaHop(r, p)
			for _, g := range GameTradable {
				a := NewTrader(g, r, p, hop, w.Spec.TransportCost)
				b := NewTrader(g, p, r, hop, w.Spec.TransportCost)
				a.Sea, b.Sea = true, true
				a.Cash, b.Cash = 0, 0 // money isn't created: owners fund new routes
				added = append(added, a, b)
			}
		}
		r.connect(added)
		for _, id := range w.Ports {
			w.Regions[id].connect(added)
		}
		w.Traders = append(w.Traders, added...)
	}
	w.Ports = append(w.Ports, r.ID)
}

// borders reports whether a tile neighbours one of the nation's tiles.
func (w *World) borders(n *Nation, r *Region) bool {
	for _, nb := range HexNeighbors(r.Row, r.Col, w.Rows, w.Cols) {
		if w.Regions[nb[0]*w.Cols+nb[1]].Nation == n {
			return true
		}
	}
	return false
}

// transfer hands a tile to a nation. Its infrastructure stays; projects and
// state-funded expansions under way are abandoned.
func (w *World) transfer(r *Region, n *Nation) {
	if old := r.Nation; old != nil {
		for i, x := range old.Regions {
			if x == r {
				old.Regions = append(old.Regions[:i:i], old.Regions[i+1:]...)
				break
			}
		}
		if old.Eliminated() {
			for j := range old.AtWar {
				if old.AtWar[j] {
					old.AtWar[j], w.Nations[j].AtWar[old.ID] = false, false
				}
			}
			w.emit(-1, "eliminated", r.ID, "%s has fallen", old.Name)
		}
	}
	r.Siege = nil
	r.Project = nil
	for _, f := range r.Firms {
		f.TFP = f.BaseTFP
		if f.Expansion != nil && f.Expansion.Public {
			f.Expansion = nil
		}
	}
	// (the new owner's applyTech below sets productivity and which firms work)
	r.Nation = n
	n.Regions = append(n.Regions, r)
	n.applyTech()
	w.refreshAllHops()
}

// Annex hands a tile to a nation outright (scenarios, tests).
func (w *World) Annex(region, nation int) { w.transfer(w.Regions[region], w.Nations[nation]) }

// Migration rules (game worlds only).
const (
	migrationRate  = 0.02 // share of a tile's people who move per tick per unit of relative utility gap
	migrationStart = 50   // ticks before anyone moves (utility records need to settle)
	minPopulation  = 20   // a tile never empties below this
)

// migrate moves people along each link from the tile with lower (smoothed)
// utility to the one with higher. Cheaper hops mean more migration. People
// take their cash with them.
func (w *World) migrate() {
	for _, r := range w.Regions {
		u := r.Utility()
		if w.Tick == 0 {
			r.UtilityEMA = u
		}
		r.UtilityEMA = 0.9*r.UtilityEMA + 0.1*u
	}
	if w.Tick < migrationStart {
		return
	}
	for _, l := range w.Links {
		from, to := w.Regions[l.A], w.Regions[l.B]
		if to.UtilityEMA < from.UtilityEMA {
			from, to = to, from
		}
		if from.UtilityEMA <= 0 {
			continue
		}
		gap := math.Min((to.UtilityEMA-from.UtilityEMA)/from.UtilityEMA, 0.3)
		pop := from.Population()
		n := math.Min(migrationRate*gap*pop/(1+l.Hop), pop-minPopulation)
		if n > 1e-9 {
			movePeople(from, to, n)
		}
	}
}

// movePeople moves n people (and their share of cash) between tiles, taking
// from and adding to every pop in proportion to its size.
func movePeople(from, to *Region, n float64) {
	fromPop, toPop := from.Population(), to.Population()
	cash := 0.0
	for _, p := range from.Pops {
		k := n * p.Size / fromPop
		c := p.Cash * k / p.Size
		p.Size -= k
		p.Cash -= c
		cash += c
	}
	for _, p := range to.Pops {
		share := p.Size / toPop
		p.Size += n * share
		p.Cash += cash * share
	}
}

// Population is the number of people in the world.
func (w *World) Population() float64 {
	s := 0.0
	for _, r := range w.Regions {
		s += r.Population()
	}
	return s
}

// CancelBuild abandons a tile's project; what was delivered is lost.
func (w *World) CancelBuild(region int) error {
	r := w.Regions[region]
	if r.Project == nil {
		return ErrNoProject
	}
	r.Project = nil
	return nil
}

// SetInfra sets a tile's infrastructure level directly (scenarios, tests).
func (w *World) SetInfra(region, level int) {
	w.Regions[region].Infra = level
	w.refreshHops(w.Regions[region])
}

func (w *World) refreshAllHops() {
	hop := func(a, b *Region) float64 { return (a.MoveCost() + b.MoveCost()) / 2 }
	for i, l := range w.Links {
		w.Links[i].Hop = hop(w.Regions[l.A], w.Regions[l.B])
	}
	for _, t := range w.Traders {
		if t.Sea {
			t.SetDistance(w.seaHop(t.Origin, t.Dest))
		} else {
			t.SetDistance(hop(t.Origin, t.Dest))
		}
	}
}

// refreshHops recomputes the hop cost of every link and route touching r.
func (w *World) refreshHops(r *Region) {
	hop := func(a, b *Region) float64 { return (a.MoveCost() + b.MoveCost()) / 2 }
	for i, l := range w.Links {
		if l.A == r.ID || l.B == r.ID {
			w.Links[i].Hop = hop(w.Regions[l.A], w.Regions[l.B])
		}
	}
	for _, t := range w.Traders {
		if !t.Sea && (t.Origin == r || t.Dest == r) {
			t.SetDistance(hop(t.Origin, t.Dest))
		}
	}
}

// GrantTech gives a nation a technology outright (scenarios, tests).
func (w *World) GrantTech(nation int, tech string) {
	n := w.Nations[nation]
	n.Known[TechID(tech)] = true
	n.applyTech()
	w.refreshAllHops()
}
