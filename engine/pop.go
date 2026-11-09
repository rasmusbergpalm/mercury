package engine

// Job is labor a pop supplies to one firm.
type Job struct {
	Firm   int // index into the region's firms
	Amount float64
}

// Class is a pop's place in society.
type Class int

const (
	// Workers sell their labor (and fill the armies).
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
	Income   float64   // this tick, after tax: wages, dividends, rebates

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
// share of the pop's cash (see Demand).
func (p *Pop) Demand(prices []float64, out []float64) {
	var d demandPlan
	d.prepare(prices)
	p.demand(&d, out)
}

// demand is Demand at prices already prepared.
func (p *Pop) demand(d *demandPlan, out []float64) {
	d.demand(p.Cash/p.Size, out)
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
