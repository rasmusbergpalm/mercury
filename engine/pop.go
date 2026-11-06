package engine

// Job is labor a pop supplies to one firm.
type Job struct {
	Firm   int // index into the region's firms
	Amount float64
}

// Pop is a group of identical people that supply labor and consume goods.
type Pop struct {
	Size     float64 // number of people = units of labor supplied
	Cash     float64
	Jobs     []Job
	Consumed []float64 // this tick, whole pop, indexed by good

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
	Demand(prices, p.Cash/p.Size, out)
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
