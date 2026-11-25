package game

import (
	"math"
	"testing"
)

// A lone tile settles to a fixed point. Renting land anew each day made a
// farm's supply so elastic that wool's market went round a five-day cycle;
// a lease keeps what a farm works steady.
func TestLoneTileSettles(t *testing.T) {
	w, err := baseGameWorld(GameSpec{Map: []string{"P"}, TransportCost: 1})
	if err != nil {
		t.Fatal(err)
	}
	for range int(SettleYears * TicksPerYear) {
		w.Step()
	}
	r := w.Regions[0]
	prev := r.Prices()
	worst, which := 0.0, ""
	for range 60 {
		w.Step()
		p := r.Prices()
		for g, m := range r.Markets {
			if m.Supply > 0 && m.Demand > 0 && prev[g] > 0 {
				if d := math.Abs(math.Log(p[g] / prev[g])); d > worst {
					worst, which = d, Goods[g].Name
				}
			}
		}
		prev = p
	}
	if worst > 2e-4 {
		t.Errorf("a settled lone tile's %s price still moves %.2e a day", which, worst)
	}
}

// A surplus is cut quickly: with more than twice its reserve in the
// treasury the computer cuts taxes as fast as it raises them at war, else a
// step at a time. When a building programme ended, a nation's treasury
// once piled up to more than three times its reserve while its taxes came
// down a step a month.
func TestSurplusIsCutQuickly(t *testing.T) {
	w := newTestGame(t)
	n := w.Nations[0]
	cut := func(reserves float64) float64 {
		n.SetTaxRate(0.2)
		n.SpendEMA, n.IncomeEMA = 100, 200 // taxes bring in twice what it spends
		n.Treasury = reserves * n.reserve()
		w.aiTax(n)
		return 0.2 - n.TaxRate
	}
	if c := cut(1.5); math.Abs(c-aiTaxStep) > 1e-12 {
		t.Errorf("with 1.5 reserves in the treasury, taxes cut by %v, want a step (%v)", c, aiTaxStep)
	}
	if c := cut(3); math.Abs(c-aiTaxStep*aiWarTaxSpeed) > 1e-12 {
		t.Errorf("with 3 reserves in the treasury, taxes cut by %v, want %v", c, aiTaxStep*aiWarTaxSpeed)
	}
}
