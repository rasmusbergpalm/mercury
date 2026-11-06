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

// Project is infrastructure being built on a tile: the next road level, or a port.
type Project struct {
	Target    int                // road level being built (ignored for a port)
	Port      bool               // building a port
	Delivered float64            // construction units delivered so far
	Materials map[GoodID]float64 // other goods delivered so far
}

// Ports: what one costs. A port lets a coast tile trade directly with every
// other port, at a cost that depends on distance at sea.
var (
	PortCost      = 300.0
	PortMaterials = map[GoodID]float64{G("wood"): 60}
)

// Required is the construction units the project needs in total.
func (p *Project) Required() float64 {
	if p.Port {
		return PortCost
	}
	return InfraLevels[p.Target].Cost
}

// RequiredMaterials are the other goods the project needs.
func (p *Project) RequiredMaterials() map[GoodID]float64 {
	if p.Port {
		return PortMaterials
	}
	return InfraLevels[p.Target].Materials
}

func (p *Project) done() bool {
	if p.Delivered < p.Required()-1e-9 {
		return false
	}
	for g, q := range p.RequiredMaterials() {
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

	// Budget is the share of income (IncomeEMA) spent each tick on each
	// of the economy (infrastructure, upkeep, firm expansions), science
	// and the military. What isn't spent builds up the treasury.
	Budget    [NumBudgets]float64
	Drill     float64 // share of wartime supplies armies use in peace
	IncomeEMA float64 // moving average of tax and tariff income per tick

	ResearchPoints float64 // research delivered towards the current tech
	Researching    int     // index into Techs, or -1 to pick automatically
	Queue          []int   // techs to research next, in order
	Known          []bool  // per tech
	// TradeWith is, per nation, a moving average of the value of goods
	// shipped between the two; ideas travel with trade.
	TradeWith []float64

	AtWar      []bool // per nation
	WarSince   []int  // per nation: tick the current war began
	PeaceOffer []bool // per nation: this nation has offered it peace
	AI         bool   // run by the computer
	// ChooseResearch: a tech finished with nothing queued, so the next one
	// was picked automatically and the player should be asked to choose.
	ChooseResearch bool

	// last tick's flows
	Revenue, TariffRevenue, Rebate, Spending float64
}

// Budget categories.
const (
	Economy = iota
	Science
	Defense
	NumBudgets
)

// BudgetNames name the budget categories.
var BudgetNames = [NumBudgets]string{"economy", "science", "military"}

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
	ErrBadDrill    = errors.New("peacetime drill must be between 20% and 80% of wartime needs")
	ErrFirmMax     = errors.New("firm cannot grow further here")
	ErrExpanding   = errors.New("firm is already expanding")
	ErrSelf        = errors.New("a nation cannot embargo itself")
	ErrNotCoast    = errors.New("only coastal tiles can have ports")
	ErrHasPort     = errors.New("tile already has a port")
	ErrFirmLocked  = errors.New("that firm needs a technology the nation lacks")
	ErrBadBudget   = errors.New("budget shares must be non-negative and sum to at most 1")
	ErrEliminated  = errors.New("the nation has no tiles left")
)

func newNation(id int, name, color string, nNations int, taxRate float64) *Nation {
	return &Nation{ID: id, Name: name, Color: color, TaxRate: taxRate,
		Reserve: 1000, RebateRate: 0.1, MaxBuildRate: 5,
		Tariff: make([]float64, NumGoods), Embargo: make([]bool, nNations),
		Budget: DefaultBudget, Drill: DefaultDrill, Researching: -1, Known: make([]bool, len(Techs)),
		TradeWith: make([]float64, nNations), AtWar: make([]bool, nNations), WarSince: make([]int, nNations),
		PeaceOffer: make([]bool, nNations)}
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
	for _, g := range GameTradable {
		n.Tariff[g] = rate
	}
	return nil
}

// DefaultBudget is how a new nation splits its income.
var DefaultBudget = [NumBudgets]float64{0.4, 0.2, 0.2}

// Peacetime drill: in peace, armies use this share of their wartime
// supplies (the nation chooses between minimal drills and realistic ones,
// which keep the arms industries busy). In war they use all of them.
const (
	MinDrill     = 0.2
	MaxDrill     = 0.8
	DefaultDrill = 0.5
)

// SetDrill sets the share of wartime supplies armies use in peace.
func (n *Nation) SetDrill(x float64) error {
	if !(x >= MinDrill-1e-9 && x <= MaxDrill+1e-9) {
		return ErrBadDrill
	}
	n.Drill = x
	return nil
}

