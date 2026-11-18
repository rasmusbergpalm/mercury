package economy

// Levers: what someone outside the economy may set to change it, and the
// accounts where what it collects piles up. They are economics, not
// politics: a tax is a wedge between what an employer pays and what a
// worker keeps, a tariff a wedge on a route, and a buyer anyone who buys
// in a tile's markets without being one of its people, firms or merchants.
// The economy reads the levers as it goes and never calls out; whoever
// sets them (a game's nations, a model of policy) takes what piles up in
// the accounts whenever it likes.
//
// On a tile (Region):
//
//   - TaxFn: the tax withheld from each income paid to its people (wages
//     and dividends), into Withheld;
//   - Buyers: outside buyers in its markets;
//   - Duties: the import duties collected on sales there (see Trader.Tariff);
//   - PaybackMult: scales the years of profit owners ask a new level to
//     repay (below 1: they build sooner, as with cheaper credit);
//   - GrowthBonus: added to its people's growth a year;
//   - RoadMult and CoastMult: scale the cost of moving past it over land
//     and by sea;
//   - SpoilageMult: scales how much of a perishable good shipped from it is
//     lost on the way;
//   - Educated, Healthy and Urban: its schooling, its public health and
//     whether its size makes its industry more productive.
//
// On a route (Trader): Tariff and Closed. On a firm: Mult, and Managed for
// a firm run for someone else. On the world: Known, the technologies known
// somewhere.

// Buyer is someone outside a tile's markets who buys in them: a
// government, a foreign merchant, an aid agency. Each day it bids for
// goods (Bid, by good); the market charges its Account, which may run
// negative until its owner settles it, and notes what it got (Got) for
// each good it bid for.
type Buyer struct {
	Bid, Got []float64
	Account  float64
}

// NewBuyer makes a buyer with no bids.
func NewBuyer() *Buyer {
	return &Buyer{Bid: make([]float64, NumGoods), Got: make([]float64, NumGoods)}
}

// PayIncome splits an income payment to a pop into what the pop keeps and
// the tax withheld.
func (r *Region) PayIncome(p *Pop, amount float64) {
	if r.TaxFn == nil {
		p.Cash += amount
		p.Income += amount
		return
	}
	tax := r.TaxFn(amount)
	p.Cash += amount - tax
	p.Income += amount - tax
	r.Withheld += tax
}
