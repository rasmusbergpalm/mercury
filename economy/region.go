package economy

import "math"

// Region is a map tile with its own pops, firms, labor market and goods markets.
type Region struct {
	ID        int
	Name      string
	Row, Col  int
	Terrain   string
	Coastal   bool // on the map's edge, by the ocean
	Firms     []*Firm
	made      []bool // shared with the world (World.refreshMade)
	Pops      []*Pop
	Markets   []*Market // indexed by good
	Wage      float64
	WageSpeed float64

	BaseMove float64  // terrain's movement cost
	Real     GDPChain // real GDP, linked monthly
	warm     bool     // past the real GDP chain's warm-up (set at each link)

	UtilityEMA float64 // smoothed utility per person; migration follows it

	// The levers (see levers.go), and the accounts.
	IncomeTax         float64  // the share withheld from every income paid here
	Withheld          float64  // taxes withheld, until someone takes them
	Duties            float64  // import duties collected here, until someone takes them
	Buyers            []*Buyer // outside buyers in its markets
	PaybackMult       float64  // scales the payback owners ask of a new level (1: as is)
	GrowthBonus       float64  // added to its people's growth a year, beyond their natural growth
	LandTransportMult float64  // scales the transport it takes to cross the tile over land (1: its terrain's)
	SeaTransportMult  float64  // scales the transport it takes to pass it by sea (1: CoastMove)
	SpoilageMult      float64  // scales the spoilage of goods shipped from it (1: as is)

	House    *TradingHouse // the tile's merchants
	outbound [][]*Trader   // routes buying here, by good (transport: all of them)
	bidders  [][]*Trader   // those of them that may bid today (see listBidders)
	inbound  [][]*Trader   // traders selling here, by good

	soldHere []*Trader // routes that sold here today: their houses are owed (see settleHouses)

	prices, demandBuf []float64 // scratch
	// firms by the goods they may buy and the good they make (built once)
	firmsBuying, firmsMaking [][]int
	mkt                      marketScratch
	popDemand                [][]float64
}

// NewRegion makes a region with n identical pops.
func NewRegion(id int, name string, firms []*Firm, nPops int, popSize, popCash float64) *Region {
	r := &Region{ID: id, Name: name, Firms: firms, Wage: 1, WageSpeed: 0.1, BaseMove: 1, LandTransportMult: 1,
		outbound: make([][]*Trader, NumGoods), inbound: make([][]*Trader, NumGoods), bidders: make([][]*Trader, NumGoods),
		prices: make([]float64, NumGoods), PaybackMult: 1, SeaTransportMult: 1, SpoilageMult: 1}
	for i := 0; i < nPops; i++ {
		r.Pops = append(r.Pops, NewPop(popSize, popCash))
		r.popDemand = append(r.popDemand, make([]float64, NumGoods))
	}
	for g := range Goods {
		r.Markets = append(r.Markets, &Market{Good: GoodID(g), Price: 1, Speed: 0.1, MinVolume: 0.1})
	}
	r.House = &TradingHouse{Region: r}
	return r
}

// connect hands each route to the trading house of the tile it leaves from
// and lists it with the markets it buys and sells in.
func connect(traders []*Trader) {
	for _, t := range traders {
		r := t.Origin
		t.House = r.House
		t.slot = len(r.House.Routes)
		r.House.Routes = append(r.House.Routes, t)
		r.outbound[t.Good] = append(r.outbound[t.Good], t)
		r.outbound[Transport] = append(r.outbound[Transport], t)
		t.Dest.inbound[t.Good] = append(t.Dest.inbound[t.Good], t)
	}
}

// Prices returns the current price of every good (a fresh slice).
func (r *Region) Prices() []float64 {
	out := make([]float64, NumGoods)
	for g, m := range r.Markets {
		out[g] = m.Price
	}
	return out
}

func (r *Region) refreshPrices() []float64 {
	for g, m := range r.Markets {
		r.prices[g] = m.Price
	}
	return r.prices
}

