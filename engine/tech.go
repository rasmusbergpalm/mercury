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

// Eras group techs by period; costs rise with each.
var Eras = []string{"Restoration (1820s–40s)", "Railway age (1840s–60s)", "Second industrial (1870s–90s)", "Machine age (1900s–1910s)"}

// Tech is a technology a nation can research. Multipliers left at zero have
// no effect.
type Tech struct {
	Name     string
	Branch   string
	Era      int
	Cost     float64  // research units
	Requires []string // techs needed first
	Effect   string   // human-readable summary

	FirmTFP     map[string]float64 // multiplies the productivity of the named firms
	PrimaryTFP  float64            // multiplies every land-bound producer (farms, mines, ...)
	IndustryTFP float64            // multiplies every manufacturing firm
	EnableFirms []string           // firms that only work for nations with this tech

	CoastMove       float64 // multiplies the movement cost of the nation's coast tiles
	TransitSpoilage float64 // multiplies spoilage on the way for goods leaving the nation
	Ports           bool    // lets the nation build ports on coast tiles

	Doctrine        string  // unlocks this army doctrine
	Mobilization    float64 // raises the share of the population that can serve (the highest counts)
	ArmySize        float64 // raises how many men one army can field (the highest counts)
	Defense         float64 // multiplies defenders' strength and slows sieges of the nation's tiles
	Attack          float64 // multiplies the army's strength
	Desertion       float64 // multiplies losses to hunger and desertion
	OccupySpeed     float64 // multiplies how fast sieges progress
	SupplyTransport float64 // multiplies the transport army supplies need

	BuildRate     float64 // multiplies how much a project can take per tick
	InvestPayback float64 // multiplies how long firms will wait for an expansion to pay off
	PopGrowth     float64 // adds to population growth per year
	Diffusion     float64 // multiplies the discount on techs known by trade partners
}

func firms(pairs ...any) map[string]float64 {
	m := map[string]float64{}
	for i := 0; i < len(pairs); i += 2 {
		m[pairs[i].(string)] = pairs[i+1].(float64)
	}
	return m
}

// Research costs per era.
const (
	era1 = 25000.0
	era2 = 90000.0
	era3 = 220000.0
	era4 = 650000.0
)

