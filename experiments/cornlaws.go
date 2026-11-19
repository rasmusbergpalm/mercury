package experiments

import (
	"fmt"

	. "github.com/rasmusbergpalm/mercury/economy"
)

func init() {
	register(Experiment{"cornlaws", "the Corn Laws: a duty on grain into a country, from none to an embargo", runCornLaws})
}

// runCornLaws asks Ricardo's question. From 1815 Britain's Corn Laws kept
// out foreign grain unless the price at home was high; they were repealed
// in 1846. Ricardo (1815, 1817) argued that they raised the price of
// bread, and with it the rents of the landowners who grew grain, at the
// expense of the workers who ate it and the manufacturers who paid their
// wages, and kept Britain on farms it would do better to leave for its
// mills, its mines and its trade.
//
// "Britain" is the map's two northern rows (inBritain): hills and mountains
// with coal, iron and sheep, and some forest and plains; the rest of the map, with
// its plains and tropics, is "the Continent". From the settled map, a duty
// is laid on grain entering Britain from the Continent, at each of several
// rates, up to an embargo; the duty Britain's tiles collect goes back to
// their people as a lump sum each day (otherwise it would leave the
// economy). After Years, the last year's averages: the price of grain and
// bread in Britain and on the Continent, Britain's grain imports, its
// people's jobs by trade, its incomes by source (wages, the farms' profits
// and the other firms'), and its living standard.
func runCornLaws(o *Out) {
	britain := inBritain
	grain, bread := G("grain"), G("bread")
	type result struct {
		Duty                     float64 // the duty, a share of the price; -1: an embargo
		GrainPrice, BreadPrice   float64 // in Britain, people-weighted
		GrainPriceAbroad         float64 // on the Continent
		Imports                  float64 // grain into Britain a day
		Farm, Land, Industry     float64 // Britain's workers: on grain and potato farms, on other land, in industry...
		Service                  float64 // ...and in services
		Wages, FarmProfit        float64 // Britain's incomes a day: the wage bill, the farms' profits...
		OtherProfit, DutyRevenue float64 // ...the other firms' profits, and the duty handed back
		RealWage, Living         float64 // the wage in loaves of bread, and the living standard, in Britain
		People                   float64 // Britain's people
		ContinentFarm            float64 // the Continent's workers on grain and potato farms
		GrainMade                float64 // grain Britain's farms make a day
	}
	run := func(duty float64, out *result) func() {
		return func() {
			w := settled()
			name := fmt.Sprintf("duty%g", 100*duty)
			if duty < 0 {
				name = "embargo"
			}
			record, done := o.Recorder(name)
			defer done()
			out.Duty = duty
			for _, t := range w.Traders {
				if t.Good == grain && britain(t.Dest) && !britain(t.Origin) {
					if duty < 0 {
						t.Closed = true
					} else {
						t.Tariff = duty
					}
				}
			}
			samples := 0.0
			for d := settle; d < end; d++ {
				w.Step()
				record(w)
				// the duty goes back to Britain's people, by head (the
				// economy has no transfers: it is written in directly)
				pool, people := 0.0, 0.0
				for _, r := range w.Regions {
					if britain(r) {
						pool += r.Duties
						r.Duties = 0
						people += r.Population()
					}
				}
				for _, r := range w.Regions {
					if britain(r) {
						for _, p := range r.Pops {
							p.Cash += pool * p.Size / people
						}
					}
				}
				if d < end-days {
					continue
				}
				samples++
				out.DutyRevenue += pool
				bp, abroad, people2, cp, gp, brp, liv := 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0
				for _, r := range w.Regions {
					pop := r.Population()
					if !britain(r) {
						abroad += r.Markets[grain].Price * pop
						cp += pop
						for _, f := range r.Firms {
							if f.Name == "Farm" || f.Name == "Potato farm" {
								out.ContinentFarm += f.Labor
							}
						}
						continue
					}
					people2 += pop
					gp += r.Markets[grain].Price * pop
					brp += r.Markets[bread].Price * pop
					bp += r.Wage / r.Markets[bread].Price * pop
					liv += r.Utility() * pop
					for _, f := range r.Firms {
						profit := f.TickRevenue - f.TickCost
						switch {
						case f.Name == "Farm" || f.Name == "Potato farm":
							out.Farm += f.Labor
							out.FarmProfit += profit
							if f.Output == grain {
								out.GrainMade += f.LastOutput
							}
						case f.Sector == Primary:
							out.Land += f.Labor
							out.OtherProfit += profit
						case f.Sector == Industry:
							out.Industry += f.Labor
							out.OtherProfit += profit
						default:
							out.Service += f.Labor
							out.OtherProfit += profit
						}
						out.Wages += f.Labor * r.Wage
					}
				}
				for _, t := range w.Traders {
					if t.Good == grain && britain(t.Dest) && !britain(t.Origin) {
						out.Imports += t.Shipped * t.Arriving
					}
				}
				out.GrainPrice += gp / people2
				out.BreadPrice += brp / people2
				out.RealWage += bp / people2
				out.Living += liv / people2
				out.People += people2
				out.GrainPriceAbroad += abroad / cp
			}
			// (prices, the real wage and the living standard were averaged
			// over people each day, then summed: now over the days)
			for _, x := range []*float64{&out.GrainPrice, &out.BreadPrice, &out.GrainPriceAbroad, &out.Imports, &out.Farm, &out.Land,
				&out.Industry, &out.Service, &out.Wages, &out.FarmProfit, &out.OtherProfit, &out.DutyRevenue, &out.RealWage,
				&out.Living, &out.People, &out.ContinentFarm, &out.GrainMade} {
				*x /= samples
			}
		}
	}
	duties := []float64{0, 0.25, 0.5, 1, -1}
	results := make([]result, len(duties))
	var jobs []func()
	for i, d := range duties {
		jobs = append(jobs, run(d, &results[i]))
	}
	parallel(jobs...)
	var tiles []int
	w := world()
	for _, r := range w.Regions {
		if britain(r) {
			tiles = append(tiles, r.ID)
		}
	}
	o.Write("cornlaws.json", map[string]any{"britain": tiles, "runs": results})
}
