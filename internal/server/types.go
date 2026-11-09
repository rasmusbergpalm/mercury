package server

import (
	"github.com/rasmusbergpalm/mercury/engine"
	"slices"
)

// JSON views of a world.

type GoodInfo struct {
	Name     string  `json:"name"`
	Bulk     float64 `json:"bulk"`
	Spoilage float64 `json:"spoilage"`
	Tier     string  `json:"tier"` // "" for goods pops don't consume
}

type TerrainInfo struct {
	Name     string  `json:"name"`
	Symbol   string  `json:"symbol"`
	MoveCost float64 `json:"move_cost"`
}

type RegionInfo struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Row     int    `json:"row"`
	Col     int    `json:"col"`
	Terrain string `json:"terrain"`
	Coastal bool   `json:"coastal"` // by the ocean: fishing, cheap shipping, ports
}

type LinkInfo struct {
	A   int     `json:"a"`
	B   int     `json:"b"`
	Hop float64 `json:"hop"`
}

type NationInfo struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

type InfraInfo struct {
	Name      string             `json:"name"`
	MoveMult  float64            `json:"move_mult"`
	Cost      float64            `json:"cost"`
	Upkeep    float64            `json:"upkeep"`
	Tech      string             `json:"tech"`
	Materials map[string]float64 `json:"materials"`
}

type TechInfo struct {
	Name     string   `json:"name"`
	Branch   string   `json:"branch"`
	Era      int      `json:"era"`
	Cost     float64  `json:"cost"`
	Requires []string `json:"requires"`
	Effect   string   `json:"effect"`
	Ports    bool     `json:"ports"` // lets the nation build ports
}

// WorldInfo is the static description of a game's world.
type WorldInfo struct {
	Rows           int            `json:"rows"`
	Cols           int            `json:"cols"`
	Goods          []GoodInfo     `json:"goods"`
	Tiers          []string       `json:"tiers"`
	Terrains       []TerrainInfo  `json:"terrains"`
	Regions        []RegionInfo   `json:"regions"`
	Links          []LinkInfo     `json:"links"`
	Nations        []NationInfo   `json:"nations"`
	Infra          []InfraInfo    `json:"infra_levels"`
	Techs          []TechInfo     `json:"techs"`
	Spending       []string       `json:"spending"`    // categories of NationState.SpentOn
	Services       []string       `json:"services"`    // the public services, by index
	Staff          float64        `json:"staff"`       // staff per funded service level
	MaxPension     int            `json:"max_pension"` // pension levels
	Branches       []string       `json:"branches"`
	LevelCost      float64        `json:"level_cost"`      // construction per firm building level
	TilesPerPort   int            `json:"tiles_per_port"`  // a nation may have a port for every so many tiles
	MinOccupiers   float64        `json:"min_occupiers"`   // men an army needs to occupy a tile or fight
	OccupyStrength float64        `json:"occupy_strength"` // strength an army needs to take ground
	Port           PortInfo       `json:"port"`
	Eras           []string       `json:"eras"`
	Doctrines      []DoctrineInfo `json:"doctrines"`
	StartYear      int            `json:"start_year"`
	Consumer       []int          `json:"consumer_goods"` // the order of RegionHistory.Prices
	Classes        []string       `json:"classes"`
	Sectors        []string       `json:"sectors"`
}

type DoctrineInfo struct {
	Name  string             `json:"name"`
	Tech  string             `json:"tech"`
	Power float64            `json:"power"`
	Needs map[string]float64 `json:"needs"` // per soldier per day
}

type PortInfo struct {
	Cost      float64            `json:"cost"`
	Materials map[string]float64 `json:"materials"`
}

type Project struct {
	Port      bool    `json:"port"`
	Target    int     `json:"target"`
	Delivered float64 `json:"delivered"`
	Required  float64 `json:"required"`
}

