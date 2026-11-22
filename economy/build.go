package economy

import (
	"fmt"
	"math"
	"sort"
)

// Recipe is a firm template: output, base TFP and Cobb-Douglas factors.
type Recipe struct {
	Firm    string
	Output  string
	TFP     float64
	Factors []Factor
	// StartsOff: the firm or method starts switched off (Firm.Enabled,
	// Method.Enabled), an invention not yet made, until whoever runs the
	// economy switches it on.
	StartsOff bool
	Service   bool // in the service sector (else industry, or primary if land-bound)
}

// The recipes (data/recipes.json).
var (
	// RawRecipes are land-bound producers; terrain decides where they exist.
	RawRecipes []Recipe
	// ProcessingRecipes can be set up anywhere.
	ProcessingRecipes []Recipe
)

// MethodRecipe is a later way of making what a firm makes, usually
// starting switched off until it is invented (StartsOff): a firm of its own beside the old one, with
// its own recipe and levels. More productive, it needs machinery and fuel
// the old one doesn't, so it spreads as owners find it worth building (and
// the old one comes back where the machinery is dear). A land-bound one
// shares its land with the old one: it grows by taking the old one's
// levels over.
type MethodRecipe struct {
	Base string // the firm whose method it is
	Recipe
	// Cap is the levels a land-bound method can work, if more than the
	// old one's (RawMaxLevels): steam pumps and blasting reach seams the
	// old mines couldn't.
	Cap int
}

// MethodRecipes are every firm's later methods (data/recipes.json).
var MethodRecipes []MethodRecipe

func makeMethodRecipes() []MethodRecipe {
	var out []MethodRecipe
	for _, d := range recipesData.Methods {
		out = append(out, MethodRecipe{d.Base, d.recipe(), d.Cap})
	}
	return out
}

// RawMaxLevels caps building levels of land-bound producers (land is scarce).
var RawMaxLevels int

// Bonus is a raw producer available on a terrain, with a TFP multiplier.
type Bonus struct {
	Firm string
	Mult float64
}

// Terrain is a tile type.
type Terrain struct {
	Name     string
	Symbol   byte
	MoveCost float64 // cost of moving through the tile
	Raw      []Bonus
}

// Terrains are the tile types (data/terrains.json).
var Terrains []Terrain

func makeTerrains() []Terrain {
	var out []Terrain
	for _, t := range terrainsData.Terrains {
		if len(t.Symbol) != 1 {
			panic(fmt.Sprintf("data/terrains.json: %s: a symbol is one letter", t.Name))
		}
		out = append(out, Terrain{t.Name, t.Symbol[0], t.Move, bonuses(t.Raw)})
	}
	return out
}

// The ocean surrounds the map. A tile on the map's edge is coastal whatever
// its terrain: it has the coast's producers (CoastRaw), and ships carry
// goods past it cheaply (CoastMove, scaled by the tile's SeaTransportMult).
var (
	CoastMove float64
	CoastRaw  []Bonus
)

// TerrainBySymbol looks a terrain up by its map symbol.
func TerrainBySymbol(sym byte) (Terrain, error) {
	for _, t := range Terrains {
		if t.Symbol == sym {
			return t, nil
		}
	}
	return Terrain{}, fmt.Errorf("unknown terrain symbol %q", sym)
}

// DefaultMap is a 6x6 hex map, "odd-r" offset layout (odd rows are shifted
// half a hex right).
var DefaultMap = []string{
	"MMHFFP",
	"HHFPPP",
	"FFPPHP",
	"PPPTTH",
	"PPTTTP",
	"HPPTPF",
}

// HexNeighbors lists the neighbours of (r, c) in an odd-r offset grid.
func HexNeighbors(r, c, rows, cols int) [][2]int {
	var deltas [][2]int
	if r%2 == 0 {
		deltas = [][2]int{{-1, -1}, {-1, 0}, {0, -1}, {0, 1}, {1, -1}, {1, 0}}
	} else {
		deltas = [][2]int{{-1, 0}, {-1, 1}, {0, -1}, {0, 1}, {1, 0}, {1, 1}}
	}
	var out [][2]int
	for _, d := range deltas {
		rr, cc := r+d[0], c+d[1]
		if rr >= 0 && rr < rows && cc >= 0 && cc < cols {
			out = append(out, [2]int{rr, cc})
		}
	}
	return out
}

