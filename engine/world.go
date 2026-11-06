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
	// and people migrate between neighbouring tiles.
	Investment, Migration bool
	// TradePrices are the prices trade happened at in the last tick, per region.
	TradePrices [][]float64
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

// Step advances the world one tick.
func (w *World) Step() {
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
		n.plan()
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
		changed = n.settle() || changed
		n.rebate()
	}
	if changed {
		w.refreshAllHops()
	}
	if w.Investment {
		for _, r := range w.Regions {
			r.settleFirms()
		}
	}
	if w.Migration {
		w.migrate()
	}
	w.Tick++
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
	case f.Levels >= f.MaxLevels:
		return ErrFirmMax
	}
	f.Expansion = &Expansion{Public: true}
	return nil
}

// Annex makes an unowned tile bordering a nation part of it. The nation pays
// AnnexCostPerHead per inhabitant, to the inhabitants.
func (w *World) Annex(region, nation int) error {
	r, n := w.Regions[region], w.Nations[nation]
	if r.Nation != nil {
		return ErrOwned
	}
	adjacent := false
	for _, nb := range HexNeighbors(r.Row, r.Col, w.Rows, w.Cols) {
		adjacent = adjacent || w.Regions[nb[0]*w.Cols+nb[1]].Nation == n
	}
	if !adjacent {
		return ErrNotAdjacent
	}
	cost := AnnexCostPerHead * r.Population()
	if n.Treasury < cost {
		return ErrTreasury
	}
	n.Treasury -= cost
	pop := r.Population()
	for _, p := range r.Pops {
		p.Cash += cost * p.Size / pop
	}
	r.Nation = n
	n.Regions = append(n.Regions, r)
	n.applyTech()
	w.refreshAllHops()
	return nil
}

// AnnexCost is what annexing a tile would cost now.
func (w *World) AnnexCost(region int) float64 {
	return AnnexCostPerHead * w.Regions[region].Population()
}

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
		t.SetDistance(hop(t.Origin, t.Dest))
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
		if t.Origin == r || t.Dest == r {
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