// Plan lets every firm plan at current prices.
func (r *Region) Plan() {
	for _, p := range r.Pops {
		p.Income = 0
	}
	r.UpdateAgglomeration()
	prices := r.refreshPrices()
	for _, f := range r.Firms {
		f.TickRevenue, f.TickCost, f.tickInputs, f.investWant = 0, 0, 0, 0
		if len(f.Methods) > 1 {
			for _, m := range f.Methods {
				m.checkInputs(r.made)
			}
		}
		f.Plan(prices, r.Wage)
	}
}

// Investment rules (when World.Investment is on).
const (
	// a new level must repay its cost in ten years of profit: a return of
	// 10% a year, about what 19th-century firms asked of new works
	investPayback = 10 * TicksPerYear
	investMinAge  = 100 // ticks before a firm's profit record counts
	buildDays     = 90  // a private level (or a refit) takes this long to build, at the quickest
	disinvestAt   = 100 // ticks of losses before a firm closes a level
	// capital wears out: each year 2% of a firm's levels must be made good
	// with construction (its upkeep, a cost like any other), or the firm
	// loses them
	depreciation = 0.02
)

// investDecideDays is how long owners take, on average, to start a project
// that pays twice what they ask: each day a project that pays starts with a
// chance in proportion to how far it beats their hurdle, so the clear winners
// are taken up within months and the marginal ones slowly, owner by owner,
// rather than every owner on the map the same day.
//
// The chance is drawn once a project, not once a day: each owner has a
// resolve for its next project (an exponential draw keyed by the world's
// seed, the tile, the firm and how many projects it has started), and each
// day the project pays its urge grows by that day's chance; it starts when
// the urge reaches the resolve. The odds are a daily draw's, but a change
// that moves a project by a day moves only that project: no later draw is
// re-rolled, so two versions of the economy stay comparable.
const investDecideDays = 180.0

// refitDecideDays is the same for refitting a working level to another
// method: owners are slower to tear out machinery that still runs than to
// build anew, so a better method spreads over years, works one by one.
const refitDecideDays = 365.0

// planInvestment starts private expansions where a level would repay itself,
// and sets what each firm buys this tick: its upkeep, and the next part of
// its expansion while what is left of it still pays at today's construction
// price (when building costs rise, projects wait, as real ones did).
func (r *Region) planInvestment(seed int64) {
	price := r.Markets[Construction].Price
	payback := float64(investPayback)
	payback *= r.PaybackMult
	for i, f := range r.Firms {
		f.upkeepWant = 0
		if f.Levels < 1 {
			continue
		}
		perLevel := f.ProfitEMA / float64(f.Levels)
		// the method a level would earn most on, and the one whose levels
		// earn least: refitting one of the second to the first adds the
		// difference
		best, bestProfit := f.BestMethod(r.prices, r.Wage)
		if f.Monopoly && best >= 0 {
			// a monopoly builds on what a level more adds to its profit
			// (at its marginal revenue), not on the markup its levels earn
			// on average: more capacity would lower its price
			perLevel = math.Min(perLevel, bestProfit)
		}
		worst, worstProfit := f.WorstMethod(r.prices, r.Wage)
		refitGain := 0.0
		if best >= 0 && worst >= 0 && best != worst {
			refitGain = bestProfit - worstProfit
		}
		if f.Expansion == nil && f.Enabled && f.Age > investMinAge && best >= 0 {
			full := f.AtCap()
			// how many times over each project repays its cost within the payback
			refit := refitGain * payback / (refitShare * LevelCost * price)
			build := perLevel * payback / (LevelCost * price)
			var refitUrge, buildUrge float64
			if refit > 1 {
				refitUrge = math.Min(1, refit-1) / refitDecideDays
			}
			if !full && build > 1 {
				buildUrge = math.Min(1, build-1) / investDecideDays
			}
			f.Urge += refitUrge + buildUrge
			// the urge built up over the days a project paid; it is acted on
			// only on a day one still does, for the one that pays better then
			if refitUrge+buildUrge > 0 && f.Urge >= investResolve(seed, r.ID, i, f.Projects) {
				f.Urge = 0
				f.Projects++
				if refitUrge >= buildUrge {
					f.Expansion = &Expansion{Refit: true, Method: best, From: worst}
				} else {
					f.Expansion = &Expansion{Method: best}
				}
			}
		}
		upkeep := 0.0
		if f.Enabled && f.ProfitEMA > 0 { // capital that loses money is left to wear out
			upkeep = depreciation * float64(f.Levels) * LevelCost / TicksPerYear
		}
		want := upkeep
		if e := f.Expansion; e != nil && !e.External {
			gain := perLevel
			if e.Refit { // what it still adds at today's prices
				gain = f.LevelProfit(e.Method, r.prices, r.Wage) - f.LevelProfit(e.From, r.prices, r.Wage)
			}
			if gain*payback >= (e.Cost()-e.Delivered)*price {
				want += math.Min(e.Cost()/buildDays, e.Cost()-e.Delivered) // a refit takes as long to put in
			}
		}
		if want <= 0 {
			continue
		}
		f.DesiredSpend += want * price
		planned := f.PlanLabor * r.Wage
		for g, q := range f.PlanQty {
			planned += q * r.prices[g]
		}
		f.investWant = math.Max(0, math.Min(want, (0.9*f.Cash-planned)/price))
		f.upkeepWant = math.Min(upkeep, f.investWant)
	}
}

