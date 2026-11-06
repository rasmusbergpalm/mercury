package server

import (
	"github.com/rasmusbergpalm/mercury/engine"
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
	Rows      int            `json:"rows"`
	Cols      int            `json:"cols"`
	Goods     []GoodInfo     `json:"goods"`
	Tiers     []string       `json:"tiers"`
	Terrains  []TerrainInfo  `json:"terrains"`
	Regions   []RegionInfo   `json:"regions"`
	Links     []LinkInfo     `json:"links"`
	Nations   []NationInfo   `json:"nations"`
	Infra     []InfraInfo    `json:"infra_levels"`
	Techs     []TechInfo     `json:"techs"`
	Budgets   []string       `json:"budgets"`
	Branches  []string       `json:"branches"`
	LevelCost float64        `json:"level_cost"` // construction per firm building level
	Port      PortInfo       `json:"port"`
	Eras      []string       `json:"eras"`
	Doctrines []DoctrineInfo `json:"doctrines"`
	StartYear int            `json:"start_year"`
	Consumer  []int          `json:"consumer_goods"` // the order of RegionHistory.Prices
	Classes   []string       `json:"classes"`
	Sectors   []string       `json:"sectors"`
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
	ID         int         `json:"id"`
	Wage       float64     `json:"wage"`
	Utility    float64     `json:"utility"`
	Population float64     `json:"population"`
	Employed   float64     `json:"employed"`
	TierSpend  []float64   `json:"tier_spend"`
	Prices     []float64   `json:"prices"`
	Nation     int         `json:"nation"` // -1 if unowned
	Infra      int         `json:"infra"`
	MoveCost   float64     `json:"move_cost"`
	Port       bool        `json:"port"`
	City       bool        `json:"city"`
	CityBonus  float64     `json:"city_bonus"`
	Growth     float64     `json:"growth"` // population growth per year
	Occupation *Occupation `json:"occupation"`
	Project    *Project    `json:"project"`
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
	Rebate         float64   `json:"rebate"`
	Spending       float64   `json:"spending"`
	Budget         []float64 `json:"budget"` // economy, science, military
	Tariff         []float64 `json:"tariff"` // per good
	Embargo        []bool    `json:"embargo"`
	Researching    int       `json:"researching"`
	Points         float64   `json:"research_points"`
	Known          []bool    `json:"known"`
	Queue          []int     `json:"queue"`
	ChooseResearch bool      `json:"choose_research"` // a tech finished: ask the player for the next
	TechCosts      []float64 `json:"tech_costs"`      // after the discount from trade partners
	AtWar          []bool    `json:"at_war"`
	PeaceOffer     []bool    `json:"peace_offer"` // this nation has offered peace to
	Soldiers       float64   `json:"soldiers"`
	MobilizeCap    float64   `json:"mobilize_cap"`
	MaxArmySize    float64   `json:"max_army_size"` // men one army can field
	Drill          float64   `json:"drill"`         // share of wartime supplies armies use in peace
	Mobilization   float64   `json:"mobilization"`  // share of people who may serve
	BestDoctrine   int       `json:"best_doctrine"`
	GDP            float64   `json:"gdp"`
	Utility        float64   `json:"utility"`
	ClassIncome    []float64 `json:"class_income"` // per head, by class
	Labor          []float64 `json:"labor"`        // share of workers by sector (industry, primary, service)
	Exports        []float64 `json:"exports"`      // value per good, today
	Imports        []float64 `json:"imports"`
}

