package game

import (
	"encoding/json"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func generated(t *testing.T, size, seed int) GameSpec {
	t.Helper()
	m := MapSizes[size]
	spec, err := GenerateGame(MapOptions{Rows: m.Rows, Cols: m.Cols, Nations: m.Nations, Seed: int64(seed)})
	if err != nil {
		t.Fatalf("%s map, seed %d: %v", m.Name, seed, err)
	}
	return spec
}

// Generated maps: the same seed gives the same map, every size makes a
// playable world (one continent, land in the asked share, nations of startTiles tiles
// apart from each other, a name for every tile, all different), and the
// world builds from it.
func TestGeneratedMaps(t *testing.T) {
	if !reflect.DeepEqual(generated(t, 1, 7), generated(t, 1, 7)) {
		t.Error("the same seed gave different maps")
	}
	if reflect.DeepEqual(generated(t, 1, 7).Map, generated(t, 1, 8).Map) {
		t.Error("different seeds gave the same map")
	}
	for size, m := range MapSizes {
		for seed := 1; seed <= 6; seed++ {
			spec := generated(t, size, seed)
			cells := m.Rows * m.Cols
			land := 0
			for _, row := range spec.Map {
				land += len(row) - strings.Count(row, string(rune(Sea)))
			}
			if share := float64(land) / float64(cells); share < 0.4 || share > 0.6 {
				t.Errorf("%s/%d: land is %.0f%% of the map", m.Name, seed, 100*share)
			}
			if len(spec.Names) != land {
				t.Errorf("%s/%d: %d names for %d tiles", m.Name, seed, len(spec.Names), land)
			}
			seen := map[string]bool{}
			for _, n := range spec.Names {
				if seen[n] || n == "" {
					t.Errorf("%s/%d: name %q twice", m.Name, seed, n)
				}
				seen[n] = true
			}
			if len(spec.Nations) != m.Nations {
				t.Errorf("%s/%d: %d nations", m.Name, seed, len(spec.Nations))
			}
			w, err := buildGame(spec, false, nil)
			if err != nil {
				t.Fatalf("%s/%d: %v", m.Name, seed, err)
			}
			if p := pieces(w, w.Regions); p != 1 {
				t.Errorf("%s/%d: the land is in %d pieces, want one continent", m.Name, seed, p)
			}
			for _, n := range w.Nations {
				if len(n.Regions) != startTiles {
					t.Errorf("%s/%d: %s starts with %d tiles", m.Name, seed, n.Name, len(n.Regions))
				}
				for _, r := range n.Regions {
					w.landNeighbours(r, func(nb *Region) bool {
						if nb.Nation != nil && nb.Nation != n {
							t.Errorf("%s/%d: %s and %s start side by side", m.Name, seed, n.Name, nb.Nation.Name)
						}
						return true
					})
				}
			}
		}
	}
}

// Sea cells hold no tile; the land next to them (or to the map's edge) is
// coast, and only land tiles are linked.
func TestSeaTiles(t *testing.T) {
	w, err := BuildWorld(1, []string{
		"PPPPPP",
		"PPPPPP",
		"PPPPPP",
		"PPPPPP",
		"PPPP~P",
		"PPPPPP",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(w.Regions) != 35 {
		t.Fatalf("%d tiles, want 35", len(w.Regions))
	}
	if w.RegionAt(4, 4) != nil || w.RegionAt(4, 3) == nil || w.RegionAt(9, 9) != nil {
		t.Error("RegionAt doesn't match the map")
	}
	for _, l := range w.Links {
		a, b := w.Regions[l.A], w.Regions[l.B]
		if HexDistance(a.Row, a.Col, b.Row, b.Col) != 1 {
			t.Errorf("link between %s and %s, which aren't neighbours", a.Name, b.Name)
		}
	}
	if w.RegionAt(2, 2).Coastal || !w.RegionAt(3, 3).Coastal || !w.RegionAt(0, 2).Coastal {
		t.Error("coast: inland (2,2), by the sea (3,3) and on the edge (0,2) are wrong")
	}
	if _, err := BuildWorld(1, []string{"~~", "~~"}); err == nil {
		t.Error("a map of sea alone was accepted")
	}
}

// Every port trades with every other.
func TestSeaRoutes(t *testing.T) {
	w, err := buildGame(generated(t, 2, 3), false, nil)
	if err != nil {
		t.Fatal(err)
	}
	opened := 0
	for _, r := range w.Regions {
		if r.Coastal && opened < 12 {
			r.Port = true
			opened++
		}
	}
	before := len(w.Traders)
	w.openNewPorts()
	if sea, want := len(w.Traders)-before, opened*(opened-1)*len(FullTradable); sea != want {
		t.Errorf("%d sea routes, want %d", sea, want)
	}
}

// Big worlds step their tiles on every core: the result must not depend on
// how many there are.
func TestParallelStepIsDeterministic(t *testing.T) {
	if testing.Short() {
		t.Skip("long")
	}
	run := func(procs int) []byte {
		defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(procs))
		spec := generated(t, 1, 11)
		spec.AI = true
		w, err := buildGame(spec, false, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(w.Regions) < ParallelMin {
			t.Fatalf("only %d tiles: the parallel path isn't tested", len(w.Regions))
		}
		money := w.Money()
		for i := 0; i < 400; i++ {
			w.Step()
		}
		if d := relDiff(w.Money(), money); d > 1e-9 {
			t.Errorf("money changed by %v (%d cores)", d, procs)
		}
		s, err := w.Snapshot()
		if err != nil {
			t.Fatal(err)
		}
		b, _ := json.Marshal(s)
		return b
	}
	if string(run(1)) != string(run(4)) {
		t.Error("one core and four gave different worlds")
	}
}

// A big generated map settles for fewer years, but its markets are calm
// when the game opens.
func TestGeneratedGamesStartSettled(t *testing.T) {
	if testing.Short() {
		t.Skip("long")
	}
	spec := generated(t, 1, 5)
	days := 0
	w, err := BuildGameProgress(spec, func(done, total int) { days = total })
	if err != nil {
		t.Fatal(err)
	}
	if n := len(w.Regions); days != PrepareDays(n) || SettleDays(n) >= SettleYears*TicksPerYear {
		t.Errorf("settled %d days for %d tiles", days, len(w.Regions))
	}
	bread := G("bread")
	before := make([]float64, len(w.Regions))
	for i, r := range w.Regions {
		before[i] = r.Markets[bread].Price
	}
	w.Step()
	for i, r := range w.Regions {
		if p := r.Markets[bread].Price; p > 1.15*before[i] || p < before[i]/1.15 {
			t.Errorf("tile %s: bread %v -> %v on the first day", r.Name, before[i], p)
		}
	}
}
