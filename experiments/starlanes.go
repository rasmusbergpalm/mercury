package experiments

import (
	. "github.com/rasmusbergpalm/mercury/economy"
	"github.com/rasmusbergpalm/mercury/examples/starlanes"
)

func init() {
	register(Experiment{"starlanes", "another economy on the same engine: riots on the one aether world of a web of star systems", runStarlanes})
}

// runStarlanes runs the star-lanes economy (examples/starlanes), not the
// 1820 one: twelve star systems on a sparse graph of lanes, whose freight
// burns aether from a single system, Cinder. The map settles for Settle
// years; then, after a year, riots on Cinder cut its aether wells'
// productivity by 85% for two years, against a run without them, and both
// go on for two more years. Each month: every system's prices of aether
// and freight, its people and living standard; every lane's trade (the
// value shipped along it, both ways, at the origins' prices); Cinder's
// aether output.
func runStarlanes(o *Out) {
	if err := starlanes.Load(); err != nil {
		panic(err)
	}
	defer func() {
		if err := LoadData(ExampleData); err != nil {
			panic(err)
		}
	}()
	const year = starlanes.TicksPerYear
	const (
		riotFrom = 1 * year // ticks after the map settles
		riotTo   = 3 * year
		length   = 5 * year
		riot     = 0.15 // the wells' productivity in the riots
	)
	w, err := starlanes.Build()
	if err != nil {
		panic(err)
	}
	for d := 0; d < Settle*year; d++ {
		w.Step()
	}
	settled := w.Snapshot()
	aether := G("aether")
	type series struct {
		Aether, Freight, People, Living [][]float64 // [system][month]
		Trade                           [][]float64 // [lane][month]
		Output                          []float64   // Cinder's aether output a day, [month]
	}
	months := length * 12 / year
	newSeries := func() *series {
		mk := func(n int) [][]float64 {
			out := make([][]float64, n)
			for i := range out {
				out[i] = make([]float64, months)
			}
			return out
		}
		n := len(starlanes.Systems)
		return &series{mk(n), mk(n), mk(n), mk(n), mk(len(starlanes.Lanes)), make([]float64, months)}
	}
	lane := map[[2]int]int{}
	for k, l := range starlanes.Lanes {
		lane[[2]int{l.A, l.B}], lane[[2]int{l.B, l.A}] = k, k
	}
	run := func(riots bool) *series {
		w, err := Restore(settled)
		if err != nil {
			panic(err)
		}
		s := newSeries()
		perMonth := make([]float64, months)
		var wells *Firm
		for _, f := range w.Regions[starlanes.Source].Firms {
			if f.Output == aether {
				wells = f
			}
		}
		for d := 0; d < length; d++ {
			if riots && d == riotFrom {
				starlanes.Riot(w, riot)
			}
			if riots && d == riotTo {
				starlanes.Riot(w, 1)
			}
			w.Step()
			m := d * 12 / year
			perMonth[m]++
			for i, r := range w.Regions {
				s.Aether[i][m] += r.Markets[aether].Price
				s.Freight[i][m] += r.Markets[Transport].Price
				s.People[i][m] += r.Population()
				s.Living[i][m] += r.Utility()
			}
			for _, t := range w.Traders {
				s.Trade[lane[[2]int{t.Origin.ID, t.Dest.ID}]][m] += t.Shipped * t.Origin.Markets[t.Good].Price
			}
			s.Output[m] += wells.LastOutput
		}
		for m, n := range perMonth {
			for _, xs := range [][][]float64{s.Aether, s.Freight, s.People, s.Living, s.Trade} {
				for i := range xs {
					xs[i][m] /= n
				}
			}
			s.Output[m] /= n
		}
		return s
	}
	var control, riots *series
	parallel(func() { control = run(false) }, func() { riots = run(true) })
	type system struct {
		Name, Terrain string
		X, Y          float64
		Hops          int
	}
	hops := starlanes.Hops()
	var systems []system
	for i, p := range starlanes.Systems {
		systems = append(systems, system{p.Name, p.Terrain, starlanes.Layout[i][0], starlanes.Layout[i][1], hops[i]})
	}
	o.Write("starlanes.json", map[string]any{
		"systems": systems, "lanes": starlanes.Lanes, "start": starlanes.StartYear + Settle,
		"riotFrom": riotFrom * 12 / year, "riotTo": riotTo * 12 / year, "riot": riot,
		"control": control, "riots": riots,
	})
}
