package engine

import "fmt"

// Branches of the tech tree.
const (
	BranchEconomy   = "economy"
	BranchTransport = "transport"
	BranchMilitary  = "military"
	BranchPolitics  = "politics"
	BranchScience   = "science"
)

// Branches lists the branches in display order.
var Branches = []string{BranchEconomy, BranchTransport, BranchMilitary, BranchPolitics, BranchScience}

// Tech is a technology a nation can research. Multipliers left at zero have
// no effect.
type Tech struct {
	Name     string
	Branch   string
	Cost     float64  // research units
	Requires []string // techs needed first
	Effect   string   // human-readable summary

	FirmTFP     map[string]float64 // multiplies the productivity of the named firms
	EnableFirms []string           // firms that only work for nations with this tech (see Recipe.Tech)

	CoastMove       float64 // multiplies the movement cost of the nation's coast tiles
	TransitSpoilage float64 // multiplies spoilage on the way for goods leaving the nation
	Ports           bool    // lets the nation build ports on coast tiles

	Defense     float64 // multiplies the defence of the nation's tiles
	Attack      float64 // multiplies the army's strength when occupying
	ArmyUpkeep  float64 // multiplies the army's decay per tick
	OccupySpeed float64 // multiplies how fast campaigns progress

	BuildRate     float64 // multiplies how much a project can take per tick
	InvestPayback float64 // multiplies how many ticks firms will wait for an expansion to pay off
}

// Techs is the tech tree, in a valid research order (prerequisites first).
var Techs = []Tech{
	// ---- tier 1
	{Name: "Crop rotation", Branch: BranchEconomy, Cost: 1200, Effect: "Farms and potato farms +20%",
		FirmTFP: map[string]float64{"Farm": 1.2, "Potato farm": 1.2}},
	{Name: "Viticulture", Branch: BranchEconomy, Cost: 1200, Effect: "Wineries can operate",
		EnableFirms: []string{"Winery"}},
	{Name: "Wagons", Branch: BranchTransport, Cost: 1200, Effect: "Transport +30%",
		FirmTFP: map[string]float64{"Transport": 1.3}},
	{Name: "Navigation", Branch: BranchTransport, Cost: 1200, Effect: "Coast tiles cost 20% less to cross",
		CoastMove: 0.8},
	{Name: "Drill", Branch: BranchMilitary, Cost: 1200, Effect: "Barracks +30%",
		FirmTFP: map[string]float64{"Barracks": 1.3}},
	{Name: "Administration", Branch: BranchPolitics, Cost: 1200, Effect: "Occupations progress 50% faster",
		OccupySpeed: 1.5},
	{Name: "Printing press", Branch: BranchScience, Cost: 1200, Effect: "Universities +25%",
		FirmTFP: map[string]float64{"University": 1.25}},

	// ---- tier 2
	{Name: "Ceramics", Branch: BranchEconomy, Cost: 2400, Effect: "Kilns can operate; clay pits +20%",
		EnableFirms: []string{"Kiln"}, FirmTFP: map[string]float64{"Clay pit": 1.2}},
	{Name: "Sugar refining", Branch: BranchEconomy, Cost: 2400, Effect: "Refineries can operate",
		EnableFirms: []string{"Refinery"}},
	{Name: "Masonry", Branch: BranchTransport, Cost: 2400, Effect: "Unlocks paved roads; construction +20%",
		FirmTFP: map[string]float64{"Construction": 1.2}},
	{Name: "Ocean sailing", Branch: BranchTransport, Cost: 2700, Requires: []string{"Navigation"},
		Effect: "Ports on coast tiles: ships sail straight to every other port", Ports: true},
	{Name: "Fortifications", Branch: BranchMilitary, Cost: 2400, Requires: []string{"Drill"}, Effect: "Defence +50%",
		Defense: 1.5},
	{Name: "Bureaucracy", Branch: BranchPolitics, Cost: 2400, Requires: []string{"Administration"},
		Effect:    "Construction +40%; projects and expansions can take twice as much per tick",
		BuildRate: 2, FirmTFP: map[string]float64{"Construction": 1.4}},
	{Name: "Scientific method", Branch: BranchScience, Cost: 2700, Requires: []string{"Printing press"},
		Effect: "Universities +30%", FirmTFP: map[string]float64{"University": 1.3}},

	// ---- tier 3
	{Name: "Silk weaving", Branch: BranchEconomy, Cost: 4200, Requires: []string{"Ceramics"},
		Effect: "Couturiers can operate; silk farms +20%", EnableFirms: []string{"Couturier"},
		FirmTFP: map[string]float64{"Silk farm": 1.2}},
	{Name: "Steam engine", Branch: BranchTransport, Cost: 4200, Requires: []string{"Wagons"},
		Effect: "Transport +40%, coal mines +20%", FirmTFP: map[string]float64{"Transport": 1.4, "Coal mine": 1.2}},
	{Name: "Professional army", Branch: BranchMilitary, Cost: 4200, Requires: []string{"Drill"},
		Effect: "Army upkeep: decays half as fast", ArmyUpkeep: 0.5},
	{Name: "Banking", Branch: BranchPolitics, Cost: 4200, Requires: []string{"Bureaucracy"},
		Effect: "Firms invest more readily (payback horizon +50%)", InvestPayback: 1.5},
	{Name: "Nationalism", Branch: BranchPolitics, Cost: 4200, Requires: []string{"Administration"},
		Effect: "Defence +25%, occupations 25% faster", Defense: 1.25, OccupySpeed: 1.25},
	{Name: "Public education", Branch: BranchScience, Cost: 4200, Requires: []string{"Scientific method"},
		Effect: "Universities +30%, all processing +5%",
		FirmTFP: map[string]float64{"University": 1.3, "Bakery": 1.05, "Brewery": 1.05, "Mill": 1.05,
			"Tailor": 1.05, "Smelter": 1.05, "Smithy": 1.05, "Carpenter": 1.05, "Refinery": 1.05,
			"Winery": 1.05, "Couturier": 1.05, "Kiln": 1.05}},

	// ---- tier 4
	{Name: "Mechanized textiles", Branch: BranchEconomy, Cost: 6600, Requires: []string{"Steam engine"},
		Effect: "Textile factories can operate", EnableFirms: []string{"Textile factory"}},
	{Name: "Steel", Branch: BranchEconomy, Cost: 6600, Requires: []string{"Steam engine"},
		Effect: "Steel works can operate; iron mines +30%", EnableFirms: []string{"Steel works"},
		FirmTFP: map[string]float64{"Iron mine": 1.3}},
	{Name: "Steamships", Branch: BranchTransport, Cost: 6600, Requires: []string{"Steam engine", "Ocean sailing"},
		Effect: "Coast tiles and sea routes cost 40% less", CoastMove: 0.6},
	{Name: "Refrigeration", Branch: BranchTransport, Cost: 6600, Requires: []string{"Steam engine"},
		Effect: "Exports spoil 60% less on the way", TransitSpoilage: 0.4},
	{Name: "Railways", Branch: BranchTransport, Cost: 7800, Requires: []string{"Steam engine", "Masonry"},
		Effect: "Unlocks railways"},
	{Name: "Rifles", Branch: BranchMilitary, Cost: 6600, Requires: []string{"Professional army", "Steel"},
		Effect: "Army strength +50% when occupying", Attack: 1.5},
	{Name: "Engineering", Branch: BranchScience, Cost: 6600, Requires: []string{"Public education", "Steam engine"},
		Effect:  "Construction +30%, smelters and smithies +20%",
		FirmTFP: map[string]float64{"Construction": 1.3, "Smelter": 1.2, "Smithy": 1.2}},
}

