package engine

import (
	"errors"
	"fmt"
)

// Snapshot is everything about a game world that carries from one tick to the
// next. Restoring it rebuilds the scenario and overwrites this state, so a
// restored world continues exactly as the original would have.
type Snapshot struct {
	Version     int          `json:"version"`
	Spec        GameSpec     `json:"spec"`
	Tick        int          `json:"tick"`
	Regions     []RegionSnap `json:"regions"`
	Traders     []TraderSnap `json:"traders"`
	Nations     []NationSnap `json:"nations"`
	TradePrices [][]float64  `json:"trade_prices"`
	Ports       []int        `json:"ports"` // in the order they opened
	Armies      []ArmySnap   `json:"armies"`
	NextArmyID  int          `json:"next_army_id"`
	Events      []Event      `json:"events"`
	History     History      `json:"history"`
}

type ArmySnap struct {
	ID, Nation, Region, Doctrine int
	Men, Target, March           float64
	Supply, Stock                []float64
	Path                         []int
	Coming                       []Draft
}

type SiegeSnap struct {
	Nation   int
	Progress float64
}

// SnapshotVersion is bumped whenever the snapshot format changes.
const SnapshotVersion = 11

type MarketSnap struct {
	Price, Supply, Demand float64
}

type PopSnap struct {
	Class      Class
	Size, Cash float64
	Jobs       []Job
	Consumed   []float64
}

type FirmSnap struct {
	TFP, Cash, Inventory, Labor, LastOutput, ProfitEMA float64
	Levels, MaxLevels, LossTicks, Age, Funded          int
	Expansion                                          *Expansion
}

type RegionSnap struct {
	Wage, Neglect, UtilityEMA float64
	HouseCash                 float64
	Infra, Battle             int
	Port                      bool
	Project                   *Project
	Siege                     *SiegeSnap
	Markets                   []MarketSnap
	Pops                      []PopSnap
	Firms                     []FirmSnap
}

type TraderSnap struct {
	Q, StockOrigin, StockDest, RelMargin, Shipped, Margin float64
}

type NationSnap struct {
	Regions                                           []int // in order
	Treasury, TaxRate, Reserve, RebateRate, BuildRate float64
	Tariff                                            []float64
	Embargo                                           []bool
	AutoBuild                                         bool
	PayScale                                          float64
	IncomeEMA, ResearchPoints                         float64
	Researching                                       int
	Known                                             []bool
	Queue                                             []int
	TradeWith                                         []float64
	AtWar, PeaceOffer                                 []bool
	WarSince                                          []int
	AI, ChooseResearch                                bool
	Drill                                             float64
	Revenue, TariffRevenue, Rebate, Spending          float64
}

// ErrNotAGame is returned when snapshotting a world that wasn't built by BuildGame.
var ErrNotAGame = errors.New("only game worlds can be saved")

// Snapshot captures the world's state.
func (w *World) Snapshot() (*Snapshot, error) {
	if w.Spec == nil {
		return nil, ErrNotAGame
	}
	s := &Snapshot{Version: SnapshotVersion, Spec: *w.Spec, Tick: w.Tick, Ports: append([]int(nil), w.Ports...)}
	for _, r := range w.Regions {
		rs := RegionSnap{Wage: r.Wage, Neglect: r.Neglect, UtilityEMA: r.UtilityEMA, HouseCash: r.House.Cash, Infra: r.Infra, Battle: r.Battle, Port: r.Port}
		if r.Siege != nil {
			rs.Siege = &SiegeSnap{r.Siege.Nation.ID, r.Siege.Progress}
		}
		if p := r.Project; p != nil {
			cp := *p
			cp.Materials = map[GoodID]float64{}
			for g, q := range p.Materials {
				cp.Materials[g] = q
			}
			rs.Project = &cp
		}
		for _, m := range r.Markets {
			rs.Markets = append(rs.Markets, MarketSnap{m.Price, m.Supply, m.Demand})
		}
		for _, p := range r.Pops {
			rs.Pops = append(rs.Pops, PopSnap{p.Class, p.Size, p.Cash, append([]Job(nil), p.Jobs...),
				append([]float64(nil), p.Consumed...)})
		}
		for _, f := range r.Firms {
			fs := FirmSnap{f.TFP, f.Cash, f.Inventory, f.Labor, f.LastOutput, f.ProfitEMA,
				f.Levels, f.MaxLevels, f.LossTicks, f.Age, f.Funded, nil}
			if f.Expansion != nil {
				e := *f.Expansion
				fs.Expansion = &e
			}
			rs.Firms = append(rs.Firms, fs)
		}
		s.Regions = append(s.Regions, rs)
	}
	for _, t := range w.Traders {
		s.Traders = append(s.Traders, TraderSnap{t.Q, t.StockOrigin, t.StockDest, t.RelMargin, t.Shipped, t.Margin})
	}
	for _, n := range w.Nations {
		ns := NationSnap{Treasury: n.Treasury, TaxRate: n.TaxRate, Reserve: n.Reserve,
			RebateRate: n.RebateRate, BuildRate: n.MaxBuildRate,
			Tariff: append([]float64(nil), n.Tariff...), Embargo: append([]bool(nil), n.Embargo...),
			AutoBuild: n.AutoBuild, PayScale: n.payScale, Drill: n.Drill, IncomeEMA: n.IncomeEMA, ResearchPoints: n.ResearchPoints,
			Researching: n.Researching, Known: append([]bool(nil), n.Known...), AI: n.AI, ChooseResearch: n.ChooseResearch,
			Queue: append([]int(nil), n.Queue...), TradeWith: append([]float64(nil), n.TradeWith...),
			AtWar: append([]bool(nil), n.AtWar...), WarSince: append([]int(nil), n.WarSince...),
			PeaceOffer: append([]bool(nil), n.PeaceOffer...),
			Revenue:    n.Revenue, TariffRevenue: n.TariffRevenue, Rebate: n.Rebate, Spending: n.Spending}
		for _, r := range n.Regions {
			ns.Regions = append(ns.Regions, r.ID)
		}
		s.Nations = append(s.Nations, ns)
	}
	for _, p := range w.TradePrices {
		s.TradePrices = append(s.TradePrices, append([]float64(nil), p...))
	}
	for _, a := range w.Armies {
		s.Armies = append(s.Armies, ArmySnap{a.ID, a.Nation.ID, a.Region, a.Doctrine, a.Men, a.Target, a.March,
			append([]float64(nil), a.Supply...), append([]float64(nil), a.Stock...), append([]int(nil), a.Path...),
			append([]Draft(nil), a.Coming...)})
	}
	s.NextArmyID = w.NextArmyID
	s.Events = append([]Event(nil), w.Events...)
	s.History = w.History // shared, read-only: Snapshot is serialized straight away
	return s, nil
}