// settleFirms updates profit records, finishes expansions and closes levels
// that keep losing money.
func (r *Region) settleFirms() {
	for _, f := range r.Firms {
		f.Age++
		f.ProfitEMA = 0.95*f.ProfitEMA + 0.05*(f.TickRevenue-f.TickCost)
		if f.Levels > 1 { // wear, less today's upkeep (made good at the market)
			f.Wear += depreciation * float64(f.Levels) / TicksPerYear
			if f.Wear >= 1 {
				f.dropLevel(r)
				f.Wear--
			}
		}
		if e := f.Expansion; e != nil && e.Delivered >= e.Cost()-1e-9 {
			f.Expansion = nil
			if e.Refit {
				f.refit(e.From, e.Method)
			} else {
				f.addLevel(e.Method)
			}
			if c := f.landCap(); c > 0 && f.Levels > c {
				f.dropLevel(r) // on full land the new method takes over the old one's
			}
		}
		if f.ProfitEMA < 0 && f.Levels > 1 {
			f.LossTicks++
			if f.LossTicks > disinvestAt {
				f.dropLevel(r)
				f.LossTicks = 0
			}
		} else {
			f.LossTicks = 0
		}
	}
}

// LaborMarket lets firms hire at the going wage. If they want more labor than
// exists, each gets the same fraction. Pops fill the jobs in turn (a pop's
// labor can be split across firms), and get paid by the firm that employs them.
func (r *Region) LaborMarket() {
	// a firm only claims the workers it can pay the full wage; the rest look
	// for work at firms that can. Outside employers (Buyer.Hire) hire after
	// the firms, and pay from their account.
	totalD := 0.0
	for _, f := range r.Firms {
		if r.Wage > 0 {
			f.PlanLabor = math.Min(f.PlanLabor, math.Max(f.Cash, 0)/r.Wage)
		}
		totalD += f.PlanLabor
	}
	for _, b := range r.Buyers {
		totalD += b.Hire
	}
	workers := r.ClassPops(Workers)
	supply := 0.0
	for _, p := range workers {
		supply += p.Size
	}
	scale := 0.0
	if totalD > 0 {
		scale = math.Min(1, supply/totalD)
	}
	for _, p := range r.Pops {
		p.Jobs = p.Jobs[:0]
	}
	pi := 0
	free := workers[0].Size
	// hire fills employer e's jobs (a firm's index, or len(Firms) on for a
	// buyer), paying from cash
	hire := func(e int, need float64, cash *float64) {
		wage := r.Wage
		for need > 1e-12 && pi < len(workers) {
			pop := workers[pi]
			take := math.Min(need, free)
			if n := len(pop.Jobs); n > 0 && pop.Jobs[n-1].Firm == e {
				pop.Jobs[n-1].Amount += take
			} else {
				pop.Jobs = append(pop.Jobs, Job{e, take})
			}
			r.PayIncome(pop, take*wage)
			*cash -= take * wage
			need -= take
			free -= take
			if free <= 1e-12 {
				pi++
				if pi < len(workers) {
					free = workers[pi].Size
				} else {
					free = 0
				}
			}
		}
	}
	for fi, f := range r.Firms {
		f.Labor = f.PlanLabor * scale
		cash0 := f.Cash
		hire(fi, f.Labor, &f.Cash)
		f.TickCost += cash0 - f.Cash
	}
	for bi, b := range r.Buyers {
		b.Hired = b.Hire * scale
		hire(len(r.Firms)+bi, b.Hired, &b.Account)
	}
	excess := (totalD - supply) / supply
	r.Wage = math.Max(MinPrice, math.Min(MaxPrice, r.Wage*math.Exp(r.WageSpeed*clip(excess)))) // bounded, like prices
}

