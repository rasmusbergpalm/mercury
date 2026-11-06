package engine

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
}

// RawRecipes are land-bound producers; terrain decides where they exist.
var RawRecipes = []Recipe{
	{"Farm", "grain", 2.0, []Factor{Labor(0.6)}},
	{"Potato farm", "potatoes", 2.5, []Factor{Labor(0.6)}},
	{"Ranch", "meat", 1.2, []Factor{Labor(0.6)}},
	{"Fishery", "fish", 1.5, []Factor{Labor(0.6)}},
	{"Forestry", "wood", 2.0, []Factor{Labor(0.6)}},
	{"Coal mine", "coal", 2.0, []Factor{Labor(0.6)}},
	{"Iron mine", "iron ore", 1.5, []Factor{Labor(0.6)}},
	{"Sheep farm", "wool", 1.5, []Factor{Labor(0.6)}},
	{"Cotton plantation", "cotton", 1.5, []Factor{Labor(0.6)}},
	{"Tea plantation", "tea", 1.0, []Factor{Labor(0.6)}},
	{"Sugar plantation", "sugar cane", 2.0, []Factor{Labor(0.6)}},
	{"Vineyard", "grapes", 1.5, []Factor{Labor(0.6)}},
	{"Silk farm", "silk", 0.8, []Factor{Labor(0.6)}},
	{"Clay pit", "clay", 2.0, []Factor{Labor(0.6)}},
}

// ProcessingRecipes can be set up anywhere. "fuel" = wood or coal, "fiber" = wool or cotton.
var ProcessingRecipes = []Recipe{
	{"Bakery", "bread", 2.0, []Factor{Labor(0.3), Input("grain", 0.4), Comp("fuel", 0.1)}},
	{"Brewery", "beer", 1.5, []Factor{Labor(0.3), Input("grain", 0.4)}},
	{"Mill", "cloth", 1.5, []Factor{Labor(0.3), Comp("fiber", 0.4)}},
	{"Tailor", "clothes", 1.5, []Factor{Labor(0.3), Input("cloth", 0.4)}},
	{"Smelter", "iron", 1.0, []Factor{Labor(0.2), Input("iron ore", 0.3), Comp("fuel", 0.2)}},
	{"Smithy", "ironware", 1.2, []Factor{Labor(0.3), Input("iron", 0.3), Comp("fuel", 0.1)}},
	{"Carpenter", "furniture", 1.2, []Factor{Labor(0.3), Input("wood", 0.4)}},
	{"Refinery", "sugar", 1.2, []Factor{Labor(0.2), Input("sugar cane", 0.4), Comp("fuel", 0.1)}},
	{"Winery", "wine", 1.2, []Factor{Labor(0.3), Input("grapes", 0.4)}},
	{"Couturier", "fine clothes", 1.0, []Factor{Labor(0.4), Input("silk", 0.3)}},
	{"Kiln", "porcelain", 1.0, []Factor{Labor(0.4), Input("clay", 0.2), Comp("fuel", 0.2)}},
	{"Transport", "transport", 2.0, []Factor{Labor(0.6)}},
}

// GameRecipes exist only in game worlds (with nations): services for
// building, research and moving goods, war industry, and industrial firms
// unlocked by technology.
var GameRecipes = []Recipe{
	{"Construction", "construction", 1.5, []Factor{Labor(0.5), Input("wood", 0.2)}},
	{"University", "research", 1.0, []Factor{Labor(0.6)}},
	{"Armory", "arms", 1.0, []Factor{Labor(0.3), Input("iron", 0.25), Input("wood", 0.1)}},
	{"Foundry", "artillery", 0.8, []Factor{Labor(0.3), Input("iron", 0.3), Input("coal", 0.1)}},
	{"Munitions plant", "munitions", 1.5, []Factor{Labor(0.3), Input("iron", 0.2), Input("coal", 0.2)}},
	{"Motor works", "vehicles", 0.8, []Factor{Labor(0.3), Input("iron", 0.3), Input("coal", 0.1)}},
	{"Aircraft factory", "aircraft", 0.8, []Factor{Labor(0.4), Input("wood", 0.15), Input("cloth", 0.1), Input("iron", 0.1)}},
	{"Textile factory", "cloth", 3.0, []Factor{Labor(0.2), Comp("fiber", 0.4), Input("coal", 0.15)}},
	{"Steel works", "iron", 2.2, []Factor{Labor(0.2), Input("iron ore", 0.35), Input("coal", 0.25)}},
}

// serviceFirms are the firms in the service sector (the rest of the
// non-primary firms are industry).
var serviceFirms = map[string]bool{"Transport": true, "Construction": true, "University": true}

// GameRawRecipes are land-bound producers that exist only in game worlds,
// with where they can be set up.
var GameRawRecipes = []Recipe{
	{"Oil well", "oil", 1.5, []Factor{Labor(0.6)}},
}

