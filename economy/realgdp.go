package economy

import (
	"math"
	"sort"
)

// publicSlot is the extra entry after the goods in a GDPChain's vectors:
// the work of outside employers (Buyer.Hire), which nobody buys, counted at
// cost (their staff's wages).
var publicSlot int

// settledMove is how far a good's price may move in a month for it to count
// towards that month's growth.
const settledMove = 2.0

// gdpWarmup is how many days the chain waits before it starts: every price
// starts at £1, and the first months' prices are the markets finding their
// feet, not prices to measure by. Until then real GDP is the month's pounds.
const gdpWarmup = 90

// GDPChain measures a tile's real GDP: its value added at constant prices,
// in pounds of the first month. Each day it adds up the tile's net output
// (what its firms made less the goods they and its merchants used up, plus
// the work of outside employers) and what the goods traded for; each month it links the month's
// average to the last one with a Fisher index, so the quantities are valued
// at both months' prices and the result does not drift as relative prices
// change. Money only changes hands, so with a fixed money stock it is
// quantities, not pounds, that show growth.
//
// Only prices goods actually traded at count: a good nobody sells has a
// price that means nothing. A good counts towards a month's growth only if
// it traded in that month and the one before, and its price moved less than
// settledMove-fold against the typical good's.
type GDPChain struct {
	Level     float64   // real GDP per day, in pounds of the first month
	PrevNet   []float64 `json:",omitempty"` // last month's average net output, per good
	PrevPrice []float64 `json:",omitempty"` // and its average traded prices (0: not traded)
	AccNet    []float64 `json:",omitempty"` // this month so far: summed daily net output
	AccValue  []float64 `json:",omitempty"` // summed value traded
	AccVolume []float64 `json:",omitempty"` // summed volume traded
	Days      int
	Group     int // the group that held the tile at the last link, plus one (0: none)
}

// accumulateGDP adds today's net output and trade to the month's sums.
func (r *Region) accumulateGDP() {
	c := &r.Real
	if c.AccNet == nil {
		c.AccNet, c.AccValue, c.AccVolume = make([]float64, NumGoods+1), make([]float64, NumGoods+1), make([]float64, NumGoods+1)
	}
	for _, f := range r.Firms {
		c.AccNet[f.Output] += f.LastOutput
		for g, q := range f.Used {
			c.AccNet[g] -= q
		}
	}
	// work for outside employers, at what it costs
	for _, b := range r.Buyers {
		c.AccNet[publicSlot] += b.Hired
		c.AccValue[publicSlot] += b.Hired * r.Wage
		c.AccVolume[publicSlot] += b.Hired
	}
	// the transport the tile's merchants used up moving goods is an input
	// to trade, like a firm's coal, not a product anyone consumes: hauling
	// adds to GDP only through the goods it brings where they are worth more
	for _, t := range r.House.today {
		c.AccNet[Transport] -= t.UsedTransport
	}
	for g, m := range r.Markets {
		if v := math.Min(m.Supply, m.Demand); v > 1e-9 {
			c.AccValue[g] += m.Price * v
			c.AccVolume[g] += v
		}
	}
	c.Days++
}

// gdpLink is one tile's part in a month's link: its net output valued at
// last month's and this month's prices, both months' quantities (settled
// goods only), and this month's value at its own prices.
type gdpLink struct {
	l0, l1, q0, q1, now float64
}

