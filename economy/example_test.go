package economy_test

import (
	"fmt"

	"github.com/rasmusbergpalm/mercury/economy"
)

// A world on the classic 6×6 map, with owners investing and people free to
// move, run for two years: the price of bread, and the people, in the
// map's first and last tiles.
func Example() {
	w, err := economy.BuildFullWorld(1, economy.DefaultMap, nil)
	if err != nil {
		panic(err)
	}
	w.Investment, w.Migration = true, true
	for day := 0; day < 2*economy.TicksPerYear; day++ {
		w.Step()
	}
	bread := economy.G("bread")
	for _, r := range []*economy.Region{w.Regions[0], w.Regions[len(w.Regions)-1]} {
		fmt.Printf("tile %s: bread £%.2f, %.0f people\n", r.Name, r.Markets[bread].Price, r.Population())
	}
	// Output:
	// tile 00: bread £28.02, 84 people
	// tile 55: bread £32.38, 138 people
}

// A 20% tax on one tile's incomes: what it has withheld after a year.
func Example_tax() {
	w, err := economy.BuildWorld(1, economy.DefaultMap)
	if err != nil {
		panic(err)
	}
	r := w.Regions[14]
	r.TaxFn = func(amount float64) float64 { return 0.2 * amount }
	for day := 0; day < economy.TicksPerYear; day++ {
		w.Step()
	}
	fmt.Printf("withheld £%.0f\n", r.Withheld)
	// Output:
	// withheld £40970
}
