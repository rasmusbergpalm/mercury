package engine

import (
	"math"
	"testing"
)

// The building budget: 60% of income while the treasury holds its reserve,
// tapering to nothing at half of it, plus a thirtieth of anything beyond.
func TestBuildingBudgetTapers(t *testing.T) {
	n := &Nation{SpendEMA: 100, IncomeEMA: 100}
	reserve := aiTreasuryDays * 100
	for _, c := range []struct{ treasury, want float64 }{
		{0, 0}, {0.5 * reserve, 0}, {0.75 * reserve, 30}, {reserve, 60}, {2 * reserve, 60 + reserve/30},
	} {
		n.Treasury = c.treasury
		if got := n.BuildingBudget(); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("treasury %v: budget %v, want %v", c.treasury, got, c.want)
		}
	}
}

// The state's budget holds steady: in the classic game, five years from the
// day it opens, no nation is ever short of money to pay its bills, its tax
// stays in bounds and its treasury doesn't run dry. (Before the building
// budget and the weekly tax, the treasuries emptied and refilled in a cycle
// about a year long, with 250 days a nation couldn't pay.)
func TestStateBudgetIsSteady(t *testing.T) {
	if testing.Short() {
		t.Skip("long")
	}
	spec := DefaultGame
	spec.AI, spec.Seed = true, 1
	w, err := BuildGame(spec)
	if err != nil {
		t.Fatal(err)
	}
	short := make([]int, len(w.Nations))
	lowest := math.Inf(1)
	for d := 0; d < int(5*TicksPerYear); d++ {
		w.Step()
		if d < 90 {
			continue
		}
		for i, n := range w.Nations {
			if n.payScale < 0.999 {
				short[i]++
			}
			if n.TaxRate < 0.01 || n.TaxRate > 0.25 {
				t.Fatalf("day %d: %s taxes at %.1f%%", d, n.Name, 100*n.TaxRate)
			}
			lowest = math.Min(lowest, n.Treasury/n.reserve())
		}
	}
	for i, n := range w.Nations {
		if short[i] > 0 {
			t.Errorf("%s couldn't pay its bills on %d days", n.Name, short[i])
		}
	}
	if lowest < 0.25 {
		t.Errorf("a treasury fell to %.2f of its reserve", lowest)
	}
	checkFinite(t, w)
}