type agentKind int

const (
	popAgent agentKind = iota
	firmAgent
	traderAgent
	buyerAgent
	investAgent
)

type agentRef struct {
	kind agentKind
	idx  int
}

// listBidders lists, per good, the routes leaving the tile that may place
// an order today: those buying (Active) or with stock waiting for transport.
// The rest (most routes, on a big map) bid nothing, so the markets skip them.
func (r *Region) listBidders(planned []*Trader) {
	if r.bidders == nil {
		r.bidders = make([][]*Trader, NumGoods)
	}
	for g := range r.bidders {
		r.bidders[g] = r.bidders[g][:0]
	}
	// routes are in the house's order, as each good's outbound list is
	for _, t := range planned {
		if t.Active || t.StockOrigin > 0 {
			r.bidders[t.Good] = append(r.bidders[t.Good], t)
			r.bidders[Transport] = append(r.bidders[Transport], t)
		}
	}
}

// marketScratch are a tile's reusable buffers for clearing its markets.
type marketScratch struct {
	bids, asks         []Order
	bidRefs, askRefs   []agentRef
	bidFills, askFills []float64
}

// indexFirms lists, per good, the firms that may buy it (it is one of their
// inputs) and those that make it, in firm order.
func (r *Region) indexFirms() {
	r.firmsBuying, r.firmsMaking = make([][]int, NumGoods), make([][]int, NumGoods)
	for i, f := range r.Firms {
		r.firmsMaking[f.Output] = append(r.firmsMaking[f.Output], i)
		for _, fc := range f.factors() {
			switch fc.Kind {
			case GoodFactor:
				r.addBuyer(fc.Good, i)
			case CompositeFactor:
				for _, g := range Composites[fc.Composite].Goods {
					r.addBuyer(g, i)
				}
			}
		}
	}
}

func (r *Region) addBuyer(g GoodID, firm int) {
	if l := r.firmsBuying[g]; len(l) == 0 || l[len(l)-1] != firm {
		r.firmsBuying[g] = append(l, firm)
	}
}

