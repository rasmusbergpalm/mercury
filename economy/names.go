package economy

import (
	"math/rand"
	"strings"
)

// Place names: each tile gets a made-up name built from a first part and an
// ending that suits its terrain (fells in the mountains, havens by the sea).
// Names are unique on a map, and the same map and seed always give the same
// names.

var namePrefixes = []string{
	"Ash", "Bel", "Bram", "Cal", "Dun", "El", "Fen", "Gal", "Har", "Kel", "Lor", "Mar",
	"Nor", "Os", "Pen", "Ros", "Sal", "Tam", "Val", "Wes", "Al", "Bran", "Cor", "Dor",
	"Ever", "Glen", "Hal", "Iver", "Kin", "Lan", "Mor", "Ral", "Stan", "Thorn", "Ul", "Wyn",
	"Ard", "Bar", "Carr", "Del", "Est", "Fal", "Gren", "Hol", "Lin", "Mel", "Ned", "Or",
	"Pal", "Quen", "Red", "Sel", "Tor", "Ver", "Wal", "Yar", "Ast", "Brem", "Cael", "Dral",
	"Elm", "Firth", "Gar", "Heth", "Ink", "Jor", "Kar", "Lis", "Mon", "Oak", "Pyr", "Rav",
}

var nameLinks = []string{"", "", "", "", "a", "e", "i", "o", "en"}

// nameEndings are the endings by terrain symbol, and for the coast.
var nameEndings = map[byte][]string{
	'P': {"field", "ham", "ton", "wick", "ley", "by", "stead", "worth", "ford", "bridge"},
	'F': {"wood", "holt", "hurst", "den", "grove", "shaw", "wald", "thorpe"},
	'H': {"don", "low", "crest", "ridge", "bury", "combe", "wold", "cliffe"},
	'M': {"berg", "peak", "fell", "crag", "stein", "mont", "horn", "scar"},
	'T': {"ora", "ana", "ica", "ara", "ilo", "enza", "ama", "ita"},
	'S': {"stan", "ovka", "insk", "dale", "mead", "plain"},
	'D': {"sand", "oasis", "abad", "dune", "wadi", "ghar"},
	'U': {"vik", "nes", "fjell", "moor", "tun", "skar"},
	0:   {"mouth", "haven", "port", "sea", "strand", "wick", "ness", "bay"},
}

// NameTiles names every land tile of a map, row by row (skipping the sea).
func NameTiles(gameMap []string, seed int64) []string {
	rng := rand.New(rand.NewSource(seed*31 + 7))
	rows, cols := len(gameMap), 0
	if rows > 0 {
		cols = len(gameMap[0])
	}
	used := map[string]bool{}
	var out []string
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			sym := gameMap[r][c]
			if sym == Sea {
				continue
			}
			coast := false
			nbs := HexNeighbors(r, c, rows, cols)
			for _, nb := range nbs {
				coast = coast || gameMap[nb[0]][nb[1]] == Sea
			}
			coast = coast || len(nbs) < 6
			out = append(out, PlaceName(rng, sym, coast, used))
		}
	}
	return out
}

// PlaceName makes up a name for a tile of the terrain, not yet used.
func PlaceName(rng *rand.Rand, terrain byte, coast bool, used map[string]bool) string {
	pick := func(xs []string) string { return xs[rng.Intn(len(xs))] }
	for try := 0; ; try++ {
		endings := nameEndings[terrain]
		if coast && rng.Intn(3) == 0 {
			endings = nameEndings[0]
		}
		end := pick(endings)
		link := pick(nameLinks)
		if strings.ContainsAny(end[:1], "aeiou") {
			link = "" // "Calora", not "Caleora"
		}
		name := pick(namePrefixes) + link + end
		if try > 20 { // a crowded map: an old and a new town of the same name
			name = pick([]string{"Upper ", "Lower ", "New ", "Old ", "Great ", "Little "}) + name
		}
		if !used[name] {
			used[name] = true
			return name
		}
	}
}
