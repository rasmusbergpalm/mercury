package engine

import (
	"math"
	"sort"
)

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
	History    History // game worlds only

	adj  [][][2]int // per tile: (neighbour, link index)
	grid []int      // per cell of the map (row*Cols+col): its region, or -1 for sea
	// shortest paths through any land, by start (see allPaths); cleared
	// when a hop changes
	paths map[int]cachedPaths
}

// Link is a pair of neighbouring regions and the cost of one hop between them:
// the average of the two tiles' movement costs.
type Link struct {
	A, B int
	Hop  float64
}

// Each tile's trading house starts with this much cash per route.
const houseCashPerRoute = 1.0

// NewWorld wires regions and traders together: every route belongs to the
// trading house of the tile it leaves from.
func NewWorld(rows, cols int, regions []*Region, traders []*Trader, links []Link) *World {
	connect(traders)
	for _, r := range regions {
		h := r.House
		h.Cash = houseCashPerRoute * float64(len(h.Routes))
		h.CashBuffer = h.Cash
	}
	w := &World{Rows: rows, Cols: cols, Regions: regions, Traders: traders, Links: links}
	for _, r := range regions {
		w.TradePrices = append(w.TradePrices, r.Prices())
	}
	w.grid = make([]int, rows*cols)
	for i := range w.grid {
		w.grid[i] = -1
	}
	for _, r := range regions {
		w.grid[r.Row*cols+r.Col] = r.ID
	}
	w.adj = make([][][2]int, len(regions))
	for i, l := range links {
		w.adj[l.A] = append(w.adj[l.A], [2]int{l.B, i})
		w.adj[l.B] = append(w.adj[l.B], [2]int{l.A, i})
	}
	return w
}

// RegionAt is the tile at a map cell, or nil for sea (or off the map).
func (w *World) RegionAt(row, col int) *Region {
	if row < 0 || row >= w.Rows || col < 0 || col >= w.Cols {
		return nil
	}
	if id := w.grid[row*w.Cols+col]; id >= 0 {
		return w.Regions[id]
	}
	return nil
}

// landNeighbours calls f for each land tile next to r.
func (w *World) landNeighbours(r *Region, f func(nb *Region) bool) {
	for _, x := range w.adj[r.ID] {
		if !f(w.Regions[x[0]]) {
			return
		}
	}
}

