package engine

import (
	"math"
	"testing"
)

// profit of producing with the given labor and inputs
func profit(f *Firm, prices []float64, wage, labor float64, inputs []float64) float64 {
	g := *f
	g.Labor = labor
	g.Inputs = append([]float64(nil), inputs...)
	g.Inventory = 0
	q := g.Produce()
	cost := labor * wage
	for i, x := range inputs {
		cost += x * prices[i]
	}
	return q*prices[f.Output] - cost
}

func TestPlanMaximisesProfit(t *testing.T) {
	prices := testPrices()
	for _, rc := range ProcessingRecipes {
		f := NewFirm(rc.Firm, G(rc.Output), rc.TFP, rc.Factors, 1e9)
		f.Plan(prices, 1.3)
		inputs := append([]float64(nil), f.PlanQty...)
		best := profit(f, prices, 1.3, f.PlanLabor, inputs)
		// scaling any one input up or down must not help
		for g := range inputs {
			if !f.PlanHas[g] {
				continue
			}
			for _, k := range []float64{0.97, 1.03} {
				in := append([]float64(nil), inputs...)
				in[g] *= k
				if p := profit(f, prices, 1.3, f.PlanLabor, in); p > best+1e-9*math.Abs(best) {
					t.Errorf("%s: scaling %s by %v raises profit %v -> %v", f.Name, Goods[g].Name, k, best, p)
				}
			}
		}
		for _, k := range []float64{0.97, 1.03} {
			if p := profit(f, prices, 1.3, f.PlanLabor*k, inputs); p > best+1e-9*math.Abs(best) {
				t.Errorf("%s: scaling labor by %v raises profit %v -> %v", f.Name, k, best, p)
			}
		}
	}
}

func TestPlanRespectsCash(t *testing.T) {
	prices := testPrices()
	rc := ProcessingRecipes[0]
	f := NewFirm(rc.Firm, G(rc.Output), rc.TFP, rc.Factors, 1e9)
	f.Plan(prices, 0.5)
	full := f.DesiredSpend
	f.Cash = full / 10
	f.Plan(prices, 0.5)
	spend := f.PlanLabor * 0.5
	for g, x := range f.PlanQty {
		spend += x * prices[g]
	}
	if spend > 0.9*f.Cash*(1+1e-12) {
		t.Errorf("planned spend %v exceeds 90%% of cash %v", spend, f.Cash)
	}
	if f.DesiredSpend != full {
		t.Errorf("DesiredSpend should stay the unconstrained %v, got %v", full, f.DesiredSpend)
	}
}

func TestProduceNeedsEveryFactor(t *testing.T) {
	rc := ProcessingRecipes[0] // bakery: labor, grain, fuel
	f := NewFirm(rc.Firm, G(rc.Output), rc.TFP, rc.Factors, 0)
	f.Labor = 5
	f.Inputs[G("grain")] = 3
	if q := f.Produce(); q != 0 {
		t.Errorf("bakery without fuel produced %v", q)
	}
	f.Labor = 5
	f.Inputs[G("grain")] = 3
	f.Inputs[G("coal")] = 1 // either fuel will do
	if q := f.Produce(); q <= 0 || f.Inventory != q {
		t.Errorf("bakery with coal as fuel produced %v (inventory %v)", q, f.Inventory)
	}
}
