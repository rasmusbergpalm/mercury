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
	// DepositFactor is a resource only the firm works (a coal seam, an oil
	// field, a fishing ground): a fixed amount of it (Firm.Deposit), shared
	// among its methods, never bought; its rent is part of the firm's profit.
	DepositFactor
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
// with sum a_i = s < 1. Every firm has its first method; inventions add others
// (MethodRecipes), more productive and needing machinery or fuel the first
// doesn't. The firm runs each of its levels on one method, and its owners
// refit a level from one method to another when the difference pays.
type Method struct {
	Name    string
	Enabled bool // the firm may run it (a later method starts off: Recipe.StartsOff)
	// short: an input it needs is out of play (nobody can make it yet, as
	// engines before any engine works is switched on): it can't run, and owners don't build it
	short   bool
	Factors []Factor
	// TFP is productivity per level, a lever (an outside improvement to the
	// method raises it); BaseTFP is the recipe's.
	TFP, BaseTFP float64

	// Capital is what one of its levels is made of: the building
	// (construction) and the machines in it (engines, tools, vehicles).
	// Owners buy it to build a level and make good its wear (Good.Wear).
	Capital []Stock
	// levels it runs; the firm's first method runs the levels the others
	// don't (see MethodLevels)
	Levels int
	Output float64 // what it made today

	planLabor, labor float64   // today's plan, and what it got
	deposit          float64   // its share of the firm's deposit today
	planQty          []float64 // per good
	powLevels        int
	levelsPow        float64 // L^(1-s), cached
}

