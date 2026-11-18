package experiments

import (
	"math"

	. "github.com/rasmusbergpalm/mercury/economy"
)

func init() {
	register(Experiment{"corridor", "a cheap route across the map: trade and people follow it", runCorridor})
}

func runCorridor(o *Out) {
	// ---- a corridor: a road (or canal, or railway) across the map. The
	// coast's cheap shipping is taken away, so every hop costs the same,
	// and the hops along a diagonal route from corner to corner cost a
	// tenth. Two runs, people free to move: the map as it is, with its
	// cheap coast ("coast"), and with the route instead ("route"). Each
	// tile's people, and the value of the trade over
	// each link (both ways, at the prices it sells for) over the last year.
	route := explainerDiagonal(world())
	type corridorRun struct {
		Name   string
		People []float64
		Trade  [][3]float64 // a, b, value a day
	}
	corridorRuns := []corridorRun{{Name: "coast"}, {Name: "route"}}
	var corridorJobs []func()
	for k := range corridorRuns {
		k := k
		corridorJobs = append(corridorJobs, func() {
			cr := &corridorRuns[k]
			w := world()
			if cr.Name == "route" {
				explainerCorridor(w, route)
			}
			value := map[[2]int]float64{}
			for d := 0; d < end; d++ {
				w.Step()
				if d < end-days {
					continue
				}
				for _, tr := range w.Traders {
					a, b := tr.Origin.ID, tr.Dest.ID
					if a > b {
						a, b = b, a
					}
					value[[2]int{a, b}] += tr.Shipped * tr.Arriving * tr.Dest.Markets[tr.Good].Price / float64(days)
				}
			}
			for _, r := range w.Regions {
				cr.People = append(cr.People, r.Population())
			}
			for _, l := range w.Links {
				a, b := min(l.A, l.B), max(l.A, l.B)
				cr.Trade = append(cr.Trade, [3]float64{float64(a), float64(b), value[[2]int{a, b}]})
			}
		})
	}
	parallel(corridorJobs...)
	o.Write("corridor.json", map[string]any{"route": route, "runs": corridorRuns})
}

// explainerCorridor takes the coast's cheap shipping away, so that every
// hop costs 1, and makes the hops along route (a list of neighbouring
// tiles) cost a tenth.
func explainerCorridor(w *World, route []int) {
	for _, r := range w.Regions {
		r.Coastal = false
	}
	on := map[[2]int]bool{}
	for i := 0; i+1 < len(route); i++ {
		on[[2]int{route[i], route[i+1]}], on[[2]int{route[i+1], route[i]}] = true, true
	}
	hop := func(a, b int) float64 {
		if on[[2]int{a, b}] {
			return 0.1
		}
		return 1
	}
	for i, l := range w.Links {
		w.Links[i].Hop = hop(l.A, l.B)
	}
	for _, tr := range w.Traders {
		tr.SetDistance(hop(tr.Origin.ID, tr.Dest.ID))
	}
}

// explainerDiagonal is a route from the map's top-left corner to its
// bottom-right: each step goes to the neighbour nearest the far corner,
// and of those, to the one nearest the straight line between the corners.
func explainerDiagonal(w *World) []int {
	cube := func(r *Region) [3]int { x := r.Col - (r.Row-(r.Row&1))/2; z := r.Row; return [3]int{x, -x - z, z} }
	dist := func(a, b *Region) int {
		ca, cb := cube(a), cube(b)
		m := 0
		for i := 0; i < 3; i++ {
			m = max(m, ca[i]-cb[i], cb[i]-ca[i])
		}
		return m
	}
	start, goal := w.Regions[0], w.Regions[len(w.Regions)-1]
	px := func(r *Region) (float64, float64) {
		return float64(r.Col) + 0.5*float64(r.Row&1), 0.866 * float64(r.Row)
	}
	off := func(r *Region) float64 {
		x0, y0 := px(start)
		x1, y1 := px(goal)
		x, y := px(r)
		return math.Abs((y1-y0)*x-(x1-x0)*y+x1*y0-y1*x0) / math.Hypot(y1-y0, x1-x0)
	}
	path := []int{start.ID}
	for cur := start; cur != goal; {
		var best *Region
		for _, nb := range w.Adjacent(cur.ID) {
			r := w.Regions[nb[0]]
			if best == nil || dist(r, goal) < dist(best, goal) || (dist(r, goal) == dist(best, goal) && off(r) < off(best)) {
				best = r
			}
		}
		cur = best
		path = append(path, cur.ID)
	}
	return path
}
