package engine

import (
	"fmt"
	"math"
	"strings"
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

// Firm turns labor and inputs into one output good with a decreasing-returns
// Cobb-Douglas technology q = A * prod f_i^a_i, sum a_i < 1.
type Firm struct {
	Name   string
	Output GoodID
	// TFP is productivity per building level, including technology; BaseTFP
	// is without technology.
	TFP, BaseTFP float64
	Factors      []Factor
	// Levels of buildings. Output is constant-returns in levels and the
	// variable factors together: q = TFP * Levels^(1-s) * prod f_i^a_i.
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
	// State firms (universities) are run by the tile's nation: it funds
	// Funded of their levels and takes their output; see university.go.
	State   bool
	Funded  int
	Public  Public  // which public service, for state firms
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
	LossTicks, Age        int // ticks of losses in a row; ticks alive
	tickRevenue, tickCost float64
	tickInputs            float64   // what the goods it used up cost this tick
	used                  []float64 // goods it used up producing this tick, by good
	investWant            float64   // construction to buy this tick for a private expansion
	powLevels             int       // the Levels levelsPow was worked out for
	levelsPow             float64   // Levels^(1-s), cached
}

// Expansion is a building level under construction for a firm.
type Expansion struct {
	Delivered float64 // construction units delivered so far
	Public    bool    // paid for by the nation (else by the firm's owners)
}

// LevelCost is the construction units one building level costs.
const LevelCost = 120.0

func (f *Firm) exponentSum() float64 {
	s := 0.0
	for _, fc := range f.Factors {
		s += fc.Exp
	}
	return s
}

// effectiveTFP is TFP * Levels^(1-s): productivity of the firm's variable factors.
func (f *Firm) effectiveTFP() float64 {
	if f.powLevels != f.Levels || f.levelsPow == 0 {
		f.powLevels, f.levelsPow = f.Levels, math.Pow(float64(f.Levels), 1-f.exponentSum())
	}
	return f.TFP * f.levelsPow * f.Agglom * f.EduMult * f.Loyal
}

// NewFirm makes a firm.
func NewFirm(name string, output GoodID, tfp float64, factors []Factor, cash float64) *Firm {
	return &Firm{Name: name, Output: output, TFP: tfp, BaseTFP: tfp, Factors: factors, Cash: cash,
		Levels: 1, MaxLevels: 8, Enabled: true, Agglom: 1, EduMult: 1, Loyal: 1,
		CashBuffer: 20, Inputs: make([]float64, NumGoods),
		PlanQty: make([]float64, NumGoods), PlanHas: make([]bool, NumGoods)}
}

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

// Plan sets the profit-maximising labor and goods demand at current prices,
// capped by cash.
//
// With q = A prod f_i^a_i and s = sum a_i < 1:
//
//	q* = (A prod (a_i p / w_i)^a_i)^(1/(1-s)),   f_i* = a_i p q* / w_i
//
// Composite factors are then split into their cheapest goods mix.
func (f *Firm) Plan(prices []float64, wage float64) {
	if !f.Enabled || f.State { // the state staffs its universities itself
		f.PlanLabor, f.DesiredSpend = 0, 0
		for i := range f.PlanQty {
			f.PlanQty[i], f.PlanHas[i] = 0, false
		}
		return
	}
	p := prices[f.Output]
	n := len(f.Factors)
	cost := make([]float64, n)
	s := 0.0
	k := f.effectiveTFP()
	for i, fc := range f.Factors {
		cost[i] = f.unitCost(fc, prices, wage)
		s += fc.Exp
	}
	for i, fc := range f.Factors {
		k *= math.Pow(fc.Exp*p/cost[i], fc.Exp)
	}
	q := math.Pow(k, 1/(1-s))
	want := make([]float64, n)
	spend := 0.0
	for i, fc := range f.Factors {
		want[i] = fc.Exp * p * q / cost[i]
	}
	for i := range f.Factors {
		spend += want[i] * cost[i]
	}
	f.DesiredSpend = spend
	budget := 0.9 * math.Max(f.Cash, 0)
	if spend > budget { // liquidity constraint
		for i := range want {
			want[i] = want[i] * budget / spend
		}
	}

	f.PlanLabor = 0
	for i := range f.PlanQty {
		f.PlanQty[i], f.PlanHas[i] = 0, false
	}
	for i, fc := range f.Factors {
		switch fc.Kind {
		case LaborFactor:
			f.PlanLabor = want[i]
		case GoodFactor:
			f.PlanQty[fc.Good], f.PlanHas[fc.Good] = want[i], true
		case CompositeFactor:
			c := Composites[fc.Composite]
			for j, x := range Split(want[i], prices, c.Goods, c.Sigma) {
				f.PlanQty[c.Goods[j]], f.PlanHas[c.Goods[j]] = x, true
			}
		}
	}
}

// FactorAmount is how much of a factor the firm has this tick.
func (f *Firm) FactorAmount(fc Factor) float64 {
	switch fc.Kind {
	case LaborFactor:
		return f.Labor
	case CompositeFactor:
		c := Composites[fc.Composite]
		return Aggregate(f.Inputs, c.Goods, c.Sigma)
	default:
		return math.Max(f.Inputs[fc.Good], 0)
	}
}

// Produce turns this tick's labor and inputs into output.
func (f *Firm) Produce() float64 {
	q := f.effectiveTFP()
	for _, fc := range f.Factors {
		q *= math.Pow(f.FactorAmount(fc), fc.Exp)
	}
	f.Inventory += q
	if f.used == nil {
		f.used = make([]float64, len(f.Inputs))
	}
	copy(f.used, f.Inputs)
	for i := range f.Inputs {
		f.Inputs[i] = 0
	}
	f.LastOutput = q
	return q
}

// Bonuses explains the firm's productivity factor by factor: its recipe's
// base, its site (terrain or coast), each technology of its nation, the
// tile's size (industry and services), its schooling, and the crowding
// malus: how much less productive it is than it would be with one more
// building level.
func (f *Firm) Bonuses(n *Nation) []Modifier {
	site := 1.0
	if f.SiteMult > 0 {
		site = f.SiteMult
	}
	out := []Modifier{{"Base", f.BaseTFP / site, "the recipe's productivity", false}}
	if f.SiteMult > 0 {
		out = append(out, Modifier{cap1(f.Site), f.SiteMult, "how well the site suits it", false})
	}
	if n != nil {
		for i, t := range Techs {
			if !n.Known[i] {
				continue
			}
			full := techFirmMult(&t, f)
			if full == 1 {
				continue
			}
			note := "technology"
			if a := n.Adoption(i); a < 1 {
				note = fmt.Sprintf("technology, %.0f%% adopted (of ×%.2f)", 100*a, full)
			}
			out = append(out, Modifier{t.Name, 1 + (full-1)*n.Adoption(i), note, false})
		}
	}
	if f.Agglom != 1 {
		out = append(out, Modifier{"Tile size", f.Agglom, "industry and services gain from the people around them", false})
	}
	if f.Loyal < 1 {
		out = append(out, Modifier{"Allegiance", f.Loyal, "the tile's people don't yet stand by their new rulers", false})
	}
	if f.EduMult != 1 {
		out = append(out, Modifier{"Schooling", f.EduMult, "the tile's educated share", false})
	}
	if !f.State && f.Levels > 0 {
		m := f.NextLevelMult()
		note := fmt.Sprintf("%d levels; one more would make every worker and input %.0f%% more productive", f.Levels, 100*(m-1))
		if f.Levels >= f.MaxLevels {
			note = fmt.Sprintf("%d levels, the most it can have here", f.Levels)
		}
		out = append(out, Modifier{"Crowding", 1 / m, note, false})
	}
	return out
}

// NextLevelMult is how much one more building level would multiply the
// firm's output from the same workers and inputs: ((n+1)/n)^(1-s). Output is
// constant-returns in levels and the variable factors together, so the gain
// depends only on the number of levels.
func (f *Firm) NextLevelMult() float64 {
	n := float64(max(f.Levels, 1))
	return math.Pow((n+1)/n, 1-f.exponentSum())
}

func cap1(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// ValueAdded is what the firm added today: its output at the given price,
// less the goods it used up (wages and profits are the value it added).
func (f *Firm) ValueAdded(price float64) float64 {
	return f.LastOutput*price - f.tickInputs
}