// Money is all cash in the world; it is conserved.
func (w *World) Money() float64 {
	s := 0.0
	for _, r := range w.Regions {
		s += r.Money()
	}
	for _, r := range w.Regions {
		s += r.House.Cash
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

// Step advances the world one tick. The tiles' own work runs in parallel
// (see eachRegion); what they owe each other is settled in order after.
func (w *World) Step() {
	w.runAI()
	w.eachRegion(func(r *Region) {
		copy(w.TradePrices[r.ID], r.refreshPrices())
		r.Plan()
		if w.Investment {
			r.planInvestment()
		}
	})
	w.eachRegion(func(r *Region) { r.House.Plan(w.Tick) })
	for _, n := range w.Nations {
		n.plan(w)
	}
	w.eachRegion(func(r *Region) {
		r.LaborMarket()
		r.GoodsMarkets()
	})
	w.settleLedgers()
	w.eachRegion(func(r *Region) { r.shipAll(len(w.Nations)) })
	w.trackTrade()
	w.eachRegion(func(r *Region) {
		r.Produce()
		owners := r.owners()
		for _, f := range r.Firms {
			if f.State {
				continue
			}
			// keep cash for what the firm wants to spend, but no more than it
			// can grow into: twice what it actually spent today
			r.PayDividends(&f.Cash, f.CashBuffer, math.Min(f.DesiredSpend, 2*f.tickCost+f.CashBuffer), owners)
		}
		// like firms: hold what today's plans need, but no more than the
		// house can grow into (twice what it actually spent today)
		h := r.House
		r.PayDividends(&h.Cash, h.CashBuffer, math.Min(h.DesiredSpend, 2*h.Spent+h.CashBuffer), owners)
	})
	w.settleLedgers()
	changed := false
	for _, n := range w.Nations {
		changed = n.settle(w) || changed
	}
	if w.Spec != nil {
		w.serviceEffects()
	}
	w.openNewPorts()
	if changed {
		w.refreshAllHops()
	}
	if len(w.Nations) > 0 {
		w.armies()
	}
	if w.Investment {
		w.eachRegion((*Region).settleFirms)
	}
	if w.Migration {
		w.migrate()
	}
	w.Tick++
	w.eachRegion(func(r *Region) {
		if w.Demography {
			r.grow()
		}
		if w.Spec != nil {
			r.accumulateGDP()
		}
	})
	if w.Spec != nil {
		if w.Tick%HistoryEvery == 0 {
			w.record()
		}
	}
}

// trackTrade updates each nation's moving average of trade with every other.
func (w *World) trackTrade() {
	if len(w.Nations) == 0 {
		return
	}
	k := len(w.Nations)
	flow := make([]float64, k*k)
	for _, r := range w.Regions { // each tile's exports, summed as it shipped them
		on := r.Nation
		for j, v := range r.exportsTo {
			if on != nil && v > 0 {
				flow[on.ID*k+j] += v
				flow[j*k+on.ID] += v
			}
		}
	}
	for _, n := range w.Nations {
		for j := range n.TradeWith {
			n.TradeWith[j] = 0.98*n.TradeWith[j] + 0.02*flow[n.ID*k+j]
		}
	}
}

// shipAll sends what each of the tile's routes can ship today and sums
// what it shipped to every other nation (for trackTrade).
func (r *Region) shipAll(nations int) {
	if len(r.exportsTo) != nations {
		r.exportsTo = make([]float64, nations)
	}
	for j := range r.exportsTo {
		r.exportsTo[j] = 0
	}
	on := r.Nation
	for _, t := range r.House.today { // the rest are idle: they ship nothing
		t.Ship()
		if dn := t.Dest.Nation; on != nil && dn != nil && on != dn && t.Shipped > 0 {
			r.exportsTo[dn.ID] += t.Shipped * r.Markets[t.Good].Price
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
	case !r.Coastal:
		return ErrNotCoast
	case r.Port:
		return ErrHasPort
	case r.Project != nil:
		return ErrBuilding
	case !r.Nation.CanBuildPorts():
		return ErrNeedTech
	case r.Nation.Ports() >= r.Nation.PortAllowance():
		return ErrPortLimit
	}
	r.Project = &Project{Port: true, Materials: map[GoodID]float64{}}
	return nil
}

// TilesPerPort: a nation may have one port for every TilesPerPort tiles it
// holds, and at least one. Every port trades with every other, so the
// routes grow with the square of the ports: the allowance keeps them to
// about the number of land routes on the biggest map. A nation that
// conquers ports keeps them, but builds no more until it is back under
// its allowance.
const TilesPerPort = 16

// PortAllowance is how many ports the nation may have.
func (n *Nation) PortAllowance() int { return max(1, len(n.Regions)/TilesPerPort) }

// Ports is how many ports the nation has, counting those being built.
func (n *Nation) Ports() int {
	k := 0
	for _, r := range n.Regions {
		if r.Port || (r.Project != nil && r.Project.Port) {
			k++
		}
	}
	return k
}

// RemovePort pulls down a tile's port: its sea routes close, and what
// they had bought or had at sea is lost. (Not in games whose ports trade
// only with their nearest few, see GameSpec.SeaRoutes: their routes depend
// on which ports were open when each was built.)
func (w *World) RemovePort(region int) error {
	r := w.Regions[region]
	switch {
	case r.Nation == nil:
		return ErrNoNation
	case !r.Port:
		return ErrNoPort
	case w.Spec != nil && w.Spec.SeaRoutes > 0:
		return ErrOldPorts
	}
	r.Port = false
	gone := func(t *Trader) bool { return t.Sea && (t.Origin == r || t.Dest == r) }
	keep := func(ts []*Trader) []*Trader {
		out := ts[:0]
		for _, t := range ts {
			if !gone(t) {
				out = append(out, t)
			}
		}
		return out
	}
	w.Traders = keep(w.Traders)
	for _, x := range w.Regions { // every port had routes to and from this one
		if !x.Port && x != r {
			continue
		}
		h := x.House
		h.Routes = keep(h.Routes)
		for i, t := range h.Routes {
			t.slot = i
		}
		h.busyKnown, h.today = false, h.today[:0]
		for g := range x.outbound {
			x.outbound[g] = keep(x.outbound[g])
			x.inbound[g] = keep(x.inbound[g])
			x.bidders[g] = x.bidders[g][:0]
		}
	}
	for i, id := range w.Ports {
		if id == r.ID {
			w.Ports = append(w.Ports[:i:i], w.Ports[i+1:]...)
			break
		}
	}
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
		for _, id := range w.seaPartners(r) {
			p := w.Regions[id]
			hop := w.seaHop(r, p)
			for _, g := range GameTradable {
				a := NewTrader(g, r, p, hop, w.Spec.TransportCost)
				b := NewTrader(g, p, r, hop, w.Spec.TransportCost)
				a.Sea, b.Sea = true, true // the houses fund them from their purse
				added = append(added, a, b)
			}
		}
		connect(added)
		w.Traders = append(w.Traders, added...)
	}
	w.Ports = append(w.Ports, r.ID)
}

// seaPartners are the open ports a new port at r trades with: every one, or
// the Spec's SeaRoutes nearest (the earlier opened first among equals).
func (w *World) seaPartners(r *Region) []int {
	k := w.Spec.SeaRoutes
	if k <= 0 || len(w.Ports) <= k {
		return w.Ports
	}
	ids := append([]int(nil), w.Ports...)
	d := func(id int) int { p := w.Regions[id]; return HexDistance(r.Row, r.Col, p.Row, p.Col) }
	sort.SliceStable(ids, func(a, b int) bool { return d(ids[a]) < d(ids[b]) })
	return ids[:k]
}

// borders reports whether a tile neighbours one of the nation's tiles.
func (w *World) borders(n *Nation, r *Region) bool {
	found := false
	w.landNeighbours(r, func(nb *Region) bool {
		found = nb.Nation == n
		return !found
	})
	return found
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
	r.applyAllegiance() // its people's loyalties stay as they were: to the old owner, or to the new one if it had it before
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
// foreignMigration is how much slower people move between tiles under
// different flags (or between a nation and free land) than within one
// nation: language, law and distance from home hold them back. A 10% gap in
// living standards draws about 2% of a border tile's people a year: as
// emigration ran in the period (faster, it emptied poorer nations into
// richer ones within decades).
const foreignMigration = 0.03

// migrationShare is the share of from's people who move to to in a day,
// for a relative gap in living standards and the hop between them.
func migrationShare(from, to *Region, gap, hop float64) float64 {
	rate := migrationRate
	if from.Nation != to.Nation {
		rate *= foreignMigration // leaving one's country is a bigger step
	}
	return rate * gap / (1 + hop)
}

func (w *World) migrate() {
	w.eachRegion(func(r *Region) {
		u := r.Utility()
		if w.Tick == 0 {
			r.UtilityEMA = u
		}
		r.UtilityEMA = 0.9*r.UtilityEMA + 0.1*u
	})
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
		n := math.Min(migrationShare(from, to, gap, l.Hop)*pop, sizeOf(movable(from))-minPopulation) // never below minPopulation workers
		if n > 1e-9 {
			movePeople(from, to, n)
		}
	}
}

// movable are the pops that move, get drafted and come home: the workers
// (or everyone, where there are no classes).
func movable(r *Region) []*Pop {
	if w := r.ClassPops(Workers); len(w) > 0 && len(w) < len(r.Pops) {
		return w
	}
	return r.Pops
}

func sizeOf(pops []*Pop) float64 {
	s := 0.0
	for _, p := range pops {
		s += p.Size
	}
	return s
}

// movePeople moves n workers (and their share of cash) between tiles,
// taking from and adding to every worker pop in proportion to its size.
func movePeople(from, to *Region, n float64) {
	src, dst := movable(from), movable(to)
	fromPop, toPop := sizeOf(src), sizeOf(dst)
	n = math.Min(n, fromPop*0.5)
	cash := 0.0
	for _, p := range src {
		k := n * p.Size / fromPop
		c := p.Cash * k / p.Size
		p.Size -= k
		p.Cash -= c
		cash += c
	}
	for _, p := range dst {
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
	w.paths = nil
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
	w.paths = nil
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

// IsCity reports whether a tile has grown into a city: it holds at least
// CityMult times the people of the average tile.
func (w *World) IsCity(r *Region) bool {
	pop := 0.0
	for _, x := range w.Regions {
		pop += x.Population()
	}
	return r.Population() >= CityMult*pop/float64(len(w.Regions))
}