// linkGDP closes the month: it links the month's average net output to the
// last month's, starts a new month, and returns the tile's part in the link
// for its group's chain (see LinkGDP).
func (r *Region) linkGDP() gdpLink {
	c := &r.Real
	if c.Days == 0 {
		r.accumulateGDP() // a sample before the first day ends: use today
	}
	net, price := make([]float64, NumGoods+1), make([]float64, NumGoods+1)
	for g := range net {
		net[g] = c.AccNet[g] / float64(c.Days)
		if c.AccVolume[g] > 0 {
			price[g] = c.AccValue[g] / c.AccVolume[g]
		}
	}
	k := gdpLink{now: math.Max(0, value(price, net))}
	if c.PrevNet != nil {
		// only goods that traded in both months at settled prices link the
		// chain: a new good's first prices are wild (it had drifted while
		// nobody sold any) and fall fast as it comes in, so it joins once
		// its price moves by less than settledMove-fold in a month against
		// the typical good's (so a general inflation or deflation doesn't
		// unsettle every good at once)
		var moves []float64
		for g := range net {
			if a, b := c.PrevPrice[g], price[g]; a > 0 && b > 0 {
				moves = append(moves, b/a)
			}
		}
		typical := median(moves)
		p0, p1 := make([]float64, len(net)), make([]float64, len(net))
		for g := range net {
			if a, b := c.PrevPrice[g], price[g]; a > 0 && b > 0 {
				if m := b / a / typical; m < settledMove && m > 1/settledMove {
					p0[g], p1[g] = a, b
				}
			}
		}
		k.l0, k.l1 = value(p0, c.PrevNet), value(p0, net)
		k.q0, k.q1 = value(p1, c.PrevNet), value(p1, net)
	}
	if r.warm {
		c.Level = chain(c.Level, k)
	} else {
		c.Level = k.now
	}
	c.PrevNet, c.PrevPrice = net, price
	for g := range c.AccNet {
		c.AccNet[g], c.AccValue[g], c.AccVolume[g] = 0, 0, 0
	}
	c.Days = 0
	return k
}

// chain moves a real GDP level on by a month: by the Fisher index, the
// geometric mean of Laspeyres (last month's prices) and Paasche (this
// month's) growth. With nothing to chain from it starts at this month's
// pounds; a month that can't be measured (net output worth nothing at one
// month's prices) leaves it where it was.
func chain(level float64, k gdpLink) float64 {
	switch {
	case level <= 0:
		return k.now
	case k.l0 <= 0 || k.q0 <= 0 || k.l1 <= 0 || k.q1 <= 0:
		return level
	default:
		return level * math.Sqrt(k.l1/k.l0*k.q1/k.q0)
	}
}

// LinkGDP closes the month in every tile (see GDPChain) and links the real
// GDP of groups of tiles (countries, say): group(r) is the tile's group
// plus one (0: none), and levels[k] is group k's real GDP, moved on in
// place. A group's tiles' values are added up before the index is taken,
// so each tile weighs by its share of the group's output: a tile with a
// little oil at a wild price can't run away with the total. Last month's
// values come from the tiles the group held then, this month's from those
// it holds now, so gaining a tile grows the group's GDP.
func (w *World) LinkGDP(group func(r *Region) int, levels []float64) {
	links := make([]gdpLink, len(w.Regions))
	for i, r := range w.Regions {
		r.warm = w.Tick > gdpWarmup
		links[i] = r.linkGDP()
	}
	for n := range levels {
		var sum gdpLink
		for i, r := range w.Regions {
			k := links[i]
			if r.Real.Group == n+1 {
				sum.l0 += k.l0
				sum.q0 += k.q0
			}
			if group(r) == n+1 {
				sum.l1 += k.l1
				sum.q1 += k.q1
				sum.now += k.now
			}
		}
		if w.Tick > gdpWarmup {
			levels[n] = chain(levels[n], sum)
		} else {
			levels[n] = sum.now
		}
	}
	for _, r := range w.Regions {
		r.Real.Group = group(r)
	}
}

// median of xs (1 if empty); xs is reordered.
func median(xs []float64) float64 {
	if len(xs) == 0 {
		return 1
	}
	sort.Float64s(xs)
	if n := len(xs); n%2 == 1 {
		return xs[n/2]
	} else {
		return (xs[n/2-1] + xs[n/2]) / 2
	}
}

func value(p, q []float64) float64 {
	s := 0.0
	for g := range p {
		s += p[g] * q[g]
	}
	return s
}

func (c GDPChain) Clone() GDPChain {
	cp := func(x []float64) []float64 { return append([]float64(nil), x...) }
	return GDPChain{c.Level, cp(c.PrevNet), cp(c.PrevPrice), cp(c.AccNet), cp(c.AccValue), cp(c.AccVolume), c.Days, c.Group}
}

// valid says whether a restored chain fits this build's goods.
func (c GDPChain) Valid() bool {
	for _, x := range [][]float64{c.PrevNet, c.PrevPrice, c.AccNet, c.AccValue, c.AccVolume} {
		if x != nil && len(x) != NumGoods+1 {
			return false
		}
	}
	return (c.PrevNet == nil) == (c.PrevPrice == nil) && (c.AccNet == nil) == (c.AccValue == nil) &&
		(c.AccNet == nil) == (c.AccVolume == nil) && c.Days >= 0 && c.Level >= 0
}
