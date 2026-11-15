package engine

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
)

// TestTransportSweep measures what transport is worth: the classic map with
// no nations (no state, army, roads or new technology), run for five years at
// each of a range of transport costs, from nearly free to so dear that no
// route pays (autarky). For each it records welfare (the living standard)
// and real GDP: the fifth year's net output (what firms made less the goods
// firms and merchants used up) valued at one set of prices for every run,
// the transport cost x1 run's, so that runs compare like for like. GDP is
// given twice: with the transport merchants use counted as an input to
// trade (as the game measures it), and counted as output. Market
// integration is the price gap between a good's dearest and cheapest tile
// (priceGap), averaged over the last 30 days. The x1 run also writes its
// map: each tile's prices and each route's shipments (writeSweepMap).
//
//	TRANSPORT_SWEEP_OUT=/tmp/sweep go test ./engine -run TestTransportSweep -timeout 30m
func TestTransportSweep(t *testing.T) {
	dir := os.Getenv("TRANSPORT_SWEEP_OUT")
	if dir == "" {
		t.Skip("set TRANSPORT_SWEEP_OUT to run")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	const years = 5
	costs := []float64{}
	for k := -9; k <= 14; k++ { // x0.044 to x128 in steps of a square root of two, through x1
		costs = append(costs, math.Pow(2, float64(k)/2))
	}
	const autarky = 1e6
	costs = append(costs, autarky)
	seeds := []int64{1} // without nations nothing is drawn at random: every seed gives the same run
	type result struct {
		living, people, transportJobs float64
		gap                           float64   // market integration: see priceGap
		net, used                     []float64 // per good, a day's average over the last year
		prices                        []float64 // average over tiles, at the end
	}
	res := map[[2]int]*result{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for ci, c := range costs {
		for si, seed := range seeds {
			wg.Add(1)
			go func(ci, si int, c float64, seed int64) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				spec := DefaultGame
				spec.TransportCost, spec.Seed = c, seed
				w, err := baseGameWorld(spec)
				if err != nil {
					t.Error(err)
					return
				}
				w.Migration, w.Investment, w.Demography = true, true, true
				r := &result{net: make([]float64, NumGoods), used: make([]float64, NumGoods), prices: make([]float64, NumGoods)}
				days := int(TicksPerYear)
				last := (years - 1) * days
				month := years*days - 30
				price := make([][]float64, len(w.Regions)) // the last 30 days' average price, per tile and good
				flow := map[*Trader]float64{}              // and each route's average daily shipment
				for i := range price {
					price[i] = make([]float64, NumGoods)
				}
				for d := 0; d < years*days; d++ {
					w.Step()
					if d >= month {
						r.gap += priceGap(w) / 30
						for i, reg := range w.Regions {
							for g, m := range reg.Markets {
								price[i][g] += m.Price / 30
							}
						}
						for _, tr := range w.Traders {
							if tr.Shipped > 0 {
								flow[tr] += tr.Shipped / 30
							}
						}
					}
					if d < last {
						continue
					}
					for _, reg := range w.Regions {
						for _, f := range reg.Firms {
							r.net[f.Output] += f.LastOutput / TicksPerYear
							for g, q := range f.used {
								r.net[g] -= q / TicksPerYear
							}
						}
						for _, tr := range reg.House.today {
							r.used[Transport] += tr.usedTransport / TicksPerYear
						}
						people := reg.Population()
						r.living += reg.Utility() * people / TicksPerYear
						r.people += people / TicksPerYear
					}
				}
				r.living /= r.people
				lt, tj := 0.0, 0.0
				for _, reg := range w.Regions {
					for _, f := range reg.Firms {
						lt += f.Labor
						if f.Output == Transport {
							tj += f.Labor
						}
					}
					for g, m := range reg.Markets {
						r.prices[g] += m.Price / float64(len(w.Regions))
					}
				}
				r.transportJobs = tj / lt
				if c == 1 {
					writeSweepMap(t, dir, w, price, flow)
				}
				mu.Lock()
				res[[2]int{ci, si}] = r
				mu.Unlock()
			}(ci, si, c, seed)
		}
	}
	wg.Wait()
	ref := res[[2]int{indexOf(costs, 1), 0}].prices // the x1 run's prices
	var b strings.Builder
	b.WriteString("cost,seed,living,people,transportJobs,gdpHead,gdpHeadWithTransport,priceGap\n")
	for ci, c := range costs {
		for si, seed := range seeds {
			r := res[[2]int{ci, si}]
			if r == nil {
				continue
			}
			gdp, withT := 0.0, 0.0
			for g := range r.net {
				withT += r.net[g] * ref[g]
				gdp += (r.net[g] - r.used[g]) * ref[g]
			}
			fmt.Fprintf(&b, "%g,%d,%.4f,%.0f,%.4f,%.5f,%.5f,%.5f\n", c, seed, r.living, r.people, r.transportJobs, gdp/r.people, withT/r.people, r.gap)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "sweep.csv"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

func indexOf(xs []float64, x float64) int {
	best := 0
	for i, v := range xs {
		if math.Abs(math.Log(v/x)) < math.Abs(math.Log(xs[best]/x)) {
			best = i
		}
	}
	return best
}

// priceGap measures how far apart a market's prices are: for each good,
// how much dearer it is in its dearest tile than in its cheapest (max/min
// - 1), among the tiles where it changes hands (some sell it and some buy
// it: elsewhere its price means nothing); the median over the goods traded
// in two tiles or more, so that one good's wild price in an empty market
// doesn't swamp the rest.
func priceGap(w *World) float64 {
	var gaps []float64
	for _, g := range GameTradable {
		lo, hi, n := math.Inf(1), 0.0, 0
		for _, r := range w.Regions {
			if m := r.Markets[g]; m.Supply > 0 && m.Demand > 0 {
				lo, hi, n = math.Min(lo, m.Price), math.Max(hi, m.Price), n+1
			}
		}
		if n >= 2 {
			gaps = append(gaps, hi/lo-1)
		}
	}
	if len(gaps) == 0 {
		return 0
	}
	sort.Float64s(gaps)
	if k := len(gaps); k%2 == 1 {
		return gaps[k/2]
	} else {
		return (gaps[k/2-1] + gaps[k/2]) / 2
	}
}

// writeSweepMap writes the x1 run's map: each tile's place, terrain and
// average price of every good over the last 30 days (map-prices.csv), and
// every route's average daily shipment (map-flows.csv).
func writeSweepMap(t *testing.T, dir string, w *World, price [][]float64, flow map[*Trader]float64) {
	var b strings.Builder
	b.WriteString("tile,name,row,col,terrain,coastal,good,price,active,made\n")
	for i, r := range w.Regions {
		made := make([]float64, NumGoods)
		for _, f := range r.Firms {
			made[f.Output] += f.LastOutput
		}
		for g := range Goods {
			m := r.Markets[g]
			fmt.Fprintf(&b, "%d,%s,%d,%d,%s,%t,%s,%.5f,%t,%.4f\n", i, r.Name, r.Row, r.Col, r.Terrain, r.Coastal, Goods[g].Name, price[i][g], m.Supply > 0 && m.Demand > 0, made[g])
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "map-prices.csv"), []byte(b.String()), 0o644); err != nil {
		t.Error(err)
	}
	b.Reset()
	b.WriteString("good,from,to,shipped\n")
	for _, tr := range w.Traders {
		if q := flow[tr]; q > 0 {
			fmt.Fprintf(&b, "%s,%d,%d,%.5f\n", Goods[tr.Good].Name, tr.Origin.ID, tr.Dest.ID, q)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "map-flows.csv"), []byte(b.String()), 0o644); err != nil {
		t.Error(err)
	}
}
