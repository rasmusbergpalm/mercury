package engine

import (
	"errors"
	"fmt"
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
	{Name: "paved road", MoveMult: 0.4, Cost: 400, Upkeep: 1.0, Tech: "Macadam roads"},
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
	RealGDP  float64 // real GDP per day in pounds of the first month, linked monthly (realgdp.go)
	// SpendEMA is a moving average of what the state spends a day.
	SpendEMA float64
	// MaxBuildRate caps construction units bought per tick per project.
	MaxBuildRate float64

	Tariff  []float64 // import tariff per good, as a share of the sale price
	Embargo []bool    // per nation: no trade across the border with it

	// SpentOn is what the nation spent today on each of the economy
	// (infrastructure, upkeep, firm expansions), science and the military
	// (supplies and pay).
	SpentOn     [NumSpending]float64
	AutoBuild   bool // the computer builds roads and expands firms for it
	AutoTax     bool // the computer sets its tax to keep a month's spending in the treasury
	Pension     int  // pension level, 0..MaxPension: pensioners get Pension/MaxPension of the average wage
	pensionBill float64
	payBill     float64     // soldiers' pay due today
	payScale    float64     // share of today's bills the treasury could cover
	Standing    float64     // share of each army's wartime size kept under arms in peace
	Temper      Temperament // how the computer plays it
	seed        int64       // the game's seed, for its deterministic choices
	IncomeEMA   float64     // moving average of tax and tariff income per tick

	ResearchPoints float64 // research delivered towards the current tech
	Researching    int     // index into Techs, or -1 to pick automatically
	// Copying is the tech the nation is catching up on (one others it
	// learns from know), or -1; CopyPoints the research delivered to it.
	// See pickCopy.
	Copying       int
	CopyPoints    float64
	Queue         []int   // techs to research next, in order
	Known         []bool  // per tech
	Coalition     int     // 1 + the nation it is allied against; 0: none (see alliance.go)
	AllianceSince int     // tick its alliance formed
	MightEMA      float64 // its might, averaged over about a year (for alliances)
	// LearnedAt is the tick each known tech was learned (0: always known, or
	// granted outright: fully adopted); see Adoption.
	LearnedAt []int
	now       int // today's tick, for Adoption
	// TradeWith is, per nation, a moving average of the value of goods
	// shipped between the two; ideas travel with trade.
	TradeWith []float64

	AtWar      []bool // per nation
	WarSince   []int  // per nation: tick the current war began, or the last peace was made
	PeaceOffer []bool // per nation: this nation has offered it peace
	AI         bool   // run by the computer
	// ChooseResearch: a tech finished with nothing queued, so the next one
	// was picked automatically and the player should be asked to choose.
	ChooseResearch bool

	// last tick's flows
	Revenue, TariffRevenue, Spending float64
}

// Spending categories.
const (
	Economy = iota
	Science
	Defense
	Admin
	Education
	HealthCare
	Pensions
	NumSpending
)

// SpendingNames name the spending categories.
var SpendingNames = [NumSpending]string{"economy", "science", "military", "administration", "education", "health", "pensions"}

func spendingOf(k claimKind) int {
	switch k {
	case armyClaim:
		return Defense
	}
	return Economy
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
	ErrBadPension  = errors.New("the pension level must be between 0 and 4")
	ErrBadStanding = errors.New("the standing army must be between 10% and 100% of wartime strength")
	ErrFirmMax     = errors.New("firm cannot grow further here")
	ErrExpanding   = errors.New("firm is already expanding")
	ErrSelf        = errors.New("a nation cannot embargo itself")
	ErrNotCoast    = errors.New("only coastal tiles can have ports")
	ErrHasPort     = errors.New("tile already has a port")
	ErrNoPort      = errors.New("tile has no port")
	ErrPortLimit   = fmt.Errorf("a nation may have one port for every %d tiles it holds (at least one)", TilesPerPort)
	ErrFirmLocked  = errors.New("that firm needs a technology the nation lacks")
	ErrMethod      = errors.New("the firm can't run that method here (its technology, or an input nobody makes yet)")
	ErrNoRefit     = errors.New("the firm has no level on another method to refit")
	ErrEliminated  = errors.New("the nation has no tiles left")
)

