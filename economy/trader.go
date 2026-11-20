package economy

import (
	"math"
	"sort"
)

// TradingHouse is a tile's merchants: one purse that funds every route
// leaving the tile, by land and by sea. When money is short it goes to the
// routes with the best margins first. Its profits go to the tile's owners.
type TradingHouse struct {
	Region *Region
	Routes []*Trader
	// busy are the routes it plans every day (see idle), in route order;
	// today those it plans today
	busy, today  []*Trader
	busyKnown    bool
	Cash         float64
	CashBuffer   float64 // kept on hand beyond today's needs
	DesiredSpend float64 // what today's plans would cost
	Spent        float64 // what it actually paid today: goods, transport, duties
}

// Plan plans every route at current prices, then fits the orders to the
// purse. If it can't pay for all of them, stock already bought is carried
// first (as much as the purse allows), then routes are funded in order of
// their margin, each up to what its market offered last tick: every order
// the house places is paid for.
//
// Idle routes (see idle), most of them on a big map, are planned in turn,
// a tenth of them a day, rather than every day: a route that turns
// profitable starts buying within idleEvery days. They place no orders and
// ship nothing in between, so the markets and shipping skip them too.
func (h *TradingHouse) Plan(tick int) {
	h.plan(tick)
	h.Region.listBidders(h.today)
}

// idleEvery is how many days apart an idle route is looked at.
const idleEvery = 10

// idle reports whether a route is doing nothing: not buying (its order at
// the floor and its margin, when it was last planned, well below paying), nothing bought
// waiting to ship, nothing unsold at the other end. Only what a save keeps
// decides it, so a restored world plans the same routes as one that ran on.
func (t *Trader) idle() bool {
	return t.Q <= t.QMin && t.RelMargin <= -idleMargin && t.StockOrigin == 0 && t.StockDest == 0
}

// idleMargin: a route this close to paying (its margin as a share of the
// price where it sells) is planned every day, not in turn, so that it
// doesn't miss the days it pays.
const idleMargin = 0.05

// planToday lists the routes to plan today, in route order: those that
// aren't idle, and the idle ones whose turn it is.
func (h *TradingHouse) planToday(tick int) {
	if !h.busyKnown {
		h.busy = h.busy[:0]
		for _, t := range h.Routes {
			if !t.idle() {
				h.busy = append(h.busy, t)
			}
		}
		h.busyKnown = true
	}
	// a route not planned yesterday was idle and has stayed so: the busy
	// ones today are among yesterday's
	today := h.today[:0]
	b := 0
	for i := tick % idleEvery; ; i += idleEvery {
		next := len(h.Routes)
		if i < len(h.Routes) {
			next = i
		}
		for ; b < len(h.busy) && h.busy[b].slot < next; b++ {
			if t := h.busy[b]; !t.idle() {
				today = append(today, t)
			}
		}
		if b < len(h.busy) && h.busy[b].slot == next {
			b++ // its turn anyway
		}
		if next == len(h.Routes) {
			break
		}
		today = append(today, h.Routes[next])
	}
	h.today = today
	h.busy = append(h.busy[:0], today...) // tomorrow's candidates
}

func (h *TradingHouse) plan(tick int) {
	h.planToday(tick)
	desired, full, carry := 0.0, 0.0, 0.0
	h.Spent = 0
	for _, t := range h.today {
		t.Plan()
		t.ship = 1
		desired += t.DesiredSpend
		full += t.bidCost() + t.carry
		carry += t.carry
	}
	h.DesiredSpend = desired
	budget := math.Max(0, 0.9*h.Cash)
	if full <= budget {
		return
	}
	if carry > budget {
		for _, t := range h.today { // (only today's can have stock)
			t.ship = budget / carry
		}
		budget = 0
	} else {
		budget -= carry
	}
	order := make([]int, len(h.today))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return h.today[order[a]].RelMargin > h.today[order[b]].RelMargin })
	for _, i := range order {
		t := h.today[i]
		if !t.Active || t.unit <= 0 {
			continue
		}
		t.Q = math.Min(t.Q, t.Origin.Markets[t.Good].Supply)
		if cost := t.Q * t.unit; cost <= budget {
			budget -= cost
		} else {
			t.Q, budget = budget/t.unit, 0
		}
	}
}

