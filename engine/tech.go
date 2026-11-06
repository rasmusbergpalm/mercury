package engine

import "fmt"

// Tech is a technology a nation can research.
type Tech struct {
	Name     string
	Cost     float64  // research units
	Requires []string // techs needed first
	Effect   string   // human-readable summary
	// FirmTFP multiplies the productivity of the named firms in the nation.
	FirmTFP map[string]float64
	// CoastMove multiplies the movement cost of the nation's coast tiles.
	CoastMove float64
	// TransitSpoilage multiplies spoilage on the way for goods leaving the nation.
	TransitSpoilage float64
}

// Techs is the tech tree, in a valid research order.
var Techs = []Tech{
	{Name: "Wagons", Cost: 500, Effect: "Transport +30%",
		FirmTFP: map[string]float64{"Transport": 1.3}},
	{Name: "Crop rotation", Cost: 500, Effect: "Farms and potato farms +20%",
		FirmTFP: map[string]float64{"Farm": 1.2, "Potato farm": 1.2}},
	{Name: "Masonry", Cost: 600, Effect: "Unlocks paved roads"},
	{Name: "Steam engine", Cost: 1200, Requires: []string{"Wagons"}, Effect: "Transport +40%, coal mines +20%",
		FirmTFP: map[string]float64{"Transport": 1.4, "Coal mine": 1.2}},
	{Name: "Steamships", Cost: 1400, Requires: []string{"Steam engine"}, Effect: "Coast tiles cost 40% less to cross",
		CoastMove: 0.6},
	{Name: "Refrigeration", Cost: 1600, Requires: []string{"Steam engine"}, Effect: "Exports spoil 60% less on the way",
		TransitSpoilage: 0.4},
	{Name: "Railways", Cost: 2000, Requires: []string{"Steam engine", "Masonry"}, Effect: "Unlocks railways"},
}

// TechID looks a tech up by name and panics if it doesn't exist.
func TechID(name string) int {
	for i, t := range Techs {
		if t.Name == name {
			return i
		}
	}
	panic(fmt.Sprintf("unknown tech %q", name))
}