type ArmyState struct {
	ID            int                `json:"id"`
	Nation        int                `json:"nation"`
	Region        int                `json:"region"`
	Men           float64            `json:"men"`
	Target        float64            `json:"target"`
	Doctrine      int                `json:"doctrine"`
	Effectiveness float64            `json:"effectiveness"`
	Strength      float64            `json:"strength"`
	Supply        map[string]float64 `json:"supply"`        // per needed good, 0..1
	StockDays     map[string]float64 `json:"stock_days"`    // days of each good it carries
	Sources       map[string]int     `json:"sources"`       // tile each good was bought in today (-1: none)
	FullStrength  float64            `json:"full_strength"` // at full size, fully supplied
	DailyLoss     float64            `json:"daily_loss"`    // share of men lost today outside battle
	Path          []int              `json:"path"`
	Attack        []engine.Modifier  `json:"attack"`    // strength per soldier, factor by factor
	Defense       []engine.Modifier  `json:"defense"`   // extra factors when defending at home
	Attrition     []engine.Modifier  `json:"attrition"` // daily losses
	Depot         int                `json:"depot"`     // where supplies are bought (-1: cut off)
	Transport     float64            `json:"transport"` // transport today's supplies needed
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
	Name      string     `json:"name"`
	Output    int        `json:"output"`
	Labor     float64    `json:"labor"`
	Produced  float64    `json:"produced"`
	Inventory float64    `json:"inventory"`
	Cash      float64    `json:"cash"`
	Levels    int        `json:"levels"`
	MaxLevels int        `json:"max_levels"`
	Profit    float64    `json:"profit"`
	Expansion *Expansion `json:"expansion"`
	Enabled   bool       `json:"enabled"`
	Requires  string     `json:"requires"` // tech that unlocks it, if any
	Sector    string     `json:"sector"`
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
	info := WorldInfo{Rows: w.Rows, Cols: w.Cols, Budgets: engine.BudgetNames[:], Branches: engine.Branches,
		Eras: engine.Eras, StartYear: engine.StartYear,
		LevelCost: engine.LevelCost, Port: PortInfo{engine.PortCost, map[string]float64{}}}
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

func regionState(w *engine.World, i int) RegionState {
	r := w.Regions[i]
	ts := r.TierSpend(w.TradePrices[i])
	employed := 0.0
	for _, p := range r.Pops {
		employed += p.Employed()
	}
	st := RegionState{ID: r.ID, Wage: r.Wage, Utility: r.Utility(), Population: r.Population(),
		Employed: employed, TierSpend: ts[:], Prices: r.Prices(), Nation: -1, Infra: r.Infra,
		MoveCost: r.MoveCost(), Port: r.Port, City: w.IsCity(r), CityBonus: r.SizeBonus(), Growth: r.GrowthRate()}
	if r.Nation != nil {
		st.Nation = r.Nation.ID
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

func worldState(w *engine.World) State {
	st := State{Tick: w.Tick, Date: w.Date().Format("2 January 2006"), Money: w.Money(), You: -1,
		Flows: []Flow{}, Armies: []ArmyState{}}
	for _, a := range w.Armies {
		sup, days, src := map[string]float64{}, map[string]float64{}, map[string]int{}
		for g := range engine.Doctrines[a.Doctrine].Needs {
			sup[engine.Goods[g].Name] = a.Supply[g]
			days[engine.Goods[g].Name] = a.StockDays(g)
			src[engine.Goods[g].Name] = a.Source(g)
		}
		att, def, attr := w.Breakdown(a)
		depot, transport := a.SupplyLine()
		st.Armies = append(st.Armies, ArmyState{a.ID, a.Nation.ID, a.Region, a.Men, a.Target, a.Doctrine,
			a.Effectiveness(), a.Strength(), sup, days, src, a.FullStrength(), w.DailyLoss(a), append([]int{}, a.Path...),
			att, def, attr, depot, transport})
	}
	for i := range w.Regions {
		st.Regions = append(st.Regions, regionState(w, i))
	}
	for _, t := range w.Traders {
		if t.Shipped > minFlow {
			st.Flows = append(st.Flows, Flow{t.Origin.ID, t.Dest.ID, int(t.Good), t.Shipped, t.Sea})
		}
	}
	trade := w.Trade()
	for _, n := range w.Nations {
		ns := NationState{ID: n.ID, Controller: "human", Eliminated: n.Eliminated(), Tiles: len(n.Regions),
			Treasury: n.Treasury, TaxRate: n.TaxRate, Income: n.IncomeEMA, Revenue: n.Revenue,
			TariffRevenue: n.TariffRevenue, Rebate: n.Rebate, Spending: n.Spending,
			Budget: append([]float64(nil), n.Budget[:]...), Tariff: n.Tariff, Embargo: n.Embargo,
			Researching: n.Researching, Points: n.ResearchPoints, Known: n.Known,
			Queue: append([]int{}, n.Queue...), AtWar: n.AtWar, PeaceOffer: n.PeaceOffer,
			MobilizeCap: w.MobilizationCap(n), Mobilization: n.Mobilization(), ChooseResearch: n.ChooseResearch,
			MaxArmySize: n.MaxArmySize(), Drill: n.Drill}
		ns.Soldiers, _ = w.Mobilized(n)
		ns.BestDoctrine = bestDoctrine(n)
		ns.Exports, ns.Imports = trade.NationOut[n.ID], trade.NationIn[n.ID]
		pop := 0.0
		for _, r := range n.Regions {
			ns.GDP += r.GDP()
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
	d := RegionDetail{Info: RegionInfo{r.ID, r.Name, r.Row, r.Col, r.Terrain, r.Coastal}, State: regionState(w, i), GDP: r.GDP(),
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
			f.Levels, f.MaxLevels, f.ProfitEMA, nil, f.Enabled, f.Requires, f.Sector.String()}
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