func makeFirms(t Terrain, coastal bool) []*Firm {
	var firms []*Firm
	addRaw := func(recipes []Recipe, bonuses []Bonus, site string) {
		for _, rc := range recipes {
			for _, b := range bonuses {
				if b.Firm == rc.Firm {
					f := NewFirm(rc.Firm, G(rc.Output), rc.TFP*b.Mult, rc.Factors, 50)
					f.MaxLevels, f.Sector = RawMaxLevels, Primary
					f.Site, f.SiteMult = site, b.Mult
					f.Enabled = !rc.StartsOff
					firms = append(firms, f)
				}
			}
		}
	}
	addRaw(RawRecipes, t.Raw, t.Name)
	if coastal {
		addRaw(RawRecipes, CoastRaw, "the coast")
	}
	for _, rc := range ProcessingRecipes {
		f := NewFirm(rc.Firm, G(rc.Output), rc.TFP, rc.Factors, 50)
		if rc.Service {
			f.Sector = Service
		}
		f.Enabled = !rc.StartsOff
		firms = append(firms, f)
	}
	for _, f := range firms {
		// the later methods, at the site's productivity; a land-bound
		// one may work as much of the land as its Cap
		for _, mr := range MethodRecipes {
			if mr.Base != f.Name {
				continue
			}
			site := 1.0
			if f.SiteMult > 0 {
				site = f.SiteMult
			}
			m := newMethod(mr.Firm, mr.TFP*site, mr.Factors)
			m.Enabled = !mr.StartsOff
			if f.Sector == Primary {
				m.Cap = max(RawMaxLevels, mr.Cap)
			}
			f.Methods = append(f.Methods, m)
		}
	}
	return firms
}

// Graph is a map: places joined by lanes. Rows and Cols, if set, are the
// grid the places sit on (a hex map's, from HexGraph), so that a tile can
// be found by its cell (World.RegionAt).
type Graph struct {
	Places     []Place
	Lanes      []Lane
	Rows, Cols int `json:",omitempty"`
}

// Place is a tile of a map: its name, its terrain's name, whether it is on
// the coast (it then has the coast's producers, CoastRaw, and goods pass it
// by sea at CoastMove), on a grid its cell, and its people at the start.
type Place struct {
	Name, Terrain string
	Coastal       bool    `json:",omitempty"`
	Row, Col      int     `json:",omitempty"`
	People        float64 `json:",omitempty"` // its people at the start (0: a hundred)
}

// Lane joins places A and B (indices into the places). A hop along it
// takes Length times the average of its two tiles' movement costs (see
// Region.MoveCost): on a hex map every lane has length 1; on a map of star
// systems, ports or towns, each its own.
type Lane struct {
	A, B   int
	Length float64
}

// BuildWorld builds a world on a map: a tile for every place, with its
// terrain's firms and every tile's (the recipes that start off switched
// off) and a hundred people of whom a few are owners, and routes for every
// good both ways along every lane. Merchants trade, and people move, only
// along the lanes; longer trade happens by re-export along a chain of
// tiles. transportCost scales every hop; +Inf means autarky (no trade).
func BuildWorld(transportCost float64, g Graph) (*World, error) {
	if math.IsNaN(transportCost) || transportCost < 0 {
		return nil, fmt.Errorf("transport cost must be a non-negative number")
	}
	if len(g.Places) == 0 {
		return nil, fmt.Errorf("the map has no places")
	}
	regions := make([]*Region, len(g.Places))
	for i, p := range g.Places {
		var t *Terrain
		for k := range Terrains {
			if Terrains[k].Name == p.Terrain {
				t = &Terrains[k]
			}
		}
		if t == nil {
			return nil, fmt.Errorf("place %s: unknown terrain %q", p.Name, p.Terrain)
		}
		if g.Rows > 0 && (p.Row < 0 || p.Row >= g.Rows || p.Col < 0 || p.Col >= g.Cols) {
			return nil, fmt.Errorf("place %s: cell (%d, %d) is off the %d×%d grid", p.Name, p.Row, p.Col, g.Rows, g.Cols)
		}
		// a hundred people (or the place's own number), who start with 10 a
		// head of spending a tick, in savings of a twentieth (DefaultSpendShare):
		// one group of workers, a few of whom are made owners
		people := 100.0
		if p.People > 0 {
			people = p.People
		}
		reg := NewRegion(i, p.Name, makeFirms(*t, p.Coastal), 1, people, 10*people/DefaultSpendShare)
		addOwnerClasses(reg)
		reg.Row, reg.Col, reg.Terrain, reg.BaseMove, reg.Coastal = p.Row, p.Col, t.Name, t.MoveCost, p.Coastal
		regions[i] = reg
	}
	// each tile's lanes, in order of the tile at their other end
	incident := make([][]int, len(regions))
	seen := map[[2]int]bool{}
	for k, l := range g.Lanes {
		if l.A < 0 || l.B < 0 || l.A >= len(regions) || l.B >= len(regions) || l.A == l.B {
			return nil, fmt.Errorf("lane %d-%d: not two of the %d places", l.A, l.B, len(regions))
		}
		if !(l.Length > 0) || math.IsInf(l.Length, 1) {
			return nil, fmt.Errorf("lane %d-%d: length must be positive", l.A, l.B)
		}
		key := [2]int{min(l.A, l.B), max(l.A, l.B)}
		if seen[key] {
			return nil, fmt.Errorf("lane %d-%d given twice", l.A, l.B)
		}
		seen[key] = true
		incident[l.A], incident[l.B] = append(incident[l.A], k), append(incident[l.B], k)
	}
	other := func(l Lane, a int) int {
		if l.A == a {
			return l.B
		}
		return l.A
	}
	links := make([]Link, len(g.Lanes))
	for k, l := range g.Lanes {
		links[k] = Link{A: l.A, B: l.B, Length: l.Length}
	}
	var traders []*Trader
	for a := range regions {
		ks := incident[a]
		sort.SliceStable(ks, func(i, j int) bool { return other(g.Lanes[ks[i]], a) < other(g.Lanes[ks[j]], a) })
		if math.IsInf(transportCost, 1) {
			continue
		}
		for _, k := range ks {
			b := regions[other(g.Lanes[k], a)]
			for _, good := range Tradable {
				t := NewTrader(good, regions[a], b, 0, transportCost)
				t.length = g.Lanes[k].Length
				traders = append(traders, t)
			}
		}
	}
	w := NewWorld(g.Rows, g.Cols, regions, traders, links)
	w.Plan = Plan{Graph: g, TransportCost: transportCost}
	w.RefreshHops()
	for _, r := range w.Regions {
		r.UpdateAgglomeration()
	}
	return w, nil
}

