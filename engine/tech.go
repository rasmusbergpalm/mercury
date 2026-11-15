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
	{Name: "Norfolk rotation", Branch: BranchEconomy, Era: 0, Cost: era1, Effect: "Farms and potato farms +15%",
		FirmTFP: firms("Farm", 1.15, "Potato farm", 1.15)},
	{Name: "Tile drainage", Branch: BranchEconomy, Era: 0, Cost: era1, Effect: "Farms, potato farms and ranches +10%",
		FirmTFP: firms("Farm", 1.1, "Potato farm", 1.1, "Ranch", 1.1)},
	{Name: "Portland cement", Branch: BranchEconomy, Era: 0, Cost: era1, Effect: "Construction +15%; clay pits +15%",
		FirmTFP: firms("Construction", 1.15, "Clay pit", 1.15)},
	{Name: "Beet sugar", Branch: BranchEconomy, Era: 0, Cost: era1, Effect: "Refineries and sugar plantations +10%",
		FirmTFP: firms("Refinery", 1.1, "Sugar plantation", 1.1)},
	{Name: "Factory system", Branch: BranchEconomy, Era: 0, Cost: era1 * 1.3, Effect: "All industry +15%",
		IndustryTFP: 1.15},
	{Name: "Canals", Branch: BranchTransport, Era: 0, Cost: era1, Effect: "Transport +20%",
		FirmTFP: firms("Transport", 1.2)},
	{Name: "Hydrographic charts", Branch: BranchTransport, Era: 0, Cost: era1, Effect: "Coast tiles cost 20% less to cross",
		CoastMove: 0.8},
	{Name: "Macadam roads", Branch: BranchTransport, Era: 0, Cost: era1, Effect: "Unlocks paved roads"},
	{Name: "Percussion caps", Branch: BranchMilitary, Era: 0, Cost: era1, Effect: "Armies of up to 20 men; attack +10% (reliable fire, in rain too)",
		ArmySize: 20, Attack: 1.1},
	{Name: "Shrapnel shells", Branch: BranchMilitary, Era: 0, Cost: era1, Requires: []string{"Percussion caps"},
		Effect: "Foundries +20%; attack +10%", FirmTFP: firms("Foundry", 1.2), Attack: 1.1},
	{Name: "Polygonal forts", Branch: BranchMilitary, Era: 0, Cost: era1, Effect: "Defence +50%",
		Defense: 1.5},
	{Name: "Statistical bureaus", Branch: BranchPolitics, Era: 0, Cost: era1, Effect: "Sieges progress 50% faster; officials administer 25% more",
		OccupySpeed: 1.5, AdminMult: 1.25},
	{Name: "Nationalism", Branch: BranchPolitics, Era: 0, Cost: era1, Requires: []string{"Statistical bureaus"},
		Effect: "Defence +25%, sieges 25% faster", Defense: 1.25, OccupySpeed: 1.25},
	{Name: "Steam printing", Branch: BranchScience, Era: 0, Cost: era1, Effect: "Universities +25%",
		FirmTFP: firms("University", 1.25)},

	// ---- era 2: Railway age, 1840s–60s
	{Name: "Steam power", Branch: BranchEconomy, Era: 1, Cost: era2, Requires: []string{"Factory system"},
		Effect:      "Engine works can operate; all industry +10%",
		EnableFirms: []string{"Engine works", "Steam sawmill", "Steam coal mine", "Steam iron mine", "Steam-gin plantation", "Steam-mill plantation", "Steam clay works", "Steam bakery", "Steam brewery", "Hot-blast furnace", "Steam-hammer forge", "Steam joinery", "Vacuum-pan refinery", "Steam-press winery", "Steam foundry"}, IndustryTFP: 1.1},
	{Name: "Mechanized textiles", Branch: BranchEconomy, Era: 1, Cost: era2, Requires: []string{"Steam power"},
		Effect: "Mills +15%", EnableFirms: []string{"Textile factory"},
		FirmTFP: firms("Mill", 1.15)},
	{Name: "Steam filatures", Branch: BranchEconomy, Era: 1, Cost: era2, Requires: []string{"Steam power"},
		Effect: "Silk farms +15%", EnableFirms: []string{"Steam filature"},
		FirmTFP: firms("Silk farm", 1.15)},
	{Name: "Seed drill", Branch: BranchEconomy, Era: 1, Cost: era2, Requires: []string{"Norfolk rotation"},
		Effect: "Potato farms +15%", EnableFirms: []string{"Seed-drill farm", "Horse-hoe potato farm", "Fodder ranch", "Bred-flock sheep farm", "Estate tea garden", "Trellised vineyard"},
		FirmTFP: firms("Potato farm", 1.15)},
	{Name: "Interchangeable parts", Branch: BranchEconomy, Era: 1, Cost: era2, Requires: []string{"Factory system"},
		Effect:      "Armories +20%",
		EnableFirms: []string{"Sewing-machine tailor", "Sewing-machine atelier", "Factory kiln", "Armory system", "Machine-tool works"}, FirmTFP: firms("Armory", 1.2)},
	{Name: "Clipper ships", Branch: BranchTransport, Era: 1, Cost: era2, Requires: []string{"Hydrographic charts"},
		Effect: "Ports on coast tiles: ships sail straight to every other port", Ports: true},
	{Name: "Locomotives", Branch: BranchTransport, Era: 1, Cost: era2, Requires: []string{"Canals"},
		Effect: "", EnableFirms: []string{"Steam transport", "Steam shovels"}},
	{Name: "Railways", Branch: BranchTransport, Era: 1, Cost: era2 * 1.2, EnableFirms: []string{"Ice-packed fishery"}, Requires: []string{"Locomotives", "Macadam roads"},
		Effect: "Unlocks railways"},
	{Name: "Mass conscription", Branch: BranchMilitary, Era: 1, Cost: era2, Requires: []string{"Percussion caps"},
		Effect: "Napoleonic corps doctrine; up to 4% of people can serve; armies of up to 40 men", Doctrine: "Napoleonic corps",
		Mobilization: 0.04, ArmySize: 40},
	{Name: "Rifled muskets", Branch: BranchMilitary, Era: 1, Cost: era2, Requires: []string{"Mass conscription", "Interchangeable parts"},
		Effect: "Rifle infantry doctrine", Doctrine: "Rifle infantry"},
	{Name: "Military logistics", Branch: BranchMilitary, Era: 1, Cost: era2, Requires: []string{"Mass conscription"},
		Effect: "Supplies need 30% less transport; desertion −30%; armies of up to 60 men", SupplyTransport: 0.7, Desertion: 0.7,
		ArmySize: 60},
	{Name: "Civil service reform", Branch: BranchPolitics, Era: 1, Cost: era2, Requires: []string{"Statistical bureaus"},
		Effect: "Construction +25%; projects can take twice as much per day; officials administer 30% more", BuildRate: 2,
		FirmTFP: firms("Construction", 1.25), AdminMult: 1.3},
	{Name: "Joint-stock banks", Branch: BranchPolitics, Era: 1, Cost: era2, Requires: []string{"Civil service reform"},
		Effect: "Firms invest more readily (payback horizon +50%)", InvestPayback: 1.5},
	{Name: "Research universities", Branch: BranchScience, Era: 1, Cost: era2, Requires: []string{"Steam printing"},
		Effect: "Universities +30%", FirmTFP: firms("University", 1.3)},
	{Name: "Public education", Branch: BranchScience, Era: 1, Cost: era2, Requires: []string{"Research universities"},
		Effect: "Universities +30%, all industry +3%; teachers teach 40% more", FirmTFP: firms("University", 1.3), IndustryTFP: 1.03,
		SchoolMult: 1.4},

	{Name: "Telegraph", Branch: BranchScience, Era: 1, Cost: era2, Requires: []string{"Research universities", "Railways"},
		Effect:  "Universities +15%; ideas from trade partners spread 50% better",
		FirmTFP: firms("University", 1.15), Diffusion: 1.5},

	// ---- era 3: Second industrial revolution, 1870s–90s
	{Name: "Steel", Branch: BranchEconomy, Era: 2, Cost: era3, Requires: []string{"Steam power"},
		Effect:      "",
		EnableFirms: []string{"Fenced ranch", "Steam trawler", "Band-saw mill", "Machine-shorn sheep farm", "Tea estate with rollers", "Steel works", "Steel arms works", "Steel gun works", "Compound-engine works"}},
	{Name: "Fertilizers", Branch: BranchEconomy, Era: 2, Cost: era3, Requires: []string{"Seed drill"},
		Effect:      "Fertilizer works can operate; ranches +20%, plantations +15%",
		EnableFirms: []string{"Fertilizer works", "Fertilized farm", "Fertilized potato farm", "Fertilized cotton plantation", "Fertilized cane plantation"},
		FirmTFP:     firms("Ranch", 1.2, "Cotton plantation", 1.15, "Sugar plantation", 1.15, "Tea plantation", 1.15)},
	{Name: "Industrial chemistry", Branch: BranchEconomy, Era: 2, Cost: era3, Requires: []string{"Steel"},
		Effect:      "Munitions plants and chemical works can operate",
		EnableFirms: []string{"Munitions plant", "Chemical works", "Blasting iron mine", "Grafted vineyard", "Selected-egg silk farm", "Blasted clay pit", "Tunnel kiln"}},
	{Name: "Electricity", Branch: BranchEconomy, Era: 2, Cost: era3, Requires: []string{"Steel"},
		Effect:      "Power stations can operate; all industry +10%",
		EnableFirms: []string{"Power station", "Mechanized coal mine", "Electric bakery", "Ring-spinning mill", "Garment factory", "Electric smithy", "Electric joinery", "Centrifugal refinery", "Electric atelier", "Electric tool works"}, IndustryTFP: 1.1},
	{Name: "Petroleum", Branch: BranchEconomy, Era: 2, Cost: era3, Requires: []string{"Industrial chemistry"},
		Effect: "Oil wells can operate", EnableFirms: []string{"Oil well"}},
	{Name: "Steamships", Branch: BranchTransport, Era: 2, Cost: era3, Requires: []string{"Locomotives", "Clipper ships"},
		Effect: "Coast tiles and sea routes cost 40% less", CoastMove: 0.6},
	{Name: "Refrigeration", Branch: BranchTransport, Era: 2, Cost: era3, EnableFirms: []string{"Refrigerated brewery"}, Requires: []string{"Locomotives"},
		Effect: "Exports spoil 60% less on the way", TransitSpoilage: 0.4},
	{Name: "Rail network", Branch: BranchTransport, Era: 2, Cost: era3, EnableFirms: []string{"Heavy rail freight"}, Requires: []string{"Railways"},
		Effect:  "Transport +20%, construction +15%, supplies need 30% less transport",
		FirmTFP: firms("Transport", 1.2, "Construction", 1.15), SupplyTransport: 0.7},
	{Name: "Breech-loading rifles", Branch: BranchMilitary, Era: 2, Cost: era3, Requires: []string{"Rifled muskets", "Industrial chemistry"},
		Effect: "Industrial army doctrine", Doctrine: "Industrial army"},
	{Name: "Universal conscription", Branch: BranchMilitary, Era: 2, Cost: era3, Requires: []string{"Mass conscription"},
		Effect: "Up to 8% of people can serve", Mobilization: 0.08},
	{Name: "General staff", Branch: BranchMilitary, Era: 2, Cost: era3, Requires: []string{"Military logistics"},
		Effect: "Attack +20%, sieges 30% faster; armies of up to 120 men", Attack: 1.2, OccupySpeed: 1.3, ArmySize: 120},
	{Name: "Public health", Branch: BranchPolitics, Era: 2, Cost: era3, Requires: []string{"Civil service reform"},
		Effect: "Population growth +0.6% a year; public health staff cover 60% more", PopGrowth: 0.006, HealthMult: 1.6},
	{Name: "Social insurance", Branch: BranchPolitics, Era: 2, Cost: era3, Requires: []string{"Public health"},
		Effect: "Population growth +0.4% a year; firms invest more readily; public health staff cover 30% more", PopGrowth: 0.004, InvestPayback: 1.2, HealthMult: 1.3},
	{Name: "Central bank", Branch: BranchPolitics, Era: 2, Cost: era3, Requires: []string{"Joint-stock banks"},
		Effect: "Firms invest more readily (payback horizon +30%)", InvestPayback: 1.3},
	{Name: "Research laboratories", Branch: BranchScience, Era: 2, Cost: era3, EnableFirms: []string{"Pasteurizing winery"}, Requires: []string{"Public education"},
		Effect: "Universities +40%", FirmTFP: firms("University", 1.4)},
	{Name: "Engineering", Branch: BranchScience, Era: 2, Cost: era3, Requires: []string{"Public education", "Locomotives"},
		Effect:      "Smelters and smithies +15%",
		EnableFirms: []string{"Steam cranes"}, FirmTFP: firms("Smelter", 1.15, "Smithy", 1.15)},

	// ---- era 4: Machine age, 1900s–1910s
	{Name: "Assembly line", Branch: BranchEconomy, Era: 3, Cost: era4, Requires: []string{"Electricity"},
		Effect:      "All industry +20%",
		EnableFirms: []string{"Automatic-loom mill", "Garment assembly line", "Couture workshop line", "Tool assembly line", "Engine assembly line", "Assembly-line works"}, IndustryTFP: 1.2},
	{Name: "Mass production", Branch: BranchEconomy, Era: 3, Cost: era4, EnableFirms: []string{"Conveyor coal mine", "Electric filature", "Industrial bakery", "Bottling brewery", "Electric arc furnace", "Drop-forge works", "Furniture factory", "Continuous refinery", "Industrial winery", "Electric kiln", "Mass-production armory", "Electric foundry", "Electrochemical works", "Haber-process plant", "Turbine power station", "Shell production line"}, Requires: []string{"Assembly line"},
		Effect: "All industry +20%; armories and munitions plants +20%", IndustryTFP: 1.2,
		FirmTFP: firms("Armory", 1.2, "Munitions plant", 1.2)},
	{Name: "Internal combustion", Branch: BranchTransport, Era: 3, Cost: era4, Requires: []string{"Petroleum", "Locomotives"},
		Effect: "Motor works can operate; transport +20%", EnableFirms: []string{"Motor works", "Motor trawler", "Motor logging", "Motor ore mine", "Excavator clay pit", "Rotary-drilled well", "Motorized construction"},
		FirmTFP: firms("Transport", 1.2)},
	{Name: "Mechanized agriculture", Branch: BranchEconomy, Era: 3, Cost: era4, Requires: []string{"Internal combustion", "Fertilizers"},
		Effect:      "All land-bound producers +15%",
		EnableFirms: []string{"Tractor farm", "Motorized potato farm", "Motorized ranch", "Motorized sheep station", "Mechanized cotton plantation", "Mechanized tea estate", "Mechanized cane plantation", "Motorized vineyard"}, PrimaryTFP: 1.15},
	{Name: "Motor trucks", Branch: BranchTransport, Era: 3, Cost: era4, Requires: []string{"Internal combustion"},
		Effect:      "Supplies need 40% less transport",
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
func techFirmMult(t *Tech, f *Firm, me *Method) float64 {
	m := 1.0
	if x, ok := t.FirmTFP[me.Name]; ok {
		m *= x
	} else if x, ok := t.FirmTFP[f.Name]; ok { // a method gains what its firm does
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

// methodMult is the technology multiplier on a firm's method's
// productivity, each tech's bonus as far as it has been adopted.
func (n *Nation) methodMult(f *Firm, me *Method) float64 {
	m := 1.0
	for i := range Techs {
		if n.Known[i] {
			m *= 1 + (techFirmMult(&Techs[i], f, me)-1)*n.Adoption(i)
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
// applyMethods brings a firm's methods up to its owner's technology (nil:
// free land, only the methods that need none, at base productivity).
func (f *Firm) applyMethods(owner *Nation) {
	for _, m := range f.Methods {
		m.TFP = m.BaseTFP
		if owner != nil {
			m.TFP *= owner.methodMult(f, m)
		}
		m.Enabled = m.Requires == "" || (owner != nil && owner.Has(m.Requires))
	}
}

func firmEnabled(f *Firm, owner *Nation) bool {
	if f.Requires == "" {
		return true
	}
	return owner != nil && owner.Has(f.Requires)
}