// Trader is one route of a trading house (origin -> dest) for one good.
//
// It buys the good in the origin market, plus tau = cost * distance * bulk
// units of the origin's transport good per unit shipped. A fraction of
// perishable goods is lost on the way: (1 - spoilage)^distance arrives, next
// tick, and is sold in the destination market. The quantity bought each tick
// adapts to the margin per unit bought
//
//	p_dest * arriving - p_origin - tau * p_transport_origin
type Trader struct {
	// the fields the markets read every day come first, to share a cache line
	Good        GoodID
	Active      bool    // false: at the floor with no margin, so don't buy at all
	Q           float64 // target purchase per tick
	buy         float64 // today's purchase: Q less the stock unsold at the destination
	Tau         float64
	ship        float64       // share of that stock the house can afford to ship today
	StockOrigin float64       // bought earlier, still waiting for transport
	StockDest   float64       // arrived, for sale in dest
	House       *TradingHouse // the origin's, which funds the route
	// sold in the destination today, and import duties paid there: owed to
	// the house, which belongs to another tile (see settleHouses)
	proceeds, duty float64
	slot           int // its place in its house's routes

	Origin, Dest  *Region
	TransportCost float64 // multiplier on every hop
	FixedDistance bool    // its distance is its own, not the land hops' (a sea route between ports)
	Distance      float64
	Tariff        float64 // import duty, a share of the price, paid where it sells (into the destination's Duties)
	Closed        bool    // embargoed: it may not trade
	Arriving      float64
	Speed         float64
	Damping       float64
	QMin          float64
	Bought        float64 // bought this tick
	TransportQty  float64 // transport units bought this tick
	UsedTransport float64 // transport units used up shipping today (for GDP: trade's input)
	Shipped       float64 // last tick's shipment (as loaded)
	Margin        float64
	RelMargin     float64
	DesiredSpend  float64
	carry         float64 // transport for stock already bought, part of DesiredSpend
	unit          float64 // cost of buying and shipping one more unit today
	spoilage      float64
}

// backlogDays is how many days a trader takes to work off stock unsold at
// its destination, by buying that much less each day.
const backlogDays = 5.0

// NewTrader makes a trader.
func NewTrader(good GoodID, origin, dest *Region, distance, transportCost float64) *Trader {
	t := &Trader{
		Good: good, Origin: origin, Dest: dest, TransportCost: transportCost,
		Speed: 0.1, Damping: 10, QMin: 0.02, Q: 0.02,
		spoilage: Goods[good].Spoilage,
	}
	t.SetDistance(distance)
	return t
}

// SetDistance changes the route's length (e.g. after a tile's LandTransportMult changes): the
// transport needed per unit and the share of perishables that arrives.
func (t *Trader) SetDistance(d float64) {
	t.Distance = d
	t.Tau = t.TransportCost * d * Goods[t.Good].Bulk
	spoil := t.spoilage
	spoil *= t.Origin.SpoilageMult
	t.Arriving = math.Pow(1-spoil, d)
}

// border returns the import tariff the route pays and whether an embargo
// closes it.
func (t *Trader) Border() (tariff float64, blocked bool) {
	return t.Tariff, t.Closed
}

// Name is "good:origin->dest".
func (t *Trader) Name() string {
	return Goods[t.Good].Name + ":" + t.Origin.Name + "->" + t.Dest.Name
}

func clip(x float64) float64 { return math.Max(-1, math.Min(1, x)) }

