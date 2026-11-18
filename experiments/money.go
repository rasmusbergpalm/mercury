package experiments

import . "github.com/rasmusbergpalm/mercury/economy"

func init() {
	register(Experiment{"money", "the quantity theory: double or halve the money, everywhere or in one tile", runMoney})
}

// runMoney tests whether money is neutral. Hume (1752) argued, and the
// quantity theory holds, that if every coin in a country were doubled
// overnight, prices would double and nothing real would change: the same
// goods would be made and traded at twice the pounds. In the short run a
// change in money moves output too, while prices catch up.
//
// A year after the map has settled, every pound held by its people, firms
// and merchants is multiplied by a factor: 2 and 1/2, against a run where
// nothing happens. Month by month: the price level (the cost of the basket
// the map's people bought the day before), the average wage, real output
// (each tile's net output valued at its prices the day before), and the
// money in the world.
//
// Then Hume's own experiment, his price-specie flow: the money of one tile
// alone is doubled. Its prices rise above its neighbours', so it buys from
// them and sells them less, and money flows out until prices are even
// again: the extra money spreads over the map, raising every price a
// little.
func runMoney(o *Out) {
	shockDay := settle + days
	type series struct {
		Factor       float64
		Tile         int // the one tile whose money changed (-1: all)
		Months       []int
		Prices, Wage []float64 // the map's price level and average wage, against the day before the change
		Real         []float64 // real output, against the day before
		Money        []float64 // the world's money, against the day before
		// one tile alone: its price level and its share of the money, and
		// its neighbours' price level
		TilePrices, TileMoney, NearPrices []float64 `json:",omitempty"`
	}
	run := func(name string, factor float64, tile int, out *series) func() {
		return func() {
			w := settled()
			record, done := o.Recorder(name)
			defer done()
			out.Factor, out.Tile = factor, tile
			n := len(w.Regions)
			var basket [][]float64 // per tile: what its people bought the day before the change
			var p0 [][]float64     // and its prices then
			near := map[int]bool{}
			if tile >= 0 {
				for _, x := range w.Adjacent(tile) {
					near[x[0]] = true
				}
			}
			level := func(f func(i int) bool) float64 { // the cost of the basket at today's prices, against then
				now, then := 0.0, 0.0
				for i, r := range w.Regions {
					if !f(i) {
						continue
					}
					for g, q := range basket[i] {
						now += q * r.Markets[g].Price
						then += q * p0[i][g]
					}
				}
				return now / then
			}
			real := func() float64 { // net output at the prices of the day before the change
				v := 0.0
				for i, r := range w.Regions {
					for _, f := range r.Firms {
						v += f.LastOutput * p0[i][f.Output]
						for g, q := range f.Used {
							v -= q * p0[i][g]
						}
					}
				}
				return v
			}
			var real0, money0, wage0, share0 float64
			wage := func() float64 {
				s, people := 0.0, 0.0
				for _, r := range w.Regions {
					s += r.Wage * r.Population()
					people += r.Population()
				}
				return s / people
			}
			for d := settle; d < end; d++ {
				if d == shockDay {
					basket, p0 = make([][]float64, n), make([][]float64, n)
					for i, r := range w.Regions {
						basket[i], p0[i] = make([]float64, NumGoods), r.Prices()
						for _, p := range r.Pops {
							for g, q := range p.Consumed {
								basket[i][g] += q
							}
						}
					}
					real0, money0, wage0 = real(), w.Money(), wage()
					if tile >= 0 {
						share0 = w.Regions[tile].Money() / money0
					}
					// the change: every pound in the hands of people, firms and
					// merchants is multiplied (the economy has no lever for
					// money: it is written in directly)
					for i, r := range w.Regions {
						if tile >= 0 && i != tile {
							continue
						}
						for _, p := range r.Pops {
							p.Cash *= factor
						}
						for _, f := range r.Firms {
							f.Cash *= factor
						}
						r.House.Cash *= factor
					}
				}
				w.Step()
				record(w)
				if d < shockDay || (d-shockDay)%30 != 0 {
					continue
				}
				out.Months = append(out.Months, (d-shockDay)/30)
				out.Prices = append(out.Prices, level(func(int) bool { return true }))
				out.Wage = append(out.Wage, wage()/wage0)
				out.Real = append(out.Real, real()/real0)
				out.Money = append(out.Money, w.Money()/money0)
				if tile >= 0 {
					out.TilePrices = append(out.TilePrices, level(func(i int) bool { return i == tile }))
					out.TileMoney = append(out.TileMoney, w.Regions[tile].Money()/w.Money()/share0)
					out.NearPrices = append(out.NearPrices, level(func(i int) bool { return near[i] }))
				}
			}
		}
	}
	const humeTile = 14 // an inland tile in the middle of the map
	runs := make([]series, 4)
	parallel(run("control", 1, -1, &runs[0]), run("double", 2, -1, &runs[1]), run("halve", 0.5, -1, &runs[2]), run("hume", 2, humeTile, &runs[3]))
	// the control run's own drift, divided out of the others: the change's effect alone
	for k := 1; k < len(runs); k++ {
		for m := range runs[k].Months {
			runs[k].Prices[m] /= runs[0].Prices[m]
			runs[k].Wage[m] /= runs[0].Wage[m]
			runs[k].Real[m] /= runs[0].Real[m]
		}
	}
	o.Write("money.json", map[string]any{"changeMonth": 12, "humeTile": humeTile, "runs": runs})
}
