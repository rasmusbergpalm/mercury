package engine

// World is a set of regions linked by traders that pay transport costs, and
// optionally nations that own the regions.
type World struct {
	Rows, Cols int
	Regions    []*Region
	Traders    []*Trader
	Links      []Link
	Nations    []*Nation
	Tick       int
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
	for _, n := range w.Nations {
		for _, r := range n.settle() {
			w.refreshHops(r)
		}
		n.rebate()
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
	}
	r.Project = &Project{Target: r.Infra + 1}
	return nil
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
