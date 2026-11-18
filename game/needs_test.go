package game

import (
	"math"
	"testing"
)

func spent(prices, x []float64) float64 {
	s := 0.0
	for g, q := range x {
		s += q * prices[g]
	}
	return s
}

func TestDemandSpendsBudget(t *testing.T) {
	prices := testPrices()
	x := make([]float64, NumGoods)
	for _, b := range []float64{0.01, 0.5, 3, 40, 1000} {
		Demand(prices, b, x)
		if s := spent(prices, x); math.Abs(s-b) > 1e-9*b {
			t.Errorf("budget %v: spent %v", b, s)
		}
		for g, q := range x {
			if q < 0 || (TierOf[g] < 0 && q != 0) {
				t.Errorf("budget %v: bad quantity %v of %s", b, q, Goods[g].Name)
			}
		}
	}
	Demand(prices, 0, x)
	if s := spent(prices, x); s != 0 {
		t.Errorf("zero budget: spent %v", s)
	}
}

// Moving a little money from any good to any other must not raise utility.
func TestDemandIsOptimal(t *testing.T) {
	prices := testPrices()
	x := make([]float64, NumGoods)
	for _, b := range []float64{0.3, 3, 30} {
		Demand(prices, b, x)
		u := Utility(x)
		eps := 1e-6 * b
		for _, from := range Needs {
			for _, a := range from.Goods {
				if x[a] < eps/prices[a] {
					continue // can't take money from a good we don't buy
				}
				for _, to := range Needs {
					for _, c := range to.Goods {
						y := append([]float64(nil), x...)
						y[a] -= eps / prices[a]
						y[c] += eps / prices[c]
						if gain := Utility(y) - u; gain > 1e-9*eps {
							t.Errorf("budget %v: moving money from %s to %s gains %v",
								b, Goods[a].Name, Goods[c].Name, gain)
						}
					}
				}
			}
		}
	}
}

func TestDemandFillsTiersInOrder(t *testing.T) {
	prices := make([]float64, NumGoods)
	for i := range prices {
		prices[i] = 1
	}
	x := make([]float64, NumGoods)
	share := func(b float64) [NumTiers]float64 {
		Demand(prices, b, x)
		var s [NumTiers]float64
		for g, q := range x {
			if TierOf[g] >= 0 {
				s[TierOf[g]] += q * prices[g] / b
			}
		}
		return s
	}
	// with every price at 1, the first non-basic need (home) enters at a budget of 0.02
	if s := share(0.01); s[Basic] < 0.999 {
		t.Errorf("a poor pop should only buy basic needs, got shares %v", s)
	}
	prev := share(0.01)
	for _, b := range []float64{0.2, 0.5, 1, 3, 10, 100} {
		s := share(b)
		if s[Basic] > prev[Basic]+1e-12 || s[Luxury] < prev[Luxury]-1e-12 {
			t.Errorf("budget %v: shares %v; basic share should fall and luxury rise with income (prev %v)", b, s, prev)
		}
		prev = s
	}
	if prev[Luxury] <= 0 {
		t.Errorf("a rich pop should buy luxuries, got shares %v", prev)
	}
}

func TestSubstitutesShiftToCheaperGood(t *testing.T) {
	prices := make([]float64, NumGoods)
	for i := range prices {
		prices[i] = 1
	}
	x := make([]float64, NumGoods)
	Demand(prices, 5, x)
	before := x[G("potatoes")] / x[G("bread")]
	prices[G("bread")] = 2
	Demand(prices, 5, x)
	if after := x[G("potatoes")] / x[G("bread")]; after <= before {
		t.Errorf("dearer bread should shift staples towards potatoes: ratio %v -> %v", before, after)
	}
}