func newNation(id int, name, color string, nNations int, taxRate float64) *Nation {
	return &Nation{ID: id, Name: name, Color: color, TaxRate: taxRate,
		MaxBuildRate: 5,
		Tariff:       make([]float64, NumGoods), Embargo: make([]bool, nNations),
		Standing: DefaultStanding, payScale: 1, Researching: -1, Copying: -1, Known: make([]bool, len(Techs)), LearnedAt: make([]int, len(Techs)),
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

// The standing army: in peace each army keeps this share of its wartime
// size under arms; the rest are reservists, living and working at home as
// ordinary people (in the nation's tiles, by their population), and are
// called up when war comes. A big standing army is ready but costly; a
// small one is cheap but takes weeks to mobilize.
const (
	MinStanding     = 0.1
	MaxStanding     = 1.0
	DefaultStanding = 0.5
)

// Pensions: the state pays the elderly (pensionerShare of the people) a
// share of the nation's average wage, in MaxPension steps up to all of it.
// The money goes straight to the people; it needs no buildings or staff.
const (
	MaxPension     = 4
	pensionerShare = 0.08
)

// SetPension sets the pension level (0..MaxPension).
func (n *Nation) SetPension(level int) error {
	if level < 0 || level > MaxPension {
		return ErrBadPension
	}
	n.Pension = level
	return nil
}

// pensions is what the nation's pensions cost today.
func (n *Nation) pensions() float64 {
	return n.population() * pensionerShare * n.averageWage() * float64(n.Pension) / MaxPension
}

// payPensions pays (what the treasury could cover of) today's pensions to
// the people, per head.
func (n *Nation) payPensions() {
	pay, people := n.pensionBill*n.payScale, n.population()
	if pay <= 0 || people <= 0 {
		return
	}
	n.Treasury -= pay
	n.SpentOn[Pensions] += pay
	for _, r := range n.Regions {
		for _, p := range r.Pops {
			x := pay * p.Size / people
			p.Cash += x
			p.Income += x
		}
	}
}

// SetStanding sets the share of each army's wartime size kept under arms in
// peace.
func (n *Nation) SetStanding(x float64) error {
	if !(x >= MinStanding-1e-9 && x <= MaxStanding+1e-9) {
		return ErrBadStanding
	}
	n.Standing = x
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
		if o != n && o.Known[i] && !n.closedTo(o) { // ideas cross only an open border: the trade of before an embargo carries none
			trade += n.TradeWith[o.ID]
		}
	}
	share := trade / math.Max(n.IncomeEMA, 1) / diffusionShare
	return math.Min(0.75, maxDiffusion*math.Min(1, share)*n.diffusionMult())
}

// techCostTiles is the map a tech's listed cost is for: the classic one.
const techCostTiles = 36

// TechCost is what a tech costs the nation now, after diffusion. Techs cost
// more on a bigger map, in proportion to its tiles. Research grows with the
// universities a nation's land holds, and a map is a whole world: a nation
// holding a quarter of a big map should take about as long over the
// century's inventions as one holding a quarter of a small one, not race
// through them because its quarter has more tiles. The cost is the same for
// every nation and stays put through the game, whatever a nation wins or loses.
func (n *Nation) TechCost(i int, w *World) float64 {
	return Techs[i].Cost * float64(len(w.Regions)) / techCostTiles * (1 - n.Discount(i, w.Nations))
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
	tax := amount * r.Nation.TaxRate * r.Collect * r.AllegianceMult() // what its administration collects, from people who pay
	p.Cash += amount - tax
	p.Income += amount - tax
	r.ledger.tax += tax // the nation gets it at settleLedgers
}

// Building (projects, their materials and state-funded expansions, public
// service levels among them) is paid from a budget: up to buildShare of the
// state's income a day while the treasury holds its reserve, less as it
// falls below it and nothing at half of it, plus whatever it holds beyond
// the reserve, spread over buildDrawdown days. So a burst of projects
// can't multiply the day's bill and empty the treasury, and the tax has
// time to follow.
const (
	buildShare    = 0.6
	buildDrawdown = 30.0
)

func isBuilding(k claimKind) bool {
	return k == infraClaim || k == materialClaim || k == expansionClaim
}

// reserve is the treasury the nation aims to hold: aiTreasuryDays of its
// spending.
func (n *Nation) reserve() float64 { return aiTreasuryDays * n.SpendEMA }

// BuildingBudget is what the nation may spend on building today.
func (n *Nation) BuildingBudget() float64 {
	reserve, k := n.reserve(), 1.0
	if reserve > 0 {
		k = math.Max(0, math.Min(1, (n.Treasury-0.5*reserve)/(0.5*reserve)))
	}
	return k*buildShare*n.IncomeEMA + math.Max(0, n.Treasury-reserve)/buildDrawdown
}

// claim is something a nation buys in a tile this tick.
type claimKind int

const (
	upkeepClaim claimKind = iota
	infraClaim
	materialClaim
	expansionClaim
	armyClaim
)

type claim struct {
	kind claimKind
	good GoodID
	exp  *Expansion
	army *Army
	qty  float64
}

// plan decides what the nation pays for today: everything it needs, which
// is upkeep, infrastructure (construction and materials), state firm
// expansions, its armies' supplies and their pay, and its universities'
// staff. Only when the treasury can't cover it all does everything shrink
// alike.
func (n *Nation) plan(w *World) {
	n.now = w.Tick
	n.Revenue, n.TariffRevenue, n.Spending = 0, 0, 0
	n.SpentOn = [NumSpending]float64{}
	n.pickResearch(w)
	n.pickCopy(w)
	if n.Eliminated() {
		return
	}
	rate := n.MaxBuildRate * n.buildRateMult()
	for _, r := range n.Regions {
		r.claims = r.claims[:0]
	}
	// what a claim will cost: no more than the market offered yesterday can
	// be bought, so an order for a good nobody sells (whose price runs away)
	// doesn't count as spending
	// every order is capped at what the market offered yesterday (and a
	// margin, so demand still shows): the state can't bid a price up without
	// limit, building goes at the pace the economy can supply, and what the
	// orders can cost is known, so the treasury is never overdrawn
	needs := 0.0
	// the nation's average price of each good where it actually traded
	avgPrice, _ := w.NationPrices(n)
	addTo := func(r *Region, c claim) {
		if !w.Made(c.good) {
			return // nobody can make it yet: there is nothing to buy
		}
		m := r.Markets[c.good]
		margin, floor, trendCap := stateBuyMargin, stateBuyFloor, stateTrendPrice
		if c.kind == armyClaim { // the armies press harder: a shortage must drive the price up
			margin, floor, trendCap = armyBuyMargin, armyBuyFloor, armyTrendPrice
		}
		c.qty = math.Min(c.qty, margin*m.Supply+floor)
		// where a good costs far more than elsewhere in the nation, buy less
		if ref := stateDearPrice * avgPrice[c.good]; m.Price > ref && ref > 0 {
			c.qty *= ref / m.Price
		}
		// and where it costs far more than it usually does, spend no more on
		// it: the price still rises and calls up supply, but a shortage
		// can't run the state's spending away (and drain the treasury)
		if ref := trendCap * m.Trend; m.Price > ref && ref > 0 {
			c.qty *= ref / m.Price
		}
		if c.qty > 0 {
			r.claims = append(r.claims, c)
			needs += c.qty * m.Price
		}
	}
	w.planSupply(n, addTo)
	n.payBill = w.soldiersPay(n)
	n.pensionBill = n.pensions()
	needs += n.payBill + n.serviceBill() + n.pensionBill
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
				add(claim{kind: expansionClaim, good: Construction, exp: e, qty: math.Min(rate, e.Cost()-e.Delivered)})
			}
		}
	}
	// building is paid from its budget
	building := 0.0
	for _, r := range n.Regions {
		for _, c := range r.claims {
			if isBuilding(c.kind) {
				building += c.qty * r.Markets[c.good].Price
			}
		}
	}
	if budget := n.BuildingBudget(); building > budget {
		k := budget / building
		for _, r := range n.Regions {
			for i := range r.claims {
				if isBuilding(r.claims[i].kind) {
					r.claims[i].qty *= k
				}
			}
		}
		needs -= building * (1 - k)
	}
	// the treasury caps the total
	n.payScale = 1.0
	if budget := 0.9 * math.Max(n.Treasury, 0); needs > budget {
		n.payScale = budget / needs
	}
	n.staffServices(n.payScale)
	for _, r := range n.Regions {
		for g := range r.publicBid {
			r.publicBid[g], r.publicGot[g] = 0, 0
		}
		for i := range r.claims {
			c := &r.claims[i]
			c.qty *= n.payScale
			r.publicBid[c.good] += c.qty
		}
	}
}

