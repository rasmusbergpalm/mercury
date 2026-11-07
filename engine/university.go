package engine

import (
	"errors"
	"math"
)

// Universities are run by the state. Each tile's university has building
// levels (built with construction, like any firm's) and the nation chooses
// how many of them to fund. A funded level employs UniStaff workers at the
// tile's wage, paid from the treasury, and all the research they produce
// goes to the nation:
//
//	research = TFP * size bonus * funded^0.4 * staff^0.6
//
// so a level's research grows with technology and with the tile's size:
// universities belong in the big cities.
const (
	UniStaff       = 2.0 // workers per funded level
	uniStartLevels = 2   // levels every tile starts with (all funded)
)

// University errors.
var (
	ErrNoUniversity = errors.New("the tile has no university")
	ErrBadFunding   = errors.New("funding must be between 0 and the university's levels")
	ErrNoLevel      = errors.New("the university has no level to close")
)

// University is the tile's university, or nil.
func (r *Region) University() *Firm {
	for _, f := range r.Firms {
		if f.State {
			return f
		}
	}
	return nil
}

// research is what a university produces today with the staff it has.
func (f *Firm) research() float64 {
	if f.Funded <= 0 || f.Labor <= 0 {
		return 0
	}
	return f.TFP * f.Agglom * math.Pow(float64(f.Funded), 0.4) * math.Pow(f.Labor, 0.6)
}

// UniversityStats are a university's figures for the day: its staff and
// what it produces, and what one funded level costs and yields at full staff.
type UniversityStats struct {
	Staff, Research float64
	PerLevel, Cost  float64 // research and wages per funded level per day, fully staffed
}

// Stats describes the tile's university.
func (r *Region) UniversityStats() UniversityStats {
	f := r.University()
	if f == nil {
		return UniversityStats{}
	}
	return UniversityStats{Staff: f.Labor, Research: f.LastOutput,
		PerLevel: f.TFP * f.Agglom * math.Pow(UniStaff, 0.6), Cost: UniStaff * r.Wage}
}

// universityBill is what the nation's funded universities would cost today.
func (n *Nation) universityBill() float64 {
	s := 0.0
	for _, r := range n.Regions {
		if f := r.University(); f != nil && f.Enabled {
			s += float64(f.Funded) * UniStaff * r.Wage
		}
	}
	return s
}

// staffUniversities lets each funded university hire its staff, paying
// (scale of) their wages into it from the treasury.
func (n *Nation) staffUniversities(scale float64) {
	for _, r := range n.Regions {
		f := r.University()
		if f == nil || !f.Enabled || f.Funded <= 0 {
			continue
		}
		f.PlanLabor = float64(f.Funded) * UniStaff * scale
		pay := f.PlanLabor * r.Wage
		n.Treasury -= pay
		f.Cash += pay
		n.SpentOn[Science] += pay
	}
}

// settleUniversities collects the day's research and returns unspent wages.
func (n *Nation) settleUniversities() {
	for _, r := range n.Regions {
		f := r.University()
		if f == nil {
			continue
		}
		f.LastOutput = f.research()
		n.ResearchPoints += f.LastOutput
		if f.Cash > 0 {
			n.Treasury += f.Cash
			n.SpentOn[Science] -= f.Cash
			f.Cash = 0
		}
	}
}

// SetFunding sets how many of a tile's university levels its nation funds.
func (w *World) SetFunding(region, levels int) error {
	r := w.Regions[region]
	f := r.University()
	switch {
	case r.Nation == nil:
		return ErrNoNation
	case f == nil:
		return ErrNoUniversity
	case levels < 0 || levels > f.Levels:
		return ErrBadFunding
	}
	f.Funded = levels
	return nil
}

// CloseUniversityLevel tears down one of a tile's university levels.
func (w *World) CloseUniversityLevel(region int) error {
	r := w.Regions[region]
	f := r.University()
	switch {
	case r.Nation == nil:
		return ErrNoNation
	case f == nil:
		return ErrNoUniversity
	case f.Levels <= 0:
		return ErrNoLevel
	}
	f.Levels--
	f.Funded = min(f.Funded, f.Levels)
	return nil
}

// ExpandUniversity has the tile's nation build one more university level
// (it is funded once built).
func (w *World) ExpandUniversity(region int) error {
	r := w.Regions[region]
	for i, f := range r.Firms {
		if f.State {
			return w.Expand(region, i)
		}
	}
	return ErrNoUniversity
}
