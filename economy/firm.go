package economy

import (
	"math"
)

// FactorKind says what a production factor is.
type FactorKind int

const (
	LaborFactor FactorKind = iota
	GoodFactor
	CompositeFactor
)

// Factor is one input of a Cobb-Douglas production function.
type Factor struct {
	Kind      FactorKind
	Good      GoodID // for GoodFactor
	Composite int    // for CompositeFactor, index into Composites
	Exp       float64
}

// Labor, Input and Comp build factors.
func Labor(exp float64) Factor              { return Factor{Kind: LaborFactor, Exp: exp} }
func Input(good string, exp float64) Factor { return Factor{Kind: GoodFactor, Good: G(good), Exp: exp} }
func Comp(name string, exp float64) Factor {
	return Factor{Kind: CompositeFactor, Composite: CompositeID(name), Exp: exp}
}

// Sector classifies firms: land-bound producers, manufacturing, and services.
type Sector int

const (
	Industry Sector = iota
	Primary
	Service
)

var sectorNames = [...]string{"industry", "primary", "service"}

func (s Sector) String() string { return sectorNames[s] }

// Method is one way a firm makes its good: a Cobb-Douglas recipe
// q = TFP * L^(1-s) * prod f_i^a_i over the L levels the firm runs it on,
// with sum a_i = s < 1. Every firm has its first method; technologies add others
// (MethodRecipes), more productive and needing machinery or fuel the first
// doesn't. The firm runs each of its levels on one method, and its owners
// refit a level from one method to another when the difference pays.
type Method struct {
	Name     string
	Requires string // the tech it needs ("" for the firm's first method)
	Enabled  bool
	// short: an input it needs is out of play (nobody can make it yet, as
	// engines before Steam power): it can't run, and owners don't build it
	short   bool
	Factors []Factor
	// TFP is productivity per level, including technology; BaseTFP is
	// without technology.
	TFP, BaseTFP float64
	// Cap is the most levels it may run (0: the firm's MaxLevels): steam
	// pumps and blasting reach seams the old mines couldn't.
	Cap int
	// levels it runs; the firm's first method runs the levels the others
	// don't (see MethodLevels)
	Levels int
	Output float64 // what it made today

	planLabor, labor float64   // today's plan, and what it got
	planQty          []float64 // per good
	powLevels        int
	levelsPow        float64 // L^(1-s), cached
}

// checkInputs marks the method short when some input of it is out of play.
func (m *Method) checkInputs(made []bool) {
	m.short = false
	if made == nil {
		return
	}
	for _, fc := range m.Factors {
		switch fc.Kind {
		case GoodFactor:
			m.short = m.short || !made[fc.Good]
		case CompositeFactor:
			any := false
			for _, g := range Composites[fc.Composite].Goods {
				any = any || made[g]
			}
			m.short = m.short || !any
		}
	}
}

// Runs reports whether the method can run: its tech known, its inputs in play.
func (m *Method) Runs() bool { return m.Enabled && !m.short }

// exponentSum is s, the variable factors' share.
func (m *Method) exponentSum() float64 {
	s := 0.0
	for _, fc := range m.Factors {
		s += fc.Exp
	}
	return s
}

// Firm turns labor and inputs into one output good, on its levels, by its
// methods.
type Firm struct {
	Name   string
	Output GoodID
	// Methods are the ways it can make its good, its first one first.
	Methods []*Method
	// Levels of buildings, over all its methods. Output is constant-returns
	// in levels and the variable factors together. MaxLevels caps them where
	// something else is scarce: land for farms and mines (RawMaxLevels), the
	// state's budget for its services; 0 is no cap (workshops and services
	// grow while a level pays for itself).
	Levels, MaxLevels int
	Expansion         *Expansion // a level under construction, or nil
	// Requires names the tech the owner needs for the firm to work; Enabled
	// says whether it does (always true in the classic world).
	Requires string
	Enabled  bool
	Sector   Sector
	// Agglom multiplies productivity for firms that gain from a city around
	// them (1 elsewhere).
	Agglom     float64
	Cash       float64
	CashBuffer float64   // cash kept beyond what the plan needs
	Inventory  float64   // stock of own output, for sale
	Inputs     []float64 // goods bought this tick, indexed by good
	Labor      float64   // labor employed this tick
	// State firms (public services) are run by the tile's government: it
	// funds Funded of their levels, staffs them and takes their output.
	State   bool
	Funded  int
	Public  int     // which public service a state firm provides (the government layer's numbering)
	EduMult float64 // productivity from the tile's schooling (industry and services)
	Loyal   float64 // productivity from the tile's allegiance to its owner (1: loyal)
	// Site and SiteMult: where a land-bound producer's productivity comes
	// from (its terrain, or the coast) and how much it multiplies it.
	Site       string
	SiteMult   float64
	LastOutput float64
	// DesiredSpend is the unconstrained cost of the optimal plan.
	DesiredSpend float64

	// The current plan: labor and goods wanted. PlanHas marks goods in the plan.
	PlanLabor float64
	PlanQty   []float64
	PlanHas   []bool

	// ProfitEMA is a moving average of profit per tick; it drives investment.
	ProfitEMA             float64
	LossTicks, Age        int     // ticks of losses in a row; ticks alive
	Wear                  float64 // levels worn out and not yet made good by upkeep (see depreciation)
	Urge                  float64 // the owners' urge to start the next project (see investDecideDays)
	Projects              int     // projects they have started
	TickRevenue, TickCost float64
	tickInputs            float64   // what the goods it used up cost this tick
	Used                  []float64 // goods it used up producing this tick, by good
	investWant            float64   // construction to buy this tick: upkeep, and a private expansion
	upkeepWant            float64   // ...of which upkeep
}

