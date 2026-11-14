package engine

import (
	"fmt"
	"math"
	"sort"
)

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
	AdminMult       float64 // multiplies what each official administers
	SchoolMult      float64 // multiplies what each teacher teaches
	HealthMult      float64 // multiplies what public health staff cover
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
	era1 = 32000.0
	era2 = 120000.0
	era3 = 300000.0
	era4 = 900000.0
)

// Techs is the tech tree, in a valid research order (prerequisites first).
var Techs = []Tech{
	// ---- era 1: Restoration, 1820s–40s
	{Name: "Crop rotation", Branch: BranchEconomy, Era: 0, Cost: era1, Effect: "Farms and potato farms +15%",
		FirmTFP: firms("Farm", 1.15, "Potato farm", 1.15)},
	{Name: "Viticulture", Branch: BranchEconomy, Era: 0, Cost: era1, Effect: "Wineries can operate",
		EnableFirms: []string{"Winery"}},
	{Name: "Ceramics", Branch: BranchEconomy, Era: 0, Cost: era1, Effect: "Kilns can operate; clay pits +15%",
		EnableFirms: []string{"Kiln"}, FirmTFP: firms("Clay pit", 1.15)},
	{Name: "Sugar refining", Branch: BranchEconomy, Era: 0, Cost: era1, Effect: "Refineries can operate",
		EnableFirms: []string{"Refinery"}},
	{Name: "Factory system", Branch: BranchEconomy, Era: 0, Cost: era1 * 1.3, Effect: "All industry +15%",
		IndustryTFP: 1.15},
	{Name: "Wagons", Branch: BranchTransport, Era: 0, Cost: era1, Effect: "Transport +20%",
		FirmTFP: firms("Transport", 1.2)},
	{Name: "Navigation", Branch: BranchTransport, Era: 0, Cost: era1, Effect: "Coast tiles cost 20% less to cross",
		CoastMove: 0.8},
	{Name: "Masonry", Branch: BranchTransport, Era: 0, Cost: era1, Effect: "Unlocks paved roads; construction +15%",
		FirmTFP: firms("Construction", 1.15)},
	{Name: "Drill", Branch: BranchMilitary, Era: 0, Cost: era1, Effect: "Line infantry doctrine; armies of up to 20 men",
		Doctrine: "Line infantry", ArmySize: 20},
	{Name: "Field artillery", Branch: BranchMilitary, Era: 0, Cost: era1, Requires: []string{"Drill"},
		Effect: "Foundries +20%; attack +10%", FirmTFP: firms("Foundry", 1.2), Attack: 1.1},
	{Name: "Fortifications", Branch: BranchMilitary, Era: 0, Cost: era1, Effect: "Defence +50%",
		Defense: 1.5},
	{Name: "Administration", Branch: BranchPolitics, Era: 0, Cost: era1, Effect: "Sieges progress 50% faster; officials administer 25% more",
		OccupySpeed: 1.5, AdminMult: 1.25},
	{Name: "Nationalism", Branch: BranchPolitics, Era: 0, Cost: era1, Requires: []string{"Administration"},
		Effect: "Defence +25%, sieges 25% faster", Defense: 1.25, OccupySpeed: 1.25},
	{Name: "Printing press", Branch: BranchScience, Era: 0, Cost: era1, Effect: "Universities +25%",
		FirmTFP: firms("University", 1.25)},

	// ---- era 2: Railway age, 1840s–60s
	{Name: "Steam power", Branch: BranchEconomy, Era: 1, Cost: era2, Requires: []string{"Factory system"},
		Effect:      "Engine works can operate; steam-pumped coal mines (engines and coal, more coal a worker); all industry +10%",
		EnableFirms: []string{"Engine works", "Steam coal mine"}, IndustryTFP: 1.1},
	{Name: "Mechanized textiles", Branch: BranchEconomy, Era: 1, Cost: era2, Requires: []string{"Steam power"},
		Effect: "Textile factories can operate; mills +15%", EnableFirms: []string{"Textile factory"},
		FirmTFP: firms("Mill", 1.15)},
	{Name: "Silk weaving", Branch: BranchEconomy, Era: 1, Cost: era2, Requires: []string{"Ceramics"},
		Effect: "Couturiers can operate; silk farms +15%", EnableFirms: []string{"Couturier"},
		FirmTFP: firms("Silk farm", 1.15)},
	{Name: "Seed drill", Branch: BranchEconomy, Era: 1, Cost: era2, Requires: []string{"Crop rotation"},
		Effect: "Seed-drill farms (tools, more grain a worker); potato farms +15%", EnableFirms: []string{"Seed-drill farm"},
		FirmTFP: firms("Potato farm", 1.15)},
	{Name: "Interchangeable parts", Branch: BranchEconomy, Era: 1, Cost: era2, Requires: []string{"Factory system"},
		Effect:      "Machine-tool works (engines and coal, more tools a worker); armories +20%",
		EnableFirms: []string{"Machine-tool works"}, FirmTFP: firms("Armory", 1.2)},
	{Name: "Ocean sailing", Branch: BranchTransport, Era: 1, Cost: era2, Requires: []string{"Navigation"},
		Effect: "Ports on coast tiles: ships sail straight to every other port", Ports: true},
	{Name: "Steam engine", Branch: BranchTransport, Era: 1, Cost: era2, Requires: []string{"Wagons"},
		Effect: "Steam transport (engines and coal, more transport a worker)", EnableFirms: []string{"Steam transport"}},
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
		Effect: "Construction +25%; projects can take twice as much per day; officials administer 30% more", BuildRate: 2,
		FirmTFP: firms("Construction", 1.25), AdminMult: 1.3},
	{Name: "Banking", Branch: BranchPolitics, Era: 1, Cost: era2, Requires: []string{"Bureaucracy"},
		Effect: "Firms invest more readily (payback horizon +50%)", InvestPayback: 1.5},
	{Name: "Scientific method", Branch: BranchScience, Era: 1, Cost: era2, Requires: []string{"Printing press"},
		Effect: "Universities +30%", FirmTFP: firms("University", 1.3)},
	{Name: "Public education", Branch: BranchScience, Era: 1, Cost: era2, Requires: []string{"Scientific method"},
		Effect: "Universities +30%, all industry +3%; teachers teach 40% more", FirmTFP: firms("University", 1.3), IndustryTFP: 1.03,
		SchoolMult: 1.4},

	// ---- era 3: Second industrial revolution, 1870s–90s
	{Name: "Steel", Branch: BranchEconomy, Era: 2, Cost: era3, Requires: []string{"Steam power"},
		Effect:      "Steel works can operate; steam-pumped iron mines (engines and coal, more ore a worker)",
		EnableFirms: []string{"Steel works", "Steam iron mine"}},
	{Name: "Fertilizers", Branch: BranchEconomy, Era: 2, Cost: era3, Requires: []string{"Seed drill"},
		Effect:      "Fertilizer works can operate; fertilized farms and potato farms (fertilizer, more a worker); ranches +20%, plantations +15%",
		EnableFirms: []string{"Fertilizer works", "Fertilized farm", "Fertilized potato farm"},
		FirmTFP:     firms("Ranch", 1.2, "Cotton plantation", 1.15, "Sugar plantation", 1.15, "Tea plantation", 1.15)},
	{Name: "Industrial chemistry", Branch: BranchEconomy, Era: 2, Cost: era3, Requires: []string{"Steel"},
		Effect:      "Munitions plants and chemical works can operate; blasting iron mines (explosives, more ore a worker)",
		EnableFirms: []string{"Munitions plant", "Chemical works", "Blasting iron mine"}},
	{Name: "Electricity", Branch: BranchEconomy, Era: 2, Cost: era3, Requires: []string{"Steel"},
		Effect:      "Power stations; electric smithies, joineries and garment factories (power, more a worker); all industry +10%",
		EnableFirms: []string{"Power station", "Electric smithy", "Electric joinery", "Garment factory"}, IndustryTFP: 1.1},
	{Name: "Petroleum", Branch: BranchEconomy, Era: 2, Cost: era3, Requires: []string{"Industrial chemistry"},
		Effect: "Oil wells can operate", EnableFirms: []string{"Oil well"}},
	{Name: "Steamships", Branch: BranchTransport, Era: 2, Cost: era3, Requires: []string{"Steam engine", "Ocean sailing"},
		Effect: "Coast tiles and sea routes cost 40% less", CoastMove: 0.6},
	{Name: "Refrigeration", Branch: BranchTransport, Era: 2, Cost: era3, Requires: []string{"Steam engine"},
		Effect: "Exports spoil 60% less on the way", TransitSpoilage: 0.4},
	{Name: "Rail network", Branch: BranchTransport, Era: 2, Cost: era3, Requires: []string{"Railways"},
		Effect:  "Transport +20%, construction +15%, supplies need 30% less transport",
		FirmTFP: firms("Transport", 1.2, "Construction", 1.15), SupplyTransport: 0.7},
	{Name: "Breech-loading rifles", Branch: BranchMilitary, Era: 2, Cost: era3, Requires: []string{"Rifled muskets", "Industrial chemistry"},
		Effect: "Industrial army doctrine", Doctrine: "Industrial army"},
	{Name: "Universal conscription", Branch: BranchMilitary, Era: 2, Cost: era3, Requires: []string{"Mass conscription"},
		Effect: "Up to 8% of people can serve", Mobilization: 0.08},
	{Name: "General staff", Branch: BranchMilitary, Era: 2, Cost: era3, Requires: []string{"Military logistics"},
		Effect: "Attack +20%, sieges 30% faster; armies of up to 120 men", Attack: 1.2, OccupySpeed: 1.3, ArmySize: 120},
	{Name: "Public health", Branch: BranchPolitics, Era: 2, Cost: era3, Requires: []string{"Bureaucracy"},
		Effect: "Population growth +0.6% a year; public health staff cover 60% more", PopGrowth: 0.006, HealthMult: 1.6},
	{Name: "Central bank", Branch: BranchPolitics, Era: 2, Cost: era3, Requires: []string{"Banking"},
		Effect: "Firms invest more readily (payback horizon +30%)", InvestPayback: 1.3},
	{Name: "Telegraph", Branch: BranchScience, Era: 2, Cost: era3, Requires: []string{"Scientific method", "Railways"},
		Effect:  "Universities +15%; ideas from trade partners spread 50% better",
		FirmTFP: firms("University", 1.15), Diffusion: 1.5},
	{Name: "Research laboratories", Branch: BranchScience, Era: 2, Cost: era3, Requires: []string{"Public education"},
		Effect: "Universities +40%", FirmTFP: firms("University", 1.4)},
	{Name: "Engineering", Branch: BranchScience, Era: 2, Cost: era3, Requires: []string{"Public education", "Steam engine"},
		Effect:      "Steam cranes (engines and coal, more construction a worker); smelters and smithies +15%",
		EnableFirms: []string{"Steam cranes"}, FirmTFP: firms("Smelter", 1.15, "Smithy", 1.15)},

	// ---- era 4: Machine age, 1900s–1910s
	{Name: "Assembly line", Branch: BranchEconomy, Era: 3, Cost: era4, Requires: []string{"Electricity"},
		Effect:      "Assembly-line motor works (power, more vehicles a worker); all industry +20%",
		EnableFirms: []string{"Assembly-line works"}, IndustryTFP: 1.2},
	{Name: "Mass production", Branch: BranchEconomy, Era: 3, Cost: era4, Requires: []string{"Assembly line"},
		Effect: "All industry +20%; armories and munitions plants +20%", IndustryTFP: 1.2,
		FirmTFP: firms("Armory", 1.2, "Munitions plant", 1.2)},
	{Name: "Internal combustion", Branch: BranchTransport, Era: 3, Cost: era4, Requires: []string{"Petroleum", "Steam engine"},
		Effect: "Motor works can operate; transport +20%", EnableFirms: []string{"Motor works"},
		FirmTFP: firms("Transport", 1.2)},
	{Name: "Mechanized agriculture", Branch: BranchEconomy, Era: 3, Cost: era4, Requires: []string{"Internal combustion", "Fertilizers"},
		Effect:      "Tractor farms (vehicles, oil and fertilizer, far more grain a worker); all land-bound producers +15%",
		EnableFirms: []string{"Tractor farm"}, PrimaryTFP: 1.15},
	{Name: "Motor trucks", Branch: BranchTransport, Era: 3, Cost: era4, Requires: []string{"Internal combustion"},
		Effect:      "Motor transport (vehicles and oil, more transport a worker); supplies need 40% less transport",
		EnableFirms: []string{"Motor transport"}, SupplyTransport: 0.6},
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
		Effect: "Population growth +0.4% a year; firms invest more readily; public health staff cover 30% more", PopGrowth: 0.004, InvestPayback: 1.2, HealthMult: 1.3},
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

