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

// GameRecipes exist only in game worlds (with nations): building work for
// infrastructure and firm expansions, research for technology, and soldiers
// (who need ironware and clothes) for armies.
var GameRecipes = []Recipe{
	{"Construction", "construction", 1.5, []Factor{Labor(0.5), Input("wood", 0.2)}},
	{"University", "research", 1.0, []Factor{Labor(0.6)}},
	{"Barracks", "military", 1.0, []Factor{Labor(0.4), Input("ironware", 0.15), Input("clothes", 0.1)}},
}

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
	for _, rc := range RawRecipes {
		for _, b := range t.Raw {
			if b.Firm == rc.Firm {
				f := NewFirm(rc.Firm, G(rc.Output), rc.TFP*b.Mult, rc.Factors, 50)
				f.MaxLevels = RawMaxLevels
				firms = append(firms, f)
			}
		}
	}
	recipes := ProcessingRecipes
	if game {
		recipes = append(append([]Recipe(nil), recipes...), GameRecipes...)
	}
	for _, rc := range recipes {
		firms = append(firms, NewFirm(rc.Firm, G(rc.Output), rc.TFP, rc.Factors, 50))
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
func buildWorld(transportCost float64, gameMap []string, construction bool) (*World, error) {
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
			reg := NewRegion(len(regions), fmt.Sprintf("%d%d", r, c), makeFirms(t, construction), 2, 50, 500)
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
				for _, g := range Tradable {
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
	w.Investment, w.Migration = true, true
	w.Spec = &spec
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
	w.refreshAllHops()
	return w, nil
}
