package engine

import "math"

// CES composites: Q = (sum_g x_g^rho)^(1/rho), with sigma = 1/(1-rho). Used for
// substitutable consumer goods (bread vs potatoes) and for substitutable
// production inputs (wood vs coal as fuel).

// PriceIndex is the cost of one unit of the composite when buying the cheapest mix.
func PriceIndex(prices []float64, goods []GoodID, sigma float64) float64 {
	if len(goods) == 1 {
		return prices[goods[0]]
	}
	s := 0.0
	for _, g := range goods {
		s += math.Pow(prices[g], 1-sigma)
	}
	return math.Pow(s, 1/(1-sigma))
}

// Split returns the cost-minimising goods mix (aligned with goods) that yields
// quantity units of the composite.
func Split(quantity float64, prices []float64, goods []GoodID, sigma float64) []float64 {
	out := make([]float64, len(goods))
	if len(goods) == 1 {
		out[0] = quantity
		return out
	}
	p := PriceIndex(prices, goods, sigma)
	for i, g := range goods {
		out[i] = quantity * math.Pow(prices[g]/p, -sigma)
	}
	return out
}

// Aggregate is the composite quantity from a goods mix (quantities indexed by good).
func Aggregate(quantities []float64, goods []GoodID, sigma float64) float64 {
	if len(goods) == 1 {
		return math.Max(quantities[goods[0]], 0)
	}
	rho := (sigma - 1) / sigma
	s := 0.0
	for _, g := range goods {
		x := math.Max(quantities[g], 0)
		if x == 0 {
			if rho < 0 { // complements: any missing good -> nothing
				return 0
			}
			continue
		}
		s += math.Pow(x, rho)
	}
	if s == 0 {
		return 0
	}
	return math.Pow(s, 1/rho)
}