// Sea marks a cell of a hex map with no tile: open water. Tiles next to it
// (or to the map's edge) are coast.
const Sea = '~'

// HexGraph makes the map of a hex grid, given as rows of terrain symbols
// (Sea for open water): a place for every land cell, coastal if it is next
// to the sea or the grid's edge, and a lane of length 1 to each
// neighbouring land cell. names, if given, names the places in order (row
// by row, skipping the sea); otherwise a place is named by its row and
// column.
func HexGraph(rows []string, names []string) (Graph, error) {
	if len(rows) == 0 {
		return Graph{}, fmt.Errorf("empty map")
	}
	nr, nc := len(rows), len(rows[0])
	for r, row := range rows {
		if len(row) != nc {
			return Graph{}, fmt.Errorf("map row %d has %d columns, want %d", r, len(row), nc)
		}
	}
	g := Graph{Rows: nr, Cols: nc}
	id := make([]int, nr*nc)
	for r := 0; r < nr; r++ {
		for c := 0; c < nc; c++ {
			id[r*nc+c] = -1
			if rows[r][c] == Sea {
				continue
			}
			t, err := TerrainBySymbol(rows[r][c])
			if err != nil {
				return Graph{}, err
			}
			land := 0
			for _, nb := range HexNeighbors(r, c, nr, nc) {
				if rows[nb[0]][nb[1]] != Sea {
					land++
				}
			}
			name := fmt.Sprintf("%d%d", r, c)
			if names != nil {
				if len(g.Places) >= len(names) {
					return Graph{}, fmt.Errorf("the map has more tiles than names")
				}
				name = names[len(g.Places)]
			}
			id[r*nc+c] = len(g.Places)
			g.Places = append(g.Places, Place{Name: name, Terrain: t.Name, Coastal: land < 6, Row: r, Col: c})
		}
	}
	if len(g.Places) == 0 {
		return Graph{}, fmt.Errorf("the map has no land")
	}
	if names != nil && len(names) != len(g.Places) {
		return Graph{}, fmt.Errorf("the map has %d tiles but %d names", len(g.Places), len(names))
	}
	for a, p := range g.Places {
		for _, nb := range HexNeighbors(p.Row, p.Col, nr, nc) {
			if b := id[nb[0]*nc+nb[1]]; b > a {
				g.Lanes = append(g.Lanes, Lane{A: a, B: b, Length: 1})
			}
		}
	}
	return g, nil
}

// Owner classes: each is this share of a tile's people and
// starts with this share of its cash (so owners are much richer per head).
const (
	OwnerPeopleShare = 0.04
	OwnerCashShare   = 0.10
)

// addOwnerClasses turns part of a tile's people into an owner pop.
func addOwnerClasses(r *Region) {
	pop, cash := r.Population(), 0.0
	for _, p := range r.Pops {
		cash += p.Cash
	}
	for _, p := range r.Pops {
		p.Size *= 1 - OwnerPeopleShare
		p.Cash *= 1 - OwnerCashShare
	}
	p := NewPop(OwnerPeopleShare*pop, OwnerCashShare*cash)
	p.Class = Owners
	r.Pops = append(r.Pops, p)
	r.popDemand = append(r.popDemand, make([]float64, NumGoods))
}
