// Package starlanes is an economy far from Mercury's 1820 example, to show
// the engine's range: twelve star systems joined by a sparse web of lanes,
// in which every ship between systems burns aether, a fuel found in only
// one of them. Nothing in the engine changes; the goods, needs, recipes
// and kinds of system are this directory's data files, and the map is a
// graph of systems and lanes.
//
// Freight, the transport merchants buy, is made from labour and aether. A
// system with no aether can still receive goods but can't send any, so the
// systems' trade, and with it their living standards, hang on the aether
// trade out of Cinder, the one system with aether wells.
//
// The economy counts ticks. Its people and owners are calibrated for a
// tick of a day (they spend a twentieth of their savings a tick, and take
// 90 ticks to build), so here a tick is a standard day, from 2300.
//
//	go run ./examples/starlanes/run
package starlanes

import (
	"embed"
	"io/fs"

	"github.com/rasmusbergpalm/mercury/economy"
)

//go:embed data/*.json
var files embed.FS

// Data is the economy's data files (goods, needs, recipes and kinds of
// system), for economy.LoadData.
var Data = func() fs.FS {
	sub, err := fs.Sub(files, "data")
	if err != nil {
		panic(err)
	}
	return sub
}()

// The calendar: a tick is a day, from 2300.
const (
	TicksPerYear = 365
	StartYear    = 2300
)

// PeopleUnit is how many people one of the economy's people stands for.
// The engine's unit of people is arbitrary (one supplies a unit of labour),
// and the economy on top says what it is, as it says what a tick is: here,
// a billion, so that the empire holds hundreds of billions.
const PeopleUnit = 1e9

// Systems are the star systems, and Lanes the lanes between them (a lane's
// length scales the aether it takes to cross). Cinder, a barren world, has
// the only aether wells.
var (
	Systems = []economy.Place{
		{Name: "Cinder", Terrain: "barren", People: 54},
		{Name: "Halcyon", Terrain: "garden", People: 54},
		{Name: "Tamsin", Terrain: "ocean", People: 54},
		{Name: "Vey", Terrain: "forge", People: 54},
		{Name: "Corran", Terrain: "ice", People: 54},
		{Name: "Pellam", Terrain: "garden", People: 54},
		{Name: "Drace", Terrain: "forge", People: 54},
		{Name: "Mirrow", Terrain: "ocean", People: 54},
		{Name: "Saltreach", Terrain: "ice", People: 54},
		{Name: "Umber", Terrain: "garden", People: 54},
		{Name: "Quillon", Terrain: "forge", People: 54},
		{Name: "Isk", Terrain: "ocean", People: 54},
	}
	Lanes = []economy.Lane{
		{A: 0, B: 1, Length: 1.5}, {A: 0, B: 3, Length: 2},
		{A: 1, B: 2, Length: 1}, {A: 1, B: 5, Length: 2},
		{A: 2, B: 4, Length: 1.5}, {A: 3, B: 6, Length: 1},
		{A: 5, B: 6, Length: 1.5}, {A: 5, B: 7, Length: 1},
		{A: 4, B: 8, Length: 2}, {A: 7, B: 9, Length: 1.5},
		{A: 8, B: 9, Length: 1}, {A: 9, B: 10, Length: 2},
		{A: 6, B: 10, Length: 2.5}, {A: 10, B: 11, Length: 1.5},
	}
	// Layout is where each system is drawn, in a unit square (x right, y down).
	Layout = [][2]float64{
		{0.10, 0.48}, {0.28, 0.24}, {0.47, 0.08}, {0.27, 0.74}, {0.68, 0.10}, {0.47, 0.44},
		{0.47, 0.78}, {0.66, 0.40}, {0.89, 0.20}, {0.85, 0.47}, {0.72, 0.80}, {0.93, 0.88},
	}
)

// Source is Cinder, the system with the aether wells.
const Source = 0

// Load makes the economy this one (economy.LoadData).
func Load() error { return economy.LoadData(Data) }

// Build builds the systems and lanes, at the normal cost of transport, with
// owners investing and people free to move. Load the data first.
func Build() (*economy.World, error) {
	w, err := economy.BuildWorld(1, economy.Graph{Places: Systems, Lanes: Lanes})
	if err != nil {
		return nil, err
	}
	w.Investment, w.Migration = true, true
	return w, nil
}

// Riot sets the productivity of Cinder's aether wells to mult of what it
// was (riots: 0.15 is 85% less; 1 when they end).
func Riot(w *economy.World, mult float64) {
	for _, f := range w.Regions[Source].Firms {
		if f.Output == economy.G("aether") {
			f.Mult = mult
		}
	}
}

// Hops counts the lanes between each system and Cinder.
func Hops() []int {
	hops := make([]int, len(Systems))
	for i := range hops {
		hops[i] = -1
	}
	hops[Source] = 0
	for queue := []int{Source}; len(queue) > 0; queue = queue[1:] {
		a := queue[0]
		for _, l := range Lanes {
			for _, e := range [][2]int{{l.A, l.B}, {l.B, l.A}} {
				if e[0] == a && hops[e[1]] < 0 {
					hops[e[1]] = hops[a] + 1
					queue = append(queue, e[1])
				}
			}
		}
	}
	return hops
}
