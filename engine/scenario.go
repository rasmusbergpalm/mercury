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
	{"Armory", "arms", 1.0, []Factor{Labor(0.3), Input("iron", 0.25), Input("wood", 0.1)}},
	{"Foundry", "artillery", 0.8, []Factor{Labor(0.3), Input("iron", 0.3), Input("coal", 0.1)}},
	{"Munitions plant", "munitions", 1.5, []Factor{Labor(0.3), Input("iron", 0.2), Input("coal", 0.2)}},
	{"Motor works", "vehicles", 0.8, []Factor{Labor(0.3), Input("iron", 0.3), Input("coal", 0.1)}},
	{"Aircraft factory", "aircraft", 0.8, []Factor{Labor(0.4), Input("wood", 0.15), Input("cloth", 0.1), Input("iron", 0.1)}},
	{"Textile factory", "cloth", 3.0, []Factor{Labor(0.12), Comp("fiber", 0.4), Input("coal", 0.08), Input("steam engines", 0.05)}},
	{"Steel works", "iron", 2.2, []Factor{Labor(0.15), Input("iron ore", 0.3), Input("coal", 0.15), Input("steam engines", 0.05)}},
	{"Toolmaker", "tools", 1.2, []Factor{Labor(0.3), Input("iron", 0.25), Input("wood", 0.1)}},
	{"Engine works", "steam engines", 0.8, []Factor{Labor(0.3), Input("iron", 0.3), Input("coal", 0.1), Input("tools", 0.1)}},
	{"Chemical works", "explosives", 1.0, []Factor{Labor(0.3), Input("coal", 0.2), Input("clay", 0.1)}},
	{"Fertilizer works", "fertilizer", 1.2, []Factor{Labor(0.3), Input("coal", 0.15), Input("clay", 0.15)}},
	{"Power station", "electricity", 2.0, []Factor{Labor(0.2), Input("coal", 0.3), Input("steam engines", 0.1)}},
}

// serviceFirms are the firms in the service sector (the rest of the
// non-primary firms are industry).
var serviceFirms = map[string]bool{"Transport": true, "Construction": true, "University": true}

// Methods are other ways of making what a firm makes, which a tech makes
// available (Tech.EnableFirms): each is a firm of its own beside the old
// one, with its own recipe and levels. More productive, they need
// machinery and fuel the old one doesn't, so they spread as owners find
// them worth building (and the old one comes back where the machinery is
// dear). A land-bound one shares its land with the old one: it grows by
// taking the old one's levels over. Like the machines of the period, a
// method replaces labour with machinery and fuel and keeps at least the
// old one's share for its capital (one less its exponents): a recipe that
// only added inputs would leave less profit a level and never spread.
var Methods = []struct {
	Base string
	Recipe
}{
	{"Coal mine", Recipe{"Steam coal mine", "coal", 3.3, []Factor{Labor(0.4), Input("steam engines", 0.08), Input("coal", 0.04)}}},
	{"Iron mine", Recipe{"Steam iron mine", "iron ore", 2.4, []Factor{Labor(0.4), Input("steam engines", 0.08), Input("coal", 0.04)}}},
	{"Iron mine", Recipe{"Blasting iron mine", "iron ore", 3.0, []Factor{Labor(0.35), Input("explosives", 0.08), Input("steam engines", 0.05)}}},
	{"Farm", Recipe{"Seed-drill farm", "grain", 2.4, []Factor{Labor(0.5), Input("tools", 0.05)}}},
	{"Farm", Recipe{"Fertilized farm", "grain", 3.2, []Factor{Labor(0.4), Input("tools", 0.05), Input("fertilizer", 0.1)}}},
	{"Farm", Recipe{"Tractor farm", "grain", 5.0, []Factor{Labor(0.25), Input("vehicles", 0.08), Input("oil", 0.05), Input("fertilizer", 0.08)}}},
	{"Potato farm", Recipe{"Fertilized potato farm", "potatoes", 3.6, []Factor{Labor(0.45), Input("fertilizer", 0.1)}}},
	{"Transport", Recipe{"Steam transport", "transport", 3.0, []Factor{Labor(0.4), Input("steam engines", 0.08), Input("coal", 0.04)}}},
	{"Transport", Recipe{"Motor transport", "transport", 4.5, []Factor{Labor(0.3), Input("vehicles", 0.1), Input("oil", 0.08)}}},
	{"Construction", Recipe{"Steam cranes", "construction", 2.0, []Factor{Labor(0.35), Input("wood", 0.15), Input("steam engines", 0.05), Input("coal", 0.03)}}},
	{"Toolmaker", Recipe{"Machine-tool works", "tools", 2.0, []Factor{Labor(0.2), Input("iron", 0.25), Input("steam engines", 0.05), Input("coal", 0.05)}}},
	{"Smithy", Recipe{"Electric smithy", "ironware", 2.5, []Factor{Labor(0.15), Input("iron", 0.3), Input("electricity", 0.1)}}},
	{"Carpenter", Recipe{"Electric joinery", "furniture", 2.5, []Factor{Labor(0.15), Input("wood", 0.4), Input("electricity", 0.07)}}},
	{"Tailor", Recipe{"Garment factory", "clothes", 3.0, []Factor{Labor(0.15), Input("cloth", 0.4), Input("electricity", 0.05), Input("tools", 0.03)}}},
	{"Motor works", Recipe{"Assembly-line works", "vehicles", 2.0, []Factor{Labor(0.15), Input("iron", 0.3), Input("coal", 0.05), Input("electricity", 0.1)}}},
}

