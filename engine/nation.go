package engine

import (
	"errors"
	"math"
)

// InfraLevel is a grade of infrastructure on a tile. Better infrastructure
// makes the tile cheaper to move through: every hop into or out of it costs
// less transport, and perishable goods lose less on the way.
type InfraLevel struct {
	Name      string
	MoveMult  float64            // multiplies the terrain's movement cost
	Cost      float64            // construction units to build it from the level below
	Materials map[GoodID]float64 // goods needed besides construction
	Upkeep    float64            // construction units per tick to maintain it
	Tech      string             // technology needed to build it, if any
}

// InfraLevels are the infrastructure grades, from none upwards.
var InfraLevels = []InfraLevel{
	{Name: "none", MoveMult: 1.0},
	{Name: "road", MoveMult: 0.6, Cost: 200, Upkeep: 0.5},
	{Name: "paved road", MoveMult: 0.4, Cost: 400, Upkeep: 1.0, Tech: "Masonry"},
	{Name: "railway", MoveMult: 0.15, Cost: 600, Upkeep: 2.0, Tech: "Railways",
		Materials: map[GoodID]float64{G("iron"): 60}},
}

// Project is infrastructure being built on a tile.
type Project struct {
	Target    int                // level being built
	Delivered float64            // construction units delivered so far
	Materials map[GoodID]float64 // other goods delivered so far
}

// Required is the construction units the project needs in total.
func (p *Project) Required() float64 { return InfraLevels[p.Target].Cost }

func (p *Project) done() bool {
	if p.Delivered < p.Required()-1e-9 {
		return false
	}
	for g, q := range InfraLevels[p.Target].Materials {
		if p.Materials[g] < q-1e-9 {
			return false
		}
	}
	return true
}

// Nation owns tiles, taxes its pops' income, sets trade policy and pays for
// infrastructure, firm expansions and research.
type Nation struct {
	ID       int
	Name     string
	Color    string
	Regions  []*Region
	TaxRate  float64 // share of pop income (wages and dividends) taken as tax
	Treasury float64
	// Reserve is the treasury the nation keeps; RebateRate of anything above it
	// is returned to its pops per head each tick.
	Reserve    float64
	RebateRate float64
	// MaxBuildRate caps construction units bought per tick per project.
	MaxBuildRate float64

	Tariff  []float64 // import tariff per good, as a share of the sale price
	Embargo []bool    // per nation: no trade across the border with it

	ResearchFunding float64 // money per tick offered for research
	ResearchPoints  float64 // research delivered towards the current tech
	Researching     int     // index into Techs, or -1 to pick automatically
	Known           []bool  // per tech

	// last tick's flows
	Revenue, TariffRevenue, Rebate, Spending float64
}

// Errors returned by nation and world commands.
var (
	ErrNoNation    = errors.New("tile belongs to no nation")
	ErrMaxLevel    = errors.New("tile already has the best infrastructure")
	ErrBuilding    = errors.New("tile already has a project under way")
	ErrNoProject   = errors.New("tile has no project under way")
	ErrBadTaxRate  = errors.New("tax rate must be in [0, 0.9]")
	ErrBadTariff   = errors.New("tariff must be in [0, 5]")
	ErrNeedTech    = errors.New("that needs a technology the nation lacks")
	ErrOwned       = errors.New("tile already belongs to a nation")
	ErrNotAdjacent = errors.New("tile does not border the nation")
	ErrTreasury    = errors.New("treasury cannot afford it")
	ErrBadTech     = errors.New("tech unknown, already known, or missing prerequisites")
	ErrFirmMax     = errors.New("firm cannot grow further here")
	ErrExpanding   = errors.New("firm is already expanding")
	ErrSelf        = errors.New("a nation cannot embargo itself")
	ErrBadFunding  = errors.New("research funding must be non-negative")
)

func newNation(id int, name, color string, nNations int, taxRate float64) *Nation {
	return &Nation{ID: id, Name: name, Color: color, TaxRate: taxRate,
		Reserve: 1000, RebateRate: 0.1, MaxBuildRate: 5,
		Tariff: make([]float64, NumGoods), Embargo: make([]bool, nNations),
		ResearchFunding: 40, Researching: -1, Known: make([]bool, len(Techs))}
}

