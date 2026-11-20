package economy

// Levers: what someone outside the economy may set to change it, and the
// accounts where what it collects piles up. They are economics, not
// politics: a tax is a wedge between what an employer pays and what a
// worker keeps, a tariff a wedge on a route, and a buyer anyone who buys
// in a tile's markets without being one of its people, firms or merchants.
// The economy reads the levers as it goes and never calls out; whoever
// sets them (a model of policy, a strategy game) takes what piles up in
// the accounts whenever it likes.
//
// On a tile (Region):
//
//   - IncomeTax: the share withheld from each income paid to its people
//     (wages and dividends), into Withheld;
//   - Buyers: outside buyers in its markets;
//   - Duties: the import duties collected on sales there (see Trader.Tariff);
//   - PaybackMult: scales the years of profit owners ask a new level to
//     repay (below 1: they build sooner, as with cheaper credit);
//   - GrowthBonus: added to its people's growth a year, beyond their
//     natural growth;
//   - LandTransportMult and SeaTransportMult: scale the transport it takes
//     to cross it over land and to pass it by sea;
//   - SpoilageMult: scales how much of a perishable good shipped from it is
//     lost on the way;
//
// On a route (Trader): Tariff and Closed. On a firm: Mult (its
// productivity), Enabled and each method's Enabled (which recipes it may
// use) and TFP, Expansion with External set (a level paid for from outside, its
// construction delivered by whoever pays), and Monopoly and Elasticity
// for a firm with market power (none has any unless it is given).

// Buyer is someone outside a tile's markets who buys in them: a state, a
// foreign merchant, an aid agency, or an employer from outside the tile's
// firms. Each day it bids for goods (Bid, by good)
// and for workers (Hire); the markets charge its Account, which may run
// negative until its owner settles it, and note what it got (Got, Hired).
// Workers are hired after the firms, at the same wage.
type Buyer struct {
	Bid, Got    []float64
	Hire, Hired float64
	Account     float64
}

// NewBuyer makes a buyer with no bids.
func NewBuyer() *Buyer {
	return &Buyer{Bid: make([]float64, NumGoods), Got: make([]float64, NumGoods)}
}

// PayIncome splits an income payment to a pop into what the pop keeps and
// the tax withheld.
func (r *Region) PayIncome(p *Pop, amount float64) {
	if r.IncomeTax == 0 {
		p.Cash += amount
		p.Income += amount
		return
	}
	tax := amount * r.IncomeTax
	p.Cash += amount - tax
	p.Income += amount - tax
	r.Withheld += tax
}