// NationPrices is the nation's price index for each good: the average price
// across its tiles, weighted by what changed hands at each (a price nobody
// pays doesn't count), with the amount traded a day. A good that didn't
// trade anywhere in the nation has price 0.
func (w *World) NationPrices(n *Nation) (price, traded []float64) {
	price, traded = make([]float64, NumGoods), make([]float64, NumGoods)
	for _, r := range n.Regions {
		for g, m := range r.Markets {
			v := math.Min(m.Supply, m.Demand)
			p := m.Price
			if w.TradePrices != nil {
				p = w.TradePrices[r.ID][g] // what it traded at today
			}
			price[g] += p * v
			traded[g] += v
		}
	}
	for g := range price {
		if traded[g] > 0 {
			price[g] /= traded[g]
		}
	}
	return price, traded
}

// The state buys up to stateBuyMargin times what a market offered yesterday,
// plus stateBuyFloor.
const (
	stateBuyMargin = 1.2
	stateBuyFloor  = 0.2
	stateDearPrice = 2.0 // above this times the nation's average price, the state buys less
	// above this times a market's long-run price (Market.Trend), the state
	// spends no more on it
	stateTrendPrice = 2.0
	// For its armies' supplies the state bids up to armyBuyMargin times what
	// was offered (plus armyBuyFloor): a shortage shows as a big excess
	// demand, which drives the price up until supply answers.
	armyBuyMargin = 3.0
	armyBuyFloor  = 1.0
	// The armies keep buying in full until a good costs armyTrendPrice times
	// its long-run price: the state pays what it takes, so the price climbs
	// at full speed and calls up supply, but a runaway shortage still can't
	// drain the treasury.
	armyTrendPrice = 10.0
)

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
			n.SpentOn[spendingOf(c.kind)] += got * w.TradePrices[r.ID][c.good]
			switch c.kind {
			case upkeepClaim:
				upkeep += got
			case infraClaim:
				r.Project.Delivered += got
			case materialClaim:
				r.Project.Materials[c.good] += got
			case expansionClaim:
				c.exp.Delivered += got
			case armyClaim:
				c.army.spent[c.good] += got * w.TradePrices[r.ID][c.good]
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
	n.settleServices()
	w.paySoldiers(n)
	n.payPensions()
	spent := 0.0
	for _, x := range n.SpentOn {
		spent += x
	}
	if n.SpendEMA == 0 {
		n.SpendEMA = spent
	}
	n.SpendEMA = 0.95*n.SpendEMA + 0.05*spent
	income := n.Revenue + n.TariffRevenue
	if n.IncomeEMA == 0 {
		n.IncomeEMA = income
	}
	n.IncomeEMA = 0.95*n.IncomeEMA + 0.05*income
	learned := false
	if i := n.Copying; i >= 0 && n.CopyPoints >= n.TechCost(i, w) {
		n.CopyPoints -= n.TechCost(i, w)
		n.learn(w, i)
		w.emit(n.ID, "tech", -1, "%s adopts %s from abroad", n.Name, Techs[i].Name)
		learned = true
	}
	if i := n.Researching; i >= 0 && n.ResearchPoints >= n.TechCost(i, w) {
		n.ResearchPoints -= n.TechCost(i, w)
		n.learn(w, i)
		queued := false
		for _, q := range n.Queue {
			queued = queued || n.CanResearch(q)
		}
		n.pickResearch(w)
		n.ChooseResearch = !n.AI && !queued && n.Researching >= 0
		w.emit(n.ID, "tech", -1, "%s discovers %s", n.Name, Techs[i].Name)
		learned = true
	}
	if learned {
		n.pickCopy(w)
		n.applyTech()
		hopsChanged = true
	} else if n.adopting() { // bonuses still coming in
		n.applyTech()
	}
	return hopsChanged
}

