package economy

import (
	"errors"
	"fmt"
)

// Plan is what a world is built from (BuildWorld): its map and the cost of
// transport.
type Plan struct {
	Graph         Graph
	TransportCost float64
}

// Snapshot is everything about an economy that carries from one tick to the
// next, its levers and accounts among them: restoring it gives a world that
// goes on exactly as the original would have. It is plain data, made to be
// written as JSON: a series of them is a record of an experiment, tick by
// tick.
type Snapshot struct {
	Plan                                        Plan
	Tick                                        int
	Investment, Migration, Demography, TrackGDP bool
	Starvation                                  bool `json:",omitempty"`
	Seed                                        int64
	Regions                                     []RegionState
	Traders                                     []TraderState
	TradePrices                                 [][]float64
}

// RegionState is a tile's state in a Snapshot.
type RegionState struct {
	Wage, UtilityEMA, HouseCash float64
	// levers and accounts
	IncomeTax, PaybackMult, GrowthBonus               float64
	LandTransportMult, SeaTransportMult, SpoilageMult float64
	SpendShare                                        float64
	Withheld, Duties                                  float64
	Buyers                                            []BuyerState `json:",omitempty"`
	Markets                                           []MarketState
	Pops                                              []PopState
	Firms                                             []FirmState
	Real                                              GDPChain // real GDP's chain
}

// MarketState is a market's state in a Snapshot.
type MarketState struct {
	Price, Supply, Demand, Trend float64
}

// PopState is a pop's state in a Snapshot.
type PopState struct {
	Class      Class
	Size, Cash float64
	Jobs       []Job
	Consumed   []float64
	Want       float64 `json:",omitempty"`
}

// FirmState is a firm's state in a Snapshot.
type FirmState struct {
	Cash, Inventory, Labor, LastOutput, ProfitEMA float64
	Levels, MaxLevels, LossTicks, Age             int
	Expansion                                     *Expansion
	Wear, Urge, Mult                              float64
	Monopoly                                      bool    `json:",omitempty"`
	Elasticity, Share                             float64 `json:",omitempty"`
	Projects                                      int
	Enabled                                       bool
	// per method: productivity, whether it works, and the levels it runs
	// (the first method's are the rest)
	TFP          []float64
	MethodOn     []bool
	MethodLevels []int `json:",omitempty"`
}

// TraderState is a route's state in a Snapshot.
type TraderState struct {
	Q, RelMargin, Margin, Distance float64
	// zero on idle routes, most of them on a big map: left out
	StockOrigin, StockDest, Shipped, Tariff float64 `json:",omitempty"`
	Closed                                  bool    `json:",omitempty"`
}

// BuyerState is an outside buyer's state in a Snapshot.
type BuyerState struct {
	Bid, Got    []float64
	Hire, Hired float64 `json:",omitempty"`
	Account     float64
}

// Snapshot captures the economy's state.
func (w *World) Snapshot() *Snapshot {
	s := &Snapshot{Plan: w.Plan, Tick: w.Tick, Investment: w.Investment, Migration: w.Migration,
		Demography: w.Demography, TrackGDP: w.TrackGDP, Starvation: w.Starvation, Seed: w.Seed}
	for _, r := range w.Regions {
		rs := RegionState{Wage: r.Wage, UtilityEMA: r.UtilityEMA, HouseCash: r.House.Cash, IncomeTax: r.IncomeTax, PaybackMult: r.PaybackMult,
			GrowthBonus: r.GrowthBonus, LandTransportMult: r.LandTransportMult, SeaTransportMult: r.SeaTransportMult, SpoilageMult: r.SpoilageMult, SpendShare: r.SpendShare,
			Withheld: r.Withheld, Duties: r.Duties, Real: r.Real.Clone()}
		for _, b := range r.Buyers {
			rs.Buyers = append(rs.Buyers, BuyerState{append([]float64(nil), b.Bid...), append([]float64(nil), b.Got...), b.Hire, b.Hired, b.Account})
		}
		for _, m := range r.Markets {
			rs.Markets = append(rs.Markets, MarketState{m.Price, m.Supply, m.Demand, m.Trend})
		}
		for _, p := range r.Pops {
			rs.Pops = append(rs.Pops, PopState{p.Class, p.Size, p.Cash, append([]Job(nil), p.Jobs...),
				append([]float64(nil), p.Consumed...), p.Want})
		}
		for _, f := range r.Firms {
			fs := FirmState{Cash: f.Cash, Inventory: f.Inventory, Labor: f.Labor, LastOutput: f.LastOutput, ProfitEMA: f.ProfitEMA,
				Levels: f.Levels, MaxLevels: f.MaxLevels, LossTicks: f.LossTicks, Age: f.Age,
				Wear: f.Wear, Urge: f.Urge, Mult: f.Mult, Projects: f.Projects, Enabled: f.Enabled,
				Monopoly: f.Monopoly, Elasticity: f.Elasticity, Share: f.share}
			for i, m := range f.Methods {
				fs.TFP = append(fs.TFP, m.TFP)
				fs.MethodOn = append(fs.MethodOn, m.Enabled)
				if i > 0 {
					fs.MethodLevels = append(fs.MethodLevels, m.Levels)
				}
			}
			if f.Expansion != nil {
				e := *f.Expansion
				fs.Expansion = &e
			}
			rs.Firms = append(rs.Firms, fs)
		}
		s.Regions = append(s.Regions, rs)
	}
	for _, t := range w.Traders {
		s.Traders = append(s.Traders, TraderState{Q: t.Q, RelMargin: t.RelMargin, Margin: t.Margin, Distance: t.Distance,
			StockOrigin: t.StockOrigin, StockDest: t.StockDest, Shipped: t.Shipped, Tariff: t.Tariff, Closed: t.Closed})
	}
	for _, p := range w.TradePrices {
		s.TradePrices = append(s.TradePrices, append([]float64(nil), p...))
	}
	return s
}

