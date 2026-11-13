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

// The mint: when the world's prices fall, it issues money to the nations
// by the size of their economies, over mintDays; when they rise, nothing.
func TestMintIssuesWhenPricesFall(t *testing.T) {
	w := newTestGame(t)
	w.Tick = gdpWarmup + 1
	w.PriceLevel = 1
	links := make([]gdpLink, len(w.Regions))
	for i, r := range w.Regions {
		// net output 1 at last month's prices, 0.9 at this month's; this
		// month's value: nations' tiles only, one tile worth twice the rest
		links[i] = gdpLink{l0: 1, l1: 1, q0: 0.9, q1: 0.9, now: 1}
		if r == w.Nations[0].Regions[0] {
			links[i].now = 2
		}
	}
	w.linkPriceLevel(links)
	if math.Abs(w.PriceLevel-0.9) > 1e-12 {
		t.Fatalf("price level %v, want 0.9", w.PriceLevel)
	}
	money := w.Money()
	want := (1/0.9 - 1) * money / mintDays
	total, nominal := 0.0, 0.0
	for _, n := range w.Nations {
		total += n.MintRate
		for _, r := range n.Regions {
			nominal += links[r.ID].now
		}
	}
	if math.Abs(total-want) > 1e-9*want {
		t.Errorf("issues %v a day, want %v", total, want)
	}
	n := w.Nations[0]
	share := float64(len(n.Regions)+1) / nominal
	if math.Abs(n.MintRate/total-share) > 1e-9 {
		t.Errorf("%s gets %v of the new money, want its share of output %v", n.Name, n.MintRate/total, share)
	}
	treasury := n.Treasury
	w.mint()
	if n.Treasury-treasury != n.MintRate || math.Abs(w.Money()-money-total) > 1e-9*money || w.Minted != total {
		t.Errorf("mint paid %v (rate %v), money %v -> %v, minted %v", n.Treasury-treasury, n.MintRate, money, w.Money(), w.Minted)
	}
	// prices rising: nothing
	for i := range links {
		links[i].q0, links[i].q1 = 1.5, 1.5
	}
	w.linkPriceLevel(links)
	for _, n := range w.Nations {
		if n.MintRate != 0 {
			t.Errorf("prices at %v: %s still gets %v a day", w.PriceLevel, n.Name, n.MintRate)
		}
	}
}
