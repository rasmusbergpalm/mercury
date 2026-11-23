package game

import (
	"fmt"
	"strings"

	"github.com/rasmusbergpalm/mercury/economy"
)

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

// baseGameWorld builds a game's map, the economy's full world with the
// public services on every tile, before any nation or settling.
func baseGameWorld(spec GameSpec) (*World, error) {
	g, err := economy.HexGraph(spec.Map, spec.Names)
	if err != nil {
		return nil, err
	}
	e, err := economy.BuildWorld(spec.TransportCost, g)
	if err != nil {
		return nil, err
	}
	w := wrap(e)
	for _, r := range w.Regions {
		r.Educated = StartEducated
		for k := Public(0); k < NumServices; k++ {
			r.Services[k] = newService(r, k)
		}
		r.applyAllegiance()
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
	w.TrackGDP = true
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
		r.setNation(n)
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

// A tech's description lists the production methods it brings, from the
// method table, so that the two can't disagree.
func init() {
	method := map[string]bool{}
	for _, m := range MethodRecipes {
		method[m.Firm] = true
	}
	for i := range Techs {
		var names []string
		for _, f := range Techs[i].EnableFirms {
			if method[f] {
				names = append(names, strings.ToLower(f[:1])+f[1:])
			}
		}
		if len(names) == 0 {
			continue
		}
		list := "new methods: " + strings.Join(names, ", ")
		if Techs[i].Effect == "" {
			Techs[i].Effect = strings.ToUpper(list[:1]) + list[1:]
		} else {
			Techs[i].Effect += "; " + list
		}
	}
}
