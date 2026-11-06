package engine

import "math"

// Trader is a merchant on one route (origin -> dest) for one good.
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
	Cash          float64
	CashBuffer    float64
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
	spoilage      float64
}

// NewTrader makes a trader.
func NewTrader(good GoodID, origin, dest *Region, distance, transportCost float64) *Trader {
	t := &Trader{
		Good: good, Origin: origin, Dest: dest, TransportCost: transportCost,
		Cash: 1, CashBuffer: 1, Speed: 0.1, Damping: 10, QMin: 0.02, Q: 0.02,
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

// Plan updates the quantity to buy this tick from current prices.
func (t *Trader) Plan() {
	po, pd := t.Origin.Markets, t.Dest.Markets
	pOrig, pDest, pT := po[t.Good].Price, pd[t.Good].Price, po[Transport].Price
	tariff, blocked := t.border()
	if blocked {
		t.Q, t.Active, t.RelMargin, t.Margin, t.DesiredSpend = t.QMin, false, 0, 0, t.StockOrigin*t.Tau*pT
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
	cost := q*(pOrig+t.Tau*pT) + t.StockOrigin*t.Tau*pT
	t.DesiredSpend = cost
	if cost > 0.9*t.Cash {
		t.Q *= 0.9 * t.Cash / cost
	}
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
		return t.Tau * (q + t.StockOrigin)
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