// Expansion is a building level under construction for a firm, run by its
// method Method: a new one, or, with Refit, one of method From's refitted
// (the building stays; the machinery is new), at refitShare of the cost.
type Expansion struct {
	Delivered float64 // construction units delivered so far
	Public    bool    // paid for by the government (else by the firm's owners)
	Method    int     `json:",omitempty"`
	Refit     bool    `json:",omitempty"`
	From      int     `json:",omitempty"`
}

// refitShare is what refitting a level to another method costs, against
// building a new one: the machinery, not the building.
const refitShare = 0.4

// Cost is the construction the expansion needs.
func (e *Expansion) Cost() float64 {
	if e.Refit {
		return refitShare * LevelCost
	}
	return LevelCost
}

// LevelCost is the construction units one building level costs. A level is
// the firm's capital (its buildings and machines; for farms and mines, its
// land too), priced so that a tile's capital is worth about three to four
// years of its output, as Europe's was in the 19th century.
const LevelCost = 1200.0

// MethodLevels is the levels the firm runs on its i-th method.
func (f *Firm) MethodLevels(i int) int {
	if i > 0 {
		return f.Methods[i].Levels
	}
	n := f.Levels
	for _, m := range f.Methods[1:] {
		n -= m.Levels
	}
	return max(0, n)
}

// addLevel gives the firm a level of its i-th method; removeLevel takes one
// away; refit moves one from method from to method to.
func (f *Firm) addLevel(i int) {
	if i > 0 {
		f.Methods[i].Levels++
	}
	f.Levels++
}
func (f *Firm) removeLevel(i int) {
	if f.MethodLevels(i) == 0 {
		return
	}
	if i > 0 {
		f.Methods[i].Levels--
	}
	f.Levels--
}
func (f *Firm) refit(from, to int) {
	if f.MethodLevels(from) == 0 {
		f.addLevel(to) // nothing left to refit: a level of its own
		return
	}
	f.removeLevel(from)
	f.addLevel(to)
}

// methodCap is the most levels method i may run.
func (f *Firm) methodCap(i int) int {
	if c := f.Methods[i].Cap; c > 0 {
		return c
	}
	return f.MaxLevels
}

// landCap is the most levels the firm may have: the most any of its working
// methods may run (0: no cap).
func (f *Firm) landCap() int {
	if f.MaxLevels == 0 {
		return 0
	}
	c := f.MaxLevels
	for i, m := range f.Methods {
		if m.Enabled {
			c = max(c, f.methodCap(i))
		}
	}
	return c
}

// effectiveTFP is method i's TFP * L^(1-s) with the tile's multipliers: the
// productivity of its variable factors.
func (f *Firm) effectiveTFP(i int) float64 {
	m, l := f.Methods[i], f.MethodLevels(i)
	if m.powLevels != l || m.levelsPow == 0 {
		m.powLevels, m.levelsPow = l, math.Pow(float64(l), 1-m.exponentSum())
	}
	return m.TFP * m.levelsPow * f.Agglom * f.EduMult * f.Loyal
}

// main is the method that runs most of the firm's levels (its first, at
// a tie): what its productivity breakdown describes.
func (f *Firm) Main() int {
	best := 0
	for i := range f.Methods {
		if f.MethodLevels(i) > f.MethodLevels(best) {
			best = i
		}
	}
	return best
}

// dropLevel closes a level of the method that earns least on it (wear the
// owners didn't make good, or losses).
func (f *Firm) dropLevel(r *Region) {
	if f.Levels <= 0 {
		return
	}
	w, _ := f.WorstMethod(r.prices, r.Wage)
	if w < 0 {
		w = 0
	}
	f.removeLevel(w)
}

