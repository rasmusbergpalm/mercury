package engine

// World is a set of regions linked by traders that pay transport costs.
type World struct {
	Rows, Cols int
	Regions    []*Region
	Traders    []*Trader
	Links      []Link
	Tick       int
	// TradePrices are the prices trade happened at in the last tick, per region.
	TradePrices [][]float64
}

// Link is a pair of neighbouring regions and the cost of one hop between them.
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
	w.Tick++
}
