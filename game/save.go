package game

import (
	"errors"
	"fmt"
	"math"

	"github.com/rasmusbergpalm/mercury/economy"
)

// Snapshot is everything about a game world that carries from one tick to the
// next: the economy's state (economy.Snapshot), and the game's. Restoring it
// rebuilds the scenario and overwrites this state, so a restored world
// continues exactly as the original would have.
type Snapshot struct {
	Version    int               `json:"version"`
	Spec       GameSpec          `json:"spec"`
	Economy    *economy.Snapshot `json:"economy"`
	Regions    []RegionSnap      `json:"regions"`
	Nations    []NationSnap      `json:"nations"`
	Ports      []int             `json:"ports"` // in the order they opened
	Armies     []ArmySnap        `json:"armies"`
	NextArmyID int               `json:"next_army_id"`
	Events     []Event           `json:"events"`
	History    History           `json:"history"`
}

// Tick is the day the snapshot was taken.
func (s *Snapshot) Tick() int { return s.Economy.Tick }

type ArmySnap struct {
	ID, Nation, Region, Doctrine int
	Men, Target, March           float64
	Supply, Stock                []float64
	Path                         []int
	Coming                       []Draft
	Retreating                   bool
	BattleStart                  float64 `json:",omitempty"`
}

type SiegeSnap struct {
	Nation   int
	Progress float64
}

// SnapshotVersion is bumped whenever the snapshot format changes; a build
// reads only its own (saves of older builds are not kept up).
const SnapshotVersion = 17

// RegionSnap is what the game knows of a tile, beyond its economy.
type RegionSnap struct {
	Neglect, Collect  float64
	Educated, Healthy float64
	Loyalties         []float64
	Infra, Battle     int
	Port              bool
	Project           *Project
	Siege             *SiegeSnap
	Services          []ServiceSnap
}

// ServiceSnap is a public service's state (its staff's are the economy's).
type ServiceSnap struct {
	Levels, MaxLevels, Funded int
	Expansion                 *Expansion
	LastOutput                float64
}

type NationSnap struct {
	Regions                                []int // in order
	Treasury, TaxRate, SpendEMA, BuildRate float64
	Tariff                                 []float64
	Embargo                                []bool
	AutoBuild, AutoTax                     bool
	Pension                                int
	PayScale                               float64
	IncomeEMA, ResearchPoints, CopyPoints  float64
	RealGDP                                float64
	Researching, Copying                   int
	Known                                  []bool
	Coalition                              int `json:",omitempty"`
	AllianceSince                          int `json:",omitempty"`
	LearnedAt                              []int
	Queue                                  []int
	TradeWith                              []float64
	AtWar, PeaceOffer                      []bool
	WarSince                               []int
	AI, ChooseResearch                     bool
	Standing                               float64
	Revenue, TariffRevenue, Spending       float64
}

// ErrNotAGame is returned when snapshotting a world that wasn't built by BuildGame.
var ErrNotAGame = errors.New("only game worlds can be saved")