// GameTerrainRaw adds game-only raw producers to terrains.
var GameTerrainRaw = map[string][]Bonus{
	"tropical": {{"Oil well", 1.5}},
	"plains":   {{"Oil well", 0.7}},
	"coast":    {{"Oil well", 0.6}},
}

// FirmTech maps a firm to the tech that enables it (from Tech.EnableFirms).
// In game worlds such firms only work for nations that know the tech.
var FirmTech = func() map[string]string {
	m := map[string]string{}
	for _, t := range Techs {
		for _, f := range t.EnableFirms {
			m[f] = t.Name
		}
	}
	return m
}()

// RawMaxLevels caps building levels of land-bound producers (land is scarce).
const RawMaxLevels = 4

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

// Terrains are the tile types.
var Terrains = []Terrain{
	{"plains", 'P', 1.0, []Bonus{{"Farm", 1.5}, {"Potato farm", 1.2}, {"Ranch", 1.3},
		{"Sheep farm", 1.0}, {"Clay pit", 1.0}}},
	{"forest", 'F', 1.0, []Bonus{{"Forestry", 1.8}, {"Potato farm", 0.8}, {"Ranch", 0.7}}},
	{"hills", 'H', 1.0, []Bonus{{"Sheep farm", 1.5}, {"Vineyard", 1.5}, {"Coal mine", 1.0},
		{"Clay pit", 1.3}, {"Potato farm", 1.0}}},
	{"mountains", 'M', 1.0, []Bonus{{"Coal mine", 1.5}, {"Iron mine", 1.5}, {"Sheep farm", 0.8}}},
	{"tropical", 'T', 1.0, []Bonus{{"Cotton plantation", 1.5}, {"Sugar plantation", 1.5},
		{"Tea plantation", 1.5}, {"Silk farm", 1.2}, {"Farm", 0.6}}},
	{"coast", 'C', 0.25, []Bonus{{"Fishery", 1.5}, {"Farm", 0.8}, {"Potato farm", 1.0},
		{"Clay pit", 0.8}}},
}

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
	"FFPPHC",
	"PPPTTC",
	"CPTTCC",
	"CCCCCC",
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

func makeFirms(t Terrain, game bool) []*Firm {
	var firms []*Firm
	addRaw := func(recipes []Recipe, bonuses []Bonus) {
		for _, rc := range recipes {
			for _, b := range bonuses {
				if b.Firm == rc.Firm {
					f := NewFirm(rc.Firm, G(rc.Output), rc.TFP*b.Mult, rc.Factors, 50)
					f.MaxLevels, f.Sector = RawMaxLevels, Primary
					firms = append(firms, f)
				}
			}
		}
	}
	addRaw(RawRecipes, t.Raw)
	if game {
		addRaw(GameRawRecipes, GameTerrainRaw[t.Name])
	}
	recipes := ProcessingRecipes
	if game {
		recipes = append(append([]Recipe(nil), recipes...), GameRecipes...)
	}
	for _, rc := range recipes {
		f := NewFirm(rc.Firm, G(rc.Output), rc.TFP, rc.Factors, 50)
		if serviceFirms[rc.Firm] {
			f.Sector = Service
		}
		firms = append(firms, f)
	}
	if game {
		for _, f := range firms {
			f.Requires = FirmTech[f.Name]
			f.Enabled = f.Requires == ""
		}
	}
	return firms
}

// BuildWorld builds the world for a hex map without nations (the model the
// Python reference specifies). transportCost scales every hop; +Inf means
// autarky (no trade).
func BuildWorld(transportCost float64, gameMap []string) (*World, error) {
	return buildWorld(transportCost, gameMap, false)
}

// The cost of a hop between neighbouring tiles is the average of the two
// tiles' movement costs (land 1, coast 1/4, less with infrastructure). Traders
// only run between neighbours; longer trade happens by re-export along a chain
// of tiles.
func buildWorld(transportCost float64, gameMap []string, game bool) (*World, error) {
	if len(gameMap) == 0 {
		return nil, fmt.Errorf("empty map")
	}
	rows, cols := len(gameMap), len(gameMap[0])
	grid := make([][]*Region, rows)
	var regions []*Region
	for r := 0; r < rows; r++ {
		if len(gameMap[r]) != cols {
			return nil, fmt.Errorf("map row %d has %d columns, want %d", r, len(gameMap[r]), cols)
		}
		grid[r] = make([]*Region, cols)
		for c := 0; c < cols; c++ {
			t, err := TerrainBySymbol(gameMap[r][c])
			if err != nil {
				return nil, err
			}
			reg := NewRegion(len(regions), fmt.Sprintf("%d%d", r, c), makeFirms(t, game), 2, 50, 500)
			reg.Row, reg.Col, reg.Terrain, reg.BaseMove = r, c, t.Name, t.MoveCost
			grid[r][c] = reg
			regions = append(regions, reg)
		}
	}

	var traders []*Trader
	var links []Link
	for _, a := range regions {
		for _, nb := range HexNeighbors(a.Row, a.Col, rows, cols) {
			b := grid[nb[0]][nb[1]]
			hop := (a.MoveCost() + b.MoveCost()) / 2
			if a.ID < b.ID {
				links = append(links, Link{a.ID, b.ID, hop})
			}
			if !math.IsInf(transportCost, 1) {
				tradable := Tradable
				if game {
					tradable = GameTradable
				}
				for _, g := range tradable {
					traders = append(traders, NewTrader(g, a, b, hop, transportCost))
				}
			}
		}
	}
	return NewWorld(rows, cols, regions, traders, links), nil
}