// techAdoptDays is how long a tech's productivity and growth bonuses take
// to come in fully: a new machine spreads through the works over years.
const techAdoptDays = 5 * 365

// Adoption is how far the nation has taken up a known tech's productivity
// and growth bonuses: from 0 the day it is learned to 1 after techAdoptDays
// (techs it always knew, or was granted outright, count as fully adopted).
func (n *Nation) Adoption(i int) float64 {
	if !n.Known[i] {
		return 0
	}
	if n.LearnedAt[i] <= 0 {
		return 1
	}
	return math.Max(0, math.Min(1, float64(n.now-n.LearnedAt[i])/techAdoptDays))
}

// adopting reports whether any known tech is still coming in (including
// the day it is complete, so its last step is applied).
func (n *Nation) adopting() bool {
	for i := range Techs {
		if n.Known[i] && n.LearnedAt[i] > 0 && n.now-n.LearnedAt[i] <= techAdoptDays {
			return true
		}
	}
	return false
}

// techFirmMult is a tech's full multiplier on a firm's productivity.
func techFirmMult(t *Tech, f *Firm) float64 {
	m := 1.0
	if x, ok := t.FirmTFP[f.Name]; ok {
		m *= x
	} else if x, ok := t.FirmTFP[f.Base]; ok && f.Base != "" { // a method gains what its firm does
		m *= x
	}
	if f.Sector == Primary && t.PrimaryTFP != 0 {
		m *= t.PrimaryTFP
	}
	if f.Sector == Industry && t.IndustryTFP != 0 {
		m *= t.IndustryTFP
	}
	return m
}

