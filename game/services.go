package game

import (
	"errors"
	"fmt"
	"math"

	"github.com/rasmusbergpalm/mercury/economy"
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
//     its industry and services somewhat more productive, and its
//     universities much more;
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
	adminPerHead     = 1.0 / 60 // officials needed per person in the capital
	adminPerHop      = 0.2      // ...and 20% more per hex from it
	pupilsPerTeacher = 40.0     // a teacher's class
	childShare       = 0.3      // share of people of school age
	// The educated share follows enrolment as schooled children grow up and
	// replace the unschooled: over a working life, so a nation that schools
	// every child from 1820 is over half educated by 1850 and nine in ten
	// by 1900, as the most literate nations of the century were.
	schoolYears = 40.0
	// Scholars come from the schooled: a university in an unschooled tile
	// finds half as much as the tech costs reckon on, in a fully educated
	// one one and a half times as much (three times the unschooled).
	scienceUnschooled = 0.5
	scienceEducation  = 1.0
	doctorPerHead     = 1.0 / 400 // health staff a tile needs per person...
	crowdedDoctors    = 1.0 / 150 // ...and this many more per person when fully crowded
	choleraPerYear    = 0.08      // chance of cholera a year in such a tile
	choleraDeaths     = 0.06      // share of its workers cholera kills
	serviceMaxLevels  = 12
	maxPublicShare    = 0.3 // most of a tile's workers its public services may employ
	publicWageCap     = 1.5 // above this times the nation's average wage, they hire fewer
	taxFarmed         = 0.3 // share of taxes collected without any officials (tax farmers, the gentry)
)

// Service errors.
var (
	ErrNoService  = errors.New("the tile has no such service")
	ErrBadFunding = errors.New("funding must be between 0 and the service's levels")
	ErrNoLevel    = errors.New("there is no level to close")
)

// ServiceNames lists the services' names, in order.
func ServiceNames() []string {
	var out []string
	for s := Public(0); s < NumServices; s++ {
		out = append(out, services[s].Name)
	}
	return out
}

// PublicService is one of a tile's public services: its building levels (built
// with construction, like a firm's), how many of them its nation funds, and
// the outside employer (economy.Buyer) through which it hires its staff in
// the tile's labour market.
type PublicService struct {
	Kind              Public
	Levels, MaxLevels int
	Funded            int
	Expansion         *Expansion // a level being built, or nil
	LastOutput        float64    // universities: research, the last day
	employer          *economy.Buyer
}

// Staff is the workers the service employs today.
func (s *PublicService) Staff() float64 { return s.employer.Hired }

// newService makes a tile's service of a kind, hiring in r's labour market.
func newService(r *Region, k Public) *PublicService {
	s := &PublicService{Kind: k, Levels: services[k].Start, Funded: services[k].Start, MaxLevels: serviceMaxLevels, employer: economy.NewBuyer()}
	r.Buyers = append(r.Buyers, s.employer)
	return s
}

// Service is the tile's service of a kind, or nil.
func (r *Region) Service(s Public) *PublicService { return r.Services[s] }

// University is the tile's university, or nil.
func (r *Region) University() *PublicService { return r.Service(University) }

// science is how much the tile's scholars find: scholars come from the
// schooled.
func (r *Region) science() float64 {
	return scienceUnschooled + scienceEducation*r.Educated
}

// research is what a university produces today with the staff it has: more
// in a big tile, a schooled one and a loyal one.
func (r *Region) research(u *PublicService) float64 {
	if u.Funded <= 0 || u.Staff() <= 0 {
		return 0
	}
	return r.SizeBonus() * r.science() * r.WorkMult() * r.Nation.staffMult(University) * math.Pow(float64(u.Funded), 0.4) * math.Pow(u.Staff(), 0.6)
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
		case University:
			return t.ResearchMult
		}
		return 0
	})
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
	return math.Min(1, f.Staff()/need)
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
		for _, sv := range r.Services {
			if sv != nil {
				s += float64(sv.Funded) * ServiceStaff * r.Wage
			}
		}
	}
	return s
}

// staffServices has each funded service hire its staff (scale of them),
// paid from the treasury once the labour market has cleared (collect).
func (n *Nation) staffServices(scale float64) {
	for _, r := range n.Regions {
		// the services may employ at most maxPublicShare of the tile's workers
		want := 0.0
		for _, sv := range r.Services {
			if sv != nil {
				want += float64(sv.Funded) * ServiceStaff
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
		for _, sv := range r.Services {
			if sv != nil {
				sv.employer.Hire = float64(max(sv.Funded, 0)) * ServiceStaff * local
			}
		}
	}
}

// settleServices collects the day's research.
func (n *Nation) settleServices() {
	for _, r := range n.Regions {
		if u := r.University(); u != nil {
			u.LastOutput = r.research(u)
			n.ResearchPoints += u.LastOutput
			if n.Copying >= 0 {
				n.CopyPoints += u.LastOutput
			}
		}
	}
}

// buildServices finishes the services' levels whose construction is
// delivered: funded once built.
func (w *World) buildServices() {
	for _, r := range w.Regions {
		for _, sv := range r.Services {
			if sv == nil || sv.Expansion == nil || sv.Expansion.Delivered < sv.Expansion.Cost()-1e-9 {
				continue
			}
			sv.Expansion = nil
			sv.Levels++
			sv.Funded++
		}
	}
}

// educate moves a tile's educated share a day towards its enrolment.
func educate(educated, enrolled float64) float64 {
	return educated + (enrolled-educated)/(schoolYears*TicksPerYear)
}

// serviceEffects applies a day of the services' work in every tile:
// administration sets the share of taxes collected, schools move the
// educated share towards enrolment, and crowding without public health
// risks cholera.
func (w *World) serviceEffects() {
	for _, r := range w.Regions {
		if r.Nation != nil {
			r.Collect = taxFarmed + (1-taxFarmed)*w.Coverage(r, Administration)
			r.driftLoyalties()
		}
		r.Educated = educate(r.Educated, w.Coverage(r, Schools))
		r.Healthy = w.Coverage(r, Health)
		if c := r.Crowding(); w.Demography && c > 0 && r.Healthy < 1 {
			chance := choleraPerYear * c * (1 - r.Healthy) / TicksPerYear
			if w.chance(r.ID, 7) < chance {
				dead := math.Min(choleraDeaths*sizeOf(movable(r.Region)), sizeOf(movable(r.Region))-minPopulation)
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
	st := ServiceStats{Staff: f.Staff(), Need: w.ServiceNeed(r, s), Coverage: w.Coverage(r, s), Cost: ServiceStaff * r.Wage}
	if s == University {
		st.Research, st.PerLevel = f.LastOutput, r.SizeBonus()*r.science()*math.Pow(ServiceStaff, 0.6)
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
	sv := r.Service(s)
	switch {
	case r.Nation == nil:
		return ErrNoNation
	case sv == nil:
		return ErrNoService
	case sv.Expansion != nil:
		return ErrExpanding
	case sv.Levels >= sv.MaxLevels:
		return ErrFirmMax
	}
	sv.Expansion = &Expansion{External: true}
	return nil
}

// SetFunding funds a tile's university (SetServiceFunding).
func (w *World) SetFunding(region, levels int) error {
	return w.SetServiceFunding(region, University, levels)
}