var techIndex = func() map[string]int {
	m := map[string]int{}
	for i, t := range Techs {
		m[t.Name] = i
	}
	return m
}()

// TechID looks a tech up by name and panics if it doesn't exist.
func TechID(name string) int {
	i, ok := techIndex[name]
	if !ok {
		panic(fmt.Sprintf("unknown tech %q", name))
	}
	return i
}

// techProduct multiplies the non-zero values of one effect over the techs a
// nation knows.
func (n *Nation) techProduct(effect func(t *Tech) float64) float64 {
	m := 1.0
	for i := range Techs {
		if n.Known[i] {
			if x := effect(&Techs[i]); x != 0 {
				m *= x
			}
		}
	}
	return m
}

func (n *Nation) firmMult(firm string) float64 {
	return n.techProduct(func(t *Tech) float64 { return t.FirmTFP[firm] })
}
func (n *Nation) coastMult() float64 {
	return n.techProduct(func(t *Tech) float64 { return t.CoastMove })
}
func (n *Nation) spoilageMult() float64 {
	return n.techProduct(func(t *Tech) float64 { return t.TransitSpoilage })
}
func (n *Nation) defenseMult() float64 {
	return n.techProduct(func(t *Tech) float64 { return t.Defense })
}
func (n *Nation) attackMult() float64 {
	return n.techProduct(func(t *Tech) float64 { return t.Attack })
}
func (n *Nation) upkeepMult() float64 {
	return n.techProduct(func(t *Tech) float64 { return t.ArmyUpkeep })
}
func (n *Nation) occupyMult() float64 {
	return n.techProduct(func(t *Tech) float64 { return t.OccupySpeed })
}
func (n *Nation) buildRateMult() float64 {
	return n.techProduct(func(t *Tech) float64 { return t.BuildRate })
}
func (n *Nation) paybackMult() float64 {
	return n.techProduct(func(t *Tech) float64 { return t.InvestPayback })
}

// CanBuildPorts reports whether the nation has a tech that allows ports.
func (n *Nation) CanBuildPorts() bool {
	for i := range Techs {
		if n.Known[i] && Techs[i].Ports {
			return true
		}
	}
	return false
}

// firmEnabled reports whether a firm works for this owner (nil: unowned).
func firmEnabled(f *Firm, owner *Nation) bool {
	if f.Requires == "" {
		return true
	}
	return owner != nil && owner.Has(f.Requires)
}
