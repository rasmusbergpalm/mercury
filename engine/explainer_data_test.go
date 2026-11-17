package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// TestExplainerData writes the data behind the economy explainer (the
// classic map with no nations: no state, armies, roads or new technology):
//
//   - tiles.json: each tile after five years: terrain, people, wage,
//     living standard, and its workers and output per firm;
//
//   - shock.json: the migration experiment: after a year one tile's firms
//     become half again as productive (a strike of coal or gold); each tile's
//     people and living standard by month, with a twin run without the
//     shock to compare;
//
//   - recipes.json: the production functions (each firm's methods, their
//     productivity and factor exponents) and the growth and migration
//     constants.
//
//     EXPLAINER_OUT=/tmp/explainer go test ./engine -run TestExplainerData -timeout 30m
func TestExplainerData(t *testing.T) {
	dir := os.Getenv("EXPLAINER_OUT")
	if dir == "" {
		t.Skip("set EXPLAINER_OUT to run")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name string, v any) {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	world := func() *World {
		w, err := baseGameWorld(DefaultGame)
		if err != nil {
			t.Fatal(err)
		}
		w.Migration, w.Investment, w.Demography = true, true, true
		return w
	}
	days := int(TicksPerYear)

	// ---- the base run: what each tile makes after five years, and how
	// prices settle along the trade routes: a route ships while the good
	// sells at its destination for more than it costs landed there (the
	// origin's price, plus the transport a unit needs at the origin's price
	// of transport, over the share that arrives unspoiled)
	w := world()
	landed := func(t *Trader) float64 {
		return (t.Origin.Markets[t.Good].Price + t.Tau*t.Origin.Markets[Transport].Price) / t.Arriving
	}
	const early = 360 // days of each route's prices kept, to show them converge
	type pricePath struct{ Dest, Landed []float64 }
	paths := make([]pricePath, len(w.Traders))
	type routeRow struct {
		Good                          string
		From, To                      int
		Origin, Dest, Landed, Shipped float64
	}
	routes := make([]routeRow, len(w.Traders))
	for d := 0; d < 5*days; d++ {
		w.Step()
		if d < early && d%3 == 0 {
			for i, t := range w.Traders {
				paths[i].Dest = append(paths[i].Dest, t.Dest.Markets[t.Good].Price)
				paths[i].Landed = append(paths[i].Landed, landed(t))
			}
		}
		if d >= 5*days-30 { // the last month's averages
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
	// the busiest routes' first year
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
	write("routes.json", map[string]any{"routes": routes, "converging": conv, "everyDays": 3})
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
				if m.levels > main.levels {
					main = m
				}
			}
			tr.Firms = append(tr.Firms, firmRow{Name: f.Name, Method: main.Name, Good: Goods[f.Output].Name, Sector: f.Sector.String(),
				Labor: f.Labor, Output: f.LastOutput, Levels: float64(f.Levels), ValueAdded: f.ValueAdded(r.Markets[f.Output].Price)})
		}
		sort.Slice(tr.Firms, func(i, j int) bool { return tr.Firms[i].Labor > tr.Firms[j].Labor })
		tiles = append(tiles, tr)
	}
	write("tiles.json", tiles)

	// ---- the migration experiment
	const shockTile = 14 // an inland tile in the middle of the map
	type series struct {
		Months  []int
		People  [][]float64 // [tile][month]
		Utility [][]float64
	}
	run := func(shock bool) series {
		w := world()
		s := series{People: make([][]float64, len(w.Regions)), Utility: make([][]float64, len(w.Regions))}
		for d := 0; d < 5*days; d++ {
			if shock && d == days {
				for _, f := range w.Regions[shockTile].Firms {
					for _, m := range f.Methods {
						m.TFP *= 1.5
					}
				}
			}
			w.Step()
			if d%30 == 0 {
				s.Months = append(s.Months, d/30)
				for i, r := range w.Regions {
					s.People[i] = append(s.People[i], r.Population())
					s.Utility[i] = append(s.Utility[i], r.Utility())
				}
			}
		}
		return s
	}
	hops := func() []int { // hops from the shocked tile
		w := world()
		dist := make([]int, len(w.Regions))
		for i := range dist {
			dist[i] = -1
		}
		dist[shockTile] = 0
		q := []int{shockTile}
		for len(q) > 0 {
			x := q[0]
			q = q[1:]
			for _, nb := range w.adj[x] {
				if dist[nb[0]] < 0 {
					dist[nb[0]] = dist[x] + 1
					q = append(q, nb[0])
				}
			}
		}
		return dist
	}()
	write("shock.json", map[string]any{"tile": shockTile, "shockMonth": days / 30, "hops": hops,
		"shocked": run(true), "control": run(false)})

	// ---- the production functions and the constants
	type factor struct {
		Kind string
		Exp  float64
	}
	type method struct {
		Name, Requires string
		TFP            float64
		Factors        []factor
	}
	type firm struct {
		Name, Good string
		Methods    []method
		Sites      map[string]float64
	}
	facs := func(fs []Factor) []factor {
		var out []factor
		for _, fc := range fs {
			switch fc.Kind {
			case LaborFactor:
				out = append(out, factor{"labour", fc.Exp})
			case GoodFactor:
				out = append(out, factor{Goods[fc.Good].Name, fc.Exp})
			case CompositeFactor:
				out = append(out, factor{Composites[fc.Composite].Name, fc.Exp})
			}
		}
		return out
	}
	var firms []firm
	add := func(rc Recipe) {
		f := firm{Name: rc.Firm, Good: rc.Output, Methods: []method{{rc.Firm, "", rc.TFP, facs(rc.Factors)}}, Sites: map[string]float64{}}
		for _, mr := range MethodRecipes {
			if mr.Base == rc.Firm {
				f.Methods = append(f.Methods, method{mr.Firm, FirmTech[mr.Firm], mr.TFP, facs(mr.Factors)})
			}
		}
		for _, tn := range Terrains {
			for _, b := range tn.Raw {
				if b.Firm == rc.Firm {
					f.Sites[tn.Name] = b.Mult
				}
			}
		}
		firms = append(firms, f)
	}
	for _, rc := range RawRecipes {
		add(rc)
	}
	for _, rc := range ProcessingRecipes {
		add(rc)
	}
	write("recipes.json", map[string]any{
		"firms": firms,
		"growth": map[string]float64{"slope": popGrowthSlope, "neutral": popGrowthNeutral, "min": popGrowthMin, "max": popGrowthMax,
			"schooled": schooledBirths},
		"migration": map[string]float64{"rate": migrationRate, "minPopulation": minPopulation},
	})
	fmt.Println("wrote", dir)
}
