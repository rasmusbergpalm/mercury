package economy_test

import (
	"fmt"

	"github.com/rasmusbergpalm/mercury/economy"
)

// The example economy reads a tick as a day.
const ticksPerYear = 365

// example is the example economy on its 6×6 example map.
func example() (*economy.World, error) {
	g, err := economy.HexGraph(economy.DefaultMap, nil)
	if err != nil {
		return nil, err
	}
	return economy.BuildWorld(1, g)
}

// The example economy on its 6×6 example map, with owners investing and people free to
// move, run for two years: the price of bread, and the people, in the
// map's first and last tiles.
func Example() {
	w, err := example()
	if err != nil {
		panic(err)
	}
	w.Investment, w.Migration = true, true
	for tick := 0; tick < 2*ticksPerYear; tick++ {
		w.Step()
	}
	bread := economy.G("bread")
	for _, r := range []*economy.Region{w.Regions[0], w.Regions[len(w.Regions)-1]} {
		fmt.Printf("tile %s: bread £%.2f, %.0f people\n", r.Name, r.Markets[bread].Price, r.Population())
	}
	// Output:
	// tile 00: bread £24.24, 87 people
	// tile 55: bread £24.20, 134 people
}

// A 20% tax on one tile's incomes: what it has withheld after a year.
func Example_tax() {
	w, err := example()
	if err != nil {
		panic(err)
	}
	r := w.Regions[14]
	r.IncomeTax = 0.2
	for tick := 0; tick < ticksPerYear; tick++ {
		w.Step()
	}
	fmt.Printf("withheld £%.0f\n", r.Withheld)
	// Output:
	// withheld £41643
}

// A customs border down the middle of the map: a 30% duty on everything
// that crosses it, both ways. Trade across the border falls, and the duty
// piles up in the tiles where the goods are sold. (Trade is valued where
// it is bought, the duty where it is sold, after the cost of carriage.)
func Example_tariff() {
	run := func(duty float64) (crossing, duties float64) {
		w, err := example()
		if err != nil {
			panic(err)
		}
		west := func(r *economy.Region) bool { return r.Col < 3 }
		for _, t := range w.Traders {
			if west(t.Origin) != west(t.Dest) {
				t.Tariff = duty
			}
		}
		for tick := 0; tick < ticksPerYear; tick++ {
			w.Step()
			for _, t := range w.Traders {
				if west(t.Origin) != west(t.Dest) {
					crossing += t.Shipped * t.Origin.Markets[t.Good].Price / ticksPerYear
				}
			}
		}
		for _, r := range w.Regions {
			duties += r.Duties / ticksPerYear
		}
		return crossing, duties
	}
	free, _ := run(0)
	taxed, duties := run(0.3)
	fmt.Printf("trade across the border: £%.0f a day free, £%.0f with the duty, which raises £%.0f a day\n", free, taxed, duties)
	// Output:
	// trade across the border: £879 a day free, £224 with the duty, which raises £154 a day
}
