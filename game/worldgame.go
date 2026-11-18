package game

import (
	"math/rand"

	"github.com/rasmusbergpalm/mercury/economy"
)

// WorldGame is the world in 1820 on about two thousand land tiles (the scale
// of a grand-strategy map): the twelve great powers on their lands, and the
// rest of the world free land, to settle or colonise. The map, its terrain,
// the tiles' names and who held them come from Natural Earth's data (see
// tools/world_map.py); a tile with no town of note gets a made-up name.
func WorldGame() GameSpec {
	return GameSpec{
		Map:           WorldMap,
		NationMap:     WorldNations,
		Nations:       WorldNationSpecs,
		TransportCost: 1,
		TaxRate:       0.1,
		Names:         worldTileNames(),
	}
}

// worldTileNames are the tiles' names, the unnamed ones given made-up
// names that suit their terrain.
func worldTileNames() []string {
	used := map[string]bool{}
	for _, n := range worldNames {
		used[n] = true
	}
	rng := rand.New(rand.NewSource(1820))
	out := make([]string, 0, len(worldNames))
	i := 0
	for r, row := range WorldMap {
		for c := 0; c < len(row); c++ {
			if row[c] == Sea {
				continue
			}
			name := worldNames[i]
			if name == "" {
				coast := false
				for _, nb := range HexNeighbors(r, c, len(WorldMap), len(row)) {
					coast = coast || WorldMap[nb[0]][nb[1]] == Sea
				}
				name = economy.PlaceName(rng, row[c], coast, used)
			}
			out = append(out, name)
			i++
		}
	}
	return out
}
