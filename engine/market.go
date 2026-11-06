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
}

// Clear trades at the current price, rationing the long side proportionally.
// Cash is transferred here; goods are the caller's job, using the returned
// fill quantities (aligned with bids and asks). Then the price moves by
// exp(speed * relative excess demand).
func (m *Market) Clear(bids, asks []Order) (bidFills, askFills []float64) {
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
	bidFills = make([]float64, len(bids))
	for i, b := range bids {
		got := b.Qty * buyFrac
		*b.Cash -= got * price
		bidFills[i] = got
	}
	askFills = make([]float64, len(asks))
	for i, a := range asks {
		sold := a.Qty * sellFrac
		*a.Cash += sold * price
		askFills[i] = sold
	}
	// relative excess demand in [-1, 1]; the floor keeps a trickle of demand
	// for a good nobody sells from pushing its price up at full speed forever
	excess := (demand - supply) / math.Max(math.Max(supply, demand), m.MinVolume)
	// a good nobody wants for decades mustn't fall (or rise) out of range
	m.Price = math.Min(maxPrice, math.Max(minPrice, m.Price*math.Exp(m.Speed*excess)))
	m.Supply, m.Demand = supply, demand
	return bidFills, askFills
}
