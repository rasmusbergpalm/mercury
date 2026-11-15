package engine

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
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
// trade (as the game measures it), and counted as output.
//
//	TRANSPORT_SWEEP_OUT=/tmp/sweep go test ./engine -run TestTransportSweep -timeout 30m
func TestTransportSweep(t *testing.T) {
	dir := os.Getenv("TRANSPORT_SWEEP_OUT")
	if dir == "" {
		t.Skip("set TRANSPORT_SWEEP_OUT to run")
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
				for d := 0; d < years*days; d++ {
					w.Step()
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
				mu.Lock()
				res[[2]int{ci, si}] = r
				mu.Unlock()
			}(ci, si, c, seed)
		}
	}
	wg.Wait()
	ref := res[[2]int{indexOf(costs, 1), 0}].prices // the x1 run's prices
	var b strings.Builder
	b.WriteString("cost,seed,living,people,transportJobs,gdpHead,gdpHeadWithTransport\n")
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
			fmt.Fprintf(&b, "%g,%d,%.4f,%.0f,%.4f,%.5f,%.5f\n", c, seed, r.living, r.people, r.transportJobs, gdp/r.people, withT/r.people)
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
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