// Restore rebuilds a world from a snapshot.
func Restore(s *Snapshot) (*World, error) {
	if s.Version != SnapshotVersion {
		return nil, fmt.Errorf("snapshot version %d, this build reads %d", s.Version, SnapshotVersion)
	}
	w, err := BuildGame(s.Spec)
	if err != nil {
		return nil, err
	}
	bad := func(what string) (*World, error) {
		return nil, fmt.Errorf("snapshot doesn't fit its scenario: %s", what)
	}
	if len(s.Regions) != len(w.Regions) || len(s.Nations) != len(w.Nations) || len(s.TradePrices) != len(w.Regions) {
		return bad("counts")
	}
	for _, id := range s.Ports { // reopen sea routes in their original order (hops are refreshed below)
		if id < 0 || id >= len(w.Regions) || !s.Regions[id].Port || w.hasPortRoutes(id) {
			return bad("ports")
		}
		w.Regions[id].Port = true
		w.openPort(w.Regions[id])
	}
	if len(s.Traders) != len(w.Traders) {
		return bad("traders")
	}
	w.Tick = s.Tick
	for _, n := range w.Nations {
		n.Regions = nil
	}
	for i, rs := range s.Regions {
		r := w.Regions[i]
		if len(rs.Markets) != len(r.Markets) || len(rs.Pops) != len(r.Pops) || len(rs.Firms) != len(r.Firms) {
			return bad("region " + r.Name)
		}
		if rs.Port && !w.hasPortRoutes(i) {
			return bad("port in " + r.Name)
		}
		r.Wage, r.Neglect, r.UtilityEMA, r.Infra, r.Project, r.Nation = rs.Wage, rs.Neglect, rs.UtilityEMA, rs.Infra, rs.Project, nil
		r.House.Cash, r.Battle = rs.HouseCash, rs.Battle
		if r.Infra < 0 || r.Infra >= len(InfraLevels) || (r.Project != nil && !r.Project.Port && (r.Project.Target <= 0 || r.Project.Target >= len(InfraLevels))) {
			return bad("infrastructure in " + r.Name)
		}
		if r.Project != nil && r.Project.Materials == nil {
			r.Project.Materials = map[GoodID]float64{}
		}
		for g, m := range rs.Markets {
			r.Markets[g].Price, r.Markets[g].Supply, r.Markets[g].Demand = m.Price, m.Supply, m.Demand
		}
		for j, ps := range rs.Pops {
			p := r.Pops[j]
			if len(ps.Consumed) != NumGoods || ps.Class != p.Class {
				return bad("pop in " + r.Name)
			}
			p.Size, p.Cash, p.Jobs = ps.Size, ps.Cash, ps.Jobs
			copy(p.Consumed, ps.Consumed)
		}
		for j, fs := range rs.Firms {
			f := r.Firms[j]
			f.TFP, f.Cash, f.Inventory, f.Labor, f.LastOutput, f.ProfitEMA = fs.TFP, fs.Cash, fs.Inventory, fs.Labor, fs.LastOutput, fs.ProfitEMA
			f.Levels, f.MaxLevels, f.LossTicks, f.Age, f.Expansion = fs.Levels, fs.MaxLevels, fs.LossTicks, fs.Age, fs.Expansion
			f.Funded = fs.Funded
		}
		if len(s.TradePrices[i]) != NumGoods {
			return bad("trade prices")
		}
		copy(w.TradePrices[i], s.TradePrices[i])
	}
	for i, ts := range s.Traders {
		t := w.Traders[i]
		t.Q, t.StockOrigin, t.StockDest, t.RelMargin, t.Shipped, t.Margin = ts.Q, ts.StockOrigin, ts.StockDest, ts.RelMargin, ts.Shipped, ts.Margin
	}
	for i, ns := range s.Nations {
		n := w.Nations[i]
		if len(ns.Tariff) != NumGoods || len(ns.Embargo) != len(w.Nations) || len(ns.Known) != len(Techs) {
			return bad("nation " + n.Name)
		}
		n.Treasury, n.TaxRate, n.Reserve, n.RebateRate, n.MaxBuildRate = ns.Treasury, ns.TaxRate, ns.Reserve, ns.RebateRate, ns.BuildRate
		n.Tariff, n.Embargo, n.AutoBuild, n.IncomeEMA, n.ResearchPoints = ns.Tariff, ns.Embargo, ns.AutoBuild, ns.IncomeEMA, ns.ResearchPoints
		if len(ns.TradeWith) != len(w.Nations) || len(ns.AtWar) != len(w.Nations) || len(ns.WarSince) != len(w.Nations) ||
			len(ns.PeaceOffer) != len(w.Nations) {
			return bad("nation " + n.Name)
		}
		for _, q := range ns.Queue {
			if q < 0 || q >= len(Techs) {
				return bad("research queue")
			}
		}
		if ns.Researching < -1 || ns.Researching >= len(Techs) {
			return bad("research")
		}
		n.Researching, n.Known, n.AI, n.Queue = ns.Researching, ns.Known, ns.AI, ns.Queue
		n.ChooseResearch, n.payScale = ns.ChooseResearch, ns.PayScale
		if err := n.SetDrill(ns.Drill); err != nil {
			return bad("nation " + n.Name)
		}
		n.TradeWith, n.AtWar, n.WarSince, n.PeaceOffer = ns.TradeWith, ns.AtWar, ns.WarSince, ns.PeaceOffer
		n.Revenue, n.TariffRevenue, n.Rebate, n.Spending = ns.Revenue, ns.TariffRevenue, ns.Rebate, ns.Spending
		for _, id := range ns.Regions {
			if id < 0 || id >= len(w.Regions) || w.Regions[id].Nation != nil {
				return bad("nation " + n.Name + " tiles")
			}
			n.Regions = append(n.Regions, w.Regions[id])
			w.Regions[id].Nation = n
		}
	}
	for i, rs := range s.Regions {
		if rs.Siege != nil {
			if rs.Siege.Nation < 0 || rs.Siege.Nation >= len(w.Nations) {
				return bad("siege")
			}
			w.Regions[i].Siege = &Siege{w.Nations[rs.Siege.Nation], rs.Siege.Progress}
		}
	}
	for _, as := range s.Armies {
		if as.Nation < 0 || as.Nation >= len(w.Nations) || as.Region < 0 || as.Region >= len(w.Regions) ||
			as.Doctrine < 0 || as.Doctrine >= len(Doctrines) || len(as.Supply) != NumGoods || len(as.Stock) != NumGoods {
			return bad("army")
		}
		for _, p := range as.Path {
			if p < 0 || p >= len(w.Regions) {
				return bad("army path")
			}
		}
		w.Armies = append(w.Armies, &Army{ID: as.ID, Nation: w.Nations[as.Nation], Region: as.Region,
			Doctrine: as.Doctrine, Men: as.Men, Target: as.Target, March: as.March, Supply: as.Supply,
			Stock: as.Stock, Path: as.Path, Coming: as.Coming})
	}
	w.NextArmyID = s.NextArmyID
	w.Events = append([]Event(nil), s.Events...)
	if h := s.History; len(h.Ticks) > 0 {
		if len(h.Nations) != len(w.Nations) || len(h.Regions) != len(w.Regions) {
			return bad("history")
		}
	}
	w.History = s.History
	// firm gating follows the (restored) owners' technology
	for _, r := range w.Regions {
		if r.Nation == nil {
			for _, f := range r.Firms {
				f.Enabled = firmEnabled(f, nil)
			}
		}
	}
	for _, n := range w.Nations {
		n.applyTech()
		for _, r := range n.Regions { // TFP exactly as saved
			for j, f := range r.Firms {
				f.TFP = s.Regions[r.ID].Firms[j].TFP
			}
		}
	}
	for _, r := range w.Regions {
		r.updateAgglomeration()
	}
	w.refreshAllHops()
	return w, nil
}
