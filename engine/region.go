package engine

import "math"

// Region is a map tile with its own pops, firms, labor market and goods markets.
type Region struct {
	ID        int
	Name      string
	Row, Col  int
	Terrain   string
	Coastal   bool // on the map's edge, by the ocean
	Firms     []*Firm
	Pops      []*Pop
	Markets   []*Market // indexed by good
	Wage      float64
	WageSpeed float64

	Nation   *Nation  // owner, or nil
	BaseMove float64  // terrain's movement cost
	Infra    int      // index into InfraLevels
	Real     GDPChain // real GDP, linked monthly
	warm     bool     // past the real GDP chain's warm-up (set at each link)
	Port     bool     // has a port: trades by sea with every other port
	Urban    bool     // industry and services gain from the tile's size (game worlds)
	Project  *Project // infrastructure under construction, or nil
	Neglect  float64  // accumulated unmet upkeep
	Siege    *Siege   // a foreign army working to take the tile, or nil
	Collect  float64  // share of its taxes the administration collects
	// Disloyalty is how far the tile's people are from standing by their
	// owner: 0 loyal, 1 just conquered (see Allegiance).
	Disloyalty float64
	Educated   float64 // share of its people who were schooled
	Healthy    float64 // share of its crowding public health covers
	Battle     int     // days the current battle on the tile has lasted (0: none)

	UtilityEMA float64 // smoothed utility per person; migration follows it

	claims               []claim   // what the nation buys here this tick
	publicBid, publicGot []float64 // by good

	House    *TradingHouse // the tile's merchants
	outbound [][]*Trader   // routes buying here, by good (transport: all of them)
	inbound  [][]*Trader   // traders selling here, by good

	prices, demandBuf []float64 // scratch
	popDemand         [][]float64
}

// NewRegion makes a region with n identical pops.
func NewRegion(id int, name string, firms []*Firm, nPops int, popSize, popCash float64) *Region {
	r := &Region{ID: id, Name: name, Firms: firms, Wage: 1, WageSpeed: 0.1, BaseMove: 1,
		outbound: make([][]*Trader, NumGoods), inbound: make([][]*Trader, NumGoods),
		prices: make([]float64, NumGoods), publicBid: make([]float64, NumGoods),
		publicGot: make([]float64, NumGoods), Collect: 1, Healthy: 1}
	for i := 0; i < nPops; i++ {
		r.Pops = append(r.Pops, NewPop(popSize, popCash))
		r.popDemand = append(r.popDemand, make([]float64, NumGoods))
	}
	for g := range Goods {
		r.Markets = append(r.Markets, &Market{Good: GoodID(g), Price: 1, Speed: 0.1, MinVolume: 0.1})
	}
	return r
}

