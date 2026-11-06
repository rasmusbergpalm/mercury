// Package server exposes a running Mercury world over a JSON HTTP API.
package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"net/http"
	"strconv"
	"sync"

	"github.com/rasmusbergpalm/mercury/engine"
)

// Server owns one world and serializes access to it.
type Server struct {
	mu   sync.Mutex
	w    *engine.World
	cost float64
	mux  *http.ServeMux
}

// MaxStep is the most ticks one /api/step call may advance.
const MaxStep = 1000

// New builds a server around a fresh world. static, if non-nil, is served at /.
func New(transportCost float64, static fs.FS) (*Server, error) {
	s := &Server{mux: http.NewServeMux()}
	if err := s.reset(transportCost); err != nil {
		return nil, err
	}
	s.mux.HandleFunc("GET /api/world", s.handleWorld)
	s.mux.HandleFunc("GET /api/state", s.handleState)
	s.mux.HandleFunc("GET /api/regions/{id}", s.handleRegion)
	s.mux.HandleFunc("POST /api/step", s.handleStep)
	s.mux.HandleFunc("POST /api/reset", s.handleReset)
	s.mux.HandleFunc("POST /api/regions/{id}/build", s.handleBuild)
	s.mux.HandleFunc("POST /api/regions/{id}/cancel", s.handleCancel)
	s.mux.HandleFunc("POST /api/nations/{id}/tax", s.handleTax)
	s.mux.HandleFunc("POST /api/nations/{id}/tariff", s.handleTariff)
	s.mux.HandleFunc("POST /api/nations/{id}/embargo", s.handleEmbargo)
	s.mux.HandleFunc("POST /api/nations/{id}/research", s.handleResearch)
	s.mux.HandleFunc("POST /api/nations/{id}/funding", s.handleFunding)
	s.mux.HandleFunc("POST /api/regions/{id}/annex", s.handleAnnex)
	s.mux.HandleFunc("POST /api/regions/{id}/firms/{firm}/expand", s.handleExpand)
	if static != nil {
		s.mux.Handle("GET /", http.FileServerFS(static))
	}
	return s, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.mux.ServeHTTP(w, r) }

func (s *Server) reset(cost float64) error {
	spec := engine.DefaultGame
	spec.TransportCost = cost
	w, err := engine.BuildGame(spec)
	if err != nil {
		return err
	}
	s.w, s.cost = w, cost
	return nil
}

// ---- JSON types ------------------------------------------------------------

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
	Nation  int    `json:"nation"` // -1 if unowned
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

type LinkInfo struct {
	A   int     `json:"a"`
	B   int     `json:"b"`
	Hop float64 `json:"hop"`
}

// WorldInfo is the static description of the world.
type WorldInfo struct {
	Rows     int           `json:"rows"`
	Cols     int           `json:"cols"`
	Goods    []GoodInfo    `json:"goods"`
	Tiers    []string      `json:"tiers"`
	Terrains []TerrainInfo `json:"terrains"`
	Regions  []RegionInfo  `json:"regions"`
	Links    []LinkInfo    `json:"links"`
	Nations  []NationInfo  `json:"nations"`
	Infra    []InfraInfo   `json:"infra_levels"`
	Techs    []TechInfo    `json:"techs"`
	// LevelCost is the construction one firm building level costs.
	LevelCost float64 `json:"level_cost"`
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
	AnnexCost  float64   `json:"annex_cost"`
	Project    *Project  `json:"project"` // null if nothing is being built
}

type Project struct {
	Target    int     `json:"target"`
	Delivered float64 `json:"delivered"`
	Required  float64 `json:"required"`
}

type NationState struct {
	ID            int       `json:"id"`
	Treasury      float64   `json:"treasury"`
	TaxRate       float64   `json:"tax_rate"`
	Revenue       float64   `json:"revenue"`
	TariffRevenue float64   `json:"tariff_revenue"`
	Rebate        float64   `json:"rebate"`
	Spending      float64   `json:"spending"`
	Tariff        []float64 `json:"tariff"`  // per good
	Embargo       []bool    `json:"embargo"` // per nation
	Funding       float64   `json:"research_funding"`
	Researching   int       `json:"researching"` // -1: none
	Points        float64   `json:"research_points"`
	Known         []bool    `json:"known"` // per tech
	Population    float64   `json:"population"`
}

type Flow struct {
	From int     `json:"from"`
	To   int     `json:"to"`
	Good int     `json:"good"`
	Qty  float64 `json:"qty"`
}

// State is the dynamic state of the world.
type State struct {
	Tick          int           `json:"tick"`
	Money         float64       `json:"money"`
	TransportCost float64       `json:"transport_cost"` // null for autarky
	Autarky       bool          `json:"autarky"`
	Regions       []RegionState `json:"regions"`
	Flows         []Flow        `json:"flows"`
	Nations       []NationState `json:"nations"`
	Hops          []float64     `json:"hops"` // aligned with WorldInfo.Links
}

