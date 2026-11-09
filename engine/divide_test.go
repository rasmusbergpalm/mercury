package engine

import "testing"

// Dividing the land leaves no free tile, leaves each nation's land in one
// piece, and shares it out evenly: no nation holds more than a tile more
// than another.
func TestDivideLand(t *testing.T) {
	check := func(name string, spec GameSpec) {
		t.Helper()
		div, err := DivideLand(spec)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		w, err := buildGame(div, false, nil)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for _, r := range w.Regions {
			if r.Nation == nil {
				t.Errorf("%s: tile %s is still free", name, r.Name)
			}
		}
		lo, hi := len(w.Regions), 0
		for _, n := range w.Nations {
			lo, hi = min(lo, len(n.Regions)), max(hi, len(n.Regions))
			if p := pieces(w, n.Regions); p != 1 {
				t.Errorf("%s: %s's land is in %d pieces", name, n.Name, p)
			}
		}
		if hi-lo > 1 {
			t.Errorf("%s: nations hold %d to %d tiles", name, lo, hi)
		}
	}
	check("classic", DefaultGame)
	for size := range MapSizes {
		for seed := 1; seed <= 6; seed++ {
			check(MapSizes[size].Name, generated(t, size, seed))
		}
	}
}

// pieces is how many connected bodies the tiles form.
func pieces(w *World, tiles []*Region) int {
	in := map[*Region]bool{}
	for _, r := range tiles {
		in[r] = true
	}
	seen := map[*Region]bool{}
	n := 0
	for _, r := range tiles {
		if seen[r] {
			continue
		}
		n++
		stack := []*Region{r}
		seen[r] = true
		for len(stack) > 0 {
			c := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			w.landNeighbours(c, func(nb *Region) bool {
				if in[nb] && !seen[nb] {
					seen[nb] = true
					stack = append(stack, nb)
				}
				return true
			})
		}
	}
	return n
}
