package experiments

import (
	"math"

	. "github.com/rasmusbergpalm/mercury/economy"
)

func init() {
	register(Experiment{"boom", "a coal boom in one tile, dose and response, with and without migration", runBoom})
}

func runBoom(o *Out) {
	shockDay := settle + days
	// ---- a coal boom, dose and response: the tile with the most coal
	// miners finds its coal 1 to 10 times as easy to dig, with people free
	// to move and with nobody moving after the map has settled (a fixed
	// workforce). Its coal is as much cheaper
	// to carry to its neighbours. The boom tile's workers by
	// trade, its people and its wage, averaged over the last two years.
	type boomRun struct {
		Boom                                     float64
		Migration                                bool
		Tile                                     int
		Coal, OtherLand, Industry, Service, Wage float64
		Haul                                     float64 // carters hauling the tile's coal away (counted in Service too)
		People                                   float64
	}
	boomDoses := []float64{1, 2, 3, 4, 6, 8, 10}
	boomRuns := make([]boomRun, 2*len(boomDoses))
	var boomJobs []func()
	for i, k := range boomDoses {
		for j, mig := range []bool{false, true} {
			out := &boomRuns[2*i+j]
			k, mig := k, mig
			boomJobs = append(boomJobs, func() {
				w := settled()
				tile := -1
				out.Boom, out.Migration = k, mig
				samples := 0.0
				for d := settle; d < end; d++ {
					if d == settle { // the same tile in every run: they are identical until now
						w.Migration = mig
						most := 0.0
						for _, r := range w.Regions {
							for _, f := range r.Firms {
								if f.Name == "Coal mine" && f.Labor > most {
									most, tile = f.Labor, r.ID
								}
							}
						}
						out.Tile = tile
					}
					if d == shockDay {
						for _, f := range w.Regions[tile].Firms {
							if f.Name == "Coal mine" {
								for _, m := range f.Methods {
									m.TFP *= k
								}
							}
						}
						// ... and its coal is as much cheaper to carry away:
						// a boom in a small country's export comes with the
						// ships and railways to carry it (Norway's oil came
						// with its pipelines), and a tile's carters can't
						// haul ten times the coal without them. The coal it
						// sends to its neighbours takes 1/k the transport.
						for _, tr := range w.Traders {
							if tr.Good == G("coal") && tr.Origin.ID == tile {
								tr.TransportCost /= k
								tr.SetDistance(tr.Distance)
							}
						}
					}
					w.Step()
					if d < end-2*days {
						continue
					}
					r := w.Regions[tile]
					for _, f := range r.Firms {
						switch {
						case f.Name == "Coal mine":
							out.Coal += f.Labor
						case f.Sector == Primary:
							out.OtherLand += f.Labor
						case f.Sector == Industry:
							out.Industry += f.Labor
						case f.Sector == Service:
							out.Service += f.Labor
						}
					}
					// the carters hauling coal: the tile's transport
					// workers, in the share of its transport its coal
					// exports use
					haul, made, carters := 0.0, 0.0, 0.0
					for _, tr := range w.Traders {
						if tr.Good == G("coal") && tr.Origin.ID == tile {
							haul += tr.Shipped * tr.Tau
						}
					}
					for _, f := range r.Firms {
						if f.Output == Transport {
							made += f.LastOutput
							carters += f.Labor
						}
					}
					if made > 0 {
						out.Haul += carters * math.Min(1, haul/made)
					}
					out.Wage += r.Wage
					out.People += r.Population()
					samples++
				}
				out.Coal /= samples
				out.OtherLand /= samples
				out.Industry /= samples
				out.Service /= samples
				out.Haul /= samples
				out.Wage /= samples
				out.People /= samples
			})
		}
	}

	parallel(boomJobs...)
	o.Write("boom.json", map[string]any{"runs": boomRuns})
}
