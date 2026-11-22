package economy

// Job is labor a pop supplies to one firm.
type Job struct {
	Firm   int // the employer: an index into the region's firms, or len(Firms) on for its buyers
	Amount float64
}

// Class is a pop's place in society.
type Class int

const (
	// Workers sell their labor.
	Workers Class = iota
	// Owners own the land and the means of production: farms, mines, works,
	// services and trading houses. They live off dividends.
	Owners
	NumClasses
)

var classNames = [...]string{"workers", "owners"}

func (c Class) String() string { return classNames[c] }

// Pop is a group of identical people of one class.
type Pop struct {
	Class    Class
	Size     float64 // number of people (workers supply one unit of labor each)
	Cash     float64
	Jobs     []Job
	Consumed []float64 // this tick, whole pop, indexed by good
	Income   float64   // this tick, after tax: wages and dividends
	// Want is how far short of their subsistence needs the people have been
	// lately (0: fed; 1: wholly without), a running average over about
	// wantTicks: what starves people is a month of want, not one bad day
	Want float64

	perPerson []float64 // scratch
}

// NewPop makes a pop.
func NewPop(size, cash float64) *Pop {
	return &Pop{Size: size, Cash: cash, Consumed: make([]float64, NumGoods),
		perPerson: make([]float64, NumGoods)}
}

// Employed is the labor the pop supplies this tick.
func (p *Pop) Employed() float64 {
	s := 0.0
	for _, j := range p.Jobs {
		s += j.Amount
	}
	return s
}

// Demand writes the pop's purchases into out: every member spends an equal
// share of DefaultSpendShare of the pop's cash (see Demand).
func (p *Pop) Demand(prices []float64, out []float64) {
	var d demandPlan
	d.prepare(prices)
	p.demand(&d, DefaultSpendShare, out)
}

// DefaultSpendShare is the share of their cash people spend a tick unless
// their tile says otherwise (Region.SpendShare); they keep the rest. Their
// savings carry them through a bad spell, so a dip in wages or a jump in
// prices moves what they buy by a twentieth as much on the tick, and a
// market's demand doesn't swing with every wage. (Spending it all, they
// turned every tick's wage straight into the next tick's demand.) It is
// the velocity of the money people hold: with the money fixed, it sets
// the price level, P·Y = share · M.
const DefaultSpendShare = 1.0 / 20

// demand is Demand at prices already prepared, spending share of the
// pop's cash.
func (p *Pop) demand(d *demandPlan, share float64, out []float64) {
	d.demand(share*p.Cash/p.Size, out)
	for i := range out {
		out[i] *= p.Size
	}
}

// Utility is utility per person.
func (p *Pop) Utility() float64 {
	for i, q := range p.Consumed {
		p.perPerson[i] = q / p.Size
	}
	return Utility(p.perPerson)
}
