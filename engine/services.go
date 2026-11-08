package engine

import (
	"errors"
	"fmt"
	"math"
)

// Public services are run by the state in every tile: universities,
// administration, schools and public health. Each has building levels
// (built with construction, like a firm's) and the nation chooses how many
// of them to fund. A funded level employs ServiceStaff workers at the tile's
// wage, paid from the treasury, and what they do depends on the service:
//
//   - universities make the nation's research;
//   - administration collects the tile's taxes: without officials only
//     taxFarmed of them come in, the rest as officials meet the need, which
//     is more for a big tile and for one far from the capital;
//   - schools raise the tile's educated share over a generation, which makes
//     its industry, services and universities more productive;
//   - public health (clean water, doctors, vaccination) raises population
//     growth everywhere, and keeps crowded tiles from killing their people:
//     crowding slows growth and brings cholera unless it is covered.
//
// Their money goes to the staff's wages, so it stays in the economy.
type Public int

const (
	University Public = iota
	Administration
	Schools
	Health
	NumServices
)

// ServiceStaff is how many people one funded level employs.
const ServiceStaff = 2.0

type serviceDef struct {
	Name, Firm string
	Start      int // levels every tile starts with (all funded)
	Spending   int // the accounts' category
}

var services = [NumServices]serviceDef{
	University:     {"universities", "University", 2, Science},
	Administration: {"administration", "Administration", 2, Admin},
	Schools:        {"schools", "Schools", 0, Education},
	Health:         {"public health", "Health board", 0, HealthCare},
}

func (s Public) String() string { return services[s].Name }

// Service needs and effects.
const (
	adminPerHead     = 1.0 / 60  // officials needed per person in the capital
	adminPerHop      = 0.2       // ...and 20% more per hex from it
	pupilsPerTeacher = 40.0      // a teacher's class
	childShare       = 0.3       // share of people of school age
	schoolYears      = 15.0      // how fast the educated share follows enrolment
	startEducated    = 0.1       // the educated share in 1820
	educationBonus   = 0.3       // industry and services of a fully educated tile: +30%
	crowdingFrom     = 150.0     // people from which a tile is crowded
	crowdingSpan     = 300.0     // ...and fully crowded this many more
	doctorPerHead    = 1.0 / 400 // health staff a tile needs per person...
	crowdedDoctors   = 1.0 / 150 // ...and this many more per person when fully crowded
	healthGrowth     = 0.006     // population growth a year a fully covered tile gains
	crowdingDeaths   = 0.02      // growth lost a year by a fully crowded, uncovered tile
	choleraPerYear   = 0.08      // chance of cholera a year in such a tile
	choleraDeaths    = 0.06      // share of its workers cholera kills
	serviceMaxLevels = 12
	maxPublicShare   = 0.3 // most of a tile's workers its public services may employ
	publicWageCap    = 1.5 // above this times the nation's average wage, they hire fewer
	taxFarmed        = 0.3 // share of taxes collected without any officials (tax farmers, the gentry)
)

// Service errors.
var (
	ErrNoService  = errors.New("the tile has no such service")
	ErrBadFunding = errors.New("funding must be between 0 and the service's levels")
	ErrNoLevel    = errors.New("there is no level to close")
	ErrBadService = errors.New("no such public service")
)

// ServiceByName looks a service up by name.
func ServiceByName(name string) (Public, error) {
	for s := Public(0); s < NumServices; s++ {
		if services[s].Name == name {
			return s, nil
		}
	}
	return 0, ErrBadService
}

// ServiceNames lists the services' names, in order.
func ServiceNames() []string {
	var out []string
	for s := Public(0); s < NumServices; s++ {
		out = append(out, services[s].Name)
	}
	return out
}

// newServiceFirm makes a tile's service.
func newServiceFirm(s Public) *Firm {
	f := NewFirm(services[s].Firm, Construction, 1, []Factor{Labor(0.6)}, 0)
	if s == University {
		f = NewFirm("University", Research, 1, []Factor{Labor(0.6)}, 0)
	}
	f.State, f.Public, f.Sector = true, s, Service
	f.Levels, f.Funded, f.MaxLevels, f.Cash = services[s].Start, services[s].Start, serviceMaxLevels, 0
	return f
}

// Service is the tile's service of a kind, or nil.
func (r *Region) Service(s Public) *Firm {
	for _, f := range r.Firms {
		if f.State && f.Public == s {
			return f
		}
	}
	return nil
}

// University is the tile's university, or nil.
func (r *Region) University() *Firm { return r.Service(University) }

