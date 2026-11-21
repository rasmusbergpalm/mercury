package economy

import (
	"fmt"
	"math"
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

// BuildWorld builds the world for a hex map: a tile for every land cell,
// with its terrain's firms and every tile's (the recipes that start off
// switched off), a hundred people of whom a few are owners, and
// routes for every good to every neighbour. transportCost scales every hop;
// +Inf means autarky (no trade). names, if given, names the tiles in order
// (row by row, skipping the sea); otherwise a tile is named by its row and
// column.
func BuildWorld(transportCost float64, gameMap, names []string) (*World, error) {
	if math.IsNaN(transportCost) || transportCost < 0 {
		return nil, fmt.Errorf("transport cost must be a non-negative number")
	}
	w, err := buildWorld(transportCost, gameMap, names)
	if err != nil {
		return nil, err
	}
	for _, r := range w.Regions {
		r.UpdateAgglomeration()
	}
	return w, nil
}

// Sea marks a map cell with no tile: open water. Tiles next to it (or to
// the map's edge) are coast.
const Sea = '~'

// The cost of a hop between neighbouring tiles is the average of the two
// tiles' movement costs (land 1, coastal 1/4, less with infrastructure). Traders
// only run between neighbours; longer trade happens by re-export along a chain
// of tiles.
// names, if given, names the tiles in order (row by row, skipping the sea);
// otherwise a tile is named by its row and column.
func buildWorld(transportCost float64, gameMap []string, names []string) (*World, error) {
	if len(gameMap) == 0 {
		return nil, fmt.Errorf("empty map")
	}
	rows, cols := len(gameMap), len(gameMap[0])
	for r, row := range gameMap {
		if len(row) != cols {
			return nil, fmt.Errorf("map row %d has %d columns, want %d", r, len(row), cols)
		}
	}
	grid := make([][]*Region, rows)
	var regions []*Region
	for r := 0; r < rows; r++ {
		if len(gameMap[r]) != cols {
			return nil, fmt.Errorf("map row %d has %d columns, want %d", r, len(gameMap[r]), cols)
		}
		grid[r] = make([]*Region, cols)
		for c := 0; c < cols; c++ {
			if gameMap[r][c] == Sea {
				continue
			}
			t, err := TerrainBySymbol(gameMap[r][c])
			if err != nil {
				return nil, err
			}
			land := 0
			for _, nb := range HexNeighbors(r, c, rows, cols) {
				if gameMap[nb[0]][nb[1]] != Sea {
					land++
				}
			}
			coastal := land < 6
			name := fmt.Sprintf("%d%d", r, c)
			if names != nil {
				if len(regions) >= len(names) {
					return nil, fmt.Errorf("the map has more tiles than names")
				}
				name = names[len(regions)]
			}
			// a hundred people, who start with 10 a head of spending a day, in
			// savings of a twentieth (spendShare): one group of workers, a few
			// of whom are made owners
			reg := NewRegion(len(regions), name, makeFirms(t, coastal), 1, 100, 1000/spendShare)
			addOwnerClasses(reg)
			reg.Row, reg.Col, reg.Terrain, reg.BaseMove, reg.Coastal = r, c, t.Name, t.MoveCost, coastal
			grid[r][c] = reg
			regions = append(regions, reg)
		}
	}

	if len(regions) == 0 {
		return nil, fmt.Errorf("the map has no land")
	}
	if names != nil && len(names) != len(regions) {
		return nil, fmt.Errorf("the map has %d tiles but %d names", len(regions), len(names))
	}
	var traders []*Trader
	var links []Link
	for _, a := range regions {
		for _, nb := range HexNeighbors(a.Row, a.Col, rows, cols) {
			b := grid[nb[0]][nb[1]]
			if b == nil {
				continue // sea
			}
			hop := (a.MoveCost() + b.MoveCost()) / 2
			if a.ID < b.ID {
				links = append(links, Link{A: a.ID, B: b.ID, Hop: hop})
			}
			if !math.IsInf(transportCost, 1) {
				for _, g := range Tradable {
					traders = append(traders, NewTrader(g, a, b, hop, transportCost))
				}
			}
		}
	}
	w := NewWorld(rows, cols, regions, traders, links)
	w.Plan = Plan{Map: gameMap, Names: names, TransportCost: transportCost}
	return w, nil
}

// Place is a tile of a map given as a graph (BuildGraphWorld): its name
// and its terrain's name.
type Place struct {
	Name, Terrain string
}

// Lane joins places A and B of a graph map (indices into its places), at a
// distance: the transport a unit of bulk takes over it, and what spoils on
// the way, scale with it (a hop between hex neighbours on open land is 1).
type Lane struct {
	A, B     int
	Distance float64
}

// BuildGraphWorld builds a world whose tiles are any places joined by any
// lanes: star systems and the lanes between them, ports and their sea
// routes, market towns and the tracks between them. Merchants trade, and
// people move, only along the lanes, each at its own fixed distance; there
// is no sea and no coast. Every tile starts as BuildWorld's do: its terrain's firms and every
// tile's, a hundred people of whom a few are owners. transportCost scales
// every lane; +Inf means autarky (no trade).
func BuildGraphWorld(transportCost float64, places []Place, lanes []Lane) (*World, error) {
	if math.IsNaN(transportCost) || transportCost < 0 {
		return nil, fmt.Errorf("transport cost must be a non-negative number")
	}
	if len(places) == 0 {
		return nil, fmt.Errorf("no places")
	}
	var regions []*Region
	for i, p := range places {
		var t *Terrain
		for k := range Terrains {
			if Terrains[k].Name == p.Terrain {
				t = &Terrains[k]
			}
		}
		if t == nil {
			return nil, fmt.Errorf("place %s: unknown terrain %q", p.Name, p.Terrain)
		}
		reg := NewRegion(i, p.Name, makeFirms(*t, false), 1, 100, 1000/spendShare)
		addOwnerClasses(reg)
		reg.Terrain, reg.BaseMove = t.Name, t.MoveCost
		regions = append(regions, reg)
	}
	var traders []*Trader
	var links []Link
	seen := map[[2]int]bool{}
	for _, l := range lanes {
		if l.A < 0 || l.B < 0 || l.A >= len(places) || l.B >= len(places) || l.A == l.B {
			return nil, fmt.Errorf("lane %d-%d: not two of the %d places", l.A, l.B, len(places))
		}
		if !(l.Distance > 0) || math.IsInf(l.Distance, 1) {
			return nil, fmt.Errorf("lane %d-%d: distance must be positive", l.A, l.B)
		}
		key := [2]int{min(l.A, l.B), max(l.A, l.B)}
		if seen[key] {
			return nil, fmt.Errorf("lane %d-%d given twice", l.A, l.B)
		}
		seen[key] = true
		a, b := regions[key[0]], regions[key[1]]
		links = append(links, Link{a.ID, b.ID, l.Distance, true})
		if !math.IsInf(transportCost, 1) {
			for _, g := range Tradable {
				for _, ends := range [][2]*Region{{a, b}, {b, a}} {
					t := NewTrader(g, ends[0], ends[1], l.Distance, transportCost)
					t.FixedDistance = true // a lane's length is its own, not the tiles' move costs
					traders = append(traders, t)
				}
			}
		}
	}
	w := NewWorld(0, 0, regions, traders, links)
	w.Plan = Plan{Places: places, Lanes: lanes, TransportCost: transportCost}
	for _, r := range w.Regions {
		r.UpdateAgglomeration()
	}
	return w, nil
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