// GoodsMarkets clears every goods market. Buyers: pops, firms (inputs),
// traders leaving from here. Sellers: local firms, traders arriving here.
func (r *Region) GoodsMarkets() {
	prices := r.refreshPrices()
	var plan demandPlan // the same prices for every pop here
	plan.prepare(prices)
	for i, p := range r.Pops {
		p.demand(&plan, r.popDemand[i])
		for g := range p.Consumed {
			p.Consumed[g] = 0
		}
	}
	if r.firmsBuying == nil {
		r.indexFirms()
	}
	sc := &r.mkt
	bids, asks, bidRefs, askRefs := sc.bids, sc.asks, sc.bidRefs, sc.askRefs
	defer func() { sc.bids, sc.asks, sc.bidRefs, sc.askRefs = bids, asks, bidRefs, askRefs }()
	for g, market := range r.Markets {
		gid := GoodID(g)
		if r.made != nil && !r.made[g] { // not in play yet (World.refreshMade)
			market.Supply, market.Demand = 0, 0
			continue
		}
		bids, asks, bidRefs, askRefs = bids[:0], asks[:0], bidRefs[:0], askRefs[:0]
		for i, p := range r.Pops {
			if q := r.popDemand[i][g]; q > 0 {
				bids = append(bids, Order{&p.Cash, q})
				bidRefs = append(bidRefs, agentRef{popAgent, i})
			}
		}
		for _, i := range r.firmsBuying[g] {
			if f := r.Firms[i]; f.PlanHas[g] {
				bids = append(bids, Order{&f.Cash, f.PlanQty[g]})
				bidRefs = append(bidRefs, agentRef{firmAgent, i})
			}
		}
		for i, t := range r.bidders[g] {
			if q := t.Bid(gid); q > 0 {
				bids = append(bids, Order{&t.House.Cash, q})
				bidRefs = append(bidRefs, agentRef{traderAgent, i})
			}
		}
		if gid == Construction {
			for i, f := range r.Firms {
				if f.investWant > 0 {
					bids = append(bids, Order{&f.Cash, f.investWant})
					bidRefs = append(bidRefs, agentRef{investAgent, i})
				}
			}
		}
		for i, b := range r.Buyers {
			if b.Bid[g] > 0 {
				bids = append(bids, Order{&b.Account, b.Bid[g]})
				bidRefs = append(bidRefs, agentRef{buyerAgent, i})
			}
		}
		power := false // a monopoly in this market notes its share of it
		for _, i := range r.firmsMaking[g] {
			if f := r.Firms[i]; f.Inventory > 0 {
				asks = append(asks, Order{&f.Cash, f.Inventory})
				askRefs = append(askRefs, agentRef{firmAgent, i})
			}
			power = power || r.Firms[i].Monopoly
		}
		for i, t := range r.inbound[g] {
			if t.StockDest > 0 { // the house is another tile's: it is paid later
				asks = append(asks, Order{&t.proceeds, t.StockDest})
				askRefs = append(askRefs, agentRef{traderAgent, i})
			}
		}

		if power {
			offered := 0.0
			for _, a := range asks {
				offered += a.Qty
			}
			for _, i := range r.firmsMaking[g] {
				if f := r.Firms[i]; f.Monopoly {
					f.share = 0
					if offered > 0 {
						f.share = f.Inventory / offered
					}
				}
			}
		}
		price := market.Price // what trade happens at; Clear then moves the price
		var bidFills, askFills []float64
		bidFills, askFills = market.clearInto(bids, asks, sc.bidFills, sc.askFills)
		sc.bidFills, sc.askFills = bidFills, askFills

		for i, ref := range bidRefs {
			got := bidFills[i]
			switch ref.kind {
			case popAgent:
				r.Pops[ref.idx].Consumed[g] = got
			case firmAgent:
				r.Firms[ref.idx].Inputs[g] += got
				r.Firms[ref.idx].TickCost += got * price
				r.Firms[ref.idx].tickInputs += got * price
			case traderAgent:
				t := r.bidders[g][ref.idx]
				t.Receive(gid, got)
				t.House.Spent += got * price
			case buyerAgent:
				r.Buyers[ref.idx].Got[g] = got
			case investAgent: // upkeep first, then the expansion
				f := r.Firms[ref.idx]
				upkeep := math.Min(got, f.upkeepWant)
				f.Wear = math.Max(0, f.Wear-upkeep/LevelCost)
				f.TickCost += upkeep * price
				if e := f.Expansion; e != nil && !e.External {
					e.Delivered += got - upkeep
				}
			}
		}
		for i, ref := range askRefs {
			sold := askFills[i]
			if ref.kind == firmAgent {
				r.Firms[ref.idx].Inventory -= sold
				r.Firms[ref.idx].TickRevenue += sold * price
			} else {
				t := r.inbound[g][ref.idx]
				t.StockDest -= sold
				r.soldHere = append(r.soldHere, t)
				if rate, _ := t.Border(); rate > 0 { // import tariff, paid on sale
					duty := sold * price * rate
					t.proceeds -= duty
					t.duty += duty
					r.Duties += duty
				}
			}
		}
	}
	r.capUnsuppliedPrices()
}

