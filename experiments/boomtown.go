package experiments

import . "github.com/rasmusbergpalm/mercury/economy"

func init() {
	register(Experiment{"boomtown", "a boomtown: one tile's trades half again as productive, with and without new houses", runBoomtown})
}

// runBoomtown watches a boom through a town's land and houses. One inland
// tile's trades (its industry and services, not its farms, mines or
// houses) become half again as productive, as Manchester's mills did,
// against a twin without the windfall, with people free to move. In one run its
// landlords build as usual; in the other no new houses may be built there
// (a building restriction, or a town hemmed in by its walls). Month by
// month for ten years: the tile's people, the price of its land and its
// housing, its houses, how its land is shared out, and the living
// standard of its workers and its owners.
func runBoomtown(o *Out) {
	const tile = 14 // the strike experiment's tile
	years := 10
	type series struct {
		Months                      []int
		People, Land, Rent, Houses  []float64
		LandHouses, LandFarms       []float64 // the tile's land held by houses and by farms, forests and the rest
		Workers, Owners, Neighbours []float64 // living standards
		Wage                        []float64
	}
	housing, land := G("housing"), G("land")
	run := func(boom, build bool, out *series) func() {
		return func() {
			w := settled()
			r := w.Regions[tile]
			near := map[int]bool{}
			for _, x := range w.Adjacent(tile) {
				near[x[0]] = true
			}
			for d := settle; d < settle+years*days; d++ {
				if d == settle {
					if boom { // its trades: industry and services
						windfall(r, 1.5, func(f *Firm) bool { return f.Sector != Primary && f.Output != housing })
					}
					for _, f := range r.Firms {
						if f.Output == housing {
							f.NoBuilding = !build
						}
					}
				}
				w.Step()
				if (d-settle)%30 != 0 {
					continue
				}
				out.Months = append(out.Months, (d-settle)/30)
				out.People = append(out.People, r.Population())
				out.Land = append(out.Land, r.Markets[land].Price)
				out.Rent = append(out.Rent, r.Markets[housing].Price)
				out.Wage = append(out.Wage, r.Wage)
				var houses, lh, lf float64
				for _, f := range r.Firms {
					if f.Output == housing {
						houses = float64(f.Levels)
					}
					if f.Held != nil {
						if f.Output == housing {
							lh += f.Held[land]
						} else {
							lf += f.Held[land]
						}
					}
				}
				out.Houses, out.LandHouses, out.LandFarms = append(out.Houses, houses), append(out.LandHouses, lh), append(out.LandFarms, lf)
				var wu, ou float64
				for _, p := range r.Pops {
					if p.Class == Owners {
						ou = p.Utility()
					} else {
						wu = p.Utility()
					}
				}
				out.Workers, out.Owners = append(out.Workers, wu), append(out.Owners, ou)
				nu, np := 0.0, 0.0
				for i, x := range w.Regions {
					if near[i] {
						nu += x.Utility() * x.Population()
						np += x.Population()
					}
				}
				out.Neighbours = append(out.Neighbours, nu/np)
			}
		}
	}
	var control, built, walled, walledControl series
	parallel(run(false, true, &control), run(true, true, &built), run(true, false, &walled), run(false, false, &walledControl))
	o.Write("boomtown.json", map[string]any{"tile": tile, "control": control, "built": built, "walled": walled, "walledControl": walledControl})
}