// SetTaxRate changes the nation's tax rate.
func (n *Nation) SetTaxRate(rate float64) error {
	if !(rate >= 0 && rate <= 0.9) {
		return ErrBadTaxRate
	}
	n.TaxRate = rate
	return nil
}

// SetTariff sets the import tariff on one good, or on every tradable good if good < 0.
func (n *Nation) SetTariff(good GoodID, rate float64) error {
	if !(rate >= 0 && rate <= 5) {
		return ErrBadTariff
	}
	if good >= 0 {
		n.Tariff[good] = rate
		return nil
	}
	for _, g := range Tradable {
		n.Tariff[g] = rate
	}
	return nil
}

// SetResearchFunding sets the money per tick offered for research.
func (n *Nation) SetResearchFunding(amount float64) error {
	if !(amount >= 0) || math.IsInf(amount, 0) {
		return ErrBadFunding
	}
	n.ResearchFunding = amount
	return nil
}

// Has reports whether the nation knows a tech.
func (n *Nation) Has(tech string) bool { return tech == "" || n.Known[TechID(tech)] }

// CanResearch reports whether a tech is available to research now.
func (n *Nation) CanResearch(i int) bool {
	if i < 0 || i >= len(Techs) || n.Known[i] {
		return false
	}
	for _, req := range Techs[i].Requires {
		if !n.Has(req) {
			return false
		}
	}
	return true
}

// SetResearch picks the tech to research; points carry over.
func (n *Nation) SetResearch(i int) error {
	if !n.CanResearch(i) {
		return ErrBadTech
	}
	n.Researching = i
	return nil
}

func (n *Nation) firmMult(firm string) float64 {
	m := 1.0
	for i, t := range Techs {
		if n.Known[i] {
			if x, ok := t.FirmTFP[firm]; ok {
				m *= x
			}
		}
	}
	return m
}

func (n *Nation) coastMult() float64 {
	m := 1.0
	for i, t := range Techs {
		if n.Known[i] && t.CoastMove > 0 {
			m *= t.CoastMove
		}
	}
	return m
}

func (n *Nation) spoilageMult() float64 {
	m := 1.0
	for i, t := range Techs {
		if n.Known[i] && t.TransitSpoilage > 0 {
			m *= t.TransitSpoilage
		}
	}
	return m
}

func (n *Nation) population() float64 {
	s := 0.0
	for _, r := range n.Regions {
		s += r.Population()
	}
	return s
}

// payIncome splits an income payment to a pop into what the pop keeps and the tax.
func (r *Region) payIncome(p *Pop, amount float64) {
	if r.Nation == nil {
		p.Cash += amount
		return
	}
	tax := amount * r.Nation.TaxRate
	p.Cash += amount - tax
	r.Nation.Treasury += tax
	r.Nation.Revenue += tax
}

// claim is something a nation buys in a tile this tick.
type claimKind int

const (
	upkeepClaim claimKind = iota
	infraClaim
	materialClaim
	expansionClaim
	researchClaim
)

type claim struct {
	kind claimKind
	good GoodID
	exp  *Expansion
	qty  float64
}

// plan decides what the nation buys in each of its tiles this tick: upkeep,
// infrastructure (construction and materials), firm expansions and research,
// scaled down together if the treasury can't pay for it all.
func (n *Nation) plan() {
	n.Revenue, n.TariffRevenue, n.Rebate, n.Spending = 0, 0, 0, 0
	n.pickResearch()
	total := 0.0
	for _, r := range n.Regions {
		r.claims = r.claims[:0]
		add := func(c claim) {
			if c.qty > 0 {
				r.claims = append(r.claims, c)
				total += c.qty * r.Markets[c.good].Price
			}
		}
		add(claim{kind: upkeepClaim, good: Construction, qty: InfraLevels[r.Infra].Upkeep})
		if p := r.Project; p != nil {
			add(claim{kind: infraClaim, good: Construction, qty: math.Min(n.MaxBuildRate, p.Required()-p.Delivered)})
			for _, g := range sortedGoods(InfraLevels[p.Target].Materials) {
				need := InfraLevels[p.Target].Materials[g] - p.Materials[g]
				add(claim{kind: materialClaim, good: g, qty: math.Min(n.MaxBuildRate, need)})
			}
		}
		for _, f := range r.Firms {
			if e := f.Expansion; e != nil && e.Public {
				add(claim{kind: expansionClaim, good: Construction, exp: e, qty: math.Min(n.MaxBuildRate, LevelCost-e.Delivered)})
			}
		}
		if n.Researching >= 0 && n.ResearchFunding > 0 {
			add(claim{kind: researchClaim, good: Research,
				qty: n.ResearchFunding / float64(len(n.Regions)) / r.Markets[Research].Price})
		}
	}
	scale := 1.0
	if budget := 0.9 * math.Max(n.Treasury, 0); total > budget {
		scale = budget / total
	}
	for _, r := range n.Regions {
		for g := range r.publicBid {
			r.publicBid[g], r.publicGot[g] = 0, 0
		}
		for i := range r.claims {
			r.claims[i].qty *= scale
			r.publicBid[r.claims[i].good] += r.claims[i].qty
		}
	}
}

