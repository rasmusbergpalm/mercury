package engine

import "math"

// HistoryEvery is how often (in days) game worlds record their statistics.
const HistoryEvery = 30

// History is a game's statistics over time, one sample every HistoryEvery days.
type History struct {
	Ticks   []int            `json:"ticks"`
	Nations []*NationHistory `json:"nations"`
	Regions []*RegionHistory `json:"regions"`
	Armies  []*ArmyHistory   `json:"armies"`
}

// NationHistory: per-sample series for one nation. Money is per day.
type NationHistory struct {
	Population  []float64             `json:"population"`
	Tiles       []float64             `json:"tiles"`
	Income      []float64             `json:"income"` // tax and tariffs
	Treasury    []float64             `json:"treasury"`
	GDP         []float64             `json:"gdp"` // value of everything produced
	Utility     []float64             `json:"utility"`
	Soldiers    []float64             `json:"soldiers"`
	Strength    []float64             `json:"strength"`
	Techs       []float64             `json:"techs"`
	Exports     []float64             `json:"exports"` // value shipped to other nations or free land
	Imports     []float64             `json:"imports"`
	ClassIncome [NumClasses][]float64 `json:"class_income"` // per head
	Labor       [3][]float64          `json:"labor"`        // share of workers by Sector
	ExportsGood [][]float64           `json:"exports_by_good"`
	ImportsGood [][]float64           `json:"imports_by_good"`
}

// RegionHistory: per-sample series for one tile.
type RegionHistory struct {
	Population  []float64             `json:"population"`
	Wage        []float64             `json:"wage"`
	Utility     []float64             `json:"utility"`
	GDP         []float64             `json:"gdp"`
	Exports     []float64             `json:"exports"` // value shipped out of the tile
	Imports     []float64             `json:"imports"`
	Owner       []float64             `json:"owner"` // nation id, -1 unowned
	ClassIncome [NumClasses][]float64 `json:"class_income"`
	Prices      [][]float64           `json:"prices"` // consumer goods, in ConsumerGoods order
}

// ArmyHistory: per-sample series for one army, from the sample it first appeared in.
type ArmyHistory struct {
	ID            int       `json:"id"`
	Nation        int       `json:"nation"`
	From          int       `json:"from"` // index into History.Ticks of its first sample
	Men           []float64 `json:"men"`
	Strength      []float64 `json:"strength"`
	Effectiveness []float64 `json:"effectiveness"`
}

// GDP is the value the tile's firms added today, in pounds: their output at
// today's prices less the goods they used up, so coal burnt in a smithy is not
// counted twice.
func (r *Region) GDP() float64 {
	s := 0.0
	for _, f := range r.Firms {
		s += f.ValueAdded(r.Markets[f.Output].Price)
	}
	return s
}

// ClassIncome is a class's income per head on the tile this tick (0 if absent).
func (r *Region) ClassIncome(c Class) float64 {
	inc, size := 0.0, 0.0
	for _, p := range r.ClassPops(c) {
		inc += p.Income
		size += p.Size
	}
	if size == 0 {
		return 0
	}
	return inc / size
}

// TradeStats are this tick's trade flows, by value at origin prices.
type TradeStats struct {
	RegionOut, RegionIn []float64   // per region
	NationOut, NationIn [][]float64 // per nation, per good
}

// Trade measures today's shipments: what leaves and enters each tile, and
// what crosses each nation's border.
func (w *World) Trade() TradeStats {
	s := TradeStats{RegionOut: make([]float64, len(w.Regions)), RegionIn: make([]float64, len(w.Regions))}
	for range w.Nations {
		s.NationOut = append(s.NationOut, make([]float64, NumGoods))
		s.NationIn = append(s.NationIn, make([]float64, NumGoods))
	}
	for _, t := range w.Traders {
		if t.Shipped <= 0 {
			continue
		}
		v := t.Shipped * t.Origin.Markets[t.Good].Price
		s.RegionOut[t.Origin.ID] += v
		s.RegionIn[t.Dest.ID] += v
		on, dn := t.Origin.Nation, t.Dest.Nation
		if on == dn {
			continue
		}
		if on != nil {
			s.NationOut[on.ID][t.Good] += v
		}
		if dn != nil {
			s.NationIn[dn.ID][t.Good] += v
		}
	}
	return s
}

