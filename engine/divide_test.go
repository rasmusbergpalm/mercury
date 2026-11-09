package engine

import "testing"

// Dividing the land leaves no free tile, keeps every nation's own tiles,
// and on one continent shares the land about evenly.
func TestDivideLand(t *testing.T) {
	check := func(name string, spec GameSpec) []int {
		t.Helper()
		div, err := DivideLand(spec)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for r, row := range spec.NationMap {
			for c := range row {
				if row[c] != '.' && div.NationMap[r][c] != row[c] {
					t.Errorf("%s: tile (%d,%d) changed hands", name, r, c)
				}
			}
		}
		w, err := buildGame(div, false, nil)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		var sizes []int
		for _, r := range w.Regions {
			if r.Nation == nil {
				t.Errorf("%s: tile %s is still free", name, r.Name)
			}
		}
		for _, n := range w.Nations {
			sizes = append(sizes, len(n.Regions))
		}
		return sizes
	}
	classic := check("classic", DefaultGame)
	lo, hi := classic[0], classic[0]
	for _, n := range classic {
		lo, hi = min(lo, n), max(hi, n)
	}
	if hi-lo > 2 {
		t.Errorf("classic map shared out unevenly: %v", classic)
	}
	for size := range MapSizes {
		for seed := 1; seed <= 4; seed++ {
			check(MapSizes[size].Name, generated(t, size, seed))
		}
	}
}
