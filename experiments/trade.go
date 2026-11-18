package experiments

import "math"

func init() {
	register(Experiment{"trade", "trade across transport costs: wages evening out, and tiles specialising", runTrade})
}

func runTrade(o *Out) {
	// ---- trade, wages and specialisation across transport costs: the map
	// settles at each cost, with people free to move or not, and the last
	// year is measured: how far apart wages are between tiles (factor-price
	// equalisation: trade in goods alone pulls them together), and how
	// specialised tiles are (Krugman's index on workers: half the sum over
	// trades of the gap between a tile's share of its workers in a trade and
	// the map's; on workers, since at high costs prices differ so much
	// between tiles that output values can't be compared),
	// and each tile's share of workers on farms (comparative advantage: with
	// cheap carriage, farming moves to the land best at it)
	type tradeRun struct {
		Cost           float64
		Migration      bool
		WageSpread     float64   // the people-weighted coefficient of variation of wages across tiles
		Specialisation float64   // Krugman's index on workers, averaged over tiles weighted by workers
		FarmShare      []float64 // per tile: share of its workers on farms (grain and potatoes)
	}
	tradeCosts := []float64{}
	for k := -4; k <= 7; k++ {
		tradeCosts = append(tradeCosts, math.Pow(2, float64(k)))
	}
	tradeCosts = append(tradeCosts, 1e6)
	tradeRuns := make([]tradeRun, 2*len(tradeCosts))
	var tradeJobs []func()
	for i, c := range tradeCosts {
		for k, mig := range []bool{false, true} {
			out := &tradeRuns[2*i+k]
			c, mig := c, mig
			tradeJobs = append(tradeJobs, func() {
				w := worldAt(c)
				out.Cost, out.Migration, out.FarmShare = c, mig, make([]float64, len(w.Regions))
				samples := 0.0
				for d := 0; d < settle; d++ {
					if d == settle/3 {
						w.Migration = mig // (people sort themselves out first, then stay or not)
					}
					w.Step()
					if d < settle-days || d%30 != 0 {
						continue
					}
					samples++
					wsum, wsq, people := 0.0, 0.0, 0.0
					mapOut := map[string]float64{}
					tileOut := make([]map[string]float64, len(w.Regions))
					for i, r := range w.Regions {
						p := r.Population()
						wsum += r.Wage * p
						wsq += r.Wage * r.Wage * p
						people += p
						tileOut[i] = map[string]float64{}
						lab, farm := 0.0, 0.0
						for _, f := range r.Firms {
							tileOut[i][f.Name] += f.Labor
							mapOut[f.Name] += f.Labor
							lab += f.Labor
							if f.Name == "Farm" || f.Name == "Potato farm" {
								farm += f.Labor
							}
						}
						out.FarmShare[i] += farm / math.Max(lab, 1e-9)
					}
					mean := wsum / people
					out.WageSpread += math.Sqrt(math.Max(0, wsq/people-mean*mean)) / mean
					total := 0.0
					for _, v := range mapOut {
						total += v
					}
					ksi, weight := 0.0, 0.0
					for i := range w.Regions {
						tt := 0.0
						for _, v := range tileOut[i] {
							tt += v
						}
						if tt <= 0 {
							continue
						}
						k := 0.0
						for g := range mapOut {
							k += math.Abs(tileOut[i][g]/tt - mapOut[g]/total)
						}
						ksi += k / 2 * tt
						weight += tt
					}
					out.Specialisation += ksi / weight
				}
				out.WageSpread /= samples
				out.Specialisation /= samples
				for i := range out.FarmShare {
					out.FarmShare[i] /= samples
				}
			})
		}
	}

	parallel(tradeJobs...)
	o.Write("trade.json", map[string]any{"runs": tradeRuns})
}