type PopState struct {
	Size     float64 `json:"size"`
	Cash     float64 `json:"cash"`
	Employed float64 `json:"employed"`
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
	Profit    float64    `json:"profit"` // moving average per tick
	Expansion *Expansion `json:"expansion"`
}

type Expansion struct {
	Delivered float64 `json:"delivered"`
	Public    bool    `json:"public"`
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

// ---- building responses ----------------------------------------------------

func (s *Server) worldInfo() WorldInfo {
	info := WorldInfo{Rows: s.w.Rows, Cols: s.w.Cols}
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
	for _, r := range s.w.Regions {
		info.Regions = append(info.Regions, regionInfo(r))
	}
	for _, l := range s.w.Links {
		info.Links = append(info.Links, LinkInfo{l.A, l.B, l.Hop})
	}
	for _, n := range s.w.Nations {
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
	info.LevelCost = engine.LevelCost
	return info
}

func regionInfo(r *engine.Region) RegionInfo {
	nation := -1
	if r.Nation != nil {
		nation = r.Nation.ID
	}
	return RegionInfo{r.ID, r.Name, r.Row, r.Col, r.Terrain, nation}
}

func (s *Server) regionState(i int) RegionState {
	r := s.w.Regions[i]
	ts := r.TierSpend(s.w.TradePrices[i])
	employed := 0.0
	for _, p := range r.Pops {
		employed += p.Employed()
	}
	nation := -1
	if r.Nation != nil {
		nation = r.Nation.ID
	}
	st := RegionState{ID: r.ID, Wage: r.Wage, Utility: r.Utility(), Population: r.Population(),
		Employed: employed, TierSpend: ts[:], Prices: r.Prices(), Nation: nation, Infra: r.Infra,
		MoveCost: r.MoveCost(), AnnexCost: s.w.AnnexCost(i)}
	if p := r.Project; p != nil {
		st.Project = &Project{p.Target, p.Delivered, p.Required()}
	}
	return st
}

const minFlow = 1e-3

func (s *Server) state() State {
	st := State{Tick: s.w.Tick, Money: s.w.Money(), TransportCost: s.cost,
		Autarky: math.IsInf(s.cost, 1)}
	if st.Autarky {
		st.TransportCost = 0
	}
	for i := range s.w.Regions {
		st.Regions = append(st.Regions, s.regionState(i))
	}
	st.Flows = []Flow{}
	for _, t := range s.w.Traders {
		if t.Shipped > minFlow {
			st.Flows = append(st.Flows, Flow{t.Origin.ID, t.Dest.ID, int(t.Good), t.Shipped})
		}
	}
	for _, n := range s.w.Nations {
		pop := 0.0
		for _, r := range n.Regions {
			pop += r.Population()
		}
		st.Nations = append(st.Nations, NationState{n.ID, n.Treasury, n.TaxRate, n.Revenue, n.TariffRevenue,
			n.Rebate, n.Spending, n.Tariff, n.Embargo, n.ResearchFunding, n.Researching, n.ResearchPoints,
			n.Known, pop})
	}
	for _, l := range s.w.Links {
		st.Hops = append(st.Hops, l.Hop)
	}
	return st
}

func (s *Server) regionDetail(i int) RegionDetail {
	r := s.w.Regions[i]
	d := RegionDetail{Info: regionInfo(r), State: s.regionState(i),
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
	for _, t := range s.w.Traders {
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

// ---- handlers --------------------------------------------------------------

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleWorld(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	writeJSON(w, s.worldInfo())
}

func (s *Server) handleState(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	writeJSON(w, s.state())
}

// pathID parses the {id} path value as an index below n.
func pathID(r *http.Request, n int) (int, bool) {
	id, err := strconv.Atoi(r.PathValue("id"))
	return id, err == nil && id >= 0 && id < n
}

func (s *Server) handleRegion(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := pathID(r, len(s.w.Regions))
	if !ok {
		http.Error(w, "no such region", http.StatusNotFound)
		return
	}
	writeJSON(w, s.regionDetail(id))
}

// act runs a game command on a region and replies with the new state, or
// 409 Conflict if the game refuses it.
func (s *Server) act(w http.ResponseWriter, r *http.Request, do func(id int) error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := pathID(r, len(s.w.Regions))
	if !ok {
		http.Error(w, "no such region", http.StatusNotFound)
		return
	}
	if err := do(id); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, s.state())
}

func (s *Server) handleBuild(w http.ResponseWriter, r *http.Request) {
	s.act(w, r, func(id int) error { return s.w.Build(id) })
}

func (s *Server) handleCancel(w http.ResponseWriter, r *http.Request) {
	s.act(w, r, func(id int) error { return s.w.CancelBuild(id) })
}

func (s *Server) handleTax(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := pathID(r, len(s.w.Nations))
	if !ok {
		http.Error(w, "no such nation", http.StatusNotFound)
		return
	}
	rate, err := strconv.ParseFloat(r.URL.Query().Get("rate"), 64)
	if err == nil {
		err = s.w.Nations[id].SetTaxRate(rate)
	}
	if err != nil {
		if !errors.Is(err, engine.ErrBadTaxRate) {
			err = engine.ErrBadTaxRate
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, s.state())
}

func (s *Server) handleStep(w http.ResponseWriter, r *http.Request) {
	n := 1
	if v := r.URL.Query().Get("n"); v != "" {
		var err error
		if n, err = strconv.Atoi(v); err != nil || n < 1 || n > MaxStep {
			http.Error(w, fmt.Sprintf("n must be an integer in [1, %d]", MaxStep), http.StatusBadRequest)
			return
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := 0; i < n; i++ {
		s.w.Step()
	}
	writeJSON(w, s.state())
}

func (s *Server) handleReset(w http.ResponseWriter, r *http.Request) {
	cost := 1.0
	if v := r.URL.Query().Get("cost"); v != "" {
		var err error
		if v == "inf" {
			cost = math.Inf(1)
		} else if cost, err = strconv.ParseFloat(v, 64); err != nil || cost < 0 || math.IsNaN(cost) {
			http.Error(w, "cost must be a non-negative number or inf", http.StatusBadRequest)
			return
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.reset(cost); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, s.state())
}

// nationAct runs a command on a nation and replies with the new state:
// 404 for an unknown nation, 400 for a bad argument, 409 if the game refuses.
func (s *Server) nationAct(w http.ResponseWriter, r *http.Request, do func(n *engine.Nation) (status int, err error)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := pathID(r, len(s.w.Nations))
	if !ok {
		http.Error(w, "no such nation", http.StatusNotFound)
		return
	}
	if status, err := do(s.w.Nations[id]); err != nil {
		http.Error(w, err.Error(), status)
		return
	}
	writeJSON(w, s.state())
}

func queryFloat(r *http.Request, key string) (float64, error) {
	v, err := strconv.ParseFloat(r.URL.Query().Get(key), 64)
	if err != nil || math.IsNaN(v) {
		return 0, fmt.Errorf("%s must be a number", key)
	}
	return v, nil
}

func (s *Server) handleTariff(w http.ResponseWriter, r *http.Request) {
	s.nationAct(w, r, func(n *engine.Nation) (int, error) {
		rate, err := queryFloat(r, "rate")
		if err != nil {
			return http.StatusBadRequest, err
		}
		good := engine.GoodID(-1)
		if name := r.URL.Query().Get("good"); name != "" {
			found := false
			for _, g := range engine.Tradable {
				if engine.Goods[g].Name == name {
					good, found = g, true
				}
			}
			if !found {
				return http.StatusBadRequest, fmt.Errorf("no tradable good %q", name)
			}
		}
		if err := n.SetTariff(good, rate); err != nil {
			return http.StatusBadRequest, err
		}
		return 0, nil
	})
}

func (s *Server) handleEmbargo(w http.ResponseWriter, r *http.Request) {
	s.nationAct(w, r, func(n *engine.Nation) (int, error) {
		target, err := strconv.Atoi(r.URL.Query().Get("target"))
		if err != nil || target < 0 || target >= len(s.w.Nations) {
			return http.StatusBadRequest, fmt.Errorf("target must be a nation id")
		}
		if target == n.ID {
			return http.StatusConflict, engine.ErrSelf
		}
		n.Embargo[target] = r.URL.Query().Get("on") != "false"
		return 0, nil
	})
}

func (s *Server) handleResearch(w http.ResponseWriter, r *http.Request) {
	s.nationAct(w, r, func(n *engine.Nation) (int, error) {
		tech, err := strconv.Atoi(r.URL.Query().Get("tech"))
		if err != nil {
			return http.StatusBadRequest, fmt.Errorf("tech must be a tech id")
		}
		if err := n.SetResearch(tech); err != nil {
			return http.StatusConflict, err
		}
		return 0, nil
	})
}

func (s *Server) handleFunding(w http.ResponseWriter, r *http.Request) {
	s.nationAct(w, r, func(n *engine.Nation) (int, error) {
		amount, err := queryFloat(r, "amount")
		if err == nil {
			err = n.SetResearchFunding(amount)
		}
		if err != nil {
			return http.StatusBadRequest, err
		}
		return 0, nil
	})
}

func (s *Server) handleAnnex(w http.ResponseWriter, r *http.Request) {
	nation, err := strconv.Atoi(r.URL.Query().Get("nation"))
	if err != nil || nation < 0 || nation >= len(s.w.Nations) {
		http.Error(w, "nation must be a nation id", http.StatusBadRequest)
		return
	}
	s.act(w, r, func(id int) error { return s.w.Annex(id, nation) })
}

func (s *Server) handleExpand(w http.ResponseWriter, r *http.Request) {
	s.act(w, r, func(id int) error {
		firm, err := strconv.Atoi(r.PathValue("firm"))
		if err != nil || firm < 0 || firm >= len(s.w.Regions[id].Firms) {
			return fmt.Errorf("no such firm")
		}
		return s.w.Expand(id, firm)
	})
}
