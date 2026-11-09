package engine

import "math"

// Order is a bid or ask: an agent's cash account and a quantity.
type Order struct {
	Cash *float64
	Qty  float64
}

// Prices stay within [minPrice, maxPrice].
const (
	minPrice = 1e-4
	maxPrice = 1e9
)

// Market is a single-good call market in one region: proportional rationing
// of the long side, then a tatonnement price update.
type Market struct {
	Good      GoodID
	Price     float64
	Speed     float64
	MinVolume float64
	Supply    float64 // last clearing
	Demand    float64
	// Trend is the price's long-run level: an average over about trendDays
	// of the days it traded (0: never traded).
	Trend float64
}

// trendDays is roughly how many days a market's price trend averages over.
const trendDays = 200.0

// Clear trades at the current price, rationing the long side proportionally.
// Cash is transferred here; goods are the caller's job, using the returned
// fill quantities (aligned with bids and asks). Then the price moves by
// exp(speed * relative excess demand).
func (m *Market) Clear(bids, asks []Order) (bidFills, askFills []float64) {
	return m.clearInto(bids, asks, nil, nil)
}

// clearInto is Clear writing the fills into the given buffers (grown as
// needed), to spare the allocations.
func (m *Market) clearInto(bids, asks []Order, bidFills, askFills []float64) ([]float64, []float64) {
	price := m.Price
	supply, demand := 0.0, 0.0
	for _, a := range asks {
		supply += a.Qty
	}
	for _, b := range bids {
		demand += b.Qty
	}
	buyFrac, sellFrac := 0.0, 0.0
	if demand > 0 {
		buyFrac = math.Min(1, supply/demand)
	}
	if supply > 0 {
		sellFrac = math.Min(1, demand/supply)
	}
	bidFills = grow(bidFills, len(bids))
	for i, b := range bids {
		got := b.Qty * buyFrac
		*b.Cash -= got * price
		bidFills[i] = got
	}
	askFills = grow(askFills, len(asks))
	for i, a := range asks {
		sold := a.Qty * sellFrac
		*a.Cash += sold * price
		askFills[i] = sold
	}
	// relative excess demand in [-1, 1]; the floor keeps a trickle of demand
	// for a good nobody sells from pushing its price up at full speed forever
	excess := (demand - supply) / math.Max(math.Max(supply, demand), m.MinVolume)
	// a good nobody wants for decades mustn't fall (or rise) out of range
	if excess != 0 { // (balanced or idle markets keep their price: spare the exp)
		m.Price = math.Min(maxPrice, math.Max(minPrice, m.Price*math.Exp(m.Speed*excess)))
	}
	// the trend follows the price on days the good trades (a price nobody
	// pays, at the floor or the ceiling, says nothing about its level); a
	// market that never traded has none
	if math.Min(supply, demand) > 0 {
		if m.Trend <= 0 {
			m.Trend = price
		}
		m.Trend += (price - m.Trend) / trendDays
	}
	m.Supply, m.Demand = supply, demand
	return bidFills, askFills
}

// grow returns buf resized to n (its contents are overwritten by the caller).
func grow(buf []float64, n int) []float64 {
	if cap(buf) < n {
		return make([]float64, n)
	}
	return buf[:n]
}
