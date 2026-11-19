package experiments

import . "github.com/rasmusbergpalm/mercury/economy"

func init() {
	register(Experiment{"monopoly", "a monopoly brewer, and how trade erodes its markup", runMonopoly})
}

// runMonopoly makes one tile's brewery a monopoly. No firm in the economy
// has market power unless it is given it: each takes its market's price as
// given and makes what pays at that price. A monopoly instead reckons with
// how its own sales lower the price, and makes less, to sell dearer
// (Firm.Monopoly). Brewers were a classic local monopoly (the tied
// house), and beer keeps well enough to travel, so merchants bringing beer
// from the neighbours compete with it as far as carriage allows.
//
// The map settles at each cost of transport, from four times the normal
// (little trade) to nearly free carriage; then the brewery of a tile in the middle of
// the map becomes a monopoly, against a run where it stays as it was. Its
// demand's elasticity is 2: beer substitutes for tea within the need for
// drink at an elasticity of 2 (data/needs.json). After Years, the last
// year's averages: the price of beer in the tile, the brewery's output and
// profit, the beer brought in from other tiles, the monopoly's share of the
// tile's market, and the tile's living standard.
func runMonopoly(o *Out) {
	const tile = 14 // an inland tile in the middle of the map
	beer := G("beer")
	type result struct {
		Cost                   float64 // the cost of transport, a multiple of the normal (autarky: no trade)
		Monopoly               bool
		Price, Output, Profit  float64 // in the tile: beer's price, and the brewery's output and profit a day
		Imports, Share, Living float64 // beer brought in a day, the brewery's share of what was offered, the living standard
		Labor                  float64 // the brewery's workers
	}
	costs := []float64{4, 2, 1, 0.5, 0.25, 0.125, 0.0625, 0.03125}
	results := make([]result, 2*len(costs))
	var jobs []func()
	for i, c := range costs {
		for k, mono := range []bool{false, true} {
			out := &results[2*i+k]
			c, mono := c, mono
			jobs = append(jobs, func() {
				w := settledAt(c)
				out.Cost, out.Monopoly = c, mono
				r := w.Regions[tile]
				var brewery *Firm
				for _, f := range r.Firms {
					if f.Output == beer {
						brewery = f
					}
				}
				if mono {
					brewery.Monopoly, brewery.Elasticity = true, 2
				}
				samples := 0.0
				for d := settle; d < end; d++ {
					w.Step()
					if d < end-days {
						continue
					}
					samples++
					out.Price += r.Markets[beer].Price
					out.Output += brewery.LastOutput
					out.Profit += brewery.TickRevenue - brewery.TickCost
					out.Labor += brewery.Labor
					out.Living += r.Utility()
					made, in := brewery.LastOutput, 0.0
					for _, t := range w.Traders {
						if t.Good == beer && t.Dest == r {
							in += t.Shipped * t.Arriving
						}
					}
					out.Imports += in
					if made+in > 0 {
						out.Share += made / (made + in)
					}
				}
				for _, x := range []*float64{&out.Price, &out.Output, &out.Profit, &out.Imports, &out.Share, &out.Living, &out.Labor} {
					*x /= samples
				}
			})
		}
	}
	parallel(jobs...)
	o.Write("monopoly.json", map[string]any{"tile": tile, "elasticity": 2, "runs": results})
}
