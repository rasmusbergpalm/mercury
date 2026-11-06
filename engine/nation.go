package engine

import (
	"errors"
	"math"
)

// InfraLevel is a grade of infrastructure on a tile. Better infrastructure
// makes the tile cheaper to move through: every hop into or out of it costs
// less transport, and perishable goods lose less on the way.
type InfraLevel struct {
	Name     string
	MoveMult float64 // multiplies the terrain's movement cost
	Cost     float64 // construction units to build this level from the one below
	Upkeep   float64 // construction units per tick to maintain it
}

// InfraLevels are the infrastructure grades, from none upwards.
var InfraLevels = []InfraLevel{
	{"none", 1.0, 0, 0},
	{"road", 0.6, 200, 0.5},
	{"paved road", 0.4, 400, 1.0},
}

// Project is infrastructure being built on a tile.
type Project struct {
	Target    int     // level being built
	Delivered float64 // construction units delivered so far
}

// Required is the construction units the project needs in total.
func (p *Project) Required() float64 { return InfraLevels[p.Target].Cost }

// Nation owns tiles, taxes its pops' income and pays for infrastructure.
type Nation struct {
	ID       int
	Name     string
	Color    string
	Regions  []*Region
	TaxRate  float64 // share of pop income (wages and dividends) taken as tax
	Treasury float64
	// Reserve is the treasury the nation keeps; RebateRate of anything above it
	// is returned to its pops per head each tick.
	Reserve    float64
	RebateRate float64
	// MaxBuildRate caps construction units bought per tick per project.
	MaxBuildRate float64

	// last tick's flows
	Revenue, Rebate, Spending float64
}

// Errors returned by Build.
var (
	ErrNoNation   = errors.New("tile belongs to no nation")
	ErrMaxLevel   = errors.New("tile already has the best infrastructure")
	ErrBuilding   = errors.New("tile already has a project under way")
	ErrNoProject  = errors.New("tile has no project under way")
	ErrBadTaxRate = errors.New("tax rate must be in [0, 0.9]")
)

// SetTaxRate changes the nation's tax rate.
func (n *Nation) SetTaxRate(rate float64) error {
	if !(rate >= 0 && rate <= 0.9) {
		return ErrBadTaxRate
	}
	n.TaxRate = rate
	return nil
}

func (n *Nation) population() float64 {
	s := 0.0
	for _, r := range n.Regions {
		s += r.Population()
	}
	return s
}

// tax splits an income payment to a pop into what the pop keeps and the tax.
func (r *Region) payIncome(p *Pop, amount float64) {
	if r.Nation == nil {
		p.Cash += amount
		return
	}
	tax := amount * r.Nation.TaxRate
	p.Cash += amount - tax
	r.Nation.Treasury += tax
	r.Nation.Revenue += tax
}

// plan sets each owned tile's bid for construction (upkeep plus projects),
// scaled down if the treasury can't pay for it all.
func (n *Nation) plan() {
	n.Revenue, n.Rebate, n.Spending = 0, 0, 0
	total := 0.0
	for _, r := range n.Regions {
		r.upkeepWant = InfraLevels[r.Infra].Upkeep
		r.projectWant = 0
		if r.Project != nil {
			r.projectWant = math.Min(n.MaxBuildRate, r.Project.Required()-r.Project.Delivered)
		}
		total += (r.upkeepWant + r.projectWant) * r.Markets[Construction].Price
	}
	scale := 1.0
	if budget := 0.9 * math.Max(n.Treasury, 0); total > budget {
		scale = budget / total
	}
	for _, r := range n.Regions {
		r.upkeepWant *= scale
		r.projectWant *= scale
		r.publicBid = r.upkeepWant + r.projectWant
		r.publicGot = 0
	}
}

// settle uses the construction each tile got: upkeep first, then its project.
// Unmet upkeep builds up neglect; enough neglect costs the tile a level.
// Returns the tiles whose infrastructure level changed.
func (n *Nation) settle() []*Region {
	var changed []*Region
	for _, r := range n.Regions {
		need := InfraLevels[r.Infra].Upkeep
		toUpkeep := math.Min(r.publicGot, need)
		r.Neglect = r.Neglect*0.98 + (need - toUpkeep)
		rest := r.publicGot - toUpkeep
		if p := r.Project; p != nil {
			p.Delivered += rest
			if p.Delivered >= p.Required()-1e-9 {
				r.Infra, r.Project, r.Neglect = p.Target, nil, 0
				changed = append(changed, r)
				continue
			}
		}
		if r.Infra > 0 && r.Neglect > 20*need {
			r.Infra--
			r.Neglect = 0
			changed = append(changed, r)
		}
	}
	return changed
}

// rebate returns part of the treasury above the reserve to the pops, per head.
func (n *Nation) rebate() {
	excess := n.Treasury - n.Reserve
	if excess <= 0 {
		return
	}
	amount := n.RebateRate * excess
	pop := n.population()
	n.Treasury -= amount
	n.Rebate = amount
	for _, r := range n.Regions {
		for _, p := range r.Pops {
			p.Cash += amount * p.Size / pop
		}
	}
}