// LaborShares is the share of a nation's workers in each sector.
func (w *World) LaborShares(n *Nation) [3]float64 {
	var l [3]float64
	tot := 0.0
	for _, r := range n.Regions {
		for _, f := range r.Firms {
			l[f.Sector] += f.Labor
			tot += f.Labor
		}
	}
	for i := range l {
		l[i] /= math.Max(tot, 1e-9)
	}
	return l
}

// NationClassIncome is a class's income per head across a nation.
func NationClassIncome(n *Nation, c Class) float64 {
	inc, size := 0.0, 0.0
	for _, r := range n.Regions {
		for _, p := range r.ClassPops(c) {
			inc += p.Income
			size += p.Size
		}
	}
	if size == 0 {
		return 0
	}
	return inc / size
}

func push(s *[]float64, v float64) { *s = append(*s, v) }

// record takes a sample of the world's statistics.
func (w *World) record() {
	h := &w.History
	if h.Nations == nil {
		for range w.Nations {
			nh := &NationHistory{ExportsGood: make([][]float64, NumGoods), ImportsGood: make([][]float64, NumGoods)}
			h.Nations = append(h.Nations, nh)
		}
		for range w.Regions {
			h.Regions = append(h.Regions, &RegionHistory{Prices: make([][]float64, len(ConsumerGoods))})
		}
	}
	idx := len(h.Ticks)
	h.Ticks = append(h.Ticks, w.Tick)
	trade := w.Trade()
	for i, n := range w.Nations {
		nh := h.Nations[i]
		pop, gdp, util := 0.0, 0.0, 0.0
		for _, r := range n.Regions {
			pop += r.Population()
			gdp += r.GDP()
		}
		for _, r := range n.Regions {
			util += r.Utility() * r.Population() / math.Max(pop, 1e-9)
		}
		men, _ := w.Mobilized(n)
		known := 0
		for _, k := range n.Known {
			if k {
				known++
			}
		}
		out, in := 0.0, 0.0
		for g := range Goods {
			out += trade.NationOut[i][g]
			in += trade.NationIn[i][g]
			push(&nh.ExportsGood[g], trade.NationOut[i][g])
			push(&nh.ImportsGood[g], trade.NationIn[i][g])
		}
		push(&nh.Population, pop+men)
		push(&nh.Tiles, float64(len(n.Regions)))
		push(&nh.Income, n.IncomeEMA)
		push(&nh.Treasury, n.Treasury)
		push(&nh.GDP, gdp)
		push(&nh.Utility, util)
		push(&nh.Soldiers, men)
		push(&nh.Strength, w.ArmyStrength(n))
		push(&nh.Techs, float64(known))
		push(&nh.Exports, out)
		push(&nh.Imports, in)
		for c := Class(0); c < NumClasses; c++ {
			push(&nh.ClassIncome[c], NationClassIncome(n, c))
		}
		l := w.LaborShares(n)
		for s := range l {
			push(&nh.Labor[s], l[s])
		}
	}
	for i, r := range w.Regions {
		rh := h.Regions[i]
		owner := -1.0
		if r.Nation != nil {
			owner = float64(r.Nation.ID)
		}
		push(&rh.Population, r.Population())
		push(&rh.Wage, r.Wage)
		push(&rh.Utility, r.Utility())
		push(&rh.GDP, r.GDP())
		push(&rh.Exports, trade.RegionOut[i])
		push(&rh.Imports, trade.RegionIn[i])
		push(&rh.Owner, owner)
		for c := Class(0); c < NumClasses; c++ {
			push(&rh.ClassIncome[c], r.ClassIncome(c))
		}
		for k, g := range ConsumerGoods {
			push(&rh.Prices[k], r.Markets[g].Price)
		}
	}
	for _, a := range w.Armies {
		var ah *ArmyHistory
		for _, x := range h.Armies {
			if x.ID == a.ID {
				ah = x
			}
		}
		if ah == nil {
			ah = &ArmyHistory{ID: a.ID, Nation: a.Nation.ID, From: idx}
			h.Armies = append(h.Armies, ah)
		}
		push(&ah.Men, a.Men)
		push(&ah.Strength, a.Strength())
		push(&ah.Effectiveness, a.Effectiveness())
	}
}
