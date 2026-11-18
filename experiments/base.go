package experiments

import (
	"sort"

	. "github.com/rasmusbergpalm/mercury/economy"
)

func init() {
	register(Experiment{"base", "the settled map: each tile's people and firms, every route's prices against its landed cost, Engel's law", runBase})
}

func runBase(o *Out) {
	// ---- the base run: what each tile makes, and how prices settle along
	// the trade routes: a route ships while the good sells at its
	// destination for more than it costs landed there (the origin's price,
	// plus the transport a unit needs at the origin's price of transport,
	// over the share that arrives unspoiled)
	w := world()
	landed := func(t *Trader) float64 {
		return (t.Origin.Markets[t.Good].Price + t.Tau*t.Origin.Markets[Transport].Price) / t.Arriving
	}
	const early = 360 // days of each route's prices kept from the start, to show them converge
	type pricePath struct{ Dest, Landed []float64 }
	paths := make([]pricePath, len(w.Traders))
	type routeRow struct {
		Good                          string
		From, To                      int
		Origin, Dest, Landed, Shipped float64
	}
	routes := make([]routeRow, len(w.Traders))
	// Engel's law: the share of spending that goes on basic needs, against
	// income a head, for the map month by month as it settles and grows
	// richer, and for each tile at the end
	basicShare := func(r *Region) (basic, all, income float64) {
		prices := make([]float64, NumGoods)
		for g, m := range r.Markets {
			prices[g] = m.Price
		}
		ts := r.TierSpend(prices)
		for k, v := range ts {
			all += v
			if Tier(k) == Basic {
				basic += v
			}
		}
		for _, p := range r.Pops {
			income += p.Income
		}
		return
	}
	type engelRow struct{ Day, BasicShare, IncomeHead float64 }
	var engelMap, engelTiles []engelRow
	for d := 0; d < end; d++ {
		w.Step()
		if d%30 == 29 {
			b, a, inc, people := 0.0, 0.0, 0.0, 0.0
			for _, r := range w.Regions {
				rb, ra, ri := basicShare(r)
				b, a, inc, people = b+rb, a+ra, inc+ri, people+r.Population()
			}
			engelMap = append(engelMap, engelRow{float64(d), b / a, inc / people})
		}
		if d < early && d%3 == 0 {
			for i, t := range w.Traders {
				paths[i].Dest = append(paths[i].Dest, t.Dest.Markets[t.Good].Price)
				paths[i].Landed = append(paths[i].Landed, landed(t))
			}
		}
		if d >= end-30 { // the last month's averages
			for i, t := range w.Traders {
				r := &routes[i]
				r.Good, r.From, r.To = Goods[t.Good].Name, t.Origin.ID, t.Dest.ID
				r.Origin += t.Origin.Markets[t.Good].Price / 30
				r.Dest += t.Dest.Markets[t.Good].Price / 30
				r.Landed += landed(t) / 30
				r.Shipped += t.Shipped / 30
			}
		}
	}
	order := make([]int, len(routes))
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(a, b int) bool {
		return routes[order[a]].Shipped*routes[order[a]].Dest > routes[order[b]].Shipped*routes[order[b]].Dest
	})
	type converging struct {
		Route routeRow
		Path  pricePath
	}
	var conv []converging
	for _, i := range order[:3] {
		conv = append(conv, converging{routes[i], paths[i]})
	}
	o.Write("routes.json", map[string]any{"routes": routes, "converging": conv, "everyDays": 3})
	for _, r := range w.Regions {
		b, a, inc := basicShare(r)
		engelTiles = append(engelTiles, engelRow{float64(r.ID), b / a, inc / r.Population()})
	}
	o.Write("engel.json", map[string]any{"map": engelMap, "tiles": engelTiles})
	type firmRow struct {
		Name, Method, Good, Sector string
		Labor, Output, Levels      float64
		ValueAdded                 float64
	}
	type tileRow struct {
		ID, Row, Col          int
		Name, Terrain         string
		Coastal               bool
		People, Wage, Utility float64
		Growth                float64
		Firms                 []firmRow
	}
	var tiles []tileRow
	for _, r := range w.Regions {
		tr := tileRow{ID: r.ID, Row: r.Row, Col: r.Col, Name: r.Name, Terrain: r.Terrain, Coastal: r.Coastal,
			People: r.Population(), Wage: r.Wage, Utility: r.Utility(), Growth: r.GrowthRate()}
		for _, f := range r.Firms {
			if f.Labor < 1e-6 && f.LastOutput < 1e-6 {
				continue
			}
			main := f.Methods[0]
			for _, m := range f.Methods {
				if m.Levels > main.Levels {
					main = m
				}
			}
			tr.Firms = append(tr.Firms, firmRow{Name: f.Name, Method: main.Name, Good: Goods[f.Output].Name, Sector: f.Sector.String(),
				Labor: f.Labor, Output: f.LastOutput, Levels: float64(f.Levels), ValueAdded: f.ValueAdded(r.Markets[f.Output].Price)})
		}
		sort.Slice(tr.Firms, func(i, j int) bool { return tr.Firms[i].Labor > tr.Firms[j].Labor })
		tiles = append(tiles, tr)
	}
	o.Write("tiles.json", tiles)
}
