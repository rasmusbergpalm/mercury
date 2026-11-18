package economy

import "math"

// Schooling and public health: a tile's educated share makes its industry
// and services more productive and its people have fewer children; its
// crowding, where public health doesn't cover it, costs it people. The
// economy takes the two shares (Region.Educated, Region.Healthy) as given:
// a government's schools and doctors move them.
const (
	StartEducated  = 0.1   // the educated share in 1820
	EducationBonus = 0.15  // industry and services of a fully educated tile: +15%
	crowdingFrom   = 150.0 // people from which a tile is crowded
	crowdingSpan   = 300.0 // ...and fully crowded this many more
	HealthCover   = 0.006 // population growth a year a fully covered tile gains
	CrowdingDeaths = 0.02  // growth lost a year by a fully crowded, uncovered tile
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
	g := -r.CrowdingPenalty()
	if r.Urban { // full worlds: public health
		g += r.HealthBonus()
	}
	return g
}