// learn makes tech i known; if the nation was researching or catching up
// on it, that slot is free again (its points carry over).
func (n *Nation) learn(w *World, i int) {
	n.Known[i] = true
	n.LearnedAt[i] = max(1, w.Tick)
	if n.Researching == i {
		n.Researching = -1
	}
	if n.Copying == i {
		n.Copying = -1
	}
}

// pickCopy chooses the tech the nation catches up on, alongside its own
// research: the cheapest it can research that a neighbour it isn't
// embargoed against (either way) already knows (not the one it researches
// itself). Its
// universities' work goes to both: a follower adopts what exists abroad
// through engineers and emigrants, journals and imported machines while it
// still works on its own, and what others know costs it less (the
// diffusion discount). That is the advantage of backwardness
// (Gerschenkron): Germany, the United States and Japan caught Britain up
// in decades. A leader has nothing to copy and gains nothing from it.
func (n *Nation) pickCopy(w *World) {
	if i := n.Copying; i >= 0 && n.CanResearch(i) && i != n.Researching && w.knownNearby(n, i) {
		return // still on it, and the border it comes across is still open
	}
	n.Copying = -1
	best := 0.0
	for i := range Techs {
		if i == n.Researching || !n.CanResearch(i) || !w.knownNearby(n, i) {
			continue
		}
		if c := n.TechCost(i, w); n.Copying < 0 || c < best {
			n.Copying, best = i, c
		}
	}
	if n.Copying < 0 {
		n.CopyPoints = 0 // nothing to catch up on
	}
}

