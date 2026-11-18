package game

import "math/rand"

var nationRoots = []string{"Mont", "Verd", "Thal", "Aust", "Bor", "Cael", "Dalm", "Esk", "Fris",
	"Gall", "Hesp", "Ilyr", "Lus", "Mor", "Norv", "Ost", "Pann", "Rhaet", "Sard", "Tav", "Ulr", "Vand", "Wess", "Zell"}
var nationEndings = []string{"ara", "ania", "assa", "ia", "onia", "avia", "esia", "ica", "and", "ovia", "eria"}

// nationName makes up a nation's name not yet used.
func nationName(rng *rand.Rand, used map[string]bool) string {
	for {
		root := nationRoots[rng.Intn(len(nationRoots))]
		end := nationEndings[rng.Intn(len(nationEndings))]
		name := root + end
		if !used[name] {
			used[name] = true
			return name
		}
	}
}
