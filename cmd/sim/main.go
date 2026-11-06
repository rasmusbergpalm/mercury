// Command sim runs the default world headless and prints a report, like
// reference/main.py. With -sweep it also compares transport costs.
package main

import (
	"flag"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/rasmusbergpalm/mercury/engine"
)

const window = 50 // ticks averaged over at the end of a run

// averages accumulates per-tick statistics over the final window.
type averages struct {
	utility, wage []float64
	tierSpend     [][engine.NumTiers]float64
	relPrice      [][]float64 // price / wage
	labor         map[string]float64
	flows         []float64
	volume        float64
	transport     float64
}

func run(cost float64, ticks int) (*engine.World, *averages) {
	w, err := engine.BuildWorld(cost, engine.DefaultMap)
	if err != nil {
		panic(err)
	}
	n := len(w.Regions)
	a := &averages{utility: make([]float64, n), wage: make([]float64, n),
		tierSpend: make([][engine.NumTiers]float64, n), relPrice: make([][]float64, n),
		labor: map[string]float64{}, flows: make([]float64, len(w.Traders))}
	for i := range a.relPrice {
		a.relPrice[i] = make([]float64, engine.NumGoods)
	}
	for t := 0; t < ticks; t++ {
		w.Step()
		if t < ticks-window {
			continue
		}
		for i, r := range w.Regions {
			a.utility[i] += r.Utility() / window
			a.wage[i] += r.Wage / window
			ts := r.TierSpend(w.TradePrices[i])
			for k := range ts {
				a.tierSpend[i][k] += ts[k] / window
			}
			for g, m := range r.Markets {
				a.relPrice[i][g] += m.Price / r.Wage / window
			}
			for _, f := range r.Firms {
				a.labor[f.Name] += f.Labor / window
				if f.Name == "Transport" {
					a.transport += f.Labor / window
				}
			}
		}
		for i, tr := range w.Traders {
			a.flows[i] += tr.Shipped / window
			a.volume += tr.Shipped / window
		}
	}
	return w, a
}

func (a *averages) shares(i int) [engine.NumTiers]float64 {
	total := 0.0
	for _, s := range a.tierSpend[i] {
		total += s
	}
	var out [engine.NumTiers]float64
	for k, s := range a.tierSpend[i] {
		out[k] = s / math.Max(total, 1e-12)
	}
	return out
}

func hexMap(w *engine.World, cell func(r *engine.Region) string) string {
	var b strings.Builder
	for row := 0; row < w.Rows; row++ {
		if row%2 == 1 {
			b.WriteString("    ")
		}
		var cells []string
		for col := 0; col < w.Cols; col++ {
			cells = append(cells, cell(w.Regions[row*w.Cols+col]))
		}
		b.WriteString(strings.Join(cells, "  ") + "\n")
	}
	return b.String()
}

func report(w *engine.World, a *averages) {
	fmt.Printf("After %d ticks (averages over the last %d). Money in system: %.0f\n\n", w.Tick, window, w.Money())
	fmt.Println("Terrain  (P plains, F forest, H hills, M mountains, T tropical, C coast)")
	fmt.Print(hexMap(w, func(r *engine.Region) string {
		return fmt.Sprintf("%s:%c  ", r.Name, engine.DefaultMap[r.Row][r.Col])
	}))
	fmt.Println("\nUtility per person")
	fmt.Print(hexMap(w, func(r *engine.Region) string { return fmt.Sprintf("%6.2f", a.utility[r.ID]) }))
	fmt.Println("\nLuxury share of pop spending (%)")
	fmt.Print(hexMap(w, func(r *engine.Region) string { return fmt.Sprintf("%6.1f", 100*a.shares(r.ID)[engine.Luxury]) }))
	for _, g := range []string{"bread", "coal", "fish", "tea", "porcelain"} {
		fmt.Printf("\nPrice of %s relative to the local wage\n", g)
		gid := engine.G(g)
		fmt.Print(hexMap(w, func(r *engine.Region) string { return fmt.Sprintf("%6.2f", a.relPrice[r.ID][gid]) }))
	}

	fmt.Println("\nBy terrain:")
	fmt.Printf("  %-10s %2s %8s %12s %12s %12s\n", "terrain", "n", "utility", "basic", "intermediate", "luxury")
	for _, t := range engine.Terrains {
		var ids []int
		for _, r := range w.Regions {
			if r.Terrain == t.Name {
				ids = append(ids, r.ID)
			}
		}
		if len(ids) == 0 {
			continue
		}
		u := 0.0
		var sh [engine.NumTiers]float64
		for _, id := range ids {
			u += a.utility[id] / float64(len(ids))
			s := a.shares(id)
			for k := range sh {
				sh[k] += s[k] / float64(len(ids))
			}
		}
		fmt.Printf("  %-10s %2d %8.2f %11.1f%% %11.1f%% %11.1f%%\n", t.Name, len(ids), u, 100*sh[0], 100*sh[1], 100*sh[2])
	}

	fmt.Println("\nEmployment by firm type (whole map, people):")
	var names []string
	for n := range a.labor {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool { return a.labor[names[i]] > a.labor[names[j]] })
	for _, n := range names {
		fmt.Printf("  %-18s %7.1f\n", n, a.labor[n])
	}

	fmt.Println("\nBusiest trade links (units loaded per tick):")
	idx := make([]int, len(w.Traders))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(i, j int) bool { return a.flows[idx[i]] > a.flows[idx[j]] })
	for _, i := range idx[:min(15, len(idx))] {
		fmt.Printf("  %-26s %6.2f\n", w.Traders[i].Name(), a.flows[i])
	}
}

func sweep(ticks int) {
	fmt.Println("\nTransport-cost sweep (multiplier on every hop):")
	fmt.Printf("%6s %10s %18s %8s %13s %13s %13s\n", "cost", "trade vol", "transport workers", "utility", "basic", "intermediate", "luxury")
	for _, cost := range []float64{0.25, 0.5, 1, 2, math.Inf(1)} {
		w, a := run(cost, ticks)
		u := 0.0
		var sh [engine.NumTiers]float64
		n := float64(len(w.Regions))
		for i := range w.Regions {
			u += a.utility[i] / n
			s := a.shares(i)
			for k := range sh {
				sh[k] += s[k] / n
			}
		}
		label := fmt.Sprintf("%.2f", cost)
		if math.IsInf(cost, 1) {
			label = "inf"
		}
		fmt.Printf("%6s %10.1f %18.1f %8.3f %12.1f%% %12.1f%% %12.1f%%\n", label, a.volume, a.transport, u, 100*sh[0], 100*sh[1], 100*sh[2])
	}
}

func main() {
	ticks := flag.Int("ticks", 1000, "ticks to run")
	cost := flag.Float64("cost", 1, "transport cost multiplier")
	doSweep := flag.Bool("sweep", false, "also sweep transport cost")
	flag.Parse()
	w, a := run(*cost, *ticks)
	report(w, a)
	if *doSweep {
		sweep(*ticks)
	}
}