// factors lists the factors of all the firm's methods (what it may buy).
func (f *Firm) factors() []Factor {
	if len(f.Methods) == 1 {
		return f.Methods[0].Factors
	}
	var out []Factor
	for _, m := range f.Methods {
		out = append(out, m.Factors...)
	}
	return out
}

// AtCap reports whether the firm can have no more levels.
func (f *Firm) AtCap() bool {
	c := f.landCap()
	return c > 0 && f.Levels >= c
}

// BestMethod is the working method a level would earn most on at today's
// prices, of those with room for one more (-1 if none), and WorstMethod
// the one with levels that earns least (-1 if none).
func (f *Firm) BestMethod(prices []float64, wage float64) (int, float64) {
	best, top := -1, 0.0
	for i, m := range f.Methods {
		if !m.Runs() || (f.methodCap(i) > 0 && f.MethodLevels(i) >= f.methodCap(i)) {
			continue
		}
		if p := f.LevelProfit(i, prices, wage); best < 0 || p > top {
			best, top = i, p
		}
	}
	return best, top
}
func (f *Firm) WorstMethod(prices []float64, wage float64) (int, float64) {
	worst, low := -1, 0.0
	for i, m := range f.Methods {
		if f.MethodLevels(i) == 0 {
			continue
		}
		p := 0.0 // a method the owner can't run (its tech lost, an input out of play) earns nothing
		if m.Runs() {
			p = f.LevelProfit(i, prices, wage)
		}
		if worst < 0 || p < low {
			worst, low = i, p
		}
	}
	return worst, low
}

// NewFirm makes a firm with one method, its first.
func NewFirm(name string, output GoodID, tfp float64, factors []Factor, cash float64) *Firm {
	return &Firm{Name: name, Output: output, Cash: cash,
		Methods: []*Method{newMethod(name, tfp, factors)},
		Levels:  1, Enabled: true, Agglom: 1, EduMult: 1, Loyal: 1,
		CashBuffer: 20, Inputs: make([]float64, NumGoods),
		PlanQty: make([]float64, NumGoods), PlanHas: make([]bool, NumGoods)}
}

func newMethod(name string, tfp float64, factors []Factor) *Method {
	return &Method{Name: name, Enabled: true, Factors: factors, TFP: tfp, BaseTFP: tfp, planQty: make([]float64, NumGoods)}
}

// TFP is the productivity per level of the firm's first method.
func (f *Firm) TFP() float64 { return f.Methods[0].TFP }

func (f *Firm) unitCost(fc Factor, prices []float64, wage float64) float64 {
	switch fc.Kind {
	case LaborFactor:
		return wage
	case CompositeFactor:
		c := Composites[fc.Composite]
		return PriceIndex(prices, c.Goods, c.Sigma)
	default:
		return prices[fc.Good]
	}
}

// LevelProfit is what one level of the firm's i-th method would earn a day
// at these prices, working at its best: with constant returns, a level's
// share of revenue, one less the exponents. Owners compare methods by it.
func (f *Firm) LevelProfit(i int, prices []float64, wage float64) float64 {
	m := f.Methods[i]
	p := prices[f.Output]
	k, s := m.TFP*f.Agglom*f.EduMult*f.Loyal, 0.0
	for _, fc := range m.Factors {
		k *= math.Pow(fc.Exp*p/f.unitCost(fc, prices, wage), fc.Exp)
		s += fc.Exp
	}
	return (1 - s) * p * math.Pow(k, 1/(1-s))
}