type RegionState struct {
	ID         int             `json:"id"`
	Wage       float64         `json:"wage"`
	Utility    float64         `json:"utility"`
	Population float64         `json:"population"`
	Employed   float64         `json:"employed"`
	TierSpend  []float64       `json:"tier_spend"`
	Prices     []float64       `json:"prices"`
	Nation     int             `json:"nation"` // -1 if unowned
	Infra      int             `json:"infra"`
	MoveCost   float64         `json:"move_cost"`
	Port       bool            `json:"port"`
	City       bool            `json:"city"`
	CityBonus  float64         `json:"city_bonus"`
	Growth     float64         `json:"growth"` // population growth per year
	Occupation *Occupation     `json:"occupation"`
	Battle     int             `json:"battle"`     // days the battle on the tile has lasted (0: none)
	Services   []*ServiceState `json:"services"`   // indexed like WorldInfo.Services
	Collect    float64         `json:"collect"`    // share of its taxes collected
	Allegiance float64         `json:"allegiance"` // how far its people stand by their owner, 0..1
	Loyalties  []float64       `json:"loyalties"`  // its people's loyalty to each nation, and last to nobody
	Educated   float64         `json:"educated"`   // share of its people schooled
	Healthy    float64         `json:"healthy"`    // share of its crowding covered
	Crowding   float64         `json:"crowding"`   // 0..1
	Project    *Project        `json:"project"`
}

// University is a tile's state university.
type ServiceState struct {
	Levels    int      `json:"levels"`
	Funded    int      `json:"funded"`
	MaxLevels int      `json:"max_levels"`
	Staff     float64  `json:"staff"`
	Need      float64  `json:"need"`      // staff needed to cover the tile fully (0: none)
	Coverage  float64  `json:"coverage"`  // share of the need met today
	Research  float64  `json:"research"`  // universities: today
	PerLevel  float64  `json:"per_level"` // universities: research per funded level, fully staffed
	Cost      float64  `json:"cost"`      // wages per funded level per day
	Building  *float64 `json:"building"`  // progress of a new level (0..1), or null
	Enabled   bool     `json:"enabled"`
}

type Occupation struct {
	Nation   int     `json:"nation"`
	Progress float64 `json:"progress"`
}

type Flow struct {
	From int     `json:"from"`
	To   int     `json:"to"`
	Good int     `json:"good"`
	Qty  float64 `json:"qty"`
	Sea  bool    `json:"sea"`
}

type NationState struct {
	ID             int       `json:"id"`
	Controller     string    `json:"controller"` // "ai" or "human"
	Eliminated     bool      `json:"eliminated"`
	Tiles          int       `json:"tiles"`
	Population     float64   `json:"population"`
	Treasury       float64   `json:"treasury"`
	TaxRate        float64   `json:"tax_rate"`
	Income         float64   `json:"income"` // moving average, tax and tariffs
	Revenue        float64   `json:"revenue"`
	TariffRevenue  float64   `json:"tariff_revenue"`
	SpendEMA       float64   `json:"spend_avg"` // average spending a day
	AutoTax        bool      `json:"auto_tax"`  // the computer sets the tax
	Pension        int       `json:"pension"`   // 0..max_pension
	Spending       float64   `json:"spending"`
	SpentOn        []float64 `json:"spent_on"`   // today, by category: economy, science, military
	AutoBuild      bool      `json:"auto_build"` // the computer builds roads and expands firms
	PayOwed        float64   `json:"pay_owed"`   // soldiers' pay due today
	Paid           float64   `json:"paid"`       // share of today's bills the treasury covered
	ResearchDay    float64   `json:"research_per_day"`
	Tariff         []float64 `json:"tariff"` // per good
	Embargo        []bool    `json:"embargo"`
	Researching    int       `json:"researching"`
	Points         float64   `json:"research_points"`
	Known          []bool    `json:"known"`
	Adoption       []float64 `json:"adoption"`       // per tech: how far its productivity and growth bonuses have come in
	AlliedAgainst  int       `json:"allied_against"` // the nation its alliance stands against, -1: none
	Queue          []int     `json:"queue"`
	ChooseResearch bool      `json:"choose_research"` // a tech finished: ask the player for the next
	TechCosts      []float64 `json:"tech_costs"`      // after the discount from trade partners
	AtWar          []bool    `json:"at_war"`
	PeaceOffer     []bool    `json:"peace_offer"` // this nation has offered peace to
	Soldiers       float64   `json:"soldiers"`
	MobilizeCap    float64   `json:"mobilize_cap"`
	MaxArmySize    float64   `json:"max_army_size"`  // men one army can field
	Ports          int       `json:"ports"`          // ports it has, counting those being built
	PortAllowance  int       `json:"port_allowance"` // ports it may have
	Standing       float64   `json:"standing"`       // share of each army's wartime size kept under arms in peace
	Temperament    string    `json:"temperament"`    // how the computer plays it
	Mobilization   float64   `json:"mobilization"`   // share of people who may serve
	BestDoctrine   int       `json:"best_doctrine"`
	GDP            float64   `json:"gdp"`
	Utility        float64   `json:"utility"`
	ClassIncome    []float64 `json:"class_income"` // per head, by class
	Labor          []float64 `json:"labor"`        // share of workers by sector (industry, primary, service)
	Exports        []float64 `json:"exports"`      // value per good, today
	Imports        []float64 `json:"imports"`
}

