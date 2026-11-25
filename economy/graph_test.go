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

// A mine's deposit is a factor it holds, fixed: its methods share it as
// each would want it, and a method makes TFP * L^(1-s) * D^d * ...
func TestDepositIsShared(t *testing.T) {
	facs := []Factor{Labor(0.6), {Kind: DepositFactor, Exp: 0.12}}
	f := NewFirm("Coal mine", G("coal"), 2, facs, 1e6)
	f.Deposit = 4
	m := newMethod("Better mine", 3, facs)
	f.Methods = append(f.Methods, m)
	f.Levels, m.Levels = 4, 2
	prices := make([]float64, NumGoods)
	prices[G("coal")] = 10
	for range 5 { // the deposit's value settles
		f.Plan(prices, 1)
	}
	a, b := f.Methods[0].deposit, m.deposit
	if math.Abs(a+b-4) > 1e-9 || b <= a {
		t.Errorf("shares %v and %v of a deposit of 4: the more productive method should get more", a, b)
	}
	f.Labor = f.PlanLabor
	q := f.Produce()
	want := 0.0
	for i, x := range f.Methods {
		want += x.TFP * math.Pow(float64(f.MethodLevels(i)), 1-0.72) * math.Pow(x.planLabor, 0.6) * math.Pow(x.deposit, 0.12)
	}
	if math.Abs(q-want) > 1e-9*want {
		t.Errorf("made %v, want %v", q, want)
	}
}

// Land is a good: each tile's Landowner makes a fixed amount a day, and its
// farms, forests and houses rent it at the price that clears it. When twice
// as many people, with their money, want houses and food, land dears, and
// what is let is still the land there is.
func TestLandIsRented(t *testing.T) {
	w, err := BuildWorld(1, Graph{Places: []Place{{Name: "A", Terrain: "plains"}}})
	if err != nil {
		t.Fatal(err)
	}
	r, land := w.Regions[0], G("land")
	var houses, owner *Firm
	for _, f := range r.Firms {
		switch {
		case f.Output == G("housing"):
			houses = f
		case f.Fixed:
			owner = f
		}
	}
	used := func(days int) (price, housesLand, let float64) {
		for range days {
			w.Step()
			price += r.Markets[land].Price / float64(days)
			housesLand += houses.Used[land] / float64(days)
			for _, f := range r.Firms {
				if f.Used != nil {
					let += f.Used[land] / float64(days)
				}
			}
		}
		return
	}
	used(400)
	p0, h0, l0 := used(100)
	if owner.LastOutput <= 0 || p0 <= 0 || math.Abs(l0-owner.LastOutput) > 0.05*owner.LastOutput {
		t.Fatalf("the landowner made %v land, at £%v, and %v was let", owner.LastOutput, p0, l0)
	}
	for _, p := range r.Pops { // twice the people, with their money
		p.Size *= 2
		p.Cash *= 2
	}
	used(400)
	p1, h1, l1 := used(100)
	if p1 <= p0 || math.Abs(l1-owner.LastOutput) > 0.05*owner.LastOutput {
		t.Errorf("land at £%.2f -> £%.2f (the houses renting %.2f -> %.2f a day), %v let of %v", p0, p1, h0, h1, l1, owner.LastOutput)
	}
}