// research is what a university produces today with the staff it has.
func (f *Firm) research() float64 {
	if f.Funded <= 0 || f.Labor <= 0 {
		return 0
	}
	return f.TFP * f.Agglom * f.EduMult * f.Loyal * math.Pow(float64(f.Funded), 0.4) * math.Pow(f.Labor, 0.6)
}

// staffMult is how much one of the nation's staff of a service achieves,
// with its technology (1 without a nation).
func (n *Nation) staffMult(s Public) float64 {
	if n == nil {
		return 1
	}
	return n.techProduct(func(t *Tech) float64 {
		switch s {
		case Administration:
			return t.AdminMult
		case Schools:
			return t.SchoolMult
		case Health:
			return t.HealthMult
		}
		return 0
	})
}

// Crowding is how crowded the tile is, 0 (not at all) to 1.
func (r *Region) Crowding() float64 {
	return math.Max(0, math.Min(1, (r.Population()-crowdingFrom)/crowdingSpan))
}

// ServiceNeed is how many staff the tile's service needs to cover it fully,
// given its nation's technology (0 if it needs none).
func (w *World) ServiceNeed(r *Region, s Public) float64 {
	pop, mult := r.Population(), r.Nation.staffMult(s)
	switch s {
	case Administration:
		return pop * adminPerHead * (1 + adminPerHop*float64(w.hopsFromCapital(r))) / mult
	case Schools:
		return pop * childShare / pupilsPerTeacher / mult
	case Health:
		return pop * (doctorPerHead + crowdedDoctors*r.Crowding()) / mult
	}
	return 0
}

// Coverage is the share of the tile's need its service meets today (1 if
// it needs none).
func (w *World) Coverage(r *Region, s Public) float64 {
	need := w.ServiceNeed(r, s)
	if need <= 0 {
		return 1
	}
	f := r.Service(s)
	if f == nil {
		return 0
	}
	return math.Min(1, f.Labor/need)
}

// capital is the nation's most populous tile.
func (n *Nation) capital() *Region {
	var best *Region
	for _, r := range n.Regions {
		if best == nil || r.Population() > best.Population() {
			best = r
		}
	}
	return best
}

func (w *World) hopsFromCapital(r *Region) int {
	if r.Nation == nil {
		return 0
	}
	c := r.Nation.capital()
	return HexDistance(r.Row, r.Col, c.Row, c.Col)
}

// serviceBill is what the nation's funded services would cost today.
func (n *Nation) serviceBill() float64 {
	s := 0.0
	for _, r := range n.Regions {
		for _, f := range r.Firms {
			if f.State && f.Enabled {
				s += float64(f.Funded) * ServiceStaff * r.Wage
			}
		}
	}
	return s
}

// staffServices lets each funded service hire its staff, paying (scale of)
// their wages into it from the treasury.
func (n *Nation) staffServices(scale float64) {
	for _, r := range n.Regions {
		// the services may employ at most maxPublicShare of the tile's workers
		want := 0.0
		for _, f := range r.Firms {
			if f.State && f.Enabled {
				want += float64(f.Funded) * ServiceStaff
			}
		}
		local := scale
		if limit := maxPublicShare * sizeOf(r.ClassPops(Workers)); want > limit {
			local *= limit / want
		}
		// where wages run far above the nation's, the state hires fewer
		if ref := publicWageCap * n.averageWage(); ref > 0 && r.Wage > ref {
			local *= ref / r.Wage
		}
		for _, f := range r.Firms {
			if !f.State || !f.Enabled || f.Funded <= 0 {
				continue
			}
			f.PlanLabor = float64(f.Funded) * ServiceStaff * local
			pay := f.PlanLabor * r.Wage
			n.Treasury -= pay
			f.Cash += pay
			n.SpentOn[services[f.Public].Spending] += pay
		}
	}
}

// settleServices collects the day's research and returns unspent wages.
func (n *Nation) settleServices() {
	for _, r := range n.Regions {
		for _, f := range r.Firms {
			if !f.State {
				continue
			}
			if f.Public == University {
				f.LastOutput = f.research()
				n.ResearchPoints += f.LastOutput
			}
			if f.Cash > 0 {
				n.Treasury += f.Cash
				n.SpentOn[services[f.Public].Spending] -= f.Cash
				f.Cash = 0
			}
		}
	}
}

