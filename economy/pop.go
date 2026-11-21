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
// share of spendShare of the pop's cash (see Demand).
func (p *Pop) Demand(prices []float64, out []float64) {
	var d demandPlan
	d.prepare(prices)
	p.demand(&d, out)
}

// People spend spendShare of their cash a tick and keep the rest: savings
// carry them through a bad spell, so a dip in wages or a jump in prices
// moves what they buy by a twentieth as much on the tick, and a market's
// demand doesn't swing with every wage. (Spending it all, they turned every
// tick's wage straight into the next tick's demand.)
const spendShare = 1.0 / 20

// demand is Demand at prices already prepared.
func (p *Pop) demand(d *demandPlan, out []float64) {
	d.demand(spendShare*p.Cash/p.Size, out)
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