// Restore builds the world a snapshot was taken of, and gives it the
// snapshot's state.
func Restore(s *Snapshot) (*World, error) {
	w, err := BuildWorld(s.Plan.TransportCost, s.Plan.Graph)
	if err != nil {
		return nil, err
	}
	for _, rs := range s.Regions {
		if len(rs.Buyers) > 0 {
			return nil, errors.New("the snapshot has outside buyers: add them to the world and use SetState")
		}
	}
	if err := w.SetState(s); err != nil {
		return nil, err
	}
	return w, nil
}

// SetState gives a world built as the snapshot's was (with the same
// routes, firms and outside buyers added since, in the same order) the
// snapshot's state.
func (w *World) SetState(s *Snapshot) error {
	bad := func(what string) error {
		return fmt.Errorf("snapshot doesn't fit the world: %s", what)
	}
	if len(s.Regions) != len(w.Regions) || len(s.TradePrices) != len(w.Regions) || len(s.Traders) != len(w.Traders) {
		return bad("counts")
	}
	w.Tick, w.Investment, w.Migration, w.Demography, w.TrackGDP, w.Seed = s.Tick, s.Investment, s.Migration, s.Demography, s.TrackGDP, s.Seed
	w.Starvation = s.Starvation
	for i, rs := range s.Regions {
		r := w.Regions[i]
		if len(rs.Markets) != len(r.Markets) || len(rs.Pops) != len(r.Pops) || len(rs.Firms) != len(r.Firms) || len(rs.Buyers) != len(r.Buyers) {
			return bad("tile " + r.Name)
		}
		r.Wage, r.UtilityEMA, r.House.Cash = rs.Wage, rs.UtilityEMA, rs.HouseCash
		r.IncomeTax, r.PaybackMult, r.GrowthBonus = rs.IncomeTax, rs.PaybackMult, rs.GrowthBonus
		r.LandTransportMult, r.SeaTransportMult, r.SpoilageMult = rs.LandTransportMult, rs.SeaTransportMult, rs.SpoilageMult
		r.SpendShare = rs.SpendShare
		r.Withheld, r.Duties = rs.Withheld, rs.Duties
		for j, bs := range rs.Buyers {
			b := r.Buyers[j]
			if len(bs.Bid) != NumGoods || len(bs.Got) != NumGoods {
				return bad("buyer in " + r.Name)
			}
			copy(b.Bid, bs.Bid)
			copy(b.Got, bs.Got)
			b.Account, b.Hire, b.Hired = bs.Account, bs.Hire, bs.Hired
		}
		r.Real = rs.Real.Clone()
		if !r.Real.Valid() {
			return bad("real GDP in " + r.Name)
		}
		for g, m := range rs.Markets {
			r.Markets[g].Price, r.Markets[g].Supply, r.Markets[g].Demand, r.Markets[g].Trend = m.Price, m.Supply, m.Demand, m.Trend
		}
		for j, ps := range rs.Pops {
			p := r.Pops[j]
			if len(ps.Consumed) != NumGoods || ps.Class != p.Class {
				return bad("people in " + r.Name)
			}
			p.Size, p.Cash, p.Jobs, p.Want = ps.Size, ps.Cash, append([]Job(nil), ps.Jobs...), ps.Want // (copies: one snapshot may restore many worlds)
			copy(p.Consumed, ps.Consumed)
		}
		for j, fs := range rs.Firms {
			f := r.Firms[j]
			if len(fs.TFP) != len(f.Methods) || len(fs.MethodOn) != len(f.Methods) || len(fs.MethodLevels) != len(f.Methods)-1 {
				return bad("firm methods in " + r.Name)
			}
			f.Cash, f.Inventory, f.Labor, f.LastOutput, f.ProfitEMA = fs.Cash, fs.Inventory, fs.Labor, fs.LastOutput, fs.ProfitEMA
			for k, m := range f.Methods {
				m.TFP, m.Enabled = fs.TFP[k], fs.MethodOn[k]
				if k > 0 {
					m.Levels = fs.MethodLevels[k-1]
				}
			}
			f.Levels, f.MaxLevels, f.LossTicks, f.Age, f.Expansion = fs.Levels, fs.MaxLevels, fs.LossTicks, fs.Age, nil
			if fs.Expansion != nil {
				e := *fs.Expansion
				f.Expansion = &e
			}
			f.Wear, f.Urge, f.Mult, f.Projects = fs.Wear, fs.Urge, fs.Mult, fs.Projects
			f.Enabled = fs.Enabled
			f.Monopoly, f.Elasticity, f.share = fs.Monopoly, fs.Elasticity, fs.Share
		}
		if len(s.TradePrices[i]) != NumGoods {
			return bad("trade prices")
		}
		copy(w.TradePrices[i], s.TradePrices[i])
	}
	for i, ts := range s.Traders {
		t := w.Traders[i]
		t.Q, t.StockOrigin, t.StockDest, t.RelMargin, t.Shipped, t.Margin = ts.Q, ts.StockOrigin, ts.StockDest, ts.RelMargin, ts.Shipped, ts.Margin
		t.Tariff, t.Closed = ts.Tariff, ts.Closed
		t.SetDistance(ts.Distance)
	}
	for _, r := range w.Regions {
		r.UpdateAgglomeration()
	}
	w.RefreshHops()
	return nil
}