// firmMult is the technology multiplier on a firm's productivity, each
// tech's bonus as far as it has been adopted.
func (n *Nation) firmMult(f *Firm) float64 {
	m := 1.0
	for i := range Techs {
		if n.Known[i] {
			m *= 1 + (techFirmMult(&Techs[i], f)-1)*n.Adoption(i)
		}
	}
	return m
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
		s += Techs[i].PopGrowth * n.Adoption(i)
	}
	return s
}

// baseMobilization is the share of people that can serve before any tech.
const baseMobilization = 0.02

// baseArmySize is how many men one army can field before any tech: a nation
// that may mobilize more needs several armies.
const baseArmySize = 10.0

// militaryAdoptDays is how long a tech that enlarges armies or the share of
// people who may serve takes to come in: officers, depots and conscription
// rolls take a couple of years to organise.
const militaryAdoptDays = 2 * 365

// adoptionOver is like Adoption, over the given number of days.
func (n *Nation) adoptionOver(i, days int) float64 {
	if !n.Known[i] {
		return 0
	}
	if n.LearnedAt[i] <= 0 {
		return 1
	}
	return math.Max(0, math.Min(1, float64(n.now-n.LearnedAt[i])/float64(days)))
}

// fadedMax is the highest value of an effect among the nation's techs (at
// least base), each raise over the level below it coming in over
// militaryAdoptDays.
func (n *Nation) fadedMax(base float64, effect func(t *Tech) float64) float64 {
	type step struct{ v, a float64 }
	var steps []step
	for i := range Techs {
		if v := effect(&Techs[i]); n.Known[i] && v > base {
			steps = append(steps, step{v, n.adoptionOver(i, militaryAdoptDays)})
		}
	}
	sort.Slice(steps, func(a, b int) bool { return steps[a].v < steps[b].v })
	m, below := base, base
	for _, s := range steps {
		if s.v > below {
			m += (s.v - below) * s.a
			below = s.v
		}
	}
	return m
}

// MaxArmySize is how many men one of the nation's armies can field.
func (n *Nation) MaxArmySize() float64 {
	return n.fadedMax(baseArmySize, func(t *Tech) float64 { return t.ArmySize })
}

// Mobilization is the share of the nation's people that can serve in its armies.
func (n *Nation) Mobilization() float64 {
	return n.fadedMax(baseMobilization, func(t *Tech) float64 { return t.Mobilization })
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
	if f.baseTech != "" && (owner == nil || !owner.Has(f.baseTech)) {
		return false
	}
	if f.Requires == "" {
		return true
	}
	return owner != nil && owner.Has(f.Requires)
}