// GameRawRecipes are land-bound producers that exist only in game worlds,
// with where they can be set up.
var GameRawRecipes = []Recipe{
	{"Oil well", "oil", 1.5, []Factor{Labor(0.6)}},
}

// GameTerrainRaw adds game-only raw producers to terrains.
var GameTerrainRaw = map[string][]Bonus{
	"tropical": {{"Oil well", 1.5}},
	"plains":   {{"Oil well", 0.7}},
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

// Terrains are the tile types. Iron ore is mined best in mountains; elsewhere
// poorer deposits (hill ores, bog iron, laterite) can be worked at a fraction
// of the productivity, so a nation cut off from mountains can still arm, dearly.
var Terrains = []Terrain{
	{"plains", 'P', 1.0, []Bonus{{"Farm", 1.5}, {"Potato farm", 1.2}, {"Ranch", 1.3},
		{"Sheep farm", 1.0}, {"Clay pit", 1.0}, {"Iron mine", 0.25}}},
	{"forest", 'F', 1.0, []Bonus{{"Forestry", 1.8}, {"Potato farm", 0.8}, {"Ranch", 0.7}, {"Iron mine", 0.3}}},
	{"hills", 'H', 1.0, []Bonus{{"Sheep farm", 1.5}, {"Vineyard", 1.5}, {"Coal mine", 1.0},
		{"Clay pit", 1.3}, {"Potato farm", 1.0}, {"Iron mine", 0.5}}},
	{"mountains", 'M', 1.0, []Bonus{{"Coal mine", 1.5}, {"Iron mine", 1.5}, {"Sheep farm", 0.8}}},
	{"tropical", 'T', 1.0, []Bonus{{"Cotton plantation", 1.5}, {"Sugar plantation", 1.5},
		{"Tea plantation", 1.5}, {"Silk farm", 1.2}, {"Farm", 0.6}, {"Iron mine", 0.25}}},
}

// The ocean surrounds the map. A tile on the map's edge is coastal whatever
// its terrain: it can fish, ships carry goods past it cheaply (CoastMove,
// less with naval techs) and it can build a port.
const CoastMove = 0.25

// CoastRaw are the producers every coastal tile has on top of its terrain's.
var CoastRaw = []Bonus{{"Fishery", 1.5}}

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

func makeFirms(t Terrain, coastal, game bool) []*Firm {
	var firms []*Firm
	addRaw := func(recipes []Recipe, bonuses []Bonus, site string) {
		for _, rc := range recipes {
			for _, b := range bonuses {
				if b.Firm == rc.Firm {
					f := NewFirm(rc.Firm, G(rc.Output), rc.TFP*b.Mult, rc.Factors, 50)
					f.MaxLevels, f.Sector = RawMaxLevels, Primary
					f.Site, f.SiteMult = site, b.Mult
					firms = append(firms, f)
				}
			}
		}
	}
	addRaw(RawRecipes, t.Raw, t.Name)
	if coastal {
		addRaw(RawRecipes, CoastRaw, "the coast")
	}
	if game {
		addRaw(GameRawRecipes, GameTerrainRaw[t.Name], t.Name)
		// a land-bound firm's methods work its land, at its site's productivity
		for _, base := range append([]*Firm(nil), firms...) {
			group := []*Firm{base}
			for _, m := range Methods {
				if m.Base == base.Name {
					f := NewFirm(m.Firm, G(m.Output), m.TFP*base.SiteMult, m.Factors, 50)
					f.MaxLevels, f.Sector, f.Site, f.SiteMult, f.Base = RawMaxLevels, Primary, base.Site, base.SiteMult, base.Name
					f.Levels = 0 // it starts on a level of the old method's, when its tech comes
					firms = append(firms, f)
					group = append(group, f)
				}
			}
			for _, f := range group {
				for _, o := range group {
					if o != f {
						f.land = append(f.land, o)
					}
				}
			}
		}
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
		if game {
			for _, m := range Methods {
				if m.Base == rc.Firm {
					g := NewFirm(m.Firm, G(m.Output), m.TFP, m.Factors, 50)
					g.Sector, g.Base = f.Sector, rc.Firm
					firms = append(firms, g)
				}
			}
		}
	}
	if game {
		for k := Public(0); k < NumServices; k++ {
			firms = append(firms, newServiceFirm(k))
		}
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
	return buildWorld(transportCost, gameMap, nil, false)
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
func buildWorld(transportCost float64, gameMap []string, names []string, game bool) (*World, error) {
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
			// people start with 10 a head of spending a day, in savings of a
			// twentieth (spendShare)
			reg := NewRegion(len(regions), name, makeFirms(t, coastal, game), 2, 50, 500/spendShare)
			if game {
				addOwnerClasses(reg)
				reg.Educated = startEducated
			}
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
	Nations       []NationSpec
	TransportCost float64
	TaxRate       float64
	AI            bool  // nations are run by the computer until a player takes one
	Seed          int64 // draws the computer nations' temperaments and their whims
	// Names names the tiles, row by row (skipping the sea); empty: by
	// their row and column.
	Names []string `json:",omitempty"`
}

// DefaultGame is the default game scenario on DefaultMap: three nations of
// four or five tiles, about equal in people and output once the map has
// settled (Montara's mountains are poor, so it has a rich forest too); the
// rest of the map is unowned and can be annexed.
var DefaultGame = GameSpec{
	Map: DefaultMap,
	NationMap: []string{
		"AA..BB",
		"AA.BB.",
		"A.....",
		"......",
		"...CC.",
		"...CC.",
	},
	Nations: []NationSpec{
		{'A', "Montara", "#b8322e"},
		{'B', "Verdania", "#2c5aa0"},
		{'C', "Thalassa", "#c08a1e"},
	},
	TransportCost: 1,
	TaxRate:       0.1,
	Names:         NameTiles(DefaultMap, 1820),
}

// BuildGame builds a game world: regions owned by nations that tax their pops,
// trade under tariffs, research technology and build infrastructure, with
// Construction and University firms on every tile, investing firms and
// migrating people.
//
// The game opens on a settled world: see SettleYears.
func BuildGame(spec GameSpec) (*World, error) {
	return buildGame(spec, true, nil)
}

// BuildGameProgress is BuildGame reporting how far the settling has got
// (days done of the total), which takes a while on a big map.
func BuildGameProgress(spec GameSpec, progress func(done, total int)) (*World, error) {
	return buildGame(spec, true, progress)
}

// baseGameWorld builds a game's map with every tile urban, before any
// nation or settling.
func baseGameWorld(spec GameSpec) (*World, error) {
	if math.IsInf(spec.TransportCost, 0) || math.IsNaN(spec.TransportCost) || spec.TransportCost < 0 {
		return nil, fmt.Errorf("transport cost must be a non-negative number")
	}
	w, err := buildWorld(spec.TransportCost, spec.Map, spec.Names, true)
	if err != nil {
		return nil, err
	}
	for _, r := range w.Regions {
		r.Urban = true
		r.updateAgglomeration()
	}
	return w, nil
}

// buildGame builds a game, settled first or not (a save restores its own
// state over it).
func buildGame(spec GameSpec, settle bool, progress func(done, total int)) (*World, error) {
	w, err := baseGameWorld(spec)
	if err != nil {
		return nil, err
	}
	if len(spec.NationMap) != w.Rows {
		return nil, fmt.Errorf("nation map has %d rows, want %d", len(spec.NationMap), w.Rows)
	}
	tiles := len(w.Regions)
	report := func(done int) {
		if progress != nil {
			progress(done, PrepareDays(tiles))
		}
	}
	if settle {
		b, err := settledBase(spec, func(done, _ int) { report(done) })
		if err != nil {
			return nil, err
		}
		w.settleFrom(b)
	}
	w.Investment, w.Migration, w.Demography = true, true, true
	w.Spec = &spec
	bySymbol := map[byte]*Nation{}
	for i, ns := range spec.Nations {
		n := newNation(i, ns.Name, ns.Color, len(spec.Nations), spec.TaxRate)
		n.AI = spec.AI
		n.Temper = drawTemperament(spec.Seed, i)
		n.seed = spec.Seed
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
	w.initLoyalties()
	w.refreshAllHops()
	if settle {
		w.burnIn(BurnInDays(tiles), func(done int) { report(SettleDays(tiles) + done) })
	}
	return w, nil
}

// Owner classes in game worlds: each is this share of a tile's people and
// starts with this share of its cash (so owners are much richer per head).
const (
	ownerPeopleShare = 0.04
	ownerCashShare   = 0.10
)

// addOwnerClasses turns part of a tile's people into an owner pop.
func addOwnerClasses(r *Region) {
	pop, cash := r.Population(), 0.0
	for _, p := range r.Pops {
		cash += p.Cash
	}
	for _, p := range r.Pops {
		p.Size *= 1 - ownerPeopleShare
		p.Cash *= 1 - ownerCashShare
	}
	p := NewPop(ownerPeopleShare*pop, ownerCashShare*cash)
	p.Class = Owners
	r.Pops = append(r.Pops, p)
	r.popDemand = append(r.popDemand, make([]float64, NumGoods))
}
