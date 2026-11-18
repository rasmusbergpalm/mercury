package economy

// Government is what a layer above the economy plugs into the tiles it
// governs: a game's nations, or a researcher's model of policy. The economy
// asks it for taxes, duties and a few policy numbers, buys what it bids for
// (Region.PublicBid) and hands it what it is owed after each day
// (Ledger). A tile with no government (Region.Gov nil) is untaxed free land
// that trades freely.
type Government interface {
	// IncomeTax is the tax taken from an income of amount paid to one of
	// the tile's people (wages and dividends).
	IncomeTax(amount float64) float64
	// Duty is the import duty on good g entering the tile from the tile
	// from, as a share of its price, and whether an embargo closes the
	// route.
	Duty(from *Region, g GoodID) (rate float64, blocked bool)
	// PaybackMult scales the years of profit owners ask a new level to
	// repay (below 1: they build sooner, as with cheaper credit).
	PaybackMult() float64
	// GrowthBonus is added to the tile's population growth a year.
	GrowthBonus() float64
	// CoastMult scales the cost of moving past a coastal tile by sea.
	CoastMult() float64
	// SpoilageMult scales how much of a perishable good shipped from the
	// tile is lost on the way.
	SpoilageMult() float64
	// Settle hands the government what the tile owes it from the day's
	// work.
	Settle(l Ledger)
}

// Hooks are the points in a day where a layer above the economy acts. Any
// may be left empty.
type Hooks interface {
	// BeginDay runs before anyone plans.
	BeginDay()
	// Plan runs once firms and merchants have planned, before the markets
	// open: governments post their purchases (Region.PublicBid).
	Plan()
	// Shipped runs once merchants have shipped the day's goods.
	Shipped()
	// Settle runs once firms have produced and paid out, and the ledgers
	// have been collected; before owners invest and people move.
	Settle()
	// EndDay runs at the end of the day, once the day has advanced.
	EndDay()
	// Knows reports whether any government knows a technology, so that the
	// goods made with it are in play.
	Knows(tech string) bool
}

// Ledger is what a tile owes its government from a day's work.
type Ledger struct {
	Tax     float64 // income tax collected from its people
	Paid    float64 // the government's purchases here: what it paid, as a negative sum
	Tariffs float64 // import duties on goods sold here
}

// PayIncome splits an income payment to a pop into what the pop keeps and
// the tax.
func (r *Region) PayIncome(p *Pop, amount float64) {
	if r.Gov == nil {
		p.Cash += amount
		p.Income += amount
		return
	}
	tax := r.Gov.IncomeTax(amount)
	p.Cash += amount - tax
	p.Income += amount - tax
	r.Ledger.Tax += tax // the government gets it at SettleLedgers
}