// SourceShare is one tile's part in an army's order for a good.
type SourceShare struct {
	Tile  int     `json:"tile"`
	Share float64 `json:"share"`
}

type ArmyState struct {
	ID            int                      `json:"id"`
	Nation        int                      `json:"nation"`
	Region        int                      `json:"region"`
	Men           float64                  `json:"men"`
	Target        float64                  `json:"target"`     // at full strength (war)
	UnderArms     float64                  `json:"under_arms"` // kept now: the target at war, its standing share in peace
	CanOccupy     bool                     `json:"can_occupy"` // strong enough to take ground
	Doctrine      int                      `json:"doctrine"`
	Effectiveness float64                  `json:"effectiveness"`
	Strength      float64                  `json:"strength"`
	Supply        map[string]float64       `json:"supply"`        // per needed good, 0..1
	StockDays     map[string]float64       `json:"stock_days"`    // days of each good it carries
	Sources       map[string]int           `json:"sources"`       // tile most of each good was bought in today (-1: none)
	Offered       map[string]float64       `json:"offered"`       // how much of each its sources' markets offered
	SourceShares  map[string][]SourceShare `json:"source_shares"` // every tile each good is ordered from, and its share, biggest first
	Coming        float64                  `json:"coming"`        // recruits marching out to it
	Cost          map[string]float64       `json:"cost"`          // today, per supply good (and "transport")
	Pay           float64                  `json:"pay"`           // soldiers' pay today
	ComingDays    int                      `json:"coming_days"`   // until the first of them arrive (-1: none)
	FullStrength  float64                  `json:"full_strength"` // at full size, fully supplied
	DailyLoss     float64                  `json:"daily_loss"`    // share of men lost today outside battle
	Path          []int                    `json:"path"`
	Retreating    bool                     `json:"retreating"` // falling back after a lost battle
	Attack        []engine.Modifier        `json:"attack"`     // strength per soldier, factor by factor
	Defense       []engine.Modifier        `json:"defense"`    // extra factors when defending at home
	Attrition     []engine.Modifier        `json:"attrition"`  // daily losses
	Depot         int                      `json:"depot"`      // where supplies are bought (-1: cut off)
	Transport     float64                  `json:"transport"`  // transport today's supplies needed
}

type TurnState struct {
	Days  int   `json:"days"`  // ticks per turn
	Ready []int `json:"ready"` // nations whose players have ended the turn
}

// State is the dynamic state of a game.
type State struct {
	Game    string         `json:"game"`
	Tick    int            `json:"tick"`
	Date    string         `json:"date"`
	Turn    *TurnState     `json:"turn"` // null in real-time games
	Armies  []ArmyState    `json:"armies"`
	Events  []engine.Event `json:"events"` // recent, for the requester
	Running bool           `json:"running"`
	Speed   float64        `json:"speed"`
	You     int            `json:"you"` // the requester's nation, or -1
	Money   float64        `json:"money"`
	Regions []RegionState  `json:"regions"`
	Flows   []Flow         `json:"flows"`
	Nations []NationState  `json:"nations"`
	Hops    []float64      `json:"hops"` // aligned with WorldInfo.Links
	Made    []bool         `json:"made"` // per good: can anyone make it yet (or still sell some)
}