// SetBudget sets the shares of income spent on economy, science and military.
func (n *Nation) SetBudget(b [NumBudgets]float64) error {
	sum := 0.0
	for _, x := range b {
		if !(x >= 0) {
			return ErrBadBudget
		}
		sum += x
	}
	if sum > 1+1e-9 {
		return ErrBadBudget
	}
	n.Budget = b
	return nil
}

// Eliminated reports whether the nation has lost all its tiles.
func (n *Nation) Eliminated() bool { return len(n.Regions) == 0 }

// Research diffusion: a tech known by trade partners is cheaper, by up to
// maxDiffusion, reached when trade with nations that know it is worth
// diffusionShare of the nation's income.
const (
	maxDiffusion   = 0.5
	diffusionShare = 0.5
)

// Discount is how much cheaper a tech is thanks to trade partners who know it.
func (n *Nation) Discount(i int, nations []*Nation) float64 {
	trade := 0.0
	for _, o := range nations {
		if o != n && o.Known[i] {
			trade += n.TradeWith[o.ID]
		}
	}
	share := trade / math.Max(n.IncomeEMA, 1) / diffusionShare
	return math.Min(0.75, maxDiffusion*math.Min(1, share)*n.diffusionMult())
}

// TechCost is what a tech costs the nation now, after diffusion.
func (n *Nation) TechCost(i int, nations []*Nation) float64 {
	return Techs[i].Cost * (1 - n.Discount(i, nations))
}

// SetQueue replaces the research queue (techs to research after the current one).
func (n *Nation) SetQueue(q []int) error {
	for _, i := range q {
		if i < 0 || i >= len(Techs) || n.Known[i] {
			return ErrBadTech
		}
	}
	n.Queue = append([]int(nil), q...)
	n.ChooseResearch = false
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

// SetResearch picks the tech to research now; points carry over.
func (n *Nation) SetResearch(i int) error {
	if !n.CanResearch(i) {
		return ErrBadTech
	}
	n.Researching = i
	n.ChooseResearch = false
	return nil
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
		p.Income += amount
		return
	}
	tax := amount * r.Nation.TaxRate
	p.Cash += amount - tax
	p.Income += amount - tax
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
	armyClaim
)

type claim struct {
	kind claimKind
	good GoodID
	exp  *Expansion
	army *Army
	qty  float64
}

