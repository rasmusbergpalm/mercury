package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"
)

// TestExplainerData writes the data behind the economy explainer: the
// classic map with no nations (no state, armies, roads or new technology)
// and no population growth (a growing population moves everything, and the
// experiments compare like with like). Each run first settles for
// explainerSettle years (owners build up their firms and people sort
// themselves between tiles), then runs its experiment for
// explainerYears more:
//
//   - tiles.json: each tile at the end: terrain, people, wage, living
//     standard, and its workers and output per firm;
//
//   - routes.json: every trade route's prices at the end, against its
//     landed cost (the origin's price plus carriage), and the busiest
//     routes' first year from the start, as their prices converge;
//
//   - shock.json: a strike: one tile's firms become half again as
//     productive; each tile's people, living standard, wage and cost of
//     living by month, with a twin run without the strike, and both again
//     with migration off (only trade then evens out living standards);
//
//   - blight.json: substitution: potato farms everywhere make half what
//     they did (the blight of the 1840s); bread and potatoes both meet the
//     need for staples: their prices and what people eat of each, against
//     a run without the blight;
//
//   - aa.json: the Alchian-Allen effect, with spoilage off: each good's
//     hops travelled per unit made against its value per unit of bulk; and
//     potatoes, coal and tea again at other bulks, to see each move;
//
//   - recipes.json: the production functions and the growth and migration
//     constants.
//
//     EXPLAINER_OUT=/tmp/explainer go test ./engine -run TestExplainerData -timeout 60m
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
		w.Migration, w.Investment, w.Demography = true, true, false
		return w
	}
	days := int(TicksPerYear)
	settle := explainerSettle * days // the day each experiment starts
	end := settle + explainerYears*days
	// parallel runs the jobs four at a time
	parallel := func(jobs ...func()) {
		var wg sync.WaitGroup
		sem := make(chan struct{}, 4)
		for _, j := range jobs {
			wg.Add(1)
			go func(j func()) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				j()
			}(j)
		}
		wg.Wait()
	}
	var basics []GoodID // the goods that meet basic needs: what living costs
	for _, nd := range Needs {
		if nd.Tier == Basic {
			basics = append(basics, nd.Goods...)
		}
	}

	// ---- the base run: what each tile makes, and how prices settle along
	// the trade routes: a route ships while the good sells at its
	// destination for more than it costs landed there (the origin's price,
	// plus the transport a unit needs at the origin's price of transport,
	// over the share that arrives unspoiled)
	base := func() {
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
		for d := 0; d < end; d++ {
			w.Step()
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
	}

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
			w := world()
			n := len(w.Regions)
			s := series{People: make([][]float64, n), Utility: make([][]float64, n), Wage: make([][]float64, n), Prices: make([][]float64, n)}
			for d := 0; d < end; d++ {
				if d == settle {
					w.Migration = migration // (people sort themselves out while the map settles, then may stay put)
				}
				if shock && d == shockDay {
					for _, f := range w.Regions[shockTile].Firms {
						for _, m := range f.Methods {
							m.TFP *= 1.5
						}
					}
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

	// ---- substitution: a potato blight
	blight := func(on bool, out *map[string][]float64) func() {
		return func() {
			w := world()
			o := map[string][]float64{}
			bread, potatoes, grain := G("bread"), G("potatoes"), G("grain")
			for d := 0; d < end; d++ {
				if on && d == shockDay {
					for _, r := range w.Regions {
						for _, f := range r.Firms {
							if f.Name == "Potato farm" {
								for _, m := range f.Methods {
									m.TFP *= 0.5
								}
							}
						}
					}
				}
				w.Step()
				if d < settle || (d-settle)%10 != 0 {
					continue
				}
				people, eat, price := 0.0, map[GoodID]float64{}, map[GoodID]float64{}
				for _, r := range w.Regions {
					people += r.Population()
					for _, p := range r.Pops {
						for _, g := range []GoodID{bread, potatoes} {
							eat[g] += p.Consumed[g]
						}
					}
					for _, g := range []GoodID{bread, potatoes, grain} {
						price[g] += r.Markets[g].Price / float64(len(w.Regions))
					}
				}
				o["day"] = append(o["day"], float64(d-settle))
				o["breadEaten"] = append(o["breadEaten"], eat[bread]/people)
				o["potatoesEaten"] = append(o["potatoesEaten"], eat[potatoes]/people)
				o["breadPrice"] = append(o["breadPrice"], price[bread])
				o["potatoPrice"] = append(o["potatoPrice"], price[potatoes])
				o["grainPrice"] = append(o["grainPrice"], price[grain])
			}
			*out = o
		}
	}
	var blighted, unblighted map[string][]float64

	parallel(base, strike(true, true, &shocked), strike(false, true, &control), strike(true, false, &shockedStill),
		strike(false, false, &controlStill), blight(true, &blighted), blight(false, &unblighted))

	hops := func() []int { // hops from the struck tile
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
		"shocked": shocked, "control": control, "shockedStill": shockedStill, "controlStill": controlStill})
	write("blight.json", map[string]any{"blightDay": days, "sigma": Needs[0].Sigma, "blight": blighted, "control": unblighted})

	// ---- the Alchian-Allen effect. With no spoilage, only carriage sets
	// how far a good goes. The goods table is changed for these runs, so
	// their worlds are built one at a time (a route takes its good's bulk
	// and spoilage when it is built) and then run together.
	spoil, bulk := make([]float64, NumGoods), make([]float64, NumGoods)
	for g := range Goods {
		spoil[g], bulk[g] = Goods[g].Spoilage, Goods[g].Bulk
	}
	defer func() {
		for g := range Goods {
			Goods[g].Spoilage, Goods[g].Bulk = spoil[g], bulk[g]
		}
	}()
	for g := range Goods {
		Goods[g].Spoilage = 0
	}
	type travel struct {
		Good                           string
		Bulk, Price, Made, HopsPerUnit float64
	}
	type aaRun struct {
		w     *World
		good  string // the good whose bulk is changed ("" for the base)
		bulks []float64
		out   []travel
	}
	var runs []*aaRun
	add := func(good string, k float64) {
		bs := append([]float64(nil), bulk...)
		if good != "" {
			Goods[G(good)].Bulk = bulk[G(good)] * k
			bs[G(good)] = Goods[G(good)].Bulk
		}
		runs = append(runs, &aaRun{w: world(), good: good, bulks: bs})
		for g := range Goods {
			Goods[g].Bulk = bulk[g]
		}
	}
	add("", 1)
	for _, name := range []string{"potatoes", "coal", "tea"} {
		for _, k := range []float64{0.25, 0.5, 2} {
			add(name, k)
		}
	}
	var jobs []func()
	for _, r := range runs {
		r := r
		jobs = append(jobs, func() {
			w := r.w
			made, hops, price := make([]float64, NumGoods), make([]float64, NumGoods), make([]float64, NumGoods)
			for d := 0; d < end; d++ {
				w.Step()
				if d < end-days {
					continue
				}
				for _, reg := range w.Regions {
					for _, f := range reg.Firms {
						made[f.Output] += f.LastOutput / TicksPerYear
					}
					for g, m := range reg.Markets {
						price[g] += m.Price / float64(len(w.Regions)) / TicksPerYear
					}
				}
				for _, tr := range w.Traders {
					hops[tr.Good] += tr.Shipped * tr.Distance / TicksPerYear
				}
			}
			for _, g := range GameTradable {
				if made[g] > 0.5 && r.bulks[g] > 0 {
					r.out = append(r.out, travel{Goods[g].Name, r.bulks[g], price[g], made[g], hops[g] / made[g]})
				}
			}
		})
	}
	parallel(jobs...)
	moves := map[string][]travel{}
	for _, r := range runs[1:] {
		for _, tv := range r.out {
			if tv.Good == r.good {
				moves[r.good] = append(moves[r.good], tv)
			}
		}
	}
	write("aa.json", map[string]any{"base": runs[0].out, "moves": moves})

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
	addFirm := func(rc Recipe) {
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
		addFirm(rc)
	}
	for _, rc := range ProcessingRecipes {
		addFirm(rc)
	}
	write("recipes.json", map[string]any{
		"firms": firms,
		"growth": map[string]float64{"slope": popGrowthSlope, "neutral": popGrowthNeutral, "min": popGrowthMin, "max": popGrowthMax,
			"schooled": schooledBirths},
		"migration": map[string]float64{"rate": migrationRate, "minPopulation": minPopulation},
		"settle":    map[string]int{"years": explainerSettle, "experimentYears": explainerYears},
	})
	fmt.Println("wrote", dir)
}

// The explainer's runs settle for explainerSettle years before each
// experiment, which then runs explainerYears.
const (
	explainerSettle = 15
	explainerYears  = 5
)