type PopState struct {
	Size     float64 `json:"size"`
	Cash     float64 `json:"cash"`
	Employed float64 `json:"employed"`
}

type Expansion struct {
	Delivered float64 `json:"delivered"`
	Public    bool    `json:"public"`
}

type FirmState struct {
	Name      string            `json:"name"`
	Output    int               `json:"output"`
	Labor     float64           `json:"labor"`
	Produced  float64           `json:"produced"`
	Inventory float64           `json:"inventory"`
	Cash      float64           `json:"cash"`
	Levels    int               `json:"levels"`
	MaxLevels int               `json:"max_levels"`
	Profit    float64           `json:"profit"`
	Expansion *Expansion        `json:"expansion"`
	Enabled   bool              `json:"enabled"`
	Requires  string            `json:"requires"` // tech that unlocks it, if any
	Sector    string            `json:"sector"`
	State     bool              `json:"state"`      // a public service
	Bonuses   []engine.Modifier `json:"bonuses"`    // productivity, factor by factor
	Wanted    float64           `json:"wanted"`     // workers it wanted (and could pay) today
	PerWorker float64           `json:"per_worker"` // profit per worker today: value added per worker less the wage
	LevelGain float64           `json:"level_gain"` // extra output value a day one more level would bring, same workers and inputs
}

type MarketState struct {
	Good      int     `json:"good"`
	Price     float64 `json:"price"`
	Supply    float64 `json:"supply"`
	Demand    float64 `json:"demand"`
	PerPerson float64 `json:"consumed_per_person"`
}

type ClassState struct {
	Class   string  `json:"class"`
	Size    float64 `json:"size"`
	Cash    float64 `json:"cash"`
	Income  float64 `json:"income"` // per head, today
	Utility float64 `json:"utility"`
}

// HouseState is a tile's trading house.
type HouseState struct {
	Cash   float64 `json:"cash"`
	Spent  float64 `json:"spent"`  // today: goods, transport and duties
	Routes int     `json:"routes"` // by land and sea, one per good and destination
	Active int     `json:"active"` // routes that shipped today
}

// RegionDetail is everything about one region.
type RegionDetail struct {
	House   HouseState    `json:"house"`
	Classes []ClassState  `json:"classes"`
	GDP     float64       `json:"gdp"`
	Info    RegionInfo    `json:"info"`
	State   RegionState   `json:"state"`
	Pops    []PopState    `json:"pops"`
	Firms   []FirmState   `json:"firms"`
	Markets []MarketState `json:"markets"`
	Exports []Flow        `json:"exports"`
	Imports []Flow        `json:"imports"`
}

