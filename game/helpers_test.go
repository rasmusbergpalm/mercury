package game

import (
	"math"
	"sort"
	"testing"

	"github.com/rasmusbergpalm/mercury/economy"
)

func relDiff(a, b float64) float64 {
	return math.Abs(a-b) / math.Max(math.Max(math.Abs(a), math.Abs(b)), 1e-6)
}

// BuildWorld builds the economy's world on a hex map (economy.HexGraph).
func BuildWorld(cost float64, rows, names []string) (*economy.World, error) {
	g, err := economy.HexGraph(rows, names)
	if err != nil {
		return nil, err
	}
	return economy.BuildWorld(cost, g)
}

// newTestWorld is the economy's world (economy.BuildWorld) as a game without
// nations.
func newTestWorld(t *testing.T, cost float64) *World {
	t.Helper()
	e, err := BuildWorld(cost, DefaultMap, nil)
	if err != nil {
		t.Fatal(err)
	}
	return wrap(e)
}

func testPrices() []float64 {
	p := make([]float64, NumGoods)
	for i := range p {
		p[i] = 1 + float64(i%7)*0.7
	}
	return p
}

// median of xs (1 if empty); xs is reordered.
func median(xs []float64) float64 {
	if len(xs) == 0 {
		return 1
	}
	sort.Float64s(xs)
	if n := len(xs); n%2 == 1 {
		return xs[n/2]
	}
	n := len(xs)
	return (xs[n/2-1] + xs[n/2]) / 2
}

// the economy's names the tests use
const (
	UnsuppliedPriceCap = economy.UnsuppliedPriceCap
	ParallelMin        = economy.ParallelMin
	PopGrowthSlope     = economy.PopGrowthSlope
	PopGrowthNeutral   = economy.PopGrowthNeutral
	MigrationRate      = economy.MigrationRate
	SizeBonusPop       = economy.SizeBonusPop
	SizeBonusExp       = economy.SizeBonusExp
	OwnerPeopleShare   = economy.OwnerPeopleShare
	LaborFactor        = economy.LaborFactor
	GoodFactor         = economy.GoodFactor
	CompositeFactor    = economy.CompositeFactor
	MinPrice           = economy.MinPrice
)

var (
	Needs             = economy.Needs
	Composites        = economy.Composites
	RawRecipes        = economy.RawRecipes
	ProcessingRecipes = economy.ProcessingRecipes
)

func MovePeople(from, to *Region, n float64) { economy.MovePeople(from.Region, to.Region, n) }
func MigrationShare(from, to *Region, gap, hop float64) float64 {
	return economy.MigrationShare(from.Region, to.Region, gap, hop)
}
