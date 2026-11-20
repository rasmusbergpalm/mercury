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
	// tile 00: bread £28.13, 84 people
	// tile 55: bread £32.37, 138 people
}

// A 20% tax on one tile's incomes: what it has withheld after a year.
func Example_tax() {
	w, err := economy.BuildWorld(1, economy.DefaultMap)
	if err != nil {
		panic(err)
	}
	r := w.Regions[14]
	r.IncomeTax = 0.2
	for day := 0; day < economy.TicksPerYear; day++ {
		w.Step()
	}
	fmt.Printf("withheld £%.0f\n", r.Withheld)
	// Output:
	// withheld £40908
}

// A customs border down the middle of the map: a 30% duty on everything
// that crosses it, both ways. Trade across the border falls, and the duty
// piles up in the tiles where the goods are sold. (Trade is valued where
// it is bought, the duty where it is sold, after the cost of carriage.)
func Example_tariff() {
	run := func(duty float64) (crossing, duties float64) {
		w, err := economy.BuildWorld(1, economy.DefaultMap)
		if err != nil {
			panic(err)
		}
		west := func(r *economy.Region) bool { return r.Col < 3 }
		for _, t := range w.Traders {
			if west(t.Origin) != west(t.Dest) {
				t.Tariff = duty
			}
		}
		for day := 0; day < economy.TicksPerYear; day++ {
			w.Step()
			for _, t := range w.Traders {
				if west(t.Origin) != west(t.Dest) {
					crossing += t.Shipped * t.Origin.Markets[t.Good].Price / economy.TicksPerYear
				}
			}
		}
		for _, r := range w.Regions {
			duties += r.Duties / economy.TicksPerYear
		}
		return crossing, duties
	}
	free, _ := run(0)
	taxed, duties := run(0.3)
	fmt.Printf("trade across the border: £%.0f a day free, £%.0f with the duty, which raises £%.0f a day\n", free, taxed, duties)
	// Output:
	// trade across the border: £943 a day free, £234 with the duty, which raises £141 a day
}
