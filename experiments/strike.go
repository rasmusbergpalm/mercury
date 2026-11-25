package experiments

import ()

func init() {
	register(Experiment{"strike", "a strike: one tile's firms half again as productive, with and without migration", runStrike})
}

func runStrike(o *Out) {
	// ---- the strike, with and without migration
	const shockTile = 14 // an inland tile in the middle of the map
	shockDay := settle + days
	type series struct {
		Months  []int
		People  [][]float64 // [tile][month]
		Utility [][]float64
		Wage    [][]float64
		Prices  [][]float64 // the basic goods' prices, averaged: what living costs
	}
	strike := func(shock, migration bool, out *series) func() {
		return func() {
			w := settled()
			n := len(w.Regions)
			s := series{People: make([][]float64, n), Utility: make([][]float64, n), Wage: make([][]float64, n), Prices: make([][]float64, n)}
			for d := settle; d < end; d++ {
				if d == settle {
					w.Migration = migration // (people sort themselves out while the map settles, then may stay put)
				}
				if shock && d == shockDay {
					windfall(w.Regions[shockTile], 1.5, nil)
				}
				w.Step()
				if d >= settle && (d-settle)%30 == 0 {
					s.Months = append(s.Months, (d-settle)/30)
					for i, r := range w.Regions {
						s.People[i] = append(s.People[i], r.Population())
						s.Utility[i] = append(s.Utility[i], r.Utility())
						s.Wage[i] = append(s.Wage[i], r.Wage)
						p := 0.0
						for _, g := range basics {
							p += r.Markets[g].Price / float64(len(basics))
						}
						s.Prices[i] = append(s.Prices[i], p)
					}
				}
			}
			*out = s
		}
	}
	var shocked, control, shockedStill, controlStill series

	parallel(strike(true, true, &shocked), strike(false, true, &control), strike(true, false, &shockedStill), strike(false, false, &controlStill))
	o.Write("shock.json", map[string]any{"tile": shockTile, "shockMonth": days / 30, "hops": hopsFrom(world(), shockTile),
		"shocked": shocked, "control": control, "shockedStill": shockedStill, "controlStill": controlStill})
}