// UnsuppliedPriceCap is how far above the tile's typical price (the median
// price of the goods it has sellers for) a good nobody sells there may climb,
// and how far below it a good nobody buys there may fall. A shortage still
// pulls in imports and producers, but a good no one can make yet doesn't
// build up a price that means nothing, which the first producer and
// merchants would then take at face value. Likewise a good no one wants for
// now (war materiel in peacetime) doesn't sink to nothing, and needn't take
// months to climb back once buyers return.
const UnsuppliedPriceCap = 10.0

// capUnsuppliedPrices holds the price of goods with no supply here to at most
// UnsuppliedPriceCap times the tile's typical price, and of goods with no
// demand to speak of (below the market's MinVolume: merchants keep a
// trickle of bids open on every route) to at least that fraction of it.
func (r *Region) capUnsuppliedPrices() {
	var sold []float64
	for _, m := range r.Markets {
		if m.Supply > 0 {
			sold = append(sold, m.Price)
		}
	}
	if len(sold) == 0 {
		return
	}
	typical := median(sold)
	high, low := UnsuppliedPriceCap*typical, typical/UnsuppliedPriceCap
	for g, m := range r.Markets {
		if r.made != nil && !r.made[g] {
			continue // not in play: no price to speak of
		}
		if m.Supply <= 0 && m.Price > high {
			m.Price = high
		}
		if m.Demand < m.MinVolume && m.Price < low { // (merchants' standing bids are a trickle)
			m.Price = low
		}
	}
}

// Produce decays unsold stock, then lets every firm produce.
func (r *Region) Produce() {
	for _, f := range r.Firms {
		f.Inventory *= 1 - Goods[f.Output].Spoilage
		f.Produce()
	}
}

// ClassPops lists the tile's pops of a class.
func (r *Region) ClassPops(c Class) []*Pop {
	var out []*Pop
	for _, p := range r.Pops {
		if p.Class == c {
			out = append(out, p)
		}
	}
	return out
}

// owners are the pops that own the tile's firms and trading house. Where
// there is no owner class everyone owns a share.
func (r *Region) owners() []*Pop {
	if o := r.ClassPops(Owners); len(o) > 0 {
		return o
	}
	return r.Pops
}

// PayDividends settles a firm or trader with its owners. Cash is pulled
// toward a target that covers what the business wants to spend: excess is
// paid out as dividends, a shortfall is met by the owners investing (at most
// 10% of their cash per tick). Money is conserved.
func (r *Region) PayDividends(cash *float64, cashBuffer, desiredSpend float64, owners []*Pop) {
	const rate, maxCall = 0.5, 0.1
	target := cashBuffer + 1.2*desiredSpend
	div := (*cash - target) * rate
	ownCash, ownSize := 0.0, 0.0
	for _, p := range owners {
		ownCash += p.Cash
		ownSize += p.Size
	}
	if div < 0 {
		div = math.Max(div, -maxCall*ownCash)
	}
	*cash -= div
	for _, p := range owners {
		if div < 0 {
			p.Cash += div * (p.Cash / ownCash) // investment is not taxed
		} else {
			r.PayIncome(p, div*(p.Size/ownSize))
		}
	}
}

// Agglomeration: on every tile, industry and services are more productive the
// more people live there: (people / SizeBonusPop)^SizeBonusExp. Busy tiles pay
// better, draw migrants and grow further, so cities emerge where trade and
// resources concentrate people; thin tiles stay rural. The pull is mild (a
// tile twice the size is 9% more productive): at 0.45 (37%) a tile that
// grew drew people from its neighbours, which made it grow further, and
// small tiles emptied to the migration floor.
const (
	SizeBonusPop = 100.0 // a starting tile's people: bonus 1
	SizeBonusExp = 0.12
	// a tile is a city when it has this many times the average tile's people
	CityMult = 2.0
)

// SizeBonus is the productivity multiplier the tile's size gives its
// non-primary firms.
func (r *Region) SizeBonus() float64 {
	return math.Pow(math.Max(r.Population(), 1)/SizeBonusPop, SizeBonusExp)
}

// UpdateAgglomeration sets what the tile's size does to its industry and
// services.
func (r *Region) UpdateAgglomeration() {
	b := r.SizeBonus()
	for _, f := range r.Firms {
		if f.Sector != Primary {
			f.Agglom = b
		}
	}
}

