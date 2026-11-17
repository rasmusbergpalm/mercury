package engine

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// TestIcebergComparison runs the economy two ways and compares them, on the
// classic map with no nations (as TestTransportSweep):
//
//   - explicit: merchants buy transport, a good transport firms make from
//     labour, per unit carried: cost x distance x the good's bulk;
//   - iceberg: merchants buy no transport; instead a share of every
//     shipment melts on the way, the same for every good (Samuelson's
//     iceberg, the standard in trade and spatial economics): exp(-w x cost
//     x distance) arrives. The melt rate w is calibrated so that at normal
//     costs (x1) trade loses the same share of its value per hop as it pays
//     for carriage in the explicit model.
//
// Each runs five years at each transport cost. Recorded: the living
// standard; output per head, net of what trade uses up (transport in the
// explicit model, melted goods in the iceberg), at the explicit x1 run's
// prices; the share of workers in transport; the price gap (priceGap).
// And, for the Alchian-Allen effect, each good's hops travelled per unit
// made at x1, against its value per unit of bulk: a cost per unit carried
// makes goods worth much for their bulk travel further; a cost in
// proportion to value does not.
//
//	ICEBERG_OUT=/tmp/iceberg go test ./engine -run TestIcebergComparison -timeout 60m
func TestIcebergComparison(t *testing.T) {
	dir := os.Getenv("ICEBERG_OUT")
	if dir == "" {
		t.Skip("set ICEBERG_OUT to run")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	const years = 5
	days := int(TicksPerYear)
	costs := []float64{}
	for k := -4; k <= 7; k++ { // x1/16 to x128
		costs = append(costs, math.Pow(2, float64(k)))
	}
	const autarky = 1e6
	costs = append(costs, autarky)

	type goodRow struct {
		Good                      string
		Bulk, Price, Made, Hops   float64 // hops: unit-hops shipped a day
		ValuePerBulk, HopsPerUnit float64
	}
	type result struct {
		Model, Cost                    float64 // model 0 explicit, 1 iceberg
		Living, GDPHead, TransportJobs float64
		PriceGap, TradeShare           float64 // trade share: value shipped / value made
		Goods                          []goodRow
		prices                         []float64
		net, lost                      []float64
		people                         float64
	}
	run := func(iceberg bool, c, melt float64) *result {
		spec := DefaultGame
		spec.TransportCost = c
		w, err := baseGameWorld(spec)
		if err != nil {
			t.Fatal(err)
		}
		w.Migration, w.Investment, w.Demography = true, true, true
		if iceberg {
			for _, tr := range w.Traders {
				tr.Tau = 0
				tr.Arriving *= math.Exp(-melt * c * tr.Distance)
			}
		}
		r := &result{Cost: c, net: make([]float64, NumGoods), lost: make([]float64, NumGoods), prices: make([]float64, NumGoods)}
		if iceberg {
			r.Model = 1
		}
		made, hops := make([]float64, NumGoods), make([]float64, NumGoods)
		shippedValue, madeValue := 0.0, 0.0
		last := (years - 1) * days
		for d := 0; d < years*days; d++ {
			w.Step()
			if d >= years*days-30 {
				r.PriceGap += priceGap(w) / 30
			}
			if d < last {
				continue
			}
			for _, reg := range w.Regions {
				for _, f := range reg.Firms {
					r.net[f.Output] += f.LastOutput / TicksPerYear
					made[f.Output] += f.LastOutput / TicksPerYear
					madeValue += f.LastOutput * reg.Markets[f.Output].Price / TicksPerYear
					for g, q := range f.used {
						r.net[g] -= q / TicksPerYear
					}
				}
				for _, tr := range reg.House.today {
					r.lost[Transport] += tr.usedTransport / TicksPerYear // the explicit model's carriage
				}
				p := reg.Population()
				r.Living += reg.Utility() * p / TicksPerYear
				r.people += p / TicksPerYear
			}
			for _, tr := range w.Traders {
				hops[tr.Good] += tr.Shipped * tr.Distance / TicksPerYear
				shippedValue += tr.Shipped * tr.Origin.Markets[tr.Good].Price / TicksPerYear
				if iceberg { // what melts on the way is trade's input
					r.lost[tr.Good] += tr.Shipped * (1 - math.Exp(-melt*c*tr.Distance)) / TicksPerYear
				}
			}
		}
		r.Living /= r.people
		r.TradeShare = shippedValue / madeValue
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
		r.TransportJobs = tj / lt
		for _, g := range GameTradable {
			if made[g] <= 0 || Goods[g].Bulk <= 0 {
				continue
			}
			r.Goods = append(r.Goods, goodRow{Good: Goods[g].Name, Bulk: Goods[g].Bulk, Price: r.prices[g], Made: made[g], Hops: hops[g],
				ValuePerBulk: r.prices[g] / Goods[g].Bulk, HopsPerUnit: hops[g] / made[g]})
		}
		return r
	}

	// calibrate the melt: at x1, carriage as a share of the value carried, per hop
	melt := func() float64 {
		spec := DefaultGame
		w, err := baseGameWorld(spec)
		if err != nil {
			t.Fatal(err)
		}
		w.Migration, w.Investment, w.Demography = true, true, true
		carriage, valueHops := 0.0, 0.0
		for d := 0; d < years*days; d++ {
			w.Step()
			if d < (years-1)*days {
				continue
			}
			for _, tr := range w.Traders {
				pT := tr.Origin.Markets[Transport].Price
				carriage += tr.Shipped * tr.Tau * pT
				valueHops += tr.Shipped * tr.Origin.Markets[tr.Good].Price * tr.Distance
			}
		}
		return carriage / valueHops
	}()
	t.Logf("iceberg melt: %.4f of the value per hop at x1", melt)

	var results []*result
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for _, iceberg := range []bool{false, true} {
		for _, c := range costs {
			wg.Add(1)
			go func(iceberg bool, c float64) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				r := run(iceberg, c, melt)
				mu.Lock()
				results = append(results, r)
				mu.Unlock()
			}(iceberg, c)
		}
	}
	wg.Wait()
	var ref []float64 // the explicit x1 run's prices value every run's output
	for _, r := range results {
		if r.Model == 0 && r.Cost == 1 {
			ref = r.prices
		}
	}
	for _, r := range results {
		gdp := 0.0
		for g := range r.net {
			gdp += (r.net[g] - r.lost[g]) * ref[g]
		}
		r.GDPHead = gdp / r.people
	}
	b, err := json.Marshal(map[string]any{"melt": melt, "runs": results})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "iceberg.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
}