// knownNearby reports whether a neighbour of n knows tech i and the border
// between them is open (closedTo): an embargo, either way, stops the
// engineers, machines and journals a tech crosses with.
func (w *World) knownNearby(n *Nation, i int) bool {
	for _, o := range w.Nations {
		if o != n && !o.Eliminated() && o.Known[i] && !n.closedTo(o) && w.nationsBorder(n, o) {
			return true
		}
	}
	return false
}

// closedTo reports whether no goods, and so no ideas, cross between n and
// o: an embargo either way, a war, or an alliance of one against the other
// (which embargoes it).
func (n *Nation) closedTo(o *Nation) bool {
	return n.Embargo[o.ID] || o.Embargo[n.ID] || n.AtWar[o.ID] ||
		n.Coalition == o.ID+1 || o.Coalition == n.ID+1
}

// pickResearch chooses what to research when the nation isn't researching
// anything: the first available tech in its queue, else the cheapest
// available one (ties go to the earlier one).
func (n *Nation) pickResearch(w *World) {
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
	if n.AI {
		n.Researching = n.drawResearch(w)
		return
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
		if n.CanResearch(i) && i != n.Copying && (n.Researching < 0 || better(i, n.Researching)) {
			n.Researching = i
		}
	}
}

// applyTech brings the nation's firms up to its technology.
func (n *Nation) applyTech() {
	for _, r := range n.Regions {
		for _, f := range r.Firms {
			f.Enabled = firmEnabled(f, n)
			f.applyMethods(n)
		}
	}
}

// neglectLimit is how many ticks' worth of unpaid upkeep (decaying by 2% a
// tick) a tile's infrastructure tolerates before it drops a level: about 60
// ticks with no upkeep at all, or a long spell below 30% of it.
const neglectLimit = 35
