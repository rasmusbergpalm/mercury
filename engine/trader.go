package engine

import (
	"math"
	"sort"
)

// TradingHouse is a tile's merchants: one purse that funds every route
// leaving the tile, by land and by sea. When money is short it goes to the
// routes with the best margins first. Its profits go to the tile's owners.
type TradingHouse struct {
	Region       *Region
	Routes       []*Trader
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
func (h *TradingHouse) Plan() {
	desired, full, carry := 0.0, 0.0, 0.0
	h.Spent = 0
	for _, t := range h.Routes {
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
		for _, t := range h.Routes {
			t.ship = budget / carry
		}
		budget = 0
	} else {
		budget -= carry
	}
	order := make([]int, len(h.Routes))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return h.Routes[order[a]].RelMargin > h.Routes[order[b]].RelMargin })
	for _, i := range order {
		t := h.Routes[i]
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
	Good          GoodID
	Origin, Dest  *Region
	TransportCost float64 // multiplier on every hop
	Sea           bool    // a sea route between two ports
	Distance      float64
	Tau           float64
	Arriving      float64
	House         *TradingHouse // the origin's, which funds the route
	Speed         float64
	Damping       float64
	QMin          float64
	Q             float64 // target purchase per tick
	Active        bool    // false: at the floor with no margin, so don't buy at all
	StockOrigin   float64 // bought earlier, still waiting for transport
	Bought        float64 // bought this tick
	StockDest     float64 // arrived, for sale in dest
	TransportQty  float64 // transport units bought this tick
	Shipped       float64 // last tick's shipment (as loaded)
	Margin        float64
	RelMargin     float64
	DesiredSpend  float64
	carry         float64 // transport for stock already bought, part of DesiredSpend
	ship          float64 // share of that stock the house can afford to ship today
	unit          float64 // cost of buying and shipping one more unit today
	spoilage      float64
}

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

// SetDistance changes the route's length (e.g. after a road is built): the
// transport needed per unit and the share of perishables that arrives.
func (t *Trader) SetDistance(d float64) {
	t.Distance = d
	t.Tau = t.TransportCost * d * Goods[t.Good].Bulk
	spoil := t.spoilage
	if n := t.Origin.Nation; n != nil {
		spoil *= n.spoilageMult()
	}
	t.Arriving = math.Pow(1-spoil, d)
}

// border returns the import tariff the route pays and whether an embargo
// closes it. Tariffs apply when goods enter a nation from outside it.
func (t *Trader) border() (tariff float64, blocked bool) {
	on, dn := t.Origin.Nation, t.Dest.Nation
	if dn == nil || dn == on {
		return 0, false
	}
	if on != nil && (dn.Embargo[on.ID] || on.Embargo[dn.ID]) {
		return 0, true
	}
	return dn.Tariff[t.Good], false
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
	tariff, blocked := t.border()
	if blocked {
		t.carry, t.unit = t.StockOrigin*t.Tau*pT, 0
		t.Q, t.Active, t.RelMargin, t.Margin, t.DesiredSpend = t.QMin, false, 0, 0, t.carry
		return
	}
	t.Margin = pDest*t.Arriving*(1-tariff) - pOrig - t.Tau*pT
	rel := clip(t.Margin / pDest)
	// grow with the margin, but brake when the margin is shrinking (damps cycles)
	signal := rel + t.Damping*(rel-t.RelMargin)
	t.RelMargin = rel
	t.Q = math.Max(t.QMin, t.Q*math.Exp(t.Speed*clip(signal)))
	t.Active = t.Margin > 0 || t.Q > t.QMin
	q := 0.0
	if t.Active {
		q = t.Q
	}
	t.carry = t.StockOrigin * t.Tau * pT
	t.unit = pOrig + t.Tau*pT
	// it can't buy more than the market offered (a standing bid for a good
	// nobody sells costs nothing and mustn't make the owners fund it)
	t.DesiredSpend = math.Min(q, po[t.Good].Supply)*t.unit + t.carry
}

// bidCost is what today's order costs if it is filled in full.
func (t *Trader) bidCost() float64 {
	if !t.Active {
		return 0
	}
	return t.Q * t.unit
}

// Bid is how much of good the trader wants to buy in its origin market.
func (t *Trader) Bid(good GoodID) float64 {
	q := 0.0
	if t.Active {
		q = t.Q
	}
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
