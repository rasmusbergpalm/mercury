package engine

import "math"

// Region is a map tile with its own pops, firms, labor market and goods markets.
type Region struct {
	ID        int
	Name      string
	Row, Col  int
	Terrain   string
	Firms     []*Firm
	Pops      []*Pop
	Markets   []*Market // indexed by good
	Wage      float64
	WageSpeed float64

	Nation   *Nation  // owner, or nil
	BaseMove float64  // terrain's movement cost
	Infra    int      // index into InfraLevels
	Project  *Project // infrastructure under construction, or nil
	Neglect  float64  // accumulated unmet upkeep

	UtilityEMA float64 // smoothed utility per person; migration follows it

	claims               []claim   // what the nation buys here this tick
	publicBid, publicGot []float64 // by good

	outbound [][]*Trader // traders buying here, by good (transport: all of them)
	inbound  [][]*Trader // traders selling here, by good

	prices, demandBuf []float64 // scratch
	popDemand         [][]float64
}

// NewRegion makes a region with n identical pops.
func NewRegion(id int, name string, firms []*Firm, nPops int, popSize, popCash float64) *Region {
	r := &Region{ID: id, Name: name, Firms: firms, Wage: 1, WageSpeed: 0.1, BaseMove: 1,
		outbound: make([][]*Trader, NumGoods), inbound: make([][]*Trader, NumGoods),
		prices: make([]float64, NumGoods), publicBid: make([]float64, NumGoods),
		publicGot: make([]float64, NumGoods)}
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
	for _, t := range traders {
		if t.Origin == r {
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
	prices := r.refreshPrices()
	for _, f := range r.Firms {
		f.tickRevenue, f.tickCost, f.investWant = 0, 0, 0
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
	for _, f := range r.Firms {
		if f.Expansion == nil && f.Levels < f.MaxLevels && f.Age > investMinAge {
			perLevel := f.ProfitEMA / float64(f.Levels)
			if perLevel > 0 && perLevel*investPayback > LevelCost*price {
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
	totalD := 0.0
	for _, f := range r.Firms {
		totalD += f.PlanLabor
	}
	supply := r.Population()
	scale := 0.0
	if totalD > 0 {
		scale = math.Min(1, supply/totalD)
	}
	for _, p := range r.Pops {
		p.Jobs = p.Jobs[:0]
	}
	pi := 0
	free := r.Pops[0].Size
	for fi, f := range r.Firms {
		f.Labor = f.PlanLabor * scale
		wage := 0.0
		if f.Labor > 0 {
			wage = math.Min(r.Wage, math.Max(f.Cash, 0)/f.Labor)
		}
		need := f.Labor
		for need > 1e-12 && pi < len(r.Pops) {
			pop := r.Pops[pi]
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
				if pi < len(r.Pops) {
					free = r.Pops[pi].Size
				} else {
					free = 0
				}
			}
		}
	}
	excess := (totalD - supply) / supply
	r.Wage *= math.Exp(r.WageSpeed * clip(excess))
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
				bids = append(bids, Order{&t.Cash, q})
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
				asks = append(asks, Order{&t.Cash, t.StockDest})
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
			case traderAgent:
				r.outbound[g][ref.idx].Receive(gid, got)
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
					t.Cash -= duty
					r.Nation.Treasury += duty
					r.Nation.TariffRevenue += duty
				}
			}
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

// PayDividends settles an owner (firm or trader) with the region's pops, who
// own it. Cash is pulled toward a target that covers what the owner wants to
// spend: excess is paid out as dividends, a shortfall is met by the pops
// investing (at most 10% of their cash per tick). Money is conserved.
func (r *Region) PayDividends(cash *float64, cashBuffer, desiredSpend float64) {
	const rate, maxCall = 0.5, 0.1
	target := cashBuffer + 1.2*desiredSpend
	div := (*cash - target) * rate
	popCash := 0.0
	for _, p := range r.Pops {
		popCash += p.Cash
	}
	if div < 0 {
		div = math.Max(div, -maxCall*popCash)
	}
	popSize := r.Population()
	*cash -= div
	for _, p := range r.Pops {
		if div < 0 {
			p.Cash += div * (p.Cash / popCash) // investment is not taxed
		} else {
			r.payIncome(p, div*(p.Size/popSize))
		}
	}
}

// MoveCost is the cost of moving through the tile, given its infrastructure
// and its owner's technology.
func (r *Region) MoveCost() float64 {
	m := r.BaseMove * InfraLevels[r.Infra].MoveMult
	if r.Nation != nil && r.Terrain == "coast" {
		m *= r.Nation.coastMult()
	}
	return m
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
