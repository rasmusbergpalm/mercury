package game

import (
	"encoding/json"
	"fmt"
	"math"
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
//   - boom.json: a coal boom of 1 to 10 times in one
//     tile, with and without migration: its workers by trade and its wage;
//
//   - corridor.json: a cheap route across the map (the coast's cheap
//     shipping taken away): each tile's people and each link's trade, on
//     the map as it is and with the route instead of the cheap coast;
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
		write("routes.json", map[string]any{"routes": routes, "converging": conv, "everyDays": 3})
		for _, r := range w.Regions {
			b, a, inc := basicShare(r)
			engelTiles = append(engelTiles, engelRow{float64(r.ID), b / a, inc / r.Population()})
		}
		write("engel.json", map[string]any{"map": engelMap, "tiles": engelTiles})
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

	// ---- trade, wages and specialisation across transport costs: the map
	// settles at each cost, with people free to move or not, and the last
	// year is measured: how far apart wages are between tiles (factor-price
	// equalisation: trade in goods alone pulls them together), and how
	// specialised tiles are (Krugman's index on workers: half the sum over
	// trades of the gap between a tile's share of its workers in a trade and
	// the map's; on workers, since at high costs prices differ so much
	// between tiles that output values can't be compared),
	// and each tile's share of workers on farms (comparative advantage: with
	// cheap carriage, farming moves to the land best at it)
	type tradeRun struct {
		Cost           float64
		Migration      bool
		WageSpread     float64   // the people-weighted coefficient of variation of wages across tiles
		Specialisation float64   // Krugman's index on workers, averaged over tiles weighted by workers
		FarmShare      []float64 // per tile: share of its workers on farms (grain and potatoes)
	}
	tradeCosts := []float64{}
	for k := -4; k <= 7; k++ {
		tradeCosts = append(tradeCosts, math.Pow(2, float64(k)))
	}
	tradeCosts = append(tradeCosts, 1e6)
	tradeRuns := make([]tradeRun, 2*len(tradeCosts))
	var tradeJobs []func()
	for i, c := range tradeCosts {
		for k, mig := range []bool{false, true} {
			out := &tradeRuns[2*i+k]
			c, mig := c, mig
			tradeJobs = append(tradeJobs, func() {
				spec := DefaultGame
				spec.TransportCost = c
				w, err := baseGameWorld(spec)
				if err != nil {
					t.Error(err)
					return
				}
				w.Migration, w.Investment, w.Demography = true, true, false
				out.Cost, out.Migration, out.FarmShare = c, mig, make([]float64, len(w.Regions))
				samples := 0.0
				for d := 0; d < settle; d++ {
					if d == settle/3 {
						w.Migration = mig // (people sort themselves out first, then stay or not)
					}
					w.Step()
					if d < settle-days || d%30 != 0 {
						continue
					}
					samples++
					wsum, wsq, people := 0.0, 0.0, 0.0
					mapOut := map[string]float64{}
					tileOut := make([]map[string]float64, len(w.Regions))
					for i, r := range w.Regions {
						p := r.Population()
						wsum += r.Wage * p
						wsq += r.Wage * r.Wage * p
						people += p
						tileOut[i] = map[string]float64{}
						lab, farm := 0.0, 0.0
						for _, f := range r.Firms {
							tileOut[i][f.Name] += f.Labor
							mapOut[f.Name] += f.Labor
							lab += f.Labor
							if f.Name == "Farm" || f.Name == "Potato farm" {
								farm += f.Labor
							}
						}
						out.FarmShare[i] += farm / math.Max(lab, 1e-9)
					}
					mean := wsum / people
					out.WageSpread += math.Sqrt(math.Max(0, wsq/people-mean*mean)) / mean
					total := 0.0
					for _, v := range mapOut {
						total += v
					}
					ksi, weight := 0.0, 0.0
					for i := range w.Regions {
						tt := 0.0
						for _, v := range tileOut[i] {
							tt += v
						}
						if tt <= 0 {
							continue
						}
						k := 0.0
						for g := range mapOut {
							k += math.Abs(tileOut[i][g]/tt - mapOut[g]/total)
						}
						ksi += k / 2 * tt
						weight += tt
					}
					out.Specialisation += ksi / weight
				}
				out.WageSpread /= samples
				out.Specialisation /= samples
				for i := range out.FarmShare {
					out.FarmShare[i] /= samples
				}
			})
		}
	}

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
				w := world()
				tile := -1
				out.Boom, out.Migration = k, mig
				samples := 0.0
				for d := 0; d < end; d++ {
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

	parallel(base, strike(true, true, &shocked), strike(false, true, &control), strike(true, false, &shockedStill),
		strike(false, false, &controlStill), blight(true, &blighted), blight(false, &unblighted))
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
	parallel(append(append(tradeJobs, boomJobs...), corridorJobs...)...)
	write("corridor.json", map[string]any{"route": route, "runs": corridorRuns})
	write("trade.json", map[string]any{"runs": tradeRuns})
	write("boom.json", map[string]any{"runs": boomRuns})

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
	w.paths = nil
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
		for _, nb := range w.adj[cur.ID] {
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