func worldInfo(w *engine.World) WorldInfo {
	info := WorldInfo{Rows: w.Rows, Cols: w.Cols, Spending: engine.SpendingNames[:], Services: engine.ServiceNames(), Staff: engine.ServiceStaff, MaxPension: engine.MaxPension, Branches: engine.Branches,
		Eras: engine.Eras, StartYear: engine.StartYear,
		LevelCost: engine.LevelCost, TilesPerPort: engine.TilesPerPort, MinOccupiers: engine.MinOccupiers, OccupyStrength: engine.OccupyMinStrength, Port: PortInfo{engine.PortCost, map[string]float64{}}}
	for g, q := range engine.PortMaterials {
		info.Port.Materials[engine.Goods[g].Name] = q
	}
	for _, g := range engine.ConsumerGoods {
		info.Consumer = append(info.Consumer, int(g))
	}
	for c := engine.Class(0); c < engine.NumClasses; c++ {
		info.Classes = append(info.Classes, c.String())
	}
	for sc := engine.Sector(0); sc < 3; sc++ {
		info.Sectors = append(info.Sectors, sc.String())
	}
	for g, good := range engine.Goods {
		tier := ""
		if t := engine.TierOf[g]; t >= 0 {
			tier = t.String()
		}
		info.Goods = append(info.Goods, GoodInfo{good.Name, good.Bulk, good.Spoilage, tier})
	}
	for t := engine.Tier(0); t < engine.NumTiers; t++ {
		info.Tiers = append(info.Tiers, t.String())
	}
	for _, t := range engine.Terrains {
		info.Terrains = append(info.Terrains, TerrainInfo{t.Name, string(t.Symbol), t.MoveCost})
	}
	for _, r := range w.Regions {
		info.Regions = append(info.Regions, RegionInfo{r.ID, r.Name, r.Row, r.Col, r.Terrain, r.Coastal})
	}
	for _, l := range w.Links {
		info.Links = append(info.Links, LinkInfo{l.A, l.B, l.Hop})
	}
	for _, n := range w.Nations {
		info.Nations = append(info.Nations, NationInfo{n.ID, n.Name, n.Color})
	}
	for _, l := range engine.InfraLevels {
		mats := map[string]float64{}
		for g, q := range l.Materials {
			mats[engine.Goods[g].Name] = q
		}
		info.Infra = append(info.Infra, InfraInfo{l.Name, l.MoveMult, l.Cost, l.Upkeep, l.Tech, mats})
	}
	for _, t := range engine.Techs {
		req := t.Requires
		if req == nil {
			req = []string{}
		}
		info.Techs = append(info.Techs, TechInfo{t.Name, t.Branch, t.Era, t.Cost, req, t.Effect, t.Ports})
	}
	for _, d := range engine.Doctrines {
		needs := map[string]float64{}
		for g, q := range d.Needs {
			needs[engine.Goods[g].Name] = q
		}
		info.Doctrines = append(info.Doctrines, DoctrineInfo{d.Name, d.Tech, d.Power, needs})
	}
	return info
}

// cityLine is how many people a tile needs to count as a city (see
// engine.World.IsCity), worked out once for every tile.
func cityLine(w *engine.World) float64 {
	pop := 0.0
	for _, r := range w.Regions {
		pop += r.Population()
	}
	return engine.CityMult * pop / float64(len(w.Regions))
}

func regionState(w *engine.World, i int, cities float64) RegionState {
	r := w.Regions[i]
	ts := r.TierSpend(w.TradePrices[i])
	employed := 0.0
	for _, p := range r.Pops {
		employed += p.Employed()
	}
	st := RegionState{ID: r.ID, Wage: r.Wage, Utility: r.Utility(), Population: r.Population(),
		Employed: employed, TierSpend: ts[:], Prices: r.Prices(), Nation: -1, Infra: r.Infra,
		MoveCost: r.MoveCost(), Port: r.Port, City: r.Population() >= cities, CityBonus: r.SizeBonus(), Growth: r.GrowthRate(),
		Battle: r.Battle}
	if r.Nation != nil {
		st.Nation = r.Nation.ID
	}
	st.Collect, st.Educated, st.Healthy, st.Crowding = r.Collect, r.Educated, r.Healthy, r.Crowding()
	st.Allegiance, st.Loyalties = r.Allegiance(), append([]float64{}, r.Loyalties...)
	for k := engine.Public(0); k < engine.NumServices; k++ {
		f := r.Service(k)
		if f == nil {
			continue
		}
		ss := w.ServiceStats(r, k)
		sv := &ServiceState{Levels: f.Levels, Funded: f.Funded, MaxLevels: f.MaxLevels, Staff: ss.Staff, Need: ss.Need,
			Coverage: ss.Coverage, Research: ss.Research, PerLevel: ss.PerLevel, Cost: ss.Cost, Enabled: f.Enabled}
		if e := f.Expansion; e != nil {
			p := e.Delivered / engine.LevelCost
			sv.Building = &p
		}
		st.Services = append(st.Services, sv)
	}
	if s := r.Siege; s != nil {
		st.Occupation = &Occupation{s.Nation.ID, s.Progress}
	}
	if p := r.Project; p != nil {
		st.Project = &Project{p.Port, p.Target, p.Delivered, p.Required()}
	}
	return st
}

const minFlow = 1e-3

// FlowsAlways is the most tiles a world can have for its state to carry
// every shipment unasked; a bigger one sends them on request (?flows=1),
// as there are tens of thousands of them.
const FlowsAlways = 100

