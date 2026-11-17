package engine

import "testing"

// The world map builds: about two thousand land tiles, each named once,
// every great power of 1820 on its lands, and the new terrains in use.
func TestWorldGame(t *testing.T) {
	w, err := buildGame(WorldGame(), false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(w.Regions); n < 1900 || n > 2200 {
		t.Errorf("%d land tiles, want about 2000", n)
	}
	names, terrains := map[string]bool{}, map[string]int{}
	for _, r := range w.Regions {
		if r.Name == "" || names[r.Name] {
			t.Errorf("tile %d,%d: name %q empty or used twice", r.Row, r.Col, r.Name)
		}
		names[r.Name] = true
		terrains[r.Terrain]++
	}
	for _, t0 := range Terrains {
		if terrains[t0.Name] == 0 {
			t.Errorf("no %s on the world map", t0.Name)
		}
	}
	for _, n := range w.Nations {
		if len(n.Regions) == 0 {
			t.Errorf("%s holds no land", n.Name)
		}
	}
	for _, want := range []string{"London", "Paris", "Vienna", "Istanbul", "Beijing", "New York"} {
		if !names[want] {
			t.Errorf("no tile named %s", want)
		}
	}
}
