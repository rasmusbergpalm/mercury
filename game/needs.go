package game

import "math"

// Pop needs: tiers of need groups; goods within a group substitute.
//
// Utility per person:  U = sum_k w_k * log(1 + Q_k / r_k)
//
//	Q_k  CES composite of the goods in need group k
//	r_k  the 'requirement' that scales the group
//	w_k  weight
//
// The marginal utility of a group at zero is w_k / r_k, so a pop only starts
// spending on a group once its budget is big enough. Basic needs have small
// requirements (high value at first, quickly satisfied), luxuries large ones
// (only worth buying once the basics are covered, but never satiated). As
// income grows, spending shares tend to the ratio of the weights.

// Tier of a need.
type Tier int

const (
	Basic Tier = iota
	Intermediate
	Luxury
	NumTiers
)

var tierNames = [...]string{"basic", "intermediate", "luxury"}

func (t Tier) String() string { return tierNames[t] }

// Need is a need group.
type Need struct {
	Name        string
	Tier        Tier
	Goods       []GoodID
	Sigma       float64 // elasticity of substitution between the goods (≠ 1)
	Weight      float64
	Requirement float64
}

func goods(names ...string) []GoodID {
	out := make([]GoodID, len(names))
	for i, n := range names {
		out[i] = G(n)
	}
	return out
}

// Needs are the need groups every pop has.
var Needs = []Need{
	{"staples", Basic, goods("bread", "potatoes"), 5.0, 1.0, 0.025},
	{"protein", Basic, goods("meat", "fish"), 4.0, 1.0, 0.025},
	{"clothing", Basic, goods("clothes"), 2.0, 1.0, 0.025},
	{"heating", Basic, goods("wood", "coal"), 5.0, 1.0, 0.025},
	{"home", Intermediate, goods("furniture", "ironware"), 0.5, 2.0, 0.015},
	{"drink", Intermediate, goods("beer", "tea"), 2.0, 2.0, 0.1},
	{"sweets", Intermediate, goods("sugar"), 2.0, 2.0, 0.1},
	{"wine", Luxury, goods("wine"), 2.0, 3.0, 0.3},
	{"finery", Luxury, goods("fine clothes", "porcelain"), 2.0, 3.0, 0.3},
}

// ConsumerGoods lists the goods pops consume, in Needs order.
var ConsumerGoods = func() []GoodID {
	var out []GoodID
	for _, n := range Needs {
		out = append(out, n.Goods...)
	}
	return out
}()

// TierOf maps a good to the tier of the need it satisfies, or -1 if pops don't consume it.
var TierOf = func() []Tier {
	out := make([]Tier, NumGoods)
	for i := range out {
		out[i] = -1
	}
	for _, n := range Needs {
		for _, g := range n.Goods {
			out[g] = n.Tier
		}
	}
	return out
}()

// maxNeeds bounds len(Needs), so Demand can keep its scratch on the stack.
const maxNeeds = 16

// Demand writes into out (indexed by good; other entries zeroed) the
// utility-maximising purchases of one person with the given budget.
//
// Spending E_k on group k buys Q_k = E_k / P_k. The FOC
//
//	w_k / (r_k P_k + E_k) = lambda
//
// gives E_k = max(0, w_k/lambda - r_k P_k): water-filling. Groups enter in
// order of w_k / (r_k P_k); solve for lambda over the active set.
func Demand(prices []float64, budget float64, out []float64) {
	var d demandPlan
	d.prepare(prices)
	d.demand(budget, out)
}

// demandPlan is the part of Demand that depends only on prices, worked out
// once for every pop facing them: each group's unit price and floor, the
// order the groups enter in, and how a group's quantity splits into goods.
type demandPlan struct {
	unit, floor [maxNeeds]float64
	order       [maxNeeds]int
	split       [maxNeeds][maxNeedGoods]float64 // goods per unit of the group
}

// maxNeedGoods bounds the goods in one need group.
const maxNeedGoods = 4

func (d *demandPlan) prepare(prices []float64) {
	n := len(Needs)
	unit, floor, order := d.unit[:n], d.floor[:n], d.order[:n]
	for k, nd := range Needs {
		unit[k] = PriceIndex(prices, nd.Goods, nd.Sigma)
		floor[k] = nd.Requirement * unit[k]
		order[k] = k
		for i, g := range nd.Goods { // as Split does it
			if len(nd.Goods) == 1 {
				d.split[k][i] = 1
			} else {
				d.split[k][i] = math.Pow(prices[g]/unit[k], -nd.Sigma)
			}
		}
	}
	// a stable insertion sort (a handful of groups): by value at zero, best first
	for i := 1; i < n; i++ {
		for j := i; j > 0; j-- {
			a, b := order[j], order[j-1]
			if !(Needs[a].Weight/floor[a] > Needs[b].Weight/floor[b]) {
				break
			}
			order[j], order[j-1] = b, a
		}
	}
}

// demand writes one person's purchases with the given budget into out.
func (d *demandPlan) demand(budget float64, out []float64) {
	for i := range out {
		out[i] = 0
	}
	budget = math.Max(budget, 0)
	n := len(Needs)
	unit, floor, order := d.unit[:n], d.floor[:n], d.order[:n]
	var m int
	var invLambda float64
	for m = 1; m <= n; m++ {
		sf, sw := 0.0, 0.0
		for _, k := range order[:m] {
			sf += floor[k]
			sw += Needs[k].Weight
		}
		invLambda = (budget + sf) / sw
		if m == n || Needs[order[m]].Weight*invLambda <= floor[order[m]] {
			break
		}
	}
	for _, k := range order[:m] {
		nd := Needs[k]
		spend := math.Max(0, nd.Weight*invLambda-floor[k])
		q := spend / unit[k]
		for i, g := range nd.Goods {
			out[g] = q * d.split[k][i]
		}
	}
}

// Utility of one person consuming consumed (indexed by good).
func Utility(consumed []float64) float64 {
	u := 0.0
	for _, nd := range Needs {
		u += nd.Weight * math.Log1p(Aggregate(consumed, nd.Goods, nd.Sigma)/nd.Requirement)
	}
	return u
}