func worldState(w *engine.World, flows bool) State {
	st := State{Tick: w.Tick, Date: w.Date().Format("2 January 2006"), Money: w.Money(), You: -1,
		Flows: []Flow{}, Armies: []ArmyState{}, Made: make([]bool, engine.NumGoods)}
	for g := range st.Made {
		st.Made[g] = w.Made(engine.GoodID(g))
	}
	for _, a := range w.Armies {
		sup, days, src, off := map[string]float64{}, map[string]float64{}, map[string]int{}, map[string]float64{}
		shares := map[string][]SourceShare{}
		for g := range engine.Doctrines[a.Doctrine].Needs {
			name := engine.Goods[g].Name
			sup[name], days[name], src[name] = a.Supply[g], a.StockDays(g), a.Source(g)
			for _, s := range a.Sources(g) {
				off[name] += w.Regions[s.Tile].Markets[g].Supply
				shares[name] = append(shares[name], SourceShare{s.Tile, s.Share})
			}
		}
		coming, comingDays := a.Reinforcements(w.Tick)
		goods, pay := a.Costs()
		cost := map[string]float64{}
		for g, x := range goods {
			cost[engine.Goods[g].Name] = x
		}
		att, def, attr := w.Breakdown(a)
		depot, transport := a.SupplyLine()
		st.Armies = append(st.Armies, ArmyState{a.ID, a.Nation.ID, a.Region, a.Men, a.Target, w.UnderArms(a), a.CanOccupy(), a.Doctrine,
			a.Effectiveness(), a.Strength(), sup, days, src, off, shares, coming, cost, pay, comingDays, a.FullStrength(), w.DailyLoss(a), append([]int{}, a.Path...), a.Retreating,
			att, def, attr, depot, transport})
	}
	cities := cityLine(w)
	for i := range w.Regions {
		st.Regions = append(st.Regions, regionState(w, i, cities))
	}
	for _, t := range w.Traders {
		if (flows || len(w.Regions) <= FlowsAlways) && t.Shipped > minFlow {
			st.Flows = append(st.Flows, Flow{t.Origin.ID, t.Dest.ID, int(t.Good), t.Shipped, t.Sea})
		}
	}
	trade := w.Trade()
	for _, n := range w.Nations {
		ns := NationState{ID: n.ID, Controller: "human", Eliminated: n.Eliminated(), Tiles: len(n.Regions),
			Treasury: n.Treasury, TaxRate: n.TaxRate, Income: n.IncomeEMA, Revenue: n.Revenue,
			TariffRevenue: n.TariffRevenue, Spending: n.Spending, SpendEMA: n.SpendEMA, AutoTax: n.AutoTax, Pension: n.Pension,
			SpentOn: append([]float64(nil), n.SpentOn[:]...), AutoBuild: n.AutoBuild, Tariff: slices.Clone(n.Tariff), Embargo: slices.Clone(n.Embargo),
			Researching: n.Researching, Points: n.ResearchPoints, Known: slices.Clone(n.Known),
			Queue: append([]int{}, n.Queue...), AtWar: slices.Clone(n.AtWar), PeaceOffer: slices.Clone(n.PeaceOffer),
			MobilizeCap: w.MobilizationCap(n), Mobilization: n.Mobilization(), ChooseResearch: n.ChooseResearch,
			MaxArmySize: n.MaxArmySize(), Ports: n.Ports(), PortAllowance: n.PortAllowance(), Standing: n.Standing, Temperament: n.Temper.Describe()}
		ns.Soldiers, _ = w.Mobilized(n)
		ns.PayOwed, ns.Paid = n.PayBill()
		for _, r := range n.Regions {
			if f := r.University(); f != nil {
				ns.ResearchDay += f.LastOutput
			}
		}
		ns.BestDoctrine = bestDoctrine(n)
		ns.AlliedAgainst = -1
		if h := w.AlliedAgainst(n); h != nil {
			ns.AlliedAgainst = h.ID
		}
		ns.Adoption = make([]float64, len(engine.Techs))
		for i := range engine.Techs {
			ns.Adoption[i] = n.Adoption(i)
		}
		ns.GDP = n.RealGDP
		ns.Exports, ns.Imports = trade.NationOut[n.ID], trade.NationIn[n.ID]
		pop := 0.0
		for _, r := range n.Regions {
			pop += r.Population()
		}
		for _, r := range n.Regions {
			ns.Utility += r.Utility() * r.Population() / pop
		}
		for c := engine.Class(0); c < engine.NumClasses; c++ {
			ns.ClassIncome = append(ns.ClassIncome, engine.NationClassIncome(n, c))
		}
		l := w.LaborShares(n)
		ns.Labor = l[:]
		for i := range engine.Techs {
			ns.TechCosts = append(ns.TechCosts, n.TechCost(i, w.Nations))
		}
		if n.AI {
			ns.Controller = "ai"
		}
		for _, r := range n.Regions {
			ns.Population += r.Population()
		}
		st.Nations = append(st.Nations, ns)
	}
	for _, l := range w.Links {
		st.Hops = append(st.Hops, l.Hop)
	}
	return st
}

