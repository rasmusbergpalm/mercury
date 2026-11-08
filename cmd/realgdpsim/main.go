// Command realgdpsim runs the default map with a single computer nation from
// 1820 to 1920 and prints its yearly real and nominal GDP as CSV.
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/rasmusbergpalm/mercury/engine"
)

func main() {
	spec := engine.DefaultGame
	spec.NationMap = []string{"AA....", "AA....", "......", "......", "......", "......"}
	spec.Nations = spec.Nations[:1]
	spec.AI, spec.Seed = true, 1
	w, err := engine.BuildGame(spec)
	if err != nil {
		panic(err)
	}
	n := w.Nations[0]
	start := time.Now()
	fmt.Println("year,real_gdp,nominal_gdp,population,tiles,utility,techs,money,tax")
	for y := 0; y <= 100; y++ {
		if y > 0 {
			for d := 0; d < 365; d++ {
				w.Step()
			}
		}
		for w.Tick < engine.HistoryEvery {
			w.Step()
		}
		h := w.History.Nations[0]
		last := len(h.GDP) - 1
		nominal := 0.0
		for _, r := range n.Regions {
			for _, f := range r.Firms {
				nominal += f.ValueAdded(r.Markets[f.Output].Price)
			}
		}
		fmt.Printf("%d,%.2f,%.2f,%.1f,%d,%.3f,%.0f,%.0f,%.3f\n", 1820+y, h.GDP[last], nominal, h.Population[last],
			len(n.Regions), h.Utility[last], h.Techs[last], w.Money(), n.TaxRate)
		fmt.Fprintf(os.Stderr, "\r%d (%s)", 1820+y, time.Since(start).Round(time.Second))
	}
	fmt.Fprintln(os.Stderr)
}