// Population growth per year: well-fed people multiply, hungry ones don't.
// Natural growth is PopGrowthSlope * (basic needs met - PopGrowthNeutral),
// within [PopGrowthMin, PopGrowthMax]: about 1.4% a year where people eat
// well, as in 1820. Anything else that moves births and deaths (medicine,
// public health, schooling) comes from outside as GrowthBonus.
const (
	PopGrowthSlope   = 0.035
	PopGrowthNeutral = 0.5
	PopGrowthMin     = -0.03
	PopGrowthMax     = 0.02
)

// BasicNeedsMet is how far the tile's people satisfy their basic needs, on
// average: Q/(Q+r) per basic need group (0 = nothing, 1 = plenty).
func (r *Region) BasicNeedsMet() float64 {
	s, n := 0.0, 0
	for _, nd := range Needs {
		if nd.Tier != Basic {
			continue
		}
		pop := r.Population()
		q := 0.0
		for _, p := range r.Pops {
			q += Aggregate(p.Consumed, nd.Goods, nd.Sigma)
		}
		q /= pop
		s += q / (q + nd.Requirement)
		n++
	}
	return s / float64(n)
}

// NaturalGrowth is the tile's population growth per year at current
// consumption before its limits: what eating well or badly does.
func (r *Region) NaturalGrowth() float64 {
	return PopGrowthSlope * (r.BasicNeedsMet() - PopGrowthNeutral)
}

// GrowthRate is the tile's population growth per year: its natural growth
// within its limits, and GrowthBonus.
func (r *Region) GrowthRate() float64 {
	return math.Max(PopGrowthMin, math.Min(PopGrowthMax, r.NaturalGrowth())) + r.GrowthBonus
}

// grow applies a day of population growth. Newborns bring no cash.
func (r *Region) grow() {
	f := 1 + r.GrowthRate()/TicksPerYear
	for _, p := range r.Pops {
		p.Size *= f
	}
}

// MoveCost is the transport it takes to move through the tile: over land,
// its terrain's cost times LandTransportMult; coastal tiles can also be
// passed by sea, at CoastMove times SeaTransportMult; the cheaper way
// counts.
func (r *Region) MoveCost() float64 {
	land := r.BaseMove * r.LandTransportMult
	if !r.Coastal {
		return land
	}
	sea := CoastMove
	sea *= r.SeaTransportMult
	return math.Min(sea, land)
}

// Money is the cash held by the region's pops and firms.
func (r *Region) Money() float64 {
	s := 0.0
	for _, p := range r.Pops {
		s += p.Cash
	}
	for _, f := range r.Firms {
		s += f.Cash
	}
	return s
}

// Population is the number of people.
func (r *Region) Population() float64 {
	s := 0.0
	for _, p := range r.Pops {
		s += p.Size
	}
	return s
}

// Utility is average utility per person.
func (r *Region) Utility() float64 {
	s := 0.0
	for _, p := range r.Pops {
		s += p.Utility() * p.Size
	}
	return s / r.Population()
}

// TierSpend is what the pops spent on each tier this tick, at the given prices.
func (r *Region) TierSpend(prices []float64) [NumTiers]float64 {
	var out [NumTiers]float64
	for _, p := range r.Pops {
		for g, q := range p.Consumed {
			if t := TierOf[g]; t >= 0 && q > 0 {
				out[t] += q * prices[g]
			}
		}
	}
	return out
}

// investResolve is a firm's resolve for its next project: an exponential
// draw (mean 1), deterministic in the world's seed, the tile, the firm and
// how many projects the firm has started.
func investResolve(seed int64, region, firm, projects int) float64 {
	x := uint64(seed)*0x9E3779B97F4A7C15 ^ uint64(region+1)*0xBF58476D1CE4E5B9 ^ uint64(firm+1)*0x94D049BB133111EB ^ uint64(projects+1)*0xD6E8FEB86659FD93
	x ^= x >> 31
	x *= 0x9E3779B97F4A7C15
	x ^= x >> 29
	u := (float64(x>>11) + 0.5) / float64(1<<53) // in (0, 1)
	return -math.Log(u)
}
