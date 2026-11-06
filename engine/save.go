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
}

// SnapshotVersion is bumped whenever the snapshot format changes.
const SnapshotVersion = 1

type MarketSnap struct {
	Price, Supply, Demand float64
}

type PopSnap struct {
	Size, Cash float64
	Jobs       []Job
	Consumed   []float64
}

type FirmSnap struct {
	TFP, Cash, Inventory, Labor, LastOutput, ProfitEMA float64
	Levels, MaxLevels, LossTicks, Age                  int
	Expansion                                          *Expansion
}

type RegionSnap struct {
	Wage, Neglect, UtilityEMA float64
	Infra                     int
	Project                   *Project
	Markets                   []MarketSnap
	Pops                      []PopSnap
	Firms                     []FirmSnap
}

type TraderSnap struct {
	Q, StockOrigin, StockDest, Cash, RelMargin, Shipped, Margin float64
}

type NationSnap struct {
	Regions                                           []int // in order
	Treasury, TaxRate, Reserve, RebateRate, BuildRate float64
	Tariff                                            []float64
	Embargo                                           []bool
	Budget                                            [NumBudgets]float64
	IncomeEMA, ResearchPoints                         float64
	Researching                                       int
	Known                                             []bool
	Army                                              float64
	Campaign                                          *Campaign
	AI                                                bool
	Revenue, TariffRevenue, Rebate, Spending          float64
}

// ErrNotAGame is returned when snapshotting a world that wasn't built by BuildGame.
var ErrNotAGame = errors.New("only game worlds can be saved")

// Snapshot captures the world's state.
func (w *World) Snapshot() (*Snapshot, error) {
	if w.Spec == nil {
		return nil, ErrNotAGame
	}
	s := &Snapshot{Version: SnapshotVersion, Spec: *w.Spec, Tick: w.Tick}
	for _, r := range w.Regions {
		rs := RegionSnap{Wage: r.Wage, Neglect: r.Neglect, UtilityEMA: r.UtilityEMA, Infra: r.Infra}
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
			rs.Pops = append(rs.Pops, PopSnap{p.Size, p.Cash, append([]Job(nil), p.Jobs...),
				append([]float64(nil), p.Consumed...)})
		}
		for _, f := range r.Firms {
			fs := FirmSnap{f.TFP, f.Cash, f.Inventory, f.Labor, f.LastOutput, f.ProfitEMA,
				f.Levels, f.MaxLevels, f.LossTicks, f.Age, nil}
			if f.Expansion != nil {
				e := *f.Expansion
				fs.Expansion = &e
			}
			rs.Firms = append(rs.Firms, fs)
		}
		s.Regions = append(s.Regions, rs)
	}
	for _, t := range w.Traders {
		s.Traders = append(s.Traders, TraderSnap{t.Q, t.StockOrigin, t.StockDest, t.Cash, t.RelMargin, t.Shipped, t.Margin})
	}
	for _, n := range w.Nations {
		ns := NationSnap{Treasury: n.Treasury, TaxRate: n.TaxRate, Reserve: n.Reserve,
			RebateRate: n.RebateRate, BuildRate: n.MaxBuildRate,
			Tariff: append([]float64(nil), n.Tariff...), Embargo: append([]bool(nil), n.Embargo...),
			Budget: n.Budget, IncomeEMA: n.IncomeEMA, ResearchPoints: n.ResearchPoints,
			Researching: n.Researching, Known: append([]bool(nil), n.Known...), Army: n.Army, AI: n.AI,
			Revenue: n.Revenue, TariffRevenue: n.TariffRevenue, Rebate: n.Rebate, Spending: n.Spending}
		for _, r := range n.Regions {
			ns.Regions = append(ns.Regions, r.ID)
		}
		if n.Campaign != nil {
			c := *n.Campaign
			ns.Campaign = &c
		}
		s.Nations = append(s.Nations, ns)
	}
	for _, p := range w.TradePrices {
		s.TradePrices = append(s.TradePrices, append([]float64(nil), p...))
	}
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
	if len(s.Regions) != len(w.Regions) || len(s.Traders) != len(w.Traders) || len(s.Nations) != len(w.Nations) ||
		len(s.TradePrices) != len(w.Regions) {
		return bad("counts")
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
		r.Wage, r.Neglect, r.UtilityEMA, r.Infra, r.Project, r.Nation = rs.Wage, rs.Neglect, rs.UtilityEMA, rs.Infra, rs.Project, nil
		if r.Infra < 0 || r.Infra >= len(InfraLevels) || (r.Project != nil && (r.Project.Target <= 0 || r.Project.Target >= len(InfraLevels))) {
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
			if len(ps.Consumed) != NumGoods {
				return bad("pop in " + r.Name)
			}
			p.Size, p.Cash, p.Jobs = ps.Size, ps.Cash, ps.Jobs
			copy(p.Consumed, ps.Consumed)
		}
		for j, fs := range rs.Firms {
			f := r.Firms[j]
			f.TFP, f.Cash, f.Inventory, f.Labor, f.LastOutput, f.ProfitEMA = fs.TFP, fs.Cash, fs.Inventory, fs.Labor, fs.LastOutput, fs.ProfitEMA
			f.Levels, f.MaxLevels, f.LossTicks, f.Age, f.Expansion = fs.Levels, fs.MaxLevels, fs.LossTicks, fs.Age, fs.Expansion
		}
		if len(s.TradePrices[i]) != NumGoods {
			return bad("trade prices")
		}
		copy(w.TradePrices[i], s.TradePrices[i])
	}
	for i, ts := range s.Traders {
		t := w.Traders[i]
		t.Q, t.StockOrigin, t.StockDest, t.Cash, t.RelMargin, t.Shipped, t.Margin = ts.Q, ts.StockOrigin, ts.StockDest, ts.Cash, ts.RelMargin, ts.Shipped, ts.Margin
	}
	for i, ns := range s.Nations {
		n := w.Nations[i]
		if len(ns.Tariff) != NumGoods || len(ns.Embargo) != len(w.Nations) || len(ns.Known) != len(Techs) {
			return bad("nation " + n.Name)
		}
		n.Treasury, n.TaxRate, n.Reserve, n.RebateRate, n.MaxBuildRate = ns.Treasury, ns.TaxRate, ns.Reserve, ns.RebateRate, ns.BuildRate
		n.Tariff, n.Embargo, n.Budget, n.IncomeEMA, n.ResearchPoints = ns.Tariff, ns.Embargo, ns.Budget, ns.IncomeEMA, ns.ResearchPoints
		n.Researching, n.Known, n.Army, n.Campaign, n.AI = ns.Researching, ns.Known, ns.Army, ns.Campaign, ns.AI
		n.Revenue, n.TariffRevenue, n.Rebate, n.Spending = ns.Revenue, ns.TariffRevenue, ns.Rebate, ns.Spending
		for _, id := range ns.Regions {
			if id < 0 || id >= len(w.Regions) || w.Regions[id].Nation != nil {
				return bad("nation " + n.Name + " tiles")
			}
			n.Regions = append(n.Regions, w.Regions[id])
			w.Regions[id].Nation = n
		}
	}
	w.refreshAllHops()
	return w, nil
}