// NationSpec describes a nation in a game scenario.
type NationSpec struct {
	Symbol byte // marks its tiles in the nation map
	Name   string
	Color  string
}

// GameSpec is a game scenario: a terrain map plus who owns which tile.
type GameSpec struct {
	Map           []string
	NationMap     []string // same shape as Map; '.' = unowned
	Cities        [][2]int // (row, col) of city tiles
	Nations       []NationSpec
	TransportCost float64
	TaxRate       float64
	AI            bool // nations are run by the computer until a player takes one
}

// DefaultGame is the default game scenario on DefaultMap: three nations of
// four tiles each; the rest of the map is unowned and can be annexed.
var DefaultGame = GameSpec{
	Map: DefaultMap,
	NationMap: []string{
		"AA..BB",
		"AA.BB.",
		"......",
		"......",
		"...CC.",
		"...CC.",
	},
	Nations: []NationSpec{
		{'A', "Montara", "#c0392b"},
		{'B', "Verdania", "#2e6fd8"},
		{'C', "Thalassa", "#17a589"},
	},
	// a capital for each nation, and a free city in the middle
	Cities:        [][2]int{{1, 1}, {1, 4}, {4, 4}, {3, 2}},
	TransportCost: 1,
	TaxRate:       0.1,
}

// BuildGame builds a game world: regions owned by nations that tax their pops,
// trade under tariffs, research technology and build infrastructure, with
// Construction and University firms on every tile, investing firms and
// migrating people.
func BuildGame(spec GameSpec) (*World, error) {
	if math.IsInf(spec.TransportCost, 0) || math.IsNaN(spec.TransportCost) || spec.TransportCost < 0 {
		return nil, fmt.Errorf("transport cost must be a non-negative number")
	}
	w, err := buildWorld(spec.TransportCost, spec.Map, true)
	if err != nil {
		return nil, err
	}
	if len(spec.NationMap) != w.Rows {
		return nil, fmt.Errorf("nation map has %d rows, want %d", len(spec.NationMap), w.Rows)
	}
	w.Investment, w.Migration, w.Demography = true, true, true
	w.Spec = &spec
	for _, c := range spec.Cities {
		if c[0] < 0 || c[0] >= w.Rows || c[1] < 0 || c[1] >= w.Cols {
			return nil, fmt.Errorf("city %v is off the map", c)
		}
		makeCity(w.Regions[c[0]*w.Cols+c[1]])
	}
	bySymbol := map[byte]*Nation{}
	for i, ns := range spec.Nations {
		n := newNation(i, ns.Name, ns.Color, len(spec.Nations), spec.TaxRate)
		n.AI = spec.AI
		if err := n.SetTaxRate(spec.TaxRate); err != nil {
			return nil, err
		}
		bySymbol[ns.Symbol] = n
		w.Nations = append(w.Nations, n)
	}
	for _, r := range w.Regions {
		row := spec.NationMap[r.Row]
		if len(row) != w.Cols {
			return nil, fmt.Errorf("nation map row %d has %d columns, want %d", r.Row, len(row), w.Cols)
		}
		if row[r.Col] == '.' {
			continue
		}
		n, ok := bySymbol[row[r.Col]]
		if !ok {
			return nil, fmt.Errorf("unknown nation symbol %q", row[r.Col])
		}
		r.Nation = n
		n.Regions = append(n.Regions, r)
	}
	for _, n := range w.Nations {
		n.applyTech()
	}
	w.refreshAllHops()
	return w, nil
}

// Cities start with this many times the people of other tiles, and their
// land-bound producers are capped at this many levels (there's little land).
const (
	cityPopMult      = 2.0
	cityRawMaxLevels = 2
)

func makeCity(r *Region) {
	r.City = true
	for _, p := range r.Pops {
		p.Size *= cityPopMult
		p.Cash *= cityPopMult
	}
	for _, f := range r.Firms {
		if f.Sector == Primary {
			f.MaxLevels = cityRawMaxLevels
		}
	}
	r.updateAgglomeration()
}