// sortedGoods lists a map's goods in GoodID order, for determinism.
func sortedGoods(m map[GoodID]float64) []GoodID {
	var out []GoodID
	for g := range Goods {
		if _, ok := m[GoodID(g)]; ok {
			out = append(out, GoodID(g))
		}
	}
	return out
}

// settle shares what each tile got among its claims, completes projects,
// expansions and research, and lets neglected infrastructure decay. Returns
// whether any tile's movement cost changed.
func (n *Nation) settle() (hopsChanged bool) {
	for _, r := range n.Regions {
		upkeep := 0.0
		for _, c := range r.claims {
			got := 0.0
			if bid := r.publicBid[c.good]; bid > 0 {
				got = c.qty * r.publicGot[c.good] / bid
			}
			switch c.kind {
			case upkeepClaim:
				upkeep += got
			case infraClaim:
				r.Project.Delivered += got
			case materialClaim:
				r.Project.Materials[c.good] += got
			case expansionClaim:
				c.exp.Delivered += got
			case researchClaim:
				n.ResearchPoints += got
			}
		}
		need := InfraLevels[r.Infra].Upkeep
		r.Neglect = r.Neglect*0.98 + math.Max(0, need-upkeep)
		if p := r.Project; p != nil && p.done() {
			r.Infra, r.Project, r.Neglect = p.Target, nil, 0
			hopsChanged = true
		} else if r.Infra > 0 && r.Neglect > neglectLimit*need {
			r.Infra--
			r.Neglect = 0
			hopsChanged = true
		}
	}
	if i := n.Researching; i >= 0 && n.ResearchPoints >= Techs[i].Cost {
		n.ResearchPoints -= Techs[i].Cost
		n.Known[i] = true
		n.Researching = -1
		n.pickResearch()
		n.applyTech()
		hopsChanged = true
	}
	return hopsChanged
}

// pickResearch chooses the first available tech if the nation isn't
// researching anything.
func (n *Nation) pickResearch() {
	if n.Researching >= 0 && !n.Known[n.Researching] {
		return
	}
	n.Researching = -1
	for i := range Techs {
		if n.CanResearch(i) {
			n.Researching = i
			return
		}
	}
}

// applyTech brings the nation's firms up to its technology.
func (n *Nation) applyTech() {
	for _, r := range n.Regions {
		for _, f := range r.Firms {
			f.TFP = f.BaseTFP * n.firmMult(f.Name)
		}
	}
}

// rebate returns part of the treasury above the reserve to the pops, per head.
func (n *Nation) rebate() {
	excess := n.Treasury - n.Reserve
	if excess <= 0 {
		return
	}
	amount := n.RebateRate * excess
	pop := n.population()
	n.Treasury -= amount
	n.Rebate = amount
	for _, r := range n.Regions {
		for _, p := range r.Pops {
			p.Cash += amount * p.Size / pop
		}
	}
}

// AnnexCostPerHead is what annexing a tile costs per person living there; it
// is paid to them.
const AnnexCostPerHead = 15.0

// neglectLimit is how many ticks' worth of unpaid upkeep (decaying by 2% a
// tick) a tile's infrastructure tolerates before it drops a level: about 60
// ticks with no upkeep at all, or a long spell below 30% of it.
const neglectLimit = 35
