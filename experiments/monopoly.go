package experiments

import (
	"math"

	. "github.com/rasmusbergpalm/mercury/economy"
)

func init() {
	register(Experiment{"monopoly", "a monopoly brewer, and how trade erodes its markup", runMonopoly})
}

// runMonopoly makes one tile's brewery a monopoly. No firm in the economy
// has market power unless it is given it: each takes its market's price as
// given and makes what pays at that price. A monopoly instead reckons with
// how its own sales lower the price, and makes less, to sell dearer
// (Firm.Monopoly). Brewers were a classic local monopoly (the tied
// house). Merchants bringing beer from the neighbours compete with it as
// far as carriage allows; so that only the cost of carriage stands in their
// way, beer here doesn't spoil (in the example economy a tenth of it is
// lost on each hop, which keeps the local brewer safe from all but the
// nearest rivals).
//
// The map settles at the normal cost of transport. Then only beer's carriage
// changes, from four times the normal (little trade) to nearly free, as if
// beer were that many times as bulky (a route's transport per unit is its
// cost times its length times the good's bulk); everything else carries as
// before, so each run is the same economy but for beer. The brewery of a
// tile in the middle of the map becomes a monopoly, against a run where it
// stays as it was. Its
// demand's elasticity is 2: beer substitutes for tea within the need for
// drink at an elasticity of 2 (data/needs.json). After Years, the last
// year's averages: the price of beer in the tile, the cheapest a merchant
// can land it there for, the brewery's output and profit, the beer brought in from other tiles, the brewery's share of the
// beer sold in the tile, and the tile's living standard.
func runMonopoly(o *Out) {
	const tile = 14 // an inland tile in the middle of the map
	beer := G("beer")
	type result struct {
		Cost                   float64 // beer's cost of transport, a multiple of the normal
		Monopoly               bool
		Price, Output, Profit  float64 // in the tile: beer's price, and the brewery's output and profit a day
		Imports, Share, Living float64 // beer brought in a day, the brewery's share of the beer sold in the tile, the living standard
		Labor                  float64 // the brewery's workers
		Landed                 float64 // the cheapest a merchant can land beer in the tile: a neighbour's price plus the carriage
	}
	costs := []float64{4, 2, 1, 0.5, 0.25, 0.125, 0.0625, 0.03125}
	results := make([]result, 2*len(costs))
	var jobs []func()
	for i, c := range costs {
		for k, mono := range []bool{false, true} {
			out := &results[2*i+k]
			c, mono := c, mono
			jobs = append(jobs, func() {
				w := settledAt(1)
				for _, t := range w.Traders { // beer c times as bulky
					if t.Good == beer {
						t.Tau *= c
					}
				}
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
				samples, sold, traded := 0.0, 0.0, 0.0
				for d := settle; d < end; d++ {
					stock := brewery.Inventory
					w.Step()
					if d < end-days {
						continue
					}
					samples++
					// what it sold today (beer here doesn't spoil), against all
					// the beer the tile's market sold
					m := r.Markets[beer]
					sold += stock + brewery.LastOutput - brewery.Inventory
					traded += math.Min(m.Supply, m.Demand)
					out.Price += r.Markets[beer].Price
					out.Output += brewery.LastOutput
					out.Profit += brewery.TickRevenue - brewery.TickCost
					out.Labor += brewery.Labor
					out.Living += r.Utility()
					in, landed := 0.0, math.Inf(1)
					for _, t := range w.Traders {
						if t.Good == beer && t.Dest == r {
							in += t.Shipped * t.Arriving
							o := t.Origin.Markets
							landed = math.Min(landed, (o[beer].Price+t.Tau*o[Transport].Price)/t.Arriving)
						}
					}
					out.Landed += landed
					out.Imports += in
				}
				for _, x := range []*float64{&out.Price, &out.Output, &out.Profit, &out.Imports, &out.Living, &out.Labor, &out.Landed} {
					*x /= samples
				}
				if traded > 0 {
					out.Share = sold / traded
				}
			})
		}
	}
	withSpoilage("beer", 0, func() { parallel(jobs...) })
	o.Write("monopoly.json", map[string]any{"tile": tile, "elasticity": 2, "runs": results})
}
