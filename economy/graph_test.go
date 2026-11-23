package economy

import (
	"encoding/json"
	"math"
	"testing"
)

// A map of three places in a line, with lanes of their own lengths. Trade and migration run along the
// lanes only, and a restored world goes on exactly as the original.
func TestGraphWorld(t *testing.T) {
	places := []Place{{Name: "A", Terrain: "plains"}, {Name: "B", Terrain: "hills"}, {Name: "C", Terrain: "forest"}}
	lanes := []Lane{{0, 1, 2}, {1, 2, 0.5}}
	w, err := BuildWorld(1, Graph{Places: places, Lanes: lanes})
	if err != nil {
		t.Fatal(err)
	}
	for _, tr := range w.Traders {
		if (tr.Origin.ID == 0 && tr.Dest.ID == 2) || (tr.Origin.ID == 2 && tr.Dest.ID == 0) {
			t.Fatalf("a route between places with no lane: %s", tr.Name())
		}
		if want := map[[2]int]float64{{0, 1}: 2, {1, 0}: 2, {1, 2}: 0.5, {2, 1}: 0.5}[[2]int{tr.Origin.ID, tr.Dest.ID}]; tr.Distance != want {
			t.Fatalf("%s: distance %v, want %v", tr.Name(), tr.Distance, want)
		}
	}
	w.Investment, w.Migration = true, true
	for i := 0; i < 200; i++ {
		w.Step()
	}
	r, err := Restore(w.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 30; i++ {
		w.Step()
		r.Step()
	}
	a, _ := json.Marshal(w.Snapshot())
	b, _ := json.Marshal(r.Snapshot())
	if string(a) != string(b) {
		t.Error("a restored graph world plays differently")
	}
	if _, err := BuildWorld(1, Graph{Places: places, Lanes: []Lane{{0, 0, 1}}}); err == nil {
		t.Error("a lane from a place to itself was accepted")
	}
	if _, err := BuildWorld(1, Graph{Places: []Place{{Name: "X", Terrain: "moon"}}}); err == nil {
		t.Error("an unknown terrain was accepted")
	}
}

// A need's subsistence level: below it a living is worth less in
// proportion, and wholly without it worth nothing.
func TestSubsistence(t *testing.T) {
	saved := Needs
	defer func() { Needs = saved }()
	Needs = append([]Need(nil), Needs...)
	food := -1
	for k := range Needs {
		if Needs[k].Tier == Basic && food < 0 {
			food = k
			Needs[k].Subsistence = 0.05
		}
	}
	plenty := make([]float64, NumGoods)
	for _, nd := range Needs {
		for _, g := range nd.Goods {
			plenty[g] = 1
		}
	}
	if m := SubsistenceMet(plenty); m != 1 {
		t.Errorf("met %v with plenty, want 1", m)
	}
	half := append([]float64(nil), plenty...)
	for _, g := range Needs[food].Goods {
		half[g] = 0
	}
	if u := Utility(half); u != 0 {
		t.Errorf("utility %v with no %s at all, want 0", u, Needs[food].Name)
	}
}

// A land-bound firm may build beyond its good land, onto poorer land: each
// level there less productive, so the average quality falls, and the next
// level adds less than its average level earns.
func TestPoorerLand(t *testing.T) {
	f := NewFirm("Farm", G("grain"), 2, []Factor{Labor(0.6)}, 50)
	f.MaxLevels = 4
	f.Levels = 4
	if q := f.LandQuality(); q != 1 || f.marginalLevel() >= 1 {
		t.Fatalf("on its good land: quality %v, next level %v", q, f.marginalLevel())
	}
	f.Levels = 8
	r := math.Pow(0.5, 0.25) // each level beyond yields this much of the one before
	if q := f.LandQuality(); math.Abs(q-(4+r*(1-0.5)/(1-r))/8) > 1e-9 {
		t.Errorf("twice its good land: quality %v", q)
	}
	if f.AtCap() {
		t.Error("full at twice its good land")
	}
	f.Levels = 12
	if !f.AtCap() {
		t.Error("room beyond three times its good land")
	}
}