// Plan sets the profit-maximising labor and goods demand at current prices,
// capped by cash: each method's, added up.
//
// With q = A prod f_i^a_i and s = sum a_i < 1:
//
//	q* = (A prod (a_i p / w_i)^a_i)^(1/(1-s)),   f_i* = a_i p q* / w_i
//
// Composite factors are then split into their cheapest goods mix.
func (f *Firm) Plan(prices []float64, wage float64) {
	f.PlanLabor, f.DesiredSpend = 0, 0
	for i := range f.PlanQty {
		f.PlanQty[i], f.PlanHas[i] = 0, false
	}
	for _, m := range f.Methods {
		m.planLabor = 0
		for g := range m.planQty {
			m.planQty[g] = 0
		}
	}
	if !f.Enabled || f.State { // the government staffs its state firms itself
		return
	}
	p := prices[f.Output]
	type plan struct {
		m    *Method
		want []float64
		cost []float64
		q, s float64
	}
	var plans []plan
	total := 0.0
	for i, m := range f.Methods {
		if !m.Runs() || f.MethodLevels(i) == 0 {
			continue
		}
		n := len(m.Factors)
		pl := plan{m: m, want: make([]float64, n), cost: make([]float64, n)}
		k := f.effectiveTFP(i)
		for j, fc := range m.Factors {
			pl.cost[j] = f.unitCost(fc, prices, wage)
			pl.s += fc.Exp
		}
		for j, fc := range m.Factors {
			k *= math.Pow(fc.Exp*p/pl.cost[j], fc.Exp)
		}
		pl.q = math.Pow(k, 1/(1-pl.s))
		for j, fc := range m.Factors {
			pl.want[j] = fc.Exp * p * pl.q / pl.cost[j]
		}
		total += pl.q
		plans = append(plans, pl)
	}
	// Stock beyond a day's output is stock it failed to sell (when it
	// plans, its stock still holds yesterday's output, not yet offered): it
	// makes what it wants to sell less that excess, so goods nobody buys
	// stop being made instead of piling up. Scaling a method's every factor
	// by x^(1/s) scales its output by x.
	if over := f.Inventory - total; over > 0 && total > 0 {
		x := math.Max(0, total-over) / total
		for _, pl := range plans {
			scale := math.Pow(x, 1/pl.s)
			for j := range pl.want {
				pl.want[j] *= scale
			}
		}
	}
	spend := 0.0
	for _, pl := range plans {
		for j := range pl.want {
			spend += pl.want[j] * pl.cost[j]
		}
	}
	f.DesiredSpend = spend
	budget := 0.9 * math.Max(f.Cash, 0)
	scale := 1.0
	if spend > budget { // liquidity constraint
		scale = budget / spend
	}
	for _, pl := range plans {
		for j, fc := range pl.m.Factors {
			w := pl.want[j] * scale
			switch fc.Kind {
			case LaborFactor:
				pl.m.planLabor += w
			case GoodFactor:
				pl.m.planQty[fc.Good] += w
				f.PlanHas[fc.Good] = true
			case CompositeFactor:
				c := Composites[fc.Composite]
				for k, x := range Split(w, prices, c.Goods, c.Sigma) {
					pl.m.planQty[c.Goods[k]] += x
					f.PlanHas[c.Goods[k]] = true
				}
			}
		}
		f.PlanLabor += pl.m.planLabor
		for g, x := range pl.m.planQty {
			f.PlanQty[g] += x
		}
	}
}

// methodInputs is method m's share of what the firm got today: of each
// good, its planned share (all of it, for a firm's only method).
func (f *Firm) methodInputs(m *Method, out []float64) {
	for g, got := range f.Inputs {
		switch {
		case f.PlanQty[g] > 0:
			out[g] = got * m.planQty[g] / f.PlanQty[g]
		case len(f.Methods) == 1:
			out[g] = got
		default:
			out[g] = 0
		}
	}
}

// factorAmount is how much of a factor a method has, of its labor and goods.
func factorAmount(fc Factor, labor float64, inputs []float64) float64 {
	switch fc.Kind {
	case LaborFactor:
		return labor
	case CompositeFactor:
		c := Composites[fc.Composite]
		return Aggregate(inputs, c.Goods, c.Sigma)
	default:
		return math.Max(inputs[fc.Good], 0)
	}
}

// Produce turns this tick's labor and inputs into output: each method's
// share of them (its share of the plan) into its output.
func (f *Firm) Produce() float64 {
	q := 0.0
	var share []float64
	for i, m := range f.Methods {
		m.Output, m.labor = 0, 0
		if !m.Runs() || f.MethodLevels(i) == 0 {
			continue
		}
		in := f.Inputs
		m.labor = f.Labor
		if len(f.Methods) > 1 {
			if share == nil {
				share = make([]float64, len(f.Inputs))
			}
			f.methodInputs(m, share)
			in = share
			if f.PlanLabor > 0 {
				m.labor = f.Labor * m.planLabor / f.PlanLabor
			}
		}
		x := f.effectiveTFP(i)
		for _, fc := range m.Factors {
			x *= math.Pow(factorAmount(fc, m.labor, in), fc.Exp)
		}
		m.Output = x
		q += x
	}
	f.Inventory += q
	if f.Used == nil {
		f.Used = make([]float64, len(f.Inputs))
	}
	copy(f.Used, f.Inputs)
	for i := range f.Inputs {
		f.Inputs[i] = 0
	}
	f.LastOutput = q
	return q
}

// NextLevelMult is how much one more building level would multiply the
// firm's output from the same workers and inputs: ((n+1)/n)^(1-s). Output is
// constant-returns in levels and the variable factors together, so the gain
// depends only on the number of levels.
func (f *Firm) NextLevelMult() float64 {
	n := float64(max(f.Levels, 1))
	return math.Pow((n+1)/n, 1-f.Methods[f.Main()].exponentSum())
}

// ValueAdded is what the firm added today: its output at the given price,
// less the goods it used up (wages and profits are the value it added).
func (f *Firm) ValueAdded(price float64) float64 {
	return f.LastOutput*price - f.tickInputs
}
