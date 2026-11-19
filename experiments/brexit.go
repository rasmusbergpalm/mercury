package experiments

import . "github.com/rasmusbergpalm/mercury/economy"

func init() {
	register(Experiment{"brexit", "a hard Brexit: no trade between Britain and the Continent, and then no migration either", runBrexit})
}

// inBritain is the "Britain" of the Corn Laws and Brexit plates: the
// map's two northern rows, hills and mountains with coal, iron and sheep,
// and a few plains.
func inBritain(r *Region) bool { return r.Row < 2 }

// runBrexit cuts Britain off from the Continent. From the settled map,
// every route between the two is closed to every good, both ways (an
// embargo: no deal at all); in a second run the border is also closed to
// people, so nobody moves across it either. Against a run where nothing
// changes, month by month for Years: the living standard of Britain's
// people and of the Continent's, real output a head (each tile's net
// output at its prices the day before) and the people on each side; and
// at the end, Britain's workers by trade.
func runBrexit(o *Out) {
	type side struct {
		Living, Output, People []float64 // by month
	}
	type result struct {
		Name               string
		Months             []int
		Britain, Continent side
		Jobs               map[string]float64 // Britain's workers by trade, the last year's average
	}
	run := func(name string, trade, people bool, out *result) func() {
		return func() {
			w := settled()
			record, done := o.Recorder(name)
			defer done()
			out.Name, out.Jobs = name, map[string]float64{}
			crosses := func(a, b *Region) bool { return inBritain(a) != inBritain(b) }
			if trade {
				for _, t := range w.Traders {
					if crosses(t.Origin, t.Dest) {
						t.Closed = true
					}
				}
			}
			if people {
				// a hop nobody would make: migration falls with the cost of
				// the hop between two tiles (trade has its own routes)
				for i, l := range w.Links {
					if crosses(w.Regions[l.A], w.Regions[l.B]) {
						w.Links[i].Hop = 1e9
					}
				}
			}
			p0 := make([][]float64, len(w.Regions)) // the prices the day before, to value output at
			for i, r := range w.Regions {
				p0[i] = r.Prices()
			}
			measure := func(s *side, in func(r *Region) bool) {
				living, output, people := 0.0, 0.0, 0.0
				for i, r := range w.Regions {
					if !in(r) {
						continue
					}
					pop := r.Population()
					people += pop
					living += r.Utility() * pop
					for _, f := range r.Firms {
						output += f.LastOutput * p0[i][f.Output]
						for g, q := range f.Used {
							output -= q * p0[i][g]
						}
					}
				}
				s.Living = append(s.Living, living/people)
				s.Output = append(s.Output, output/people)
				s.People = append(s.People, people)
			}
			samples := 0.0
			for d := settle; d < end; d++ {
				w.Step()
				record(w)
				if (d-settle)%30 == 0 {
					out.Months = append(out.Months, (d-settle)/30)
					measure(&out.Britain, inBritain)
					measure(&out.Continent, func(r *Region) bool { return !inBritain(r) })
				}
				if d < end-days {
					continue
				}
				samples++
				for _, r := range w.Regions {
					if !inBritain(r) {
						continue
					}
					for _, f := range r.Firms {
						out.Jobs[f.Name] += f.Labor
					}
				}
			}
			for k := range out.Jobs {
				out.Jobs[k] /= samples
			}
		}
	}
	runs := make([]result, 3)
	parallel(run("control", false, false, &runs[0]), run("embargo", true, false, &runs[1]), run("closed", true, true, &runs[2]))
	var tiles []int
	for _, r := range world().Regions {
		if inBritain(r) {
			tiles = append(tiles, r.ID)
		}
	}
	o.Write("brexit.json", map[string]any{"britain": tiles, "runs": runs})
}
