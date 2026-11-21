package game

import "math"

// Schooling and public health are the game's: a tile's educated share and
// its public-health cover, which its schools and doctors move (see
// services.go). What they do to the economy goes through its levers: a
// schooled tile's industry and services are more productive (each firm's
// Mult), and its people have fewer children; a crowded tile its public
// health doesn't cover loses people, and covered tiles gain (each tile's
// GrowthBonus).
const (
	StartEducated  = 0.1   // the educated share in 1820
	EducationBonus = 0.15  // industry and services of a fully educated tile: +15%
	crowdingFrom   = 150.0 // people from which a tile is crowded
	crowdingSpan   = 300.0 // ...and fully crowded this many more
	HealthCover    = 0.006 // population growth a year a fully covered tile gains
	CrowdingDeaths = 0.02  // growth lost a year by a fully crowded, uncovered tile
	// SchooledBirths: schooled people have fewer children (the demographic
	// transition: birth rates fell about a third as literacy rose from half
	// to nearly all), so growth falls from about 1.4% a year in 1820 to
	// under 1% as a nation schools everyone.
	SchooledBirths = 0.012
)

// Crowding is how crowded the tile is, 0 (not at all) to 1.
func (r *Region) Crowding() float64 {
	return math.Max(0, math.Min(1, (r.Population()-crowdingFrom)/crowdingSpan))
}

// CrowdingPenalty is the population growth a tile loses a year to crowding
// that public health doesn't cover.
func (r *Region) CrowdingPenalty() float64 {
	return CrowdingDeaths * r.Crowding() * (1 - r.Healthy)
}

// HealthBonus is the population growth public health adds a year.
func (r *Region) HealthBonus() float64 {
	return HealthCover * r.Healthy
}

// HealthGrowth is what public health, or the lack of it, does to the tile's
// population growth a year: the bonus of its coverage less what uncovered
// crowding costs. Negative means the tile needs more public health.
func (r *Region) HealthGrowth() float64 {
	return r.HealthBonus() - r.CrowdingPenalty()
}

// growthBonus is what the game adds to the tile's natural growth (the
// economy's GrowthBonus, a tick's): its nation's medicine and its public
// health move growth within the natural limits (people can't multiply
// faster than they can), and schooling lowers it beyond them. tech, the
// health and schooling figures are a year's.
func (r *Region) growthBonus(tech float64) float64 {
	clamp := func(g float64) float64 { return math.Max(PopGrowthMin, math.Min(PopGrowthMax, g)) }
	nat := r.NaturalGrowth() * TicksPerYear
	return (clamp(nat+r.HealthGrowth()+tech) - clamp(nat) - SchooledBirths*r.Educated) / TicksPerYear
}

// eduMult is what the tile's schooling does to a firm's productivity:
// industry and services gain, land-bound producers don't.
func (r *Region) eduMult(f *Firm) float64 {
	if f.Sector == Primary {
		return 1
	}
	return 1 + EducationBonus*r.Educated
}
