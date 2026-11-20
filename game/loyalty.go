package game

import "math"

// Allegiance: a tile's people hold a share of loyalty for each nation, and
// for nobody, adding up to 1. A nation's starting tiles stand wholly by it,
// free land by nobody. Day by day the shares drift towards whoever owns the
// tile (a time constant of allegianceYears), so a conquered tile starts out
// loyal to its old owner, and one lost and retaken soon still mostly stands
// by the nation that retook it. Its allegiance to its owner is the owner's
// share plus freeLandAllegiance of nobody's (people who answer to no one are
// halfway willing); until it is whole the tile pays less tax and gives
// fewer recruits, and, while people there stand by another nation, produces
// less (WorkMult).
const (
	allegianceYears    = 8.0
	freeLandAllegiance = 0.4
)

// allegianceDrift is the share of the other loyalties that moves to the
// owner each day.
var allegianceDrift = 1 - math.Exp(-1/(allegianceYears*TicksPerYear))

// initLoyalties gives every tile its starting loyalties: wholly to its
// owner, or to nobody.
func (w *World) initLoyalties() {
	for _, r := range w.Regions {
		r.Loyalties = make([]float64, len(w.Nations)+1)
		if r.Nation != nil {
			r.Loyalties[r.Nation.ID] = 1
		} else {
			r.Loyalties[len(w.Nations)] = 1
		}
		r.applyAllegiance()
	}
}

// driftLoyalties moves a day's share of the tile's loyalties to its owner.
func (r *Region) driftLoyalties() {
	if r.Nation == nil || r.Loyalties == nil {
		return
	}
	for i := range r.Loyalties {
		if i == r.Nation.ID {
			r.Loyalties[i] += allegianceDrift * (1 - r.Loyalties[i])
		} else {
			r.Loyalties[i] *= 1 - allegianceDrift
		}
	}
	r.applyAllegiance()
}

// Allegiance is how far the tile's people stand by their owner, 0..1 (1 on
// free land and outside games with nations: nobody to stand by).
func (r *Region) Allegiance() float64 {
	if r.Nation == nil || r.Loyalties == nil {
		return 1
	}
	return r.Loyalties[r.Nation.ID] + freeLandAllegiance*r.Loyalties[len(r.Loyalties)-1]
}

// AllegianceMult is what allegiance does to the tile's taxes: half at none,
// all of it when loyal.
func (r *Region) AllegianceMult() float64 { return 0.5 + 0.5*r.Allegiance() }

// WorkMult is what allegiance does to the tile's production: people loyal
// to another nation work half-heartedly for their new masters (a share s of
// them costs s/2 of the output); people who answered to nobody work as they
// did (a free tile taken over doesn't become poorer for it).
func (r *Region) WorkMult() float64 {
	if r.Nation == nil || r.Loyalties == nil {
		return 1
	}
	rival := 1 - r.Loyalties[r.Nation.ID] - r.Loyalties[len(r.Loyalties)-1]
	return 1 - 0.5*math.Max(0, rival)
}

// applyAllegiance sets the tile's firms' productivity (the economy's
// Mult) from its allegiance and its schooling.
func (r *Region) applyAllegiance() {
	m := r.WorkMult()
	for _, f := range r.Firms {
		f.Mult = m * r.eduMult(f)
	}
}