// Snapshot captures the world's state.
func (w *World) Snapshot() (*Snapshot, error) {
	if w.Spec == nil {
		return nil, ErrNotAGame
	}
	s := &Snapshot{Version: SnapshotVersion, Spec: *w.Spec, Economy: w.World.Snapshot(), Ports: append([]int(nil), w.Ports...)}
	for _, r := range w.Regions {
		rs := RegionSnap{Neglect: r.Neglect, Collect: r.Collect, Educated: r.Educated, Healthy: r.Healthy, Loyalties: append([]float64(nil), r.Loyalties...), Infra: r.Infra, Battle: r.Battle, Port: r.Port}
		for _, sv := range r.Services {
			ss := ServiceSnap{Levels: sv.Levels, MaxLevels: sv.MaxLevels, Funded: sv.Funded, LastOutput: sv.LastOutput}
			if sv.Expansion != nil {
				ss.Expansion = sv.Expansion.Copy()
			}
			rs.Services = append(rs.Services, ss)
		}
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
		s.Regions = append(s.Regions, rs)
	}
	for _, n := range w.Nations {
		ns := NationSnap{Treasury: n.Treasury, TaxRate: n.TaxRate, SpendEMA: n.SpendEMA, BuildRate: n.MaxBuildRate,
			Tariff: append([]float64(nil), n.Tariff...), Embargo: append([]bool(nil), n.Embargo...),
			AutoBuild: n.AutoBuild, AutoTax: n.AutoTax, Pension: n.Pension, PayScale: n.payScale, Standing: n.Standing, IncomeEMA: n.IncomeEMA, ResearchPoints: n.ResearchPoints, CopyPoints: n.CopyPoints, Copying: n.Copying, RealGDP: n.RealGDP,
			Researching: n.Researching, Known: append([]bool(nil), n.Known...), Coalition: n.Coalition, AllianceSince: n.AllianceSince, LearnedAt: append([]int(nil), n.LearnedAt...), AI: n.AI, ChooseResearch: n.ChooseResearch,
			Queue: append([]int(nil), n.Queue...), TradeWith: append([]float64(nil), n.TradeWith...),
			AtWar: append([]bool(nil), n.AtWar...), WarSince: append([]int(nil), n.WarSince...),
			PeaceOffer: append([]bool(nil), n.PeaceOffer...),
			Revenue:    n.Revenue, TariffRevenue: n.TariffRevenue, Spending: n.Spending}
		for _, r := range n.Regions {
			ns.Regions = append(ns.Regions, r.ID)
		}
		s.Nations = append(s.Nations, ns)
	}
	for _, a := range w.Armies {
		s.Armies = append(s.Armies, ArmySnap{a.ID, a.Nation.ID, a.Region, a.Doctrine, a.Men, a.Target, a.March,
			append([]float64(nil), a.Supply...), append([]float64(nil), a.Stock...), append([]int(nil), a.Path...),
			append([]Draft(nil), a.Coming...), a.Retreating, a.BattleStart})
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
	w, err := buildGame(s.Spec, false, nil) // the snapshot holds the settled state
	if err != nil {
		return nil, err
	}
	bad := func(what string) (*World, error) {
		return nil, fmt.Errorf("snapshot doesn't fit its scenario: %s", what)
	}
	if s.Economy == nil || len(s.Regions) != len(w.Regions) || len(s.Nations) != len(w.Nations) {
		return bad("counts")
	}
	for _, id := range s.Ports { // reopen sea routes in their original order (hops are refreshed below)
		if id < 0 || id >= len(w.Regions) || !s.Regions[id].Port || w.hasPortRoutes(id) {
			return bad("ports")
		}
		w.Regions[id].Port = true
		w.openPort(w.Regions[id])
	}
	if err := w.World.SetState(s.Economy); err != nil {
		return nil, err
	}
	for _, n := range w.Nations {
		n.Regions = nil
	}
	for i, rs := range s.Regions {
		r := w.Regions[i]
		if rs.Port && !w.hasPortRoutes(i) {
			return bad("port in " + r.Name)
		}
		r.Neglect, r.Project, r.Battle, r.Collect = rs.Neglect, rs.Project, rs.Battle, rs.Collect
		r.Educated, r.Healthy = rs.Educated, rs.Healthy
		if len(rs.Services) != int(NumServices) {
			return bad("services in " + r.Name)
		}
		for k, ss := range rs.Services {
			sv := r.Services[k]
			sv.Levels, sv.MaxLevels, sv.Funded, sv.Expansion, sv.LastOutput = ss.Levels, ss.MaxLevels, ss.Funded, ss.Expansion, ss.LastOutput
			if ss.Expansion != nil {
				sv.Expansion = ss.Expansion.Copy()
			}
		}
		r.setInfra(rs.Infra)
		r.setNation(nil)
		if len(rs.Loyalties) != len(r.Loyalties) {
			return bad("allegiance in " + r.Name)
		}
		copy(r.Loyalties, rs.Loyalties)
		if r.Infra < 0 || r.Infra >= len(InfraLevels) || (r.Project != nil && !r.Project.Port && (r.Project.Target <= 0 || r.Project.Target >= len(InfraLevels))) {
			return bad("infrastructure in " + r.Name)
		}
	}
	for i, ns := range s.Nations {
		n := w.Nations[i]
		if len(ns.Tariff) != NumGoods || len(ns.Embargo) != len(w.Nations) || len(ns.Known) != len(Techs) {
			return bad("nation " + n.Name)
		}
		n.Treasury, n.TaxRate, n.SpendEMA, n.MaxBuildRate = ns.Treasury, ns.TaxRate, ns.SpendEMA, ns.BuildRate
		n.Tariff, n.Embargo, n.AutoBuild, n.IncomeEMA, n.ResearchPoints = ns.Tariff, ns.Embargo, ns.AutoBuild, ns.IncomeEMA, ns.ResearchPoints
		n.CopyPoints = ns.CopyPoints
		n.RealGDP = math.Max(0, ns.RealGDP)
		n.now = s.Tick()
		if ns.Coalition < 0 || ns.Coalition > len(s.Nations) || ns.Coalition == n.ID+1 {
			return bad("alliance of " + n.Name)
		}
		n.Coalition, n.AllianceSince = ns.Coalition, ns.AllianceSince
		if len(ns.LearnedAt) != len(Techs) {
			return bad("nation " + n.Name)
		}
		copy(n.LearnedAt, ns.LearnedAt)
		if len(ns.TradeWith) != len(w.Nations) || len(ns.AtWar) != len(w.Nations) || len(ns.WarSince) != len(w.Nations) ||
			len(ns.PeaceOffer) != len(w.Nations) {
			return bad("nation " + n.Name)
		}
		for _, q := range ns.Queue {
			if q < 0 || q >= len(Techs) {
				return bad("research queue")
			}
		}
		if ns.Copying < -1 || ns.Copying >= len(Techs) {
			return bad("research")
		}
		n.Copying = ns.Copying
		if ns.Researching < -1 || ns.Researching >= len(Techs) {
			return bad("research")
		}
		n.Researching, n.Known, n.AI, n.Queue = ns.Researching, ns.Known, ns.AI, ns.Queue
		n.ChooseResearch, n.payScale, n.AutoTax = ns.ChooseResearch, ns.PayScale, ns.AutoTax
		if err := n.SetPension(ns.Pension); err != nil {
			return bad("nation " + n.Name)
		}
		if err := n.SetStanding(ns.Standing); err != nil {
			return bad("nation " + n.Name)
		}
		n.TradeWith, n.AtWar, n.WarSince, n.PeaceOffer = ns.TradeWith, ns.AtWar, ns.WarSince, ns.PeaceOffer
		n.Revenue, n.TariffRevenue, n.Spending = ns.Revenue, ns.TariffRevenue, ns.Spending
		for _, id := range ns.Regions {
			if id < 0 || id >= len(w.Regions) || w.Regions[id].Nation != nil {
				return bad("nation " + n.Name + " tiles")
			}
			n.Regions = append(n.Regions, w.Regions[id])
			w.Regions[id].setNation(n)
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
			Stock: as.Stock, Path: as.Path, Coming: as.Coming, Retreating: as.Retreating, BattleStart: as.BattleStart})
	}
	w.NextArmyID = s.NextArmyID
	w.Events = append([]Event(nil), s.Events...)
	if h := s.History; len(h.Ticks) > 0 {
		if len(h.Nations) != len(w.Nations) || len(h.Regions) != len(w.Regions) {
			return bad("history")
		}
	}
	w.History = s.History
	for _, r := range w.Regions {
		r.applyAllegiance()
	}
	// firm gating follows the (restored) owners' technology
	for _, r := range w.Regions {
		if r.Nation == nil {
			for _, f := range r.Firms {
				f.Enabled = firmEnabled(f, nil)
				applyMethods(f, nil)
			}
		}
	}
	for _, n := range w.Nations {
		n.applyTech()
		for _, r := range n.Regions { // TFP exactly as saved
			for j, f := range r.Firms {
				for k, m := range f.Methods {
					m.TFP = s.Economy.Regions[r.ID].Firms[j].TFP[k]
				}
			}
		}
	}
	for _, r := range w.Regions {
		r.UpdateAgglomeration()
	}
	w.refreshAllHops()
	return w, nil
}
