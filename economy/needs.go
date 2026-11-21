package economy

import (
	"fmt"
	"math"
)

// Pop needs: tiers of need groups; goods within a group substitute.
//
// Utility per person:  U = sum_k w_k * log(1 + Q_k / r_k)
//
//	Q_k  CES composite of the goods in need group k
//	r_k  the 'requirement' that scales the group
//	w_k  weight
//
// A group may have a subsistence level s_k, a quantity below which a person
// can't live well however much else they have (food, water): then U is
// multiplied by min(1, Q_k / s_k), so that someone with no food gets
// nothing from their furniture. This is the idea of Stone-Geary
// preferences, where utility collapses below subsistence. It changes what
// a living is worth, not what people buy: they already meet their basic
// needs first (see Demand). Groups without one (s_k = 0) leave U alone.
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
	Subsistence float64 // per person per tick; below it every need is worth less (0: none)
}

func goods(names ...string) []GoodID {
	out := make([]GoodID, len(names))
	for i, n := range names {
		out[i] = G(n)
	}
	return out
}

// Needs are the need groups every pop has (data/needs.json).
var Needs []Need

func makeNeeds() []Need {
	var f needsFile
	loadJSON("needs.json", &f)
	var out []Need
	for _, n := range f.Needs {
		t := Tier(-1)
		for i, name := range tierNames {
			if name == n.Tier {
				t = Tier(i)
			}
		}
		if t < 0 {
			panic(fmt.Sprintf("data/needs.json: %s: unknown tier %q", n.Name, n.Tier))
		}
		out = append(out, Need{n.Name, t, goods(n.Goods...), n.Sigma, n.Weight, n.Requirement, n.Subsistence})
	}
	return out
}

// ConsumerGoods lists the goods pops consume, in Needs order.
var ConsumerGoods []GoodID

func makeConsumerGoods() []GoodID {
	var out []GoodID
	for _, n := range Needs {
		out = append(out, n.Goods...)
	}
	return out
}

// TierOf maps a good to the tier of the need it satisfies, or -1 if pops don't consume it.
var TierOf []Tier

func makeTierOf() []Tier {
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
}

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
	return u * SubsistenceMet(consumed)
}

// SubsistenceMet is how far one person consuming consumed meets the needs
// that have a subsistence level: the product over them of min(1, Q / s),
// 1 when every one is met (or none has a level), 0 when one is wholly
// unmet.
func SubsistenceMet(consumed []float64) float64 {
	met := 1.0
	for _, nd := range Needs {
		if nd.Subsistence <= 0 {
			continue
		}
		if q := Aggregate(consumed, nd.Goods, nd.Sigma); q < nd.Subsistence {
			met *= q / nd.Subsistence
		}
	}
	return met
}

// hasSubsistence: some need has a subsistence level, so people can starve.
func hasSubsistence() bool {
	for _, nd := range Needs {
		if nd.Subsistence > 0 {
			return true
		}
	}
	return false
}

// StarvationRate is the share of a group of people who die a tick when
// they have been wholly without their subsistence needs for a while;
// short of that, the share is StarvationRate * Want^2 (Pop.Want, the
// running average of 1 - SubsistenceMet), so a slight shortfall kills few
// and a famine many. At a tick a day, with nothing at all, about half die
// in two months, as in the worst famines; nobody starves in an economy
// whose needs have no subsistence level.
const StarvationRate = 0.01

// starveFloor is the fewest people a famine leaves on a tile.
const starveFloor = 1.0

// wantTicks is about how many ticks of want Pop.Want averages over: people
// get through a bad day or week on what they have put by, not through a
// month (at a tick a day).
const wantTicks = 30.0

// starve applies a tick of deaths from want, whether or not the people
// otherwise grow (World.Demography). A famine can take a tile far below
// MinPopulation, the floor migration keeps, but never empties it
// (starveFloor). The dead's savings stay with the living.
func (r *Region) starve() {
	deaths := make([]float64, len(r.Pops))
	total := 0.0
	for i, p := range r.Pops {
		if p.Size <= 0 {
			continue
		}
		for g, q := range p.Consumed {
			p.perPerson[g] = q / p.Size
		}
		p.Want += (1 - SubsistenceMet(p.perPerson) - p.Want) / wantTicks
		deaths[i] = p.Size * StarvationRate * p.Want * p.Want
		total += deaths[i]
	}
	if total <= 0 {
		return
	}
	scale := math.Min(1, math.Max(0, r.Population()-starveFloor)/total)
	for i, p := range r.Pops {
		p.Size -= deaths[i] * scale
	}
}