// serviceEffects applies a day of the services' work in every tile:
// administration sets the share of taxes collected, schools move the
// educated share towards enrolment, and crowding without public health
// risks cholera.
func (w *World) serviceEffects() {
	for _, r := range w.Regions {
		if r.Nation != nil {
			r.Collect = taxFarmed + (1-taxFarmed)*w.Coverage(r, Administration)
			r.Disloyalty *= allegianceGrowth
			r.applyAllegiance()
		}
		enrolled := w.Coverage(r, Schools)
		r.Educated += (enrolled - r.Educated) / (schoolYears * TicksPerYear)
		r.Healthy = w.Coverage(r, Health)
		if c := r.Crowding(); w.Demography && c > 0 && r.Healthy < 1 {
			chance := choleraPerYear * c * (1 - r.Healthy) / TicksPerYear
			if w.chance(r.ID, 7) < chance {
				dead := math.Min(choleraDeaths*sizeOf(movable(r)), sizeOf(movable(r))-minPopulation)
				if dead > 0 {
					removePeople(r, dead)
					owner := -1
					if r.Nation != nil {
						owner = r.Nation.ID
					}
					w.emit(owner, "cholera", r.ID, "Cholera in tile %s kills %s", r.Name, fmt.Sprintf("%.0f", dead))
				}
			}
		}
	}
}

// chance is a deterministic pseudo-random number in [0, 1) for an event in
// a tile today.
func (w *World) chance(region, k int) float64 {
	seed := int64(0)
	if w.Spec != nil {
		seed = w.Spec.Seed
	}
	x := uint64(seed)*0x9E3779B97F4A7C15 ^ uint64(region+1)*0xC2B2AE3D27D4EB4F ^ uint64(w.Tick)*0x165667B19E3779F9 ^ uint64(k+1)*0x27D4EB2F165667C5
	x ^= x >> 33
	x *= 0xFF51AFD7ED558CCD
	x ^= x >> 33
	return float64(x>>11) / float64(1<<53)
}

// crowdingPenalty is the population growth a tile loses a year to crowding
// that public health doesn't cover.
func (r *Region) crowdingPenalty() float64 {
	return crowdingDeaths * r.Crowding() * (1 - r.Healthy)
}

// healthBonus is the population growth public health adds a year.
func (r *Region) healthBonus() float64 {
	return healthGrowth * r.Healthy
}

// ServiceStats are a tile's service figures for the day.
type ServiceStats struct {
	Staff, Need, Coverage float64
	Cost                  float64 // wages per funded level per day
	Research, PerLevel    float64 // universities: today, and per funded level fully staffed
}

// ServiceStats describes one of the tile's services.
func (w *World) ServiceStats(r *Region, s Public) ServiceStats {
	f := r.Service(s)
	if f == nil {
		return ServiceStats{}
	}
	st := ServiceStats{Staff: f.Labor, Need: w.ServiceNeed(r, s), Coverage: w.Coverage(r, s), Cost: ServiceStaff * r.Wage}
	if s == University {
		st.Research, st.PerLevel = f.LastOutput, f.TFP*f.Agglom*f.EduMult*math.Pow(ServiceStaff, 0.6)
	}
	return st
}

// SetServiceFunding sets how many of a tile's service levels its nation funds.
func (w *World) SetServiceFunding(region int, s Public, levels int) error {
	r := w.Regions[region]
	f := r.Service(s)
	switch {
	case r.Nation == nil:
		return ErrNoNation
	case f == nil:
		return ErrNoService
	case levels < 0 || levels > f.Levels:
		return ErrBadFunding
	}
	f.Funded = levels
	return nil
}

// CloseServiceLevel tears down one of a tile's service levels.
func (w *World) CloseServiceLevel(region int, s Public) error {
	r := w.Regions[region]
	f := r.Service(s)
	switch {
	case r.Nation == nil:
		return ErrNoNation
	case f == nil:
		return ErrNoService
	case f.Levels <= 0:
		return ErrNoLevel
	}
	f.Levels--
	f.Funded = min(f.Funded, f.Levels)
	return nil
}

// ExpandService has the tile's nation build one more level of a service
// (funded once built).
func (w *World) ExpandService(region int, s Public) error {
	r := w.Regions[region]
	for i, f := range r.Firms {
		if f.State && f.Public == s {
			return w.Expand(region, i)
		}
	}
	return ErrNoService
}

// SetFunding, CloseUniversityLevel and ExpandUniversity act on universities.
func (w *World) SetFunding(region, levels int) error {
	return w.SetServiceFunding(region, University, levels)
}
func (w *World) CloseUniversityLevel(region int) error {
	return w.CloseServiceLevel(region, University)
}
func (w *World) ExpandUniversity(region int) error { return w.ExpandService(region, University) }
