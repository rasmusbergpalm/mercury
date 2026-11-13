package engine

import "math"

// The mint. Money is not a fixed stock: the world's mint issues it as the
// economy grows, so that prices hold steady instead of falling as output
// rises (with a fixed stock they fell 8-17-fold over a century, and every
// pound held for a year bought more for doing nothing).
//
// The world's price level is linked each month as real GDP is (realgdp.go):
// a Fisher index of every tile's net output at last month's and this
// month's prices, settled goods only, so new goods and wild prices don't
// move it. It is 1 when the game opens. Below 1, the mint issues money:
// over the coming month, each day, mintShareDay of the gap's worth of the
// money in the world (a gap of 1% issues 1% more money over mintDays). It
// never takes money back: prices above 1 are left to fall as output grows.
//
// The new money goes to the nations' treasuries, each in proportion to its
// output (nominal GDP): seigniorage, an income like taxes, so a growing
// economy pays a little less tax. Free land gets none. With no nations
// there is no mint.
const mintDays = 180.0

// linkPriceLevel moves the world's price level on by the month's links and
// sets the mint's issue for the month.
func (w *World) linkPriceLevel(links []gdpLink) {
	if w.burning || w.Tick <= gdpWarmup || w.PriceLevel <= 0 {
		w.PriceLevel = 1
	} else {
		var sum gdpLink
		for _, k := range links {
			sum.l0, sum.q0, sum.l1, sum.q1 = sum.l0+k.l0, sum.q0+k.q0, sum.l1+k.l1, sum.q1+k.q1
		}
		if sum.l0 > 0 && sum.q0 > 0 && sum.l1 > 0 && sum.q1 > 0 {
			// Fisher: Laspeyres (last month's quantities) times Paasche (this
			// month's), square-rooted
			w.PriceLevel *= math.Sqrt(sum.q0 / sum.l0 * sum.q1 / sum.l1)
		}
	}
	nominal := 0.0
	for _, n := range w.Nations {
		n.MintRate = 0
		for i, r := range w.Regions {
			if r.Nation == n {
				n.MintRate += links[i].now
			}
		}
		nominal += n.MintRate
	}
	issue := 0.0
	if !w.burning && w.PriceLevel < 1 && nominal > 0 {
		issue = (1 - w.PriceLevel) / w.PriceLevel * w.Money() / mintDays
	}
	for _, n := range w.Nations {
		if issue > 0 {
			n.MintRate *= issue / nominal
		} else {
			n.MintRate = 0
		}
	}
}

// mint pays each nation today's new money.
func (w *World) mint() {
	for _, n := range w.Nations {
		if n.Eliminated() {
			n.MintRate = 0
		}
		n.Minted = n.MintRate
		n.Treasury += n.Minted
		w.Minted += n.Minted
	}
}