// checkInputs marks the method short when some input of it, or a good its
// levels are made of, is out of play.
func (m *Method) checkInputs(made []bool) {
	m.short = false
	if made == nil {
		return
	}
	for _, s := range m.Capital {
		m.short = m.short || !made[s.Good]
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

// Runs reports whether the method can run: switched on, its inputs in play.
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
	// something else is scarce, as land for farms and mines (RawMaxLevels);
	// 0 is no cap (workshops and services
	// grow while a level pays for itself).
	Levels, MaxLevels int
	Expansion         *Expansion // a level under construction, or nil
	// Payback multiplies the ticks of profit its owners accept a new level
	// to repay (0: as any other firm's): houses were a safe, long-lived
	// asset, bought for a lower return than a works.
	Payback float64
	// Fixed: its levels are all it will have and don't wear (the tile's
	// land: Recipe.Fixed).
	Fixed bool
	// Deposit is how much of its deposit it works (a DepositFactor's
	// amount), and DepositRent what a unit of it earned the firm in the
	// last tick: the value its methods share it by.
	Deposit, DepositRent float64
	// Enabled: the firm works at all (one whose recipe StartsOff doesn't
	// until it is switched on).
	Enabled bool
	Sector  Sector
	// Agglom multiplies productivity for firms that gain from a city around
	// them (1 elsewhere).
	Agglom     float64
	Cash       float64
	CashBuffer float64   // cash kept beyond what the plan needs
	Inventory  float64   // stock of own output, for sale
	Inputs     []float64 // goods bought this tick, indexed by good
	Labor      float64   // labor employed this tick
	// Market power, off by default: every firm takes its market's price as
	// given, as one of many small producers would (the tile's farms, mills
	// or bakeries, together). A Monopoly is the tile's only maker of its
	// good, and reckons with how its own sales move the price, given the
	// Elasticity of the demand it faces (see worth).
	Monopoly   bool
	Elasticity float64
	share      float64 // its share of what the tile's market offered at the last clearing (a monopoly's)
	Mult       float64 // productivity from outside the economy (1: none)
	// Site and SiteMult: where a land-bound producer's productivity comes
	// from (its terrain, or the coast) and how much it multiplies it.
	Site       string
	SiteMult   float64
	LastOutput float64
	// Held is how much of each leased good (Good.Lease) it holds, by good.
	Held []float64
	// DesiredSpend is the unconstrained cost of the optimal plan.
	DesiredSpend float64

	// The current plan: labor and goods wanted. PlanHas marks goods in the plan.
	PlanLabor float64
	PlanQty   []float64
	PlanHas   []bool

	// ProfitEMA is a moving average of profit per tick; it drives investment.
	ProfitEMA             float64
	LossTicks, Age        int     // ticks of losses in a row; ticks alive
	Wear                  float64 // levels worn out and not yet made good by upkeep (see Good.Wear)
	Urge                  float64 // the owners' urge to start the next project (see investDecideTicks)
	TickRevenue, TickCost float64
	tickInputs            float64   // what the goods it used up cost this tick
	Used                  []float64 // goods it used up producing this tick, by good
	// CapitalIn is what its owners bought for its capital this tick, by
	// good: upkeep, and their expansion.
	CapitalIn  []float64
	investWant []float64 // capital goods to buy this tick, by good: upkeep, and a private expansion
	upkeepWant []float64 // ...of which upkeep
	levelValue float64   // what an average level of it is worth at today's prices
}

// Stock is a quantity of a good.
type Stock struct {
	Good GoodID
	Qty  float64
}

// Expansion is a level under construction for a firm, run by its method
// Method: a new one, or, with Refit, one of method From's refitted. Need is
// what it takes, Got what has been delivered of each.
type Expansion struct {
	Need     []Stock
	Got      []float64
	External bool // paid for from outside (else by the firm's owners)
	Levels   int  `json:",omitempty"` // the levels it builds (0: one)
	Method   int  `json:",omitempty"`
	Refit    bool `json:",omitempty"`
	From     int  `json:",omitempty"`
}

// refitShare is the share of a level's building work a refit takes: the
// building stays, the new machines are put in.
const refitShare = 0.4

// NewLevel is an expansion that builds a level of the firm's method-th
// method: all it is made of.
func NewLevel(f *Firm, method int, external bool) *Expansion {
	return NewLevels(f, method, 1, external)
}

// NewLevels is an expansion that builds n levels of the firm's method-th
// method at once.
func NewLevels(f *Firm, method, n int, external bool) *Expansion {
	need := make([]Stock, len(f.Methods[method].Capital))
	for i, s := range f.Methods[method].Capital {
		need[i] = Stock{s.Good, s.Qty * float64(n)}
	}
	e := newExpansion(need, external, method)
	if n > 1 {
		e.Levels = n
	}
	return e
}

// projectShare is the share of its levels a firm's owners build in one
// project. A tile's firm is a whole industry, many works and builders: it
// adds capacity in proportion to its size, not a works at a time.
const projectShare = 0.05

// projectLevels is how many levels the firm's owners build at once: a
// projectShare of what it has, at least one.
func (f *Firm) projectLevels() int {
	return max(1, int(projectShare*float64(f.Levels)))
}

// NewRefit is an expansion that refits one of the firm's levels from its
// method from to its method to: the machines the new method has more of
// than the old, and refitShare of its building work.
func NewRefit(f *Firm, from, to int, external bool) *Expansion {
	old, new := f.Methods[from].Capital, f.Methods[to].Capital
	var need []Stock
	for _, s := range new {
		q := s.Qty
		if s.Good == Construction {
			q *= refitShare
		} else {
			for _, o := range old {
				if o.Good == s.Good {
					q -= o.Qty
				}
			}
		}
		if q > 0 {
			need = append(need, Stock{s.Good, q})
		}
	}
	if len(need) == 0 {
		need = []Stock{{Construction, refitShare * LevelCost}}
	}
	e := newExpansion(need, external, to)
	e.Refit, e.From = true, from
	return e
}

// NewBuilding is an expansion that takes qty of construction alone.
func NewBuilding(qty float64, external bool) *Expansion {
	return newExpansion([]Stock{{Construction, qty}}, external, 0)
}

func newExpansion(need []Stock, external bool, method int) *Expansion {
	return &Expansion{Need: append([]Stock(nil), need...), Got: make([]float64, len(need)), External: external, Method: method}
}

// Copy is a copy of the expansion that shares nothing with it.
func (e *Expansion) Copy() *Expansion {
	c := *e
	c.Need, c.Got = append([]Stock(nil), e.Need...), append([]float64(nil), e.Got...)
	return &c
}

// Left is how much of good g the expansion still needs.
func (e *Expansion) Left(g GoodID) float64 {
	for i, s := range e.Need {
		if s.Good == g {
			return math.Max(0, s.Qty-e.Got[i])
		}
	}
	return 0
}

// Deliver puts qty of good g into the expansion.
func (e *Expansion) Deliver(g GoodID, qty float64) {
	for i, s := range e.Need {
		if s.Good == g {
			e.Got[i] += qty
			return
		}
	}
}

// Done reports whether everything the expansion needs has been delivered.
func (e *Expansion) Done() bool {
	for i, s := range e.Need {
		if e.Got[i] < s.Qty-1e-9 {
			return false
		}
	}
	return true
}

// Progress is the share of what it needs delivered, each good alike.
func (e *Expansion) Progress() float64 {
	p := 0.0
	for i, s := range e.Need {
		p += math.Min(1, e.Got[i]/s.Qty) / float64(len(e.Need))
	}
	return p
}

// Value is what is still to be delivered, at prices.
func (e *Expansion) Value(prices []float64) float64 {
	v := 0.0
	for i, s := range e.Need {
		v += math.Max(0, s.Qty-e.Got[i]) * prices[s.Good]
	}
	return v
}

// LevelCost is the construction a level takes where its recipe names no
// capital: priced so that a tile's capital is worth about three to four
// years of its output, as Europe's was in the 19th century.
const LevelCost = 1200.0

// levelOf is a recipe's capital, or LevelCost of construction if it names
// none.
func levelOf(c []Stock) []Stock {
	if len(c) == 0 {
		return []Stock{{Construction, LevelCost}}
	}
	return c
}

// LevelValue is what a level of the method is made of, at prices.
func (m *Method) LevelValue(prices []float64) float64 {
	v := 0.0
	for _, s := range m.Capital {
		v += s.Qty * prices[s.Good]
	}
	return v
}

// CapitalValue is what the firm's levels are made of, at prices.
func (f *Firm) CapitalValue(prices []float64) float64 {
	v := 0.0
	for i, m := range f.Methods {
		if l := f.MethodLevels(i); l > 0 {
			v += float64(l) * m.LevelValue(prices)
		}
	}
	return v
}

// wear is what the firm's levels wear out in a tick, at prices.
func (f *Firm) wear(prices []float64) float64 {
	v := 0.0
	for i, m := range f.Methods {
		if l := f.MethodLevels(i); l > 0 {
			for _, s := range m.Capital {
				v += float64(l) * s.Qty * Goods[s.Good].Wear * prices[s.Good]
			}
		}
	}
	return v
}

// capitalStock adds up the goods the firm's levels are made of, by good.
func (f *Firm) capitalStock(out []float64) {
	for i, m := range f.Methods {
		if l := f.MethodLevels(i); l > 0 {
			for _, s := range m.Capital {
				out[s.Good] += float64(l) * s.Qty
			}
		}
	}
}

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

// effectiveTFP is method i's TFP * L^(1-s) with the tile's multipliers: the
// productivity of its variable factors.
func (f *Firm) effectiveTFP(i int) float64 {
	m, l := f.Methods[i], f.MethodLevels(i)
	if m.powLevels != l || m.levelsPow == 0 {
		m.powLevels, m.levelsPow = l, math.Pow(float64(l), 1-m.exponentSum())
	}
	return m.TFP * m.levelsPow * f.Agglom * f.Mult
}

// depositRent is the rent of the firm's deposit a tick: what it is worth
// to it (DepositRent a unit), part of its profit.
func (f *Firm) depositRent() float64 { return f.DepositRent * f.Deposit }

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
func (f *Firm) AtCap() bool { return f.Fixed }

// BestMethod is the working method a level would earn most on at today's
// prices, of those with room for one more (-1 if none), and WorstMethod
// the one with levels that earns least (-1 if none).
func (f *Firm) BestMethod(prices []float64, wage float64) (int, float64) {
	best, top := -1, 0.0
	for i, m := range f.Methods {
		if !m.Runs() || f.AtCap() {
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
		p := 0.0 // a method the owner can't run (switched off, an input out of play) earns nothing
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
		Levels:  1, Enabled: true, Agglom: 1, Mult: 1,
		CashBuffer: 20, Inputs: make([]float64, NumGoods), CapitalIn: make([]float64, NumGoods),
		investWant: make([]float64, NumGoods), upkeepWant: make([]float64, NumGoods),
		PlanQty: make([]float64, NumGoods), PlanHas: make([]bool, NumGoods)}
}

func newMethod(name string, tfp float64, factors []Factor) *Method {
	return &Method{Name: name, Enabled: true, Factors: factors, TFP: tfp, BaseTFP: tfp, Capital: levelOf(nil), planQty: make([]float64, NumGoods)}
}

// TFP is the productivity per level of the firm's first method.
func (f *Firm) TFP() float64 { return f.Methods[0].TFP }

func (f *Firm) unitCost(fc Factor, prices []float64, wage float64) float64 {
	switch fc.Kind {
	case LaborFactor:
		return wage
	case DepositFactor: // what a unit of its deposit earns it
		return math.Max(f.DepositRent, 1e-9)
	case CompositeFactor:
		c := Composites[fc.Composite]
		return PriceIndex(prices, c.Goods, c.Sigma)
	default:
		return prices[fc.Good]
	}
}

// LevelProfit is what one level of the firm's i-th method would earn a tick
// at these prices, working at its best: with constant returns, a level's
// share of revenue, one less the exponents. Owners compare methods by it.
func (f *Firm) LevelProfit(i int, prices []float64, wage float64) float64 {
	m := f.Methods[i]
	p := f.worth(prices[f.Output])
	k, s := m.TFP*f.Agglom*f.Mult, 0.0
	for _, fc := range m.Factors {
		k *= math.Pow(fc.Exp*p/f.unitCost(fc, prices, wage), fc.Exp)
		s += fc.Exp
	}
	return (1 - s) * p * math.Pow(k, 1/(1-s))
}

// worth is what a unit of the firm's output is worth to it when it plans
// at price p. A firm that takes the price as given (the default) plans to
// make what it can sell at a profit at that price: it hires and buys until
// the last worker or input adds as much to its sales as it costs, so price
// equals its marginal cost. A monopoly knows that selling more lowers the
// price: a unit more earns only its marginal revenue, the price less what
// the lower price loses on the rest, p(1 - s/Elasticity) for a firm with a
// share s of its market. It plans on that, so it makes less, and the
// market's price is higher, by a markup over marginal cost. Its share is
// what it offered of all the tile's market offered, imports included (the
// merchants who bring the good from other tiles are its rivals), so cheap
// imports shrink its share and its markup with it.
func (f *Firm) worth(p float64) float64 {
	if !f.Monopoly {
		return p
	}
	return p * math.Max(0.05, 1-f.share/f.Elasticity)
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
	if !f.Enabled {
		return
	}
	p := f.worth(prices[f.Output])
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
	// Held factors: leased goods (land) and the firm's deposit. The firm
	// holds them and shares them among its methods as each would want them
	// at their price (a deposit's: what a unit of it earns); each method
	// plans its other inputs around its share, as a tenant farms the acres
	// he has. What it holds of a leased good moves a lease's fraction of
	// the way to what its methods want; its deposit is fixed.
	held := false
	for _, pl := range plans {
		held = held || pl.m.holds()
	}
	if held {
		if f.Held == nil {
			f.Held = make([]float64, NumGoods)
		}
		want := make([]float64, NumGoods)
		deposit := 0.0
		for _, pl := range plans {
			for j, fc := range pl.m.Factors {
				switch {
				case fc.Kind == DepositFactor:
					deposit += pl.want[j]
				case leaseOf(fc) > 0:
					want[fc.Good] += pl.want[j]
				}
			}
		}
		for g, x := range want {
			if l := Goods[g].Lease; l > 0 {
				if f.Held[g] <= 0 {
					f.Held[g] = x // a new firm takes up its land
				} else {
					f.Held[g] += (x - f.Held[g]) / l
				}
			}
		}
		total = 0
		rent := 0.0 // what its methods' shares of the deposit earn
		for k := range plans {
			pl := &plans[k]
			pl.m.deposit = 0
			if !pl.m.holds() {
				total += pl.q
				continue
			}
			kk, s := f.effectiveTFP(f.methodIndex(pl.m)), 0.0
			for j, fc := range pl.m.Factors {
				switch {
				case fc.Kind == DepositFactor:
					pl.m.deposit = f.Deposit * pl.want[j] / math.Max(deposit, 1e-300)
					pl.want[j] = pl.m.deposit
					kk *= math.Pow(pl.want[j], fc.Exp)
				case leaseOf(fc) > 0:
					pl.want[j] = f.Held[fc.Good] * pl.want[j] / math.Max(want[fc.Good], 1e-300)
					kk *= math.Pow(pl.want[j], fc.Exp)
				default:
					kk *= math.Pow(fc.Exp*p/pl.cost[j], fc.Exp)
					s += fc.Exp
				}
			}
			pl.q, pl.s = math.Pow(kk, 1/(1-s)), s
			for j, fc := range pl.m.Factors {
				switch {
				case fc.Kind == DepositFactor:
					rent += fc.Exp * p * pl.q
				case leaseOf(fc) == 0:
					pl.want[j] = fc.Exp * p * pl.q / pl.cost[j]
				}
			}
			total += pl.q
		}
		if f.Deposit > 0 {
			f.DepositRent = rent / f.Deposit
		}
	}
	// Stock beyond a tick's output is stock it failed to sell (when it
	// plans, its stock still holds yesterday's output, not yet offered): it
	// makes what it wants to sell less that excess, so goods nobody buys
	// stop being made instead of piling up. Scaling a method's every factor
	// by x^(1/s) scales its output by x.
	if over := f.Inventory - total; over > 0 && total > 0 {
		x := math.Max(0, total-over) / total
		for _, pl := range plans {
			scale := math.Pow(x, 1/pl.s)
			for j, fc := range pl.m.Factors {
				if !heldFactor(fc) { // what it holds it keeps
					pl.want[j] *= scale
				}
			}
		}
	}
	spend := 0.0
	for _, pl := range plans {
		for j, fc := range pl.m.Factors {
			if fc.Kind != DepositFactor { // its own: nothing to pay
				spend += pl.want[j] * pl.cost[j]
			}
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

// leaseOf is how many ticks a factor's holding takes to change (0: it
// isn't leased).
func leaseOf(fc Factor) float64 {
	if fc.Kind == GoodFactor {
		return Goods[fc.Good].Lease
	}
	return 0
}

// methodIndex is m's index among the firm's methods.
func (f *Firm) methodIndex(m *Method) int {
	for i, x := range f.Methods {
		if x == m {
			return i
		}
	}
	return -1
}

// heldFactor reports whether the firm holds a factor rather than buying
// what it wants each tick: a leased good, or its deposit.
func heldFactor(fc Factor) bool { return fc.Kind == DepositFactor || leaseOf(fc) > 0 }

// holds reports whether any of the method's factors is held.
func (m *Method) holds() bool {
	for _, fc := range m.Factors {
		if heldFactor(fc) {
			return true
		}
	}
	return false
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

// factorAmount is how much of a factor a method has, of its labor, goods
// and its share of the deposit.
func factorAmount(fc Factor, m *Method, labor float64, inputs []float64) float64 {
	switch fc.Kind {
	case LaborFactor:
		return labor
	case DepositFactor:
		return m.deposit
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
			x *= math.Pow(factorAmount(fc, m, m.labor, in), fc.Exp)
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

// CrowdingMult is how much one more level would multiply the firm's
// output from the same workers, inputs and deposit:
// ((n+1)/n)^(1-s). Output is constant-returns in levels and the variable
// factors together, so the gain depends only on the number of levels.
func (f *Firm) CrowdingMult() float64 {
	n := float64(max(f.Levels, 1))
	return math.Pow((n+1)/n, 1-f.Methods[f.Main()].exponentSum())
}

// NextLevelMult is how much one more level would multiply the firm's
// output from the same workers, inputs and deposit.
func (f *Firm) NextLevelMult() float64 { return f.CrowdingMult() }

// ValueAdded is what the firm added today: its output at the given price,
// less the goods it used up (wages and profits are the value it added).
func (f *Firm) ValueAdded(price float64) float64 {
	return f.LastOutput*price - f.tickInputs
}