func regionDetail(w *engine.World, i int) RegionDetail {
	r := w.Regions[i]
	d := RegionDetail{Info: RegionInfo{r.ID, r.Name, r.Row, r.Col, r.Terrain, r.Coastal}, State: regionState(w, i, cityLine(w)), GDP: r.GDP(),
		Exports: []Flow{}, Imports: []Flow{}}
	h := r.House
	d.House = HouseState{Cash: h.Cash, Spent: h.Spent, Routes: len(h.Routes)}
	for _, t := range h.Routes {
		if t.Shipped > 0 {
			d.House.Active++
		}
	}
	for _, p := range r.Pops {
		d.Pops = append(d.Pops, PopState{p.Size, p.Cash, p.Employed()})
	}
	for c := engine.Class(0); c < engine.NumClasses; c++ {
		cs := ClassState{Class: c.String()}
		u := 0.0
		for _, p := range r.ClassPops(c) {
			cs.Size += p.Size
			cs.Cash += p.Cash
			u += p.Utility() * p.Size
		}
		if cs.Size > 0 {
			cs.Income, cs.Utility = r.ClassIncome(c), u/cs.Size
			d.Classes = append(d.Classes, cs)
		}
	}
	for _, f := range r.Firms {
		fs := FirmState{f.Name, int(f.Output), f.Labor, f.LastOutput, f.Inventory, f.Cash,
			f.Levels, f.MaxLevels, f.ProfitEMA, nil, f.Enabled, f.Requires, f.Sector.String(), f.State, f.Bonuses(r.Nation), 0, 0, 0}
		fs.Wanted = f.PlanLabor
		if !f.State && f.Levels < f.MaxLevels {
			fs.LevelGain = f.LastOutput * r.Markets[f.Output].Price * (f.NextLevelMult() - 1)
		}
		if f.Labor > 1e-9 {
			fs.PerWorker = f.ValueAdded(r.Markets[f.Output].Price)/f.Labor - r.Wage
		}
		if e := f.Expansion; e != nil {
			fs.Expansion = &Expansion{e.Delivered, e.Public}
		}
		d.Firms = append(d.Firms, fs)
	}
	pop := r.Population()
	for g, m := range r.Markets {
		consumed := 0.0
		for _, p := range r.Pops {
			consumed += p.Consumed[g]
		}
		d.Markets = append(d.Markets, MarketState{g, m.Price, m.Supply, m.Demand, consumed / pop})
	}
	for _, t := range w.Traders {
		if t.Shipped <= minFlow {
			continue
		}
		f := Flow{t.Origin.ID, t.Dest.ID, int(t.Good), t.Shipped, t.Sea}
		if t.Origin == r {
			d.Exports = append(d.Exports, f)
		}
		if t.Dest == r {
			d.Imports = append(d.Imports, f)
		}
	}
	return d
}

func bestDoctrine(n *engine.Nation) int {
	best := 0
	for i, d := range engine.Doctrines {
		if n.Has(d.Tech) {
			best = i
		}
	}
	return best
}
