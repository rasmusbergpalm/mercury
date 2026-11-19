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
// people, so nobody moves across it either. Then the realistic hard
// Brexit: no embargo, but each side charges the other the EU's standard
// tariffs (mfnTariff), with and without the border closed to people; the
// duties go back to the people of the side that collects them. And the
// tariffs again with the border's other costs added (borderFriction). Against a run where nothing
// changes, month by month for Years: the living standard of Britain's
// people and of the Continent's, real output a head (each tile's net
// output at its prices the day before), the people on each side, and the
// goods crossing the border (at the same prices); and
// at the end, Britain's workers by trade.
func runBrexit(o *Out) {
	type side struct {
		Living, Output, People []float64 // by month
	}
	type result struct {
		Name               string
		Months             []int
		Britain, Continent side
		Trade              []float64          // goods crossing the border both ways a day, at the prices the day before
		Jobs               map[string]float64 // Britain's workers by trade, the last year's average
	}
	// trade: "embargo" closes every route across the border; "tariff" puts
	// the EU's standard (most-favoured-nation) duties on them, both ways;
	// "friction" adds the border's paperwork, checks and delays to those
	// duties (see borderFriction)
	run := func(name, trade string, people bool, out *result) func() {
		return func() {
			w := settled()
			record, done := o.Recorder(name)
			defer done()
			out.Name, out.Jobs = name, map[string]float64{}
			crosses := func(a, b *Region) bool { return inBritain(a) != inBritain(b) }
			for _, t := range w.Traders {
				if crosses(t.Origin, t.Dest) {
					switch trade {
					case "embargo":
						t.Closed = true
					case "tariff":
						t.Tariff = mfnTariff(t.Good)
					case "friction":
						t.Tariff = mfnTariff(t.Good)
						// more carriage for each unit: worth borderFriction of
						// the good's price, at today's prices
						p, pt := t.Origin.Markets[t.Good].Price, t.Origin.Markets[Transport].Price
						t.TransportCost += borderFriction * p / (pt * t.Distance * Goods[t.Good].Bulk)
						t.SetDistance(t.Distance)
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
				rebateDuties(w, inBritain)
				rebateDuties(w, func(r *Region) bool { return !inBritain(r) })
				if (d-settle)%30 == 0 {
					out.Months = append(out.Months, (d-settle)/30)
					cross := 0.0
					for _, t := range w.Traders {
						if crosses(t.Origin, t.Dest) {
							cross += t.Shipped * p0[t.Origin.ID][t.Good]
						}
					}
					out.Trade = append(out.Trade, cross)
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
	runs := make([]result, 7)
	parallel(run("control", "", false, &runs[0]), run("embargo", "embargo", false, &runs[1]), run("closed", "embargo", true, &runs[2]),
		run("tariff", "tariff", false, &runs[3]), run("tariffClosed", "tariff", true, &runs[4]),
		run("friction", "friction", false, &runs[5]), run("frictionClosed", "friction", true, &runs[6]))
	var tiles []int
	for _, r := range world().Regions {
		if inBritain(r) {
			tiles = append(tiles, r.ID)
		}
	}
	o.Write("brexit.json", map[string]any{"britain": tiles, "runs": runs})
}

// mfnTariff is the EU's standard import duty on a good, its most-favoured-
// nation rate: what it charges a country with no trade deal, as Britain
// would have after leaving with none (and what Britain's own tariff copied).
// The WTO's tariff profile of the EU gives a simple average of about 11% on
// farm goods and 4% on the rest; fish counts with the rest, as in the WTO's
// tables.
func mfnTariff(g GoodID) float64 {
	switch Goods[g].Name {
	case "bread", "potatoes", "meat", "beer", "tea", "sugar", "wine", "grain", "wool", "cotton", "sugar cane", "grapes", "silk":
		return 0.11
	}
	return 0.041
}

// rebateDuties hands the duties the tiles in collected today back to their
// people, by head (the economy has no transfers: they are written in).
func rebateDuties(w *World, in func(r *Region) bool) {
	pool, people := 0.0, 0.0
	for _, r := range w.Regions {
		if in(r) {
			pool += r.Duties
			r.Duties = 0
			people += r.Population()
		}
	}
	if pool == 0 {
		return
	}
	for _, r := range w.Regions {
		if in(r) {
			for _, p := range r.Pops {
				p.Cash += pool * p.Size / people
			}
		}
	}
}

// borderFriction is what crossing the border costs beyond the duty, as a
// share of a good's value: customs declarations, checks, proving where a
// good was made, and the delays they cause. Estimates of these costs for
// Britain leaving the EU with no deal run from about 5% to 15% of the
// value of goods traded; 9% is in the middle. They are real costs, paid in
// work rather than to a treasury, so they are added to the route's
// carriage: more transport for each unit that crosses.
const borderFriction = 0.09