// Techs is the tech tree, in a valid research order (prerequisites first).
var Techs = []Tech{
	// ---- era 1: Restoration, 1820s–40s
	{Name: "Crop rotation", Branch: BranchEconomy, Era: 0, Cost: era1, Effect: "Farms and potato farms +20%",
		FirmTFP: firms("Farm", 1.2, "Potato farm", 1.2)},
	{Name: "Viticulture", Branch: BranchEconomy, Era: 0, Cost: era1, Effect: "Wineries can operate",
		EnableFirms: []string{"Winery"}},
	{Name: "Ceramics", Branch: BranchEconomy, Era: 0, Cost: era1, Effect: "Kilns can operate; clay pits +20%",
		EnableFirms: []string{"Kiln"}, FirmTFP: firms("Clay pit", 1.2)},
	{Name: "Sugar refining", Branch: BranchEconomy, Era: 0, Cost: era1, Effect: "Refineries can operate",
		EnableFirms: []string{"Refinery"}},
	{Name: "Factory system", Branch: BranchEconomy, Era: 0, Cost: era1 * 1.3, Effect: "All industry +20%",
		IndustryTFP: 1.2},
	{Name: "Wagons", Branch: BranchTransport, Era: 0, Cost: era1, Effect: "Transport +30%",
		FirmTFP: firms("Transport", 1.3)},
	{Name: "Navigation", Branch: BranchTransport, Era: 0, Cost: era1, Effect: "Coast tiles cost 20% less to cross",
		CoastMove: 0.8},
	{Name: "Masonry", Branch: BranchTransport, Era: 0, Cost: era1, Effect: "Unlocks paved roads; construction +20%",
		FirmTFP: firms("Construction", 1.2)},
	{Name: "Drill", Branch: BranchMilitary, Era: 0, Cost: era1, Effect: "Line infantry doctrine; armies of up to 20 men",
		Doctrine: "Line infantry", ArmySize: 20},
	{Name: "Field artillery", Branch: BranchMilitary, Era: 0, Cost: era1, Requires: []string{"Drill"},
		Effect: "Foundries +30%; attack +10%", FirmTFP: firms("Foundry", 1.3), Attack: 1.1},
	{Name: "Fortifications", Branch: BranchMilitary, Era: 0, Cost: era1, Effect: "Defence +50%",
		Defense: 1.5},
	{Name: "Administration", Branch: BranchPolitics, Era: 0, Cost: era1, Effect: "Sieges progress 50% faster",
		OccupySpeed: 1.5},
	{Name: "Nationalism", Branch: BranchPolitics, Era: 0, Cost: era1, Requires: []string{"Administration"},
		Effect: "Defence +25%, sieges 25% faster", Defense: 1.25, OccupySpeed: 1.25},
	{Name: "Printing press", Branch: BranchScience, Era: 0, Cost: era1, Effect: "Universities +25%",
		FirmTFP: firms("University", 1.25)},

	// ---- era 2: Railway age, 1840s–60s
	{Name: "Steam power", Branch: BranchEconomy, Era: 1, Cost: era2, Requires: []string{"Factory system"},
		Effect: "All industry +30%, coal mines +20%", IndustryTFP: 1.3, FirmTFP: firms("Coal mine", 1.2)},
	{Name: "Mechanized textiles", Branch: BranchEconomy, Era: 1, Cost: era2, Requires: []string{"Steam power"},
		Effect: "Textile factories can operate; mills +20%", EnableFirms: []string{"Textile factory"},
		FirmTFP: firms("Mill", 1.2)},
	{Name: "Silk weaving", Branch: BranchEconomy, Era: 1, Cost: era2, Requires: []string{"Ceramics"},
		Effect: "Couturiers can operate; silk farms +20%", EnableFirms: []string{"Couturier"},
		FirmTFP: firms("Silk farm", 1.2)},
	{Name: "Seed drill", Branch: BranchEconomy, Era: 1, Cost: era2, Requires: []string{"Crop rotation"},
		Effect: "Farms and potato farms +25%", FirmTFP: firms("Farm", 1.25, "Potato farm", 1.25)},
	{Name: "Interchangeable parts", Branch: BranchEconomy, Era: 1, Cost: era2, Requires: []string{"Factory system"},
		Effect: "All industry +15%, armories +30%", IndustryTFP: 1.15, FirmTFP: firms("Armory", 1.3)},
	{Name: "Ocean sailing", Branch: BranchTransport, Era: 1, Cost: era2, Requires: []string{"Navigation"},
		Effect: "Ports on coast tiles: ships sail straight to every other port", Ports: true},
	{Name: "Steam engine", Branch: BranchTransport, Era: 1, Cost: era2, Requires: []string{"Wagons"},
		Effect: "Transport +40%", FirmTFP: firms("Transport", 1.4)},
	{Name: "Railways", Branch: BranchTransport, Era: 1, Cost: era2 * 1.2, Requires: []string{"Steam engine", "Masonry"},
		Effect: "Unlocks railways"},
	{Name: "Mass conscription", Branch: BranchMilitary, Era: 1, Cost: era2, Requires: []string{"Drill"},
		Effect: "Napoleonic corps doctrine; up to 4% of people can serve; armies of up to 40 men", Doctrine: "Napoleonic corps",
		Mobilization: 0.04, ArmySize: 40},
	{Name: "Rifled muskets", Branch: BranchMilitary, Era: 1, Cost: era2, Requires: []string{"Mass conscription", "Interchangeable parts"},
		Effect: "Rifle infantry doctrine", Doctrine: "Rifle infantry"},
	{Name: "Military logistics", Branch: BranchMilitary, Era: 1, Cost: era2, Requires: []string{"Mass conscription"},
		Effect: "Supplies need 30% less transport; desertion −30%; armies of up to 60 men", SupplyTransport: 0.7, Desertion: 0.7,
		ArmySize: 60},
	{Name: "Bureaucracy", Branch: BranchPolitics, Era: 1, Cost: era2, Requires: []string{"Administration"},
		Effect: "Construction +40%; projects can take twice as much per day", BuildRate: 2,
		FirmTFP: firms("Construction", 1.4)},
	{Name: "Banking", Branch: BranchPolitics, Era: 1, Cost: era2, Requires: []string{"Bureaucracy"},
		Effect: "Firms invest more readily (payback horizon +50%)", InvestPayback: 1.5},
	{Name: "Scientific method", Branch: BranchScience, Era: 1, Cost: era2, Requires: []string{"Printing press"},
		Effect: "Universities +30%", FirmTFP: firms("University", 1.3)},
	{Name: "Public education", Branch: BranchScience, Era: 1, Cost: era2, Requires: []string{"Scientific method"},
		Effect: "Universities +30%, all industry +5%", FirmTFP: firms("University", 1.3), IndustryTFP: 1.05},

	// ---- era 3: Second industrial revolution, 1870s–90s
	{Name: "Steel", Branch: BranchEconomy, Era: 2, Cost: era3, Requires: []string{"Steam power"},
		Effect: "Steel works can operate; iron mines +30%", EnableFirms: []string{"Steel works"},
		FirmTFP: firms("Iron mine", 1.3)},
	{Name: "Fertilizers", Branch: BranchEconomy, Era: 2, Cost: era3, Requires: []string{"Seed drill"},
		Effect: "Farms, potato farms and ranches +30%, plantations +20%",
		FirmTFP: firms("Farm", 1.3, "Potato farm", 1.3, "Ranch", 1.3, "Cotton plantation", 1.2,
			"Sugar plantation", 1.2, "Tea plantation", 1.2)},
	{Name: "Industrial chemistry", Branch: BranchEconomy, Era: 2, Cost: era3, Requires: []string{"Steel"},
		Effect: "Munitions plants can operate; all industry +10%", EnableFirms: []string{"Munitions plant"},
		IndustryTFP: 1.1},
	{Name: "Electricity", Branch: BranchEconomy, Era: 2, Cost: era3, Requires: []string{"Steel"},
		Effect: "All industry +40%", IndustryTFP: 1.4},
	{Name: "Petroleum", Branch: BranchEconomy, Era: 2, Cost: era3, Requires: []string{"Industrial chemistry"},
		Effect: "Oil wells can operate", EnableFirms: []string{"Oil well"}},
	{Name: "Steamships", Branch: BranchTransport, Era: 2, Cost: era3, Requires: []string{"Steam engine", "Ocean sailing"},
		Effect: "Coast tiles and sea routes cost 40% less", CoastMove: 0.6},
	{Name: "Refrigeration", Branch: BranchTransport, Era: 2, Cost: era3, Requires: []string{"Steam engine"},
		Effect: "Exports spoil 60% less on the way", TransitSpoilage: 0.4},
	{Name: "Rail network", Branch: BranchTransport, Era: 2, Cost: era3, Requires: []string{"Railways"},
		Effect:  "Transport +30%, construction +20%, supplies need 30% less transport",
		FirmTFP: firms("Transport", 1.3, "Construction", 1.2), SupplyTransport: 0.7},
	{Name: "Breech-loading rifles", Branch: BranchMilitary, Era: 2, Cost: era3, Requires: []string{"Rifled muskets", "Industrial chemistry"},
		Effect: "Industrial army doctrine", Doctrine: "Industrial army"},
	{Name: "Universal conscription", Branch: BranchMilitary, Era: 2, Cost: era3, Requires: []string{"Mass conscription"},
		Effect: "Up to 8% of people can serve", Mobilization: 0.08},
	{Name: "General staff", Branch: BranchMilitary, Era: 2, Cost: era3, Requires: []string{"Military logistics"},
		Effect: "Attack +20%, sieges 30% faster; armies of up to 120 men", Attack: 1.2, OccupySpeed: 1.3, ArmySize: 120},
	{Name: "Public health", Branch: BranchPolitics, Era: 2, Cost: era3, Requires: []string{"Bureaucracy"},
		Effect: "Population growth +0.6% a year", PopGrowth: 0.006},
	{Name: "Central bank", Branch: BranchPolitics, Era: 2, Cost: era3, Requires: []string{"Banking"},
		Effect: "Firms invest more readily (payback horizon +30%)", InvestPayback: 1.3},
	{Name: "Telegraph", Branch: BranchScience, Era: 2, Cost: era3, Requires: []string{"Scientific method", "Railways"},
		Effect:  "Universities +15%; ideas from trade partners spread 50% better",
		FirmTFP: firms("University", 1.15), Diffusion: 1.5},
	{Name: "Research laboratories", Branch: BranchScience, Era: 2, Cost: era3, Requires: []string{"Public education"},
		Effect: "Universities +40%", FirmTFP: firms("University", 1.4)},
	{Name: "Engineering", Branch: BranchScience, Era: 2, Cost: era3, Requires: []string{"Public education", "Steam engine"},
		Effect:  "Construction +30%, smelters and smithies +20%",
		FirmTFP: firms("Construction", 1.3, "Smelter", 1.2, "Smithy", 1.2)},

	// ---- era 4: Machine age, 1900s–1910s
	{Name: "Assembly line", Branch: BranchEconomy, Era: 3, Cost: era4, Requires: []string{"Electricity"},
		Effect: "All industry +50%, motor works +30%", IndustryTFP: 1.5, FirmTFP: firms("Motor works", 1.3)},
	{Name: "Mass production", Branch: BranchEconomy, Era: 3, Cost: era4, Requires: []string{"Assembly line"},
		Effect: "All industry +30%; armories and munitions plants +30%", IndustryTFP: 1.3,
		FirmTFP: firms("Armory", 1.3, "Munitions plant", 1.3)},
	{Name: "Internal combustion", Branch: BranchTransport, Era: 3, Cost: era4, Requires: []string{"Petroleum", "Steam engine"},
		Effect: "Motor works can operate; transport +30%", EnableFirms: []string{"Motor works"},
		FirmTFP: firms("Transport", 1.3)},
	{Name: "Mechanized agriculture", Branch: BranchEconomy, Era: 3, Cost: era4, Requires: []string{"Internal combustion", "Fertilizers"},
		Effect: "All land-bound producers +60%", PrimaryTFP: 1.6},
	{Name: "Motor trucks", Branch: BranchTransport, Era: 3, Cost: era4, Requires: []string{"Internal combustion"},
		Effect: "Transport +50%; supplies need 40% less transport", FirmTFP: firms("Transport", 1.5),
		SupplyTransport: 0.6},
	{Name: "Aviation", Branch: BranchTransport, Era: 3, Cost: era4, Requires: []string{"Internal combustion"},
		Effect: "Aircraft factories can operate", EnableFirms: []string{"Aircraft factory"}},
	{Name: "Machine guns", Branch: BranchMilitary, Era: 3, Cost: era4, Requires: []string{"Breech-loading rifles", "Industrial chemistry"},
		Effect: "Machine-gun infantry doctrine; defence +30%", Doctrine: "Machine-gun infantry", Defense: 1.3},
	{Name: "Total war", Branch: BranchMilitary, Era: 3, Cost: era4, Requires: []string{"Universal conscription"},
		Effect: "Up to 15% of people can serve; armies of up to 250 men", Mobilization: 0.15, ArmySize: 250},
	{Name: "Military aviation", Branch: BranchMilitary, Era: 3, Cost: era4, Requires: []string{"Aviation", "Machine guns"},
		Effect: "Recon planes: attack +20%", Attack: 1.2},
	{Name: "Tanks", Branch: BranchMilitary, Era: 3, Cost: era4 * 1.2, Requires: []string{"Military aviation", "Internal combustion"},
		Effect: "Combined arms doctrine: tanks and planes", Doctrine: "Combined arms"},
	{Name: "Propaganda", Branch: BranchPolitics, Era: 3, Cost: era4, Requires: []string{"Nationalism"},
		Effect: "Desertion −50%, defence +10%", Desertion: 0.5, Defense: 1.1},
	{Name: "Social insurance", Branch: BranchPolitics, Era: 3, Cost: era4, Requires: []string{"Public health"},
		Effect: "Population growth +0.4% a year; firms invest more readily", PopGrowth: 0.004, InvestPayback: 1.2},
	{Name: "Radio", Branch: BranchScience, Era: 3, Cost: era4, Requires: []string{"Telegraph", "Electricity"},
		Effect:  "Universities +30%; ideas from trade partners spread 50% better",
		FirmTFP: firms("University", 1.3), Diffusion: 1.5},
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

// firmMult is the technology multiplier on a firm's productivity.
func (n *Nation) firmMult(f *Firm) float64 {
	return n.techProduct(func(t *Tech) float64 {
		m := 1.0
		if x, ok := t.FirmTFP[f.Name]; ok {
			m *= x
		}
		if f.Sector == Primary && t.PrimaryTFP != 0 {
			m *= t.PrimaryTFP
		}
		if f.Sector == Industry && t.IndustryTFP != 0 {
			m *= t.IndustryTFP
		}
		return m
	})
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
func (n *Nation) desertionMult() float64 {
	return n.techProduct(func(t *Tech) float64 { return t.Desertion })
}
func (n *Nation) occupyMult() float64 {
	return n.techProduct(func(t *Tech) float64 { return t.OccupySpeed })
}
func (n *Nation) supplyTransportMult() float64 {
	return n.techProduct(func(t *Tech) float64 { return t.SupplyTransport })
}
func (n *Nation) buildRateMult() float64 {
	return n.techProduct(func(t *Tech) float64 { return t.BuildRate })
}
func (n *Nation) paybackMult() float64 {
	return n.techProduct(func(t *Tech) float64 { return t.InvestPayback })
}
func (n *Nation) diffusionMult() float64 {
	return n.techProduct(func(t *Tech) float64 { return t.Diffusion })
}

// popGrowthBonus is the extra population growth per year from technology.
func (n *Nation) popGrowthBonus() float64 {
	s := 0.0
	for i := range Techs {
		if n.Known[i] {
			s += Techs[i].PopGrowth
		}
	}
	return s
}

// baseMobilization is the share of people that can serve before any tech.
const baseMobilization = 0.02

// baseArmySize is how many men one army can field before any tech: a nation
// that may mobilize more needs several armies.
const baseArmySize = 10.0

// MaxArmySize is how many men one of the nation's armies can field.
func (n *Nation) MaxArmySize() float64 {
	m := baseArmySize
	for i := range Techs {
		if n.Known[i] && Techs[i].ArmySize > m {
			m = Techs[i].ArmySize
		}
	}
	return m
}

// Mobilization is the share of the nation's people that can serve in its armies.
func (n *Nation) Mobilization() float64 {
	m := baseMobilization
	for i := range Techs {
		if n.Known[i] && Techs[i].Mobilization > m {
			m = Techs[i].Mobilization
		}
	}
	return m
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
