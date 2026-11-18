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

// flatTax is the simplest government: it takes a share of every income
// and keeps it.
type flatTax struct {
	rate, treasury float64
}

func (g *flatTax) IncomeTax(amount float64) float64 { return g.rate * amount }
func (g *flatTax) Duty(from *economy.Region, good economy.GoodID) (float64, bool) {
	return 0, false
}
func (g *flatTax) PaybackMult() float64    { return 1 }
func (g *flatTax) GrowthBonus() float64    { return 0 }
func (g *flatTax) CoastMult() float64      { return 1 }
func (g *flatTax) SpoilageMult() float64   { return 1 }
func (g *flatTax) Settle(l economy.Ledger) { g.treasury += l.Tax + l.Paid + l.Tariffs }

// A government taxing one tile's incomes at 20%: what it has collected
// after a year.
func ExampleGovernment() {
	w, err := economy.BuildWorld(1, economy.DefaultMap)
	if err != nil {
		panic(err)
	}
	g := &flatTax{rate: 0.2}
	w.Regions[14].Gov = g
	for day := 0; day < economy.TicksPerYear; day++ {
		w.Step()
	}
	fmt.Printf("collected £%.0f\n", g.treasury)
	// Output:
	// collected £40970
}