// plan decides what the nation buys in each of its tiles this tick: upkeep,
// infrastructure (construction and materials), firm expansions, research and
// its armies' supplies. Each budget caps its own spending; all of it is
// scaled down together if the treasury can't pay.
func (n *Nation) plan(w *World) {
	n.Revenue, n.TariffRevenue, n.Rebate, n.Spending = 0, 0, 0, 0
	n.pickResearch()
	if n.Eliminated() {
		return
	}
	tiles := float64(len(n.Regions))
	rate := n.MaxBuildRate * n.buildRateMult()
	var spend [NumBudgets]float64
	for _, r := range n.Regions {
		r.claims = r.claims[:0]
	}
	category := func(k claimKind) int {
		switch k {
		case researchClaim:
			return Science
		case armyClaim:
			return Defense
		}
		return Economy
	}
	addTo := func(r *Region, c claim) {
		if c.qty > 0 {
			r.claims = append(r.claims, c)
			spend[category(c.kind)] += c.qty * r.Markets[c.good].Price
		}
	}
	w.planSupply(n, addTo)
	for _, r := range n.Regions {
		add := func(c claim) { addTo(r, c) }
		add(claim{kind: upkeepClaim, good: Construction, qty: InfraLevels[r.Infra].Upkeep})
		if p := r.Project; p != nil {
			add(claim{kind: infraClaim, good: Construction, qty: math.Min(rate, p.Required()-p.Delivered)})
			mats := p.RequiredMaterials()
			for _, g := range sortedGoods(mats) {
				need := mats[g] - p.Materials[g]
				add(claim{kind: materialClaim, good: g, qty: math.Min(rate, need)})
			}
		}
		for _, f := range r.Firms {
			if e := f.Expansion; e != nil && e.Public {
				add(claim{kind: expansionClaim, good: Construction, exp: e, qty: math.Min(rate, LevelCost-e.Delivered)})
			}
		}
		if n.Researching >= 0 {
			add(claim{kind: researchClaim, good: Research,
				qty: n.Budget[Science] * n.IncomeEMA / tiles / r.Markets[Research].Price})
		}
	}
	// each budget caps its category; then the treasury caps the total
	var catScale [NumBudgets]float64
	total := 0.0
	for k := range spend {
		catScale[k] = 1
		if limit := n.Budget[k] * n.IncomeEMA; spend[k] > limit {
			catScale[k] = limit / spend[k]
		}
		total += spend[k] * catScale[k]
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
			c := &r.claims[i]
			c.qty *= scale * catScale[category(c.kind)]
			r.publicBid[c.good] += c.qty
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
// expansions and research, supplies armies, and lets neglected
// infrastructure decay. Returns whether any tile's movement cost changed.
func (n *Nation) settle(w *World) (hopsChanged bool) {
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
			case armyClaim:
				if c.good == Transport {
					c.army.gotT += got
				} else {
					c.army.got[c.good] += got
				}
			}
		}
		need := InfraLevels[r.Infra].Upkeep
		r.Neglect = r.Neglect*0.98 + math.Max(0, need-upkeep)
		if p := r.Project; p != nil && p.done() && p.Port {
			r.Port, r.Project = true, nil // the world opens its sea routes
			w.emit(n.ID, "built", r.ID, "Port finished at tile %s", r.Name)
		} else if p != nil && p.done() {
			r.Infra, r.Project, r.Neglect = p.Target, nil, 0
			hopsChanged = true
			w.emit(n.ID, "built", r.ID, "%s finished at tile %s", InfraLevels[r.Infra].Name, r.Name)
		} else if r.Infra > 0 && r.Neglect > neglectLimit*need {
			r.Infra--
			r.Neglect = 0
			hopsChanged = true
		}
	}
	for _, a := range w.ArmiesOf(n) {
		a.settleSupply()
	}
	income := n.Revenue + n.TariffRevenue
	if n.IncomeEMA == 0 {
		n.IncomeEMA = income
	}
	n.IncomeEMA = 0.95*n.IncomeEMA + 0.05*income
	if i := n.Researching; i >= 0 && n.ResearchPoints >= n.TechCost(i, w.Nations) {
		n.ResearchPoints -= n.TechCost(i, w.Nations)
		n.Known[i] = true
		n.Researching = -1
		queued := false
		for _, q := range n.Queue {
			queued = queued || n.CanResearch(q)
		}
		n.pickResearch()
		n.ChooseResearch = !n.AI && !queued && n.Researching >= 0
		n.applyTech()
		hopsChanged = true
		w.emit(n.ID, "tech", -1, "%s discovers %s", n.Name, Techs[i].Name)
	}
	return hopsChanged
}

// pickResearch chooses what to research when the nation isn't researching
// anything: the first available tech in its queue, else the cheapest
// available one (ties go to the earlier one).
func (n *Nation) pickResearch() {
	if n.Researching >= 0 && !n.Known[n.Researching] {
		return
	}
	n.Researching = -1
	for len(n.Queue) > 0 && n.Known[n.Queue[0]] {
		n.Queue = n.Queue[1:]
	}
	for qi, i := range n.Queue {
		if n.CanResearch(i) {
			n.Researching = i
			n.Queue = append(n.Queue[:qi:qi], n.Queue[qi+1:]...)
			return
		}
	}
	// else the cheapest available tech, preferring the branch the nation
	// knows least of (so research stays balanced)
	known := map[string]int{}
	for i, k := range n.Known {
		if k {
			known[Techs[i].Branch]++
		}
	}
	better := func(i, j int) bool {
		if Techs[i].Cost != Techs[j].Cost {
			return Techs[i].Cost < Techs[j].Cost
		}
		return known[Techs[i].Branch] < known[Techs[j].Branch]
	}
	for i := range Techs {
		if n.CanResearch(i) && (n.Researching < 0 || better(i, n.Researching)) {
			n.Researching = i
		}
	}
}

// applyTech brings the nation's firms up to its technology.
func (n *Nation) applyTech() {
	for _, r := range n.Regions {
		for _, f := range r.Firms {
			f.TFP = f.BaseTFP * n.firmMult(f)
			f.Enabled = firmEnabled(f, n)
		}
	}
}

// rebate returns part of the treasury above the reserve to the pops, per head.
func (n *Nation) rebate() {
	excess := n.Treasury - n.Reserve
	if excess <= 0 || n.Eliminated() {
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

// neglectLimit is how many ticks' worth of unpaid upkeep (decaying by 2% a
// tick) a tile's infrastructure tolerates before it drops a level: about 60
// ticks with no upkeep at all, or a long spell below 30% of it.
const neglectLimit = 35
