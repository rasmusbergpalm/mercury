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

// A land-bound firm's land is fixed: its levels share it, so more levels
// on it each make less, and the next adds less than each earns.
func TestLandIsFixed(t *testing.T) {
	f := NewFirm("Farm", G("grain"), 2, []Factor{Labor(0.6)}, 50)
	f.Methods[0].Land = 0.25
	f.MaxLevels, f.Levels = 4, 4
	if m := f.LandMult(); m != 1 {
		t.Errorf("on as many levels as its land holds: %v", m)
	}
	f.Levels = 8
	if m := f.LandMult(); math.Abs(m-math.Pow(0.5, 0.25)) > 1e-12 {
		t.Errorf("on twice as many: %v, want 0.5^0.25", m)
	}
	if m := f.marginalLevel(); math.Abs(m-0.15/0.4) > 1e-12 {
		t.Errorf("the next level adds %v of what each earns, want (1-0.6-0.25)/(1-0.6)", m)
	}
	if f.AtCap() {
		t.Error("full at twice its land")
	}
	f.Levels = 4 * landReach
	if !f.AtCap() {
		t.Errorf("room beyond %d times its land", landReach)
	}
}

// Land is a good: each tile's Landowner makes a fixed amount a day, and its
// farms, forests and houses rent it at the price that clears it. When the
// town's houses can pay more, land dears and the farm rents less of it.
func TestLandIsRented(t *testing.T) {
	w, err := BuildWorld(1, Graph{Places: []Place{{Name: "A", Terrain: "plains"}}})
	if err != nil {
		t.Fatal(err)
	}
	r, land := w.Regions[0], G("land")
	firm := func(name string) *Firm {
		for _, f := range r.Firms {
			if f.Name == name {
				return f
			}
		}
		t.Fatalf("no %s on the plains", name)
		return nil
	}
	houses, farm, owner := firm("Houses"), firm("Farm"), firm("Landowner")
	used := func(days int) (price, farmLand float64) {
		for range days {
			w.Step()
			price += r.Markets[land].Price / float64(days)
			farmLand += farm.Used[land] / float64(days)
		}
		return
	}
	used(400)
	p0, f0 := used(100)
	if owner.LastOutput <= 0 || p0 <= 0 {
		t.Fatalf("the landowner made %v land, at £%v", owner.LastOutput, p0)
	}
	houses.Mult = 3 // a town whose houses are worth far more
	used(400)
	p1, f1 := used(100)
	if p1 <= p0 || f1 >= f0 {
		t.Errorf("land at £%.2f -> £%.2f, the farm renting %.2f -> %.2f a day", p0, p1, f0, f1)
	}
}
