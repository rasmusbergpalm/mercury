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
	Cost     float64  `json:"cost"`
	Requires []string `json:"requires"`
	Effect   string   `json:"effect"`
}

// WorldInfo is the static description of a game's world.
type WorldInfo struct {
	Rows      int           `json:"rows"`
	Cols      int           `json:"cols"`
	Goods     []GoodInfo    `json:"goods"`
	Tiers     []string      `json:"tiers"`
	Terrains  []TerrainInfo `json:"terrains"`
	Regions   []RegionInfo  `json:"regions"`
	Links     []LinkInfo    `json:"links"`
	Nations   []NationInfo  `json:"nations"`
	Infra     []InfraInfo   `json:"infra_levels"`
	Techs     []TechInfo    `json:"techs"`
	Budgets   []string      `json:"budgets"`
	LevelCost float64       `json:"level_cost"` // construction per firm building level
}

type Project struct {
	Target    int     `json:"target"`
	Delivered float64 `json:"delivered"`
	Required  float64 `json:"required"`
}

type RegionState struct {
	ID         int       `json:"id"`
	Wage       float64   `json:"wage"`
	Utility    float64   `json:"utility"`
	Population float64   `json:"population"`
	Employed   float64   `json:"employed"`
	TierSpend  []float64 `json:"tier_spend"`
	Prices     []float64 `json:"prices"`
	Nation     int       `json:"nation"` // -1 if unowned
	Infra      int       `json:"infra"`
	MoveCost   float64   `json:"move_cost"`
	Defense    float64   `json:"defense"`
	Project    *Project  `json:"project"`
}

type Flow struct {
	From int     `json:"from"`
	To   int     `json:"to"`
	Good int     `json:"good"`
	Qty  float64 `json:"qty"`
}

type Campaign struct {
	Target   int     `json:"target"`
	Progress float64 `json:"progress"`
}

type NationState struct {
	ID            int       `json:"id"`
	Controller    string    `json:"controller"` // "ai" or "human"
	Eliminated    bool      `json:"eliminated"`
	Tiles         int       `json:"tiles"`
	Population    float64   `json:"population"`
	Treasury      float64   `json:"treasury"`
	TaxRate       float64   `json:"tax_rate"`
	Income        float64   `json:"income"` // moving average, tax and tariffs
	Revenue       float64   `json:"revenue"`
	TariffRevenue float64   `json:"tariff_revenue"`
	Rebate        float64   `json:"rebate"`
	Spending      float64   `json:"spending"`
	Budget        []float64 `json:"budget"` // economy, science, military
	Tariff        []float64 `json:"tariff"` // per good
	Embargo       []bool    `json:"embargo"`
	Researching   int       `json:"researching"`
	Points        float64   `json:"research_points"`
	Known         []bool    `json:"known"`
	Army          float64   `json:"army"`
	Defense       float64   `json:"defense"`
	Campaign      *Campaign `json:"campaign"`
}

// State is the dynamic state of a game.
type State struct {
	Game    string        `json:"game"`
	Tick    int           `json:"tick"`
	Running bool          `json:"running"`
	Speed   float64       `json:"speed"`
	You     int           `json:"you"` // the requester's nation, or -1
	Money   float64       `json:"money"`
	Regions []RegionState `json:"regions"`
	Flows   []Flow        `json:"flows"`
	Nations []NationState `json:"nations"`
	Hops    []float64     `json:"hops"` // aligned with WorldInfo.Links
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
}

type MarketState struct {
	Good      int     `json:"good"`
	Price     float64 `json:"price"`
	Supply    float64 `json:"supply"`
	Demand    float64 `json:"demand"`
	PerPerson float64 `json:"consumed_per_person"`
}

// RegionDetail is everything about one region.
type RegionDetail struct {
	Info    RegionInfo    `json:"info"`
	State   RegionState   `json:"state"`
	Pops    []PopState    `json:"pops"`
	Firms   []FirmState   `json:"firms"`
	Markets []MarketState `json:"markets"`
	Exports []Flow        `json:"exports"`
	Imports []Flow        `json:"imports"`
}

func worldInfo(w *engine.World) WorldInfo {
	info := WorldInfo{Rows: w.Rows, Cols: w.Cols, Budgets: engine.BudgetNames[:], LevelCost: engine.LevelCost}
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
		info.Regions = append(info.Regions, RegionInfo{r.ID, r.Name, r.Row, r.Col, r.Terrain})
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
		info.Techs = append(info.Techs, TechInfo{t.Name, t.Cost, req, t.Effect})
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
		MoveCost: r.MoveCost()}
	if r.Nation != nil {
		st.Nation, st.Defense = r.Nation.ID, r.Nation.Defense()
	}
	if p := r.Project; p != nil {
		st.Project = &Project{p.Target, p.Delivered, p.Required()}
	}
	return st
}

const minFlow = 1e-3

func worldState(w *engine.World) State {
	st := State{Tick: w.Tick, Money: w.Money(), You: -1, Flows: []Flow{}}
	for i := range w.Regions {
		st.Regions = append(st.Regions, regionState(w, i))
	}
	for _, t := range w.Traders {
		if t.Shipped > minFlow {
			st.Flows = append(st.Flows, Flow{t.Origin.ID, t.Dest.ID, int(t.Good), t.Shipped})
		}
	}
	for _, n := range w.Nations {
		ns := NationState{ID: n.ID, Controller: "human", Eliminated: n.Eliminated(), Tiles: len(n.Regions),
			Treasury: n.Treasury, TaxRate: n.TaxRate, Income: n.IncomeEMA, Revenue: n.Revenue,
			TariffRevenue: n.TariffRevenue, Rebate: n.Rebate, Spending: n.Spending,
			Budget: append([]float64(nil), n.Budget[:]...), Tariff: n.Tariff, Embargo: n.Embargo,
			Researching: n.Researching, Points: n.ResearchPoints, Known: n.Known, Army: n.Army,
			Defense: n.Defense()}
		if n.AI {
			ns.Controller = "ai"
		}
		for _, r := range n.Regions {
			ns.Population += r.Population()
		}
		if c := n.Campaign; c != nil {
			ns.Campaign = &Campaign{c.Target, c.Progress}
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
	d := RegionDetail{Info: RegionInfo{r.ID, r.Name, r.Row, r.Col, r.Terrain}, State: regionState(w, i),
		Exports: []Flow{}, Imports: []Flow{}}
	for _, p := range r.Pops {
		d.Pops = append(d.Pops, PopState{p.Size, p.Cash, p.Employed()})
	}
	for _, f := range r.Firms {
		fs := FirmState{f.Name, int(f.Output), f.Labor, f.LastOutput, f.Inventory, f.Cash,
			f.Levels, f.MaxLevels, f.ProfitEMA, nil}
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
		f := Flow{t.Origin.ID, t.Dest.ID, int(t.Good), t.Shipped}
		if t.Origin == r {
			d.Exports = append(d.Exports, f)
		}
		if t.Dest == r {
			d.Imports = append(d.Imports, f)
		}
	}
	return d
}
