package economy

import (
	"math"
	"math/rand"
	"testing"
)

func testPrices() []float64 {
	p := make([]float64, NumGoods)
	for i := range p {
		p[i] = 1 + float64(i%7)*0.7
	}
	return p
}

func TestCESSplitRoundTrips(t *testing.T) {
	prices := testPrices()
	goods := []GoodID{G("wood"), G("coal"), G("bread")}
	for _, sigma := range []float64{0.5, 2, 3, 5} {
		q := Split(2.5, prices, goods, sigma)
		x := make([]float64, NumGoods)
		cost := 0.0
		for i, g := range goods {
			x[g] = q[i]
			cost += q[i] * prices[g]
		}
		if got := Aggregate(x, goods, sigma); math.Abs(got-2.5) > 1e-12 {
			t.Errorf("sigma %v: aggregate of split = %v, want 2.5", sigma, got)
		}
		if want := 2.5 * PriceIndex(prices, goods, sigma); math.Abs(cost-want) > 1e-9 {
			t.Errorf("sigma %v: cost of split = %v, want Q*P = %v", sigma, cost, want)
		}
	}
}

func TestCESSplitIsCheapest(t *testing.T) {
	prices := testPrices()
	goods := []GoodID{G("wood"), G("coal")}
	rng := rand.New(rand.NewSource(1))
	for _, sigma := range []float64{0.5, 3} {
		best := PriceIndex(prices, goods, sigma) // cost of one unit
		for i := 0; i < 200; i++ {
			x := make([]float64, NumGoods)
			x[goods[0]], x[goods[1]] = rng.Float64()+0.01, rng.Float64()+0.01
			scale := 1 / Aggregate(x, goods, sigma) // rescale to one unit of composite
			cost := scale * (x[goods[0]]*prices[goods[0]] + x[goods[1]]*prices[goods[1]])
			if cost < best-1e-12 {
				t.Fatalf("sigma %v: mix %v costs %v, below price index %v", sigma, x, cost, best)
			}
		}
	}
}

func TestCESComplementsNeedEveryGood(t *testing.T) {
	x := make([]float64, NumGoods)
	x[G("furniture")] = 3
	goods := []GoodID{G("furniture"), G("ironware")}
	if got := Aggregate(x, goods, 0.5); got != 0 {
		t.Errorf("complements with a missing good: aggregate %v, want 0", got)
	}
	if got := Aggregate(x, goods, 3); got <= 0 {
		t.Errorf("substitutes with a missing good: aggregate %v, want > 0", got)
	}
}