func (r *Region) connect(traders []*Trader) {
	if r.House == nil {
		r.House = &TradingHouse{Region: r}
	}
	for _, t := range traders {
		if t.Origin == r {
			t.House = r.House
			r.House.Routes = append(r.House.Routes, t)
			r.outbound[t.Good] = append(r.outbound[t.Good], t)
			r.outbound[Transport] = append(r.outbound[Transport], t)
		}
		if t.Dest == r {
			r.inbound[t.Good] = append(r.inbound[t.Good], t)
		}
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
	if r.Urban {
		r.updateAgglomeration()
	}
	prices := r.refreshPrices()
	for _, f := range r.Firms {
		f.tickRevenue, f.tickCost, f.tickInputs, f.investWant = 0, 0, 0, 0
		f.Plan(prices, r.Wage)
	}
}

// Investment rules (game worlds only).
const (
	investPayback = 50  // ticks of profit a new level must repay its cost in
	investMinAge  = 100 // ticks before a firm's profit record counts
	investRate    = 3.0 // construction units a private expansion buys per tick at most
	disinvestAt   = 100 // ticks of losses before a firm closes a level
)

// planInvestment starts private expansions where a level would repay itself,
// and sets what expanding firms buy this tick.
func (r *Region) planInvestment() {
	price := r.Markets[Construction].Price
	payback := float64(investPayback)
	if r.Nation != nil {
		payback *= r.Nation.paybackMult()
	}
	for _, f := range r.Firms {
		if f.Expansion == nil && !f.State && f.Enabled && f.Levels < f.MaxLevels && f.Age > investMinAge {
			perLevel := f.ProfitEMA / float64(f.Levels)
			if perLevel > 0 && perLevel*payback > LevelCost*price {
				f.Expansion = &Expansion{}
			}
		}
		if e := f.Expansion; e != nil && !e.Public {
			want := math.Min(investRate, LevelCost-e.Delivered)
			f.DesiredSpend += want * price
			planned := f.PlanLabor * r.Wage
			for g, q := range f.PlanQty {
				planned += q * r.prices[g]
			}
			f.investWant = math.Max(0, math.Min(want, (0.9*f.Cash-planned)/price))
		}
	}
}

// settleFirms updates profit records, finishes expansions and closes levels
// that keep losing money.
func (r *Region) settleFirms() {
	for _, f := range r.Firms {
		f.Age++
		f.ProfitEMA = 0.95*f.ProfitEMA + 0.05*(f.tickRevenue-f.tickCost)
		if e := f.Expansion; e != nil && e.Delivered >= LevelCost-1e-9 {
			f.Levels++
			f.Expansion = nil
			if f.State {
				f.Funded++ // a new university level is funded once built
			}
		}
		if f.State {
			continue // the state decides about its universities
		}
		if f.ProfitEMA < 0 && f.Levels > 1 {
			f.LossTicks++
			if f.LossTicks > disinvestAt {
				f.Levels--
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
	// for work at firms that can
	totalD := 0.0
	for _, f := range r.Firms {
		if r.Wage > 0 {
			f.PlanLabor = math.Min(f.PlanLabor, math.Max(f.Cash, 0)/r.Wage)
		}
		totalD += f.PlanLabor
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
	for fi, f := range r.Firms {
		f.Labor = f.PlanLabor * scale
		wage := r.Wage
		need := f.Labor
		for need > 1e-12 && pi < len(workers) {
			pop := workers[pi]
			take := math.Min(need, free)
			if n := len(pop.Jobs); n > 0 && pop.Jobs[n-1].Firm == fi {
				pop.Jobs[n-1].Amount += take
			} else {
				pop.Jobs = append(pop.Jobs, Job{fi, take})
			}
			r.payIncome(pop, take*wage)
			f.Cash -= take * wage
			f.tickCost += take * wage
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
	excess := (totalD - supply) / supply
	r.Wage = math.Max(minPrice, math.Min(maxPrice, r.Wage*math.Exp(r.WageSpeed*clip(excess)))) // bounded, like prices
}

type agentKind int

const (
	popAgent agentKind = iota
	firmAgent
	traderAgent
	nationAgent
	investAgent
)

type agentRef struct {
	kind agentKind
	idx  int
}

// GoodsMarkets clears every goods market. Buyers: pops, firms (inputs),
// traders leaving from here. Sellers: local firms, traders arriving here.
func (r *Region) GoodsMarkets() {
	prices := r.refreshPrices()
	for i, p := range r.Pops {
		p.Demand(prices, r.popDemand[i])
		for g := range p.Consumed {
			p.Consumed[g] = 0
		}
	}
	var bids, asks []Order
	var bidRefs, askRefs []agentRef
	for g, market := range r.Markets {
		gid := GoodID(g)
		bids, asks, bidRefs, askRefs = bids[:0], asks[:0], bidRefs[:0], askRefs[:0]
		for i, p := range r.Pops {
			if q := r.popDemand[i][g]; q > 0 {
				bids = append(bids, Order{&p.Cash, q})
				bidRefs = append(bidRefs, agentRef{popAgent, i})
			}
		}
		for i, f := range r.Firms {
			if f.PlanHas[g] {
				bids = append(bids, Order{&f.Cash, f.PlanQty[g]})
				bidRefs = append(bidRefs, agentRef{firmAgent, i})
			}
		}
		for i, t := range r.outbound[g] {
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
		if r.Nation != nil && r.publicBid[g] > 0 {
			bids = append(bids, Order{&r.Nation.Treasury, r.publicBid[g]})
			bidRefs = append(bidRefs, agentRef{nationAgent, 0})
		}
		for i, f := range r.Firms {
			if f.Output == gid && f.Inventory > 0 {
				asks = append(asks, Order{&f.Cash, f.Inventory})
				askRefs = append(askRefs, agentRef{firmAgent, i})
			}
		}
		for i, t := range r.inbound[g] {
			if t.StockDest > 0 {
				asks = append(asks, Order{&t.House.Cash, t.StockDest})
				askRefs = append(askRefs, agentRef{traderAgent, i})
			}
		}

		price := market.Price // what trade happens at; Clear then moves the price
		bidFills, askFills := market.Clear(bids, asks)

		for i, ref := range bidRefs {
			got := bidFills[i]
			switch ref.kind {
			case popAgent:
				r.Pops[ref.idx].Consumed[g] = got
			case firmAgent:
				r.Firms[ref.idx].Inputs[g] += got
				r.Firms[ref.idx].tickCost += got * price
				r.Firms[ref.idx].tickInputs += got * price
			case traderAgent:
				t := r.outbound[g][ref.idx]
				t.Receive(gid, got)
				t.House.Spent += got * price
			case nationAgent:
				r.publicGot[g] = got
				r.Nation.Spending += got * price
			case investAgent:
				r.Firms[ref.idx].Expansion.Delivered += got
			}
		}
		for i, ref := range askRefs {
			sold := askFills[i]
			if ref.kind == firmAgent {
				r.Firms[ref.idx].Inventory -= sold
				r.Firms[ref.idx].tickRevenue += sold * price
			} else {
				t := r.inbound[g][ref.idx]
				t.StockDest -= sold
				if rate, _ := t.border(); rate > 0 { // import tariff, paid on sale
					duty := sold * price * rate
					t.House.Cash -= duty
					t.House.Spent += duty
					r.Nation.Treasury += duty
					r.Nation.TariffRevenue += duty
				}
			}
		}
	}
	r.capUnsuppliedPrices()
}

// unsuppliedPriceCap is how far above the tile's typical price (the median
// price of the goods it has sellers for) a good nobody sells there may climb.
// A shortage still pulls in imports and producers, but a good no one can
// make yet doesn't build up a price that means nothing, which the first
// producer and merchants would then take at face value.
const unsuppliedPriceCap = 10.0

// capUnsuppliedPrices holds the price of goods with no supply here to
// unsuppliedPriceCap times the tile's typical price.
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
	limit := unsuppliedPriceCap * median(sold)
	for _, m := range r.Markets {
		if m.Supply <= 0 && m.Price > limit {
			m.Price = limit
		}
	}
}

// Produce decays unsold stock, then lets every firm produce.
func (r *Region) Produce() {
	for _, f := range r.Firms {
		if f.State {
			continue // universities produce for their nation (settleUniversities)
		}
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
// there is no owner class (as in the classic world) everyone owns a share.
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
			r.payIncome(p, div*(p.Size/ownSize))
		}
	}
}

// Agglomeration: on every tile, industry and services are more productive the
// more people live there: (people / sizeBonusPop)^sizeBonusExp. Busy tiles pay
// better, draw migrants and grow further, so cities emerge where trade and
// resources concentrate people; thin tiles stay rural.
const (
	sizeBonusPop = 100.0 // a starting tile's people: bonus 1
	sizeBonusExp = 0.45
	// a tile is a city when it has this many times the average tile's people
	cityMult = 2.0
)

// SizeBonus is the productivity multiplier the tile's size gives its
// non-primary firms (1 outside game worlds).
func (r *Region) SizeBonus() float64 {
	if !r.Urban {
		return 1
	}
	return math.Pow(math.Max(r.Population(), 1)/sizeBonusPop, sizeBonusExp)
}

func (r *Region) updateAgglomeration() {
	b, e := r.SizeBonus(), 1+educationBonus*r.Educated
	for _, f := range r.Firms {
		if f.Sector != Primary {
			f.Agglom, f.EduMult = b, e
		}
	}
}

// Population growth per year: well-fed people multiply, hungry ones don't.
// growth = popGrowthSlope * (basic needs met - popGrowthNeutral) + tech bonus,
// within [popGrowthMin, popGrowthMax].
const (
	popGrowthSlope   = 0.035
	popGrowthNeutral = 0.5
	popGrowthMin     = -0.03
	popGrowthMax     = 0.02
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

// GrowthRate is the tile's population growth per year at current consumption.
func (r *Region) GrowthRate() float64 {
	g := popGrowthSlope*(r.BasicNeedsMet()-popGrowthNeutral) - r.crowdingPenalty()
	if r.Urban { // game worlds: public health
		g += r.healthBonus()
	}
	if r.Nation != nil {
		g += r.Nation.popGrowthBonus()
	}
	return math.Max(popGrowthMin, math.Min(popGrowthMax, g))
}

// grow applies a day of population growth. Newborns bring no cash.
func (r *Region) grow() {
	f := 1 + r.GrowthRate()/TicksPerYear
	for _, p := range r.Pops {
		p.Size *= f
	}
}

// MoveCost is the cost of moving through the tile. Roads and railways
// improve movement over land (a land tile's base cost times the
// infrastructure multiplier). Coastal tiles can also be passed by sea, at
// CoastMove times the owner's shipping technology; the cheaper way counts.
func (r *Region) MoveCost() float64 {
	land := r.BaseMove * InfraLevels[r.Infra].MoveMult
	if !r.Coastal {
		return land
	}
	sea := CoastMove
	if r.Nation != nil {
		sea *= r.Nation.coastMult()
	}
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

// Allegiance: a tile's people come to stand by their owner slowly. A
// nation's starting tiles are loyal; a conquered tile starts at nothing (its
// people stand by the old owner), free land at freeLandAllegiance (they
// answer to nobody). It grows towards loyalty with a time constant of
// allegianceYears, and until then the tile produces less, pays less tax and
// gives fewer recruits.
const (
	allegianceYears    = 8.0
	freeLandAllegiance = 0.4
)

// allegianceGrowth is how much disloyalty is left after a day.
var allegianceGrowth = math.Exp(-1 / (allegianceYears * TicksPerYear))

// Allegiance is how far the tile's people stand by their owner, 0..1.
func (r *Region) Allegiance() float64 { return 1 - r.Disloyalty }

// AllegianceMult is what allegiance does to the tile's production and taxes:
// half at none, all of it when loyal.
func (r *Region) AllegianceMult() float64 { return 0.5 + 0.5*r.Allegiance() }

// applyAllegiance sets the tile's firms' productivity from its allegiance.
func (r *Region) applyAllegiance() {
	m := r.AllegianceMult()
	for _, f := range r.Firms {
		f.Loyal = m
	}
}