// Plan updates the quantity to buy this tick from current prices (its house
// then fits it to the purse).
func (t *Trader) Plan() {
	po, pd := t.Origin.Markets, t.Dest.Markets
	pOrig, pDest, pT := po[t.Good].Price, pd[t.Good].Price, po[Transport].Price
	tariff, blocked := t.Border()
	if m := t.Origin.made; m != nil && !m[t.Good] {
		blocked = true // nobody can make it yet: there's nothing to deal in
	}
	if blocked {
		t.carry, t.unit = t.StockOrigin*t.Tau*pT, 0
		t.Q, t.Active, t.RelMargin, t.Margin, t.DesiredSpend, t.buy = t.QMin, false, 0, 0, t.carry, 0
		return
	}
	t.Margin = pDest*t.Arriving*(1-tariff) - pOrig - t.Tau*pT
	rel := clip(t.Margin / pDest)
	// grow with the margin, but brake when the margin is shrinking (damps cycles)
	signal := rel + t.Damping*(rel-t.RelMargin)
	t.RelMargin = rel
	if t.Q <= t.QMin && signal <= 0 {
		t.Q = t.QMin // at the floor and not growing (most idle routes): spare the exp
	} else {
		t.Q = math.Max(t.QMin, t.Q*math.Exp(t.Speed*clip(signal)))
	}
	t.Active = t.Margin > 0 || t.Q > t.QMin
	q := 0.0
	if t.Active {
		// stock still unsold at the destination (beyond what arrives today) is
		// stock it failed to sell: it buys less, working the backlog off over
		// backlogDays, so goods stop piling up there instead of the route
		// swinging between gluts and shortages
		backlog := math.Max(0, t.StockDest-t.Shipped*t.Arriving)
		q = math.Max(0, t.Q-backlog/backlogDays)
	}
	t.buy = q
	t.carry = t.StockOrigin * t.Tau * pT
	t.unit = pOrig + t.Tau*pT
	// it can't buy more than the market offered (a standing bid for a good
	// nobody sells costs nothing and mustn't make the owners fund it)
	t.DesiredSpend = math.Min(q, po[t.Good].Supply)*t.unit + t.carry
}

// bidCost is what today's order costs if it is filled in full.
func (t *Trader) bidCost() float64 {
	return t.buy * t.unit
}

// Bid is how much of good the trader wants to buy in its origin market.
func (t *Trader) Bid(good GoodID) float64 {
	q := t.buy
	switch good {
	case t.Good:
		return q
	case Transport:
		return t.Tau * (q + t.ship*t.StockOrigin)
	}
	return 0
}

// Receive takes delivery of what the trader bought.
func (t *Trader) Receive(good GoodID, qty float64) {
	switch good {
	case t.Good:
		t.Bought += qty
	case Transport:
		t.TransportQty += qty
	}
}

// Ship sends what transport allows. Stock that has already waited a tick
// (unsold at dest, or stuck at origin) spoils; fresh purchases don't, except
// for what is lost in transit.
func (t *Trader) Ship() {
	t.UsedTransport = t.TransportQty
	if t.StockDest == 0 && t.StockOrigin == 0 && t.Bought == 0 && t.TransportQty == 0 {
		t.Shipped = 0 // an idle route (most of them): nothing moves
		return
	}
	t.StockDest *= 1 - t.spoilage
	t.StockOrigin = t.StockOrigin*(1-t.spoilage) + t.Bought
	t.Bought = 0
	n := t.StockOrigin
	if t.Tau != 0 {
		n = math.Min(t.StockOrigin, t.TransportQty/t.Tau)
	}
	t.StockOrigin -= n
	t.StockDest += n * t.Arriving // arrives next tick
	t.Shipped = n
	t.TransportQty = 0 // transport is a service: unused capacity is lost
}

// Today lists the house's routes that trade today (the rest are idle).
func (h *TradingHouse) Today() []*Trader { return h.today }
