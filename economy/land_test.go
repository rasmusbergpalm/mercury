package economy

import (
	"math"
	"testing"
)

// landTile is a lone plains tile, its markets settled.
func landTile(t *testing.T) (*World, *Region, *Firm) {
	t.Helper()
	w, err := BuildWorld(1, Graph{Places: []Place{{Name: "A", Terrain: "plains"}}})
	if err != nil {
		t.Fatal(err)
	}
	for range 2 * 365 {
		w.Step()
	}
	var owner *Firm
	for _, f := range w.Regions[0].Firms {
		if f.Fixed {
			owner = f
		}
	}
	return w, w.Regions[0], owner
}

// held is how much land the tile's firms hold.
func held(r *Region) float64 {
	h := 0.0
	for _, f := range r.Firms {
		if f.Held != nil {
			h += f.Held[G("land")]
		}
	}
	return h
}

// The land a tile's firms hold is never more than the tile has, even in a
// boom. Bidding for slow-moving holdings at a fast price once had them
// holding half again as much land as there was while its price swung.
func TestLandHeldIsTheLandThereIs(t *testing.T) {
	w, r, owner := landTile(t)
	for _, p := range r.Pops { // a boom: twice the people, with their money
		p.Size *= 2
		p.Cash *= 2
	}
	land := r.Markets[G("land")]
	for d := range 2 * 365 {
		before := land.Price
		w.Step()
		if h := held(r); h > 1.01*owner.LastOutput {
			t.Fatalf("day %d of the boom: the tile's firms hold %.1f land of %.1f", d, h, owner.LastOutput)
		}
		if math.Abs(math.Log(land.Price/before)) > 0.11 {
			t.Fatalf("day %d: land's price jumped %v -> %v", d, before, land.Price)
		}
	}
}

// A refit takes no new land at once: a farm holds its land, and its methods
// share it. When each method held its own, a level refitted to a new
// method took all the land that method wanted the same day, and the price
// of land doubled for years whenever a new farming method came.
func TestRefitTakesNoLandAtOnce(t *testing.T) {
	f := NewFirm("Farm", G("grain"), 2, []Factor{Labor(0.6), Input("land", 0.25)}, 1e6)
	m := newMethod("Better farm", 4, []Factor{Labor(0.6), Input("land", 0.25)})
	f.Methods = append(f.Methods, m)
	f.Levels = 4
	prices := make([]float64, NumGoods)
	prices[G("grain")], prices[G("land")] = 10, 3
	f.Plan(prices, 1)
	before := f.Held[G("land")]
	f.refit(0, 1) // a level of the new method
	f.Plan(prices, 1)
	if h := f.Held[G("land")]; h != before {
		t.Errorf("a refit moved the farm's land %v -> %v at once", before, h)
	}
	share := 0.0
	for _, x := range f.Methods {
		if x.holding != nil {
			share += x.holding[1]
		}
	}
	if math.Abs(share-before) > 1e-9*before || m.holding[1] <= 0 {
		t.Errorf("the methods work %v of the farm's %v land (the new one %v)", share, before, m.holding[1])
	}
}

// Only the land itself is full: a firm that rents land, or works a
// deposit, may build as long as a level pays, its land making each add
// less. Limits on levels once stopped a growing town's houses at ten
// times their land while they still paid twice what their owners asked.
func TestOnlyLandIsFull(t *testing.T) {
	_, r, owner := landTile(t)
	for _, f := range r.Firms {
		if f == owner {
			continue
		}
		f.Levels = 1000
		if f.AtCap() {
			t.Errorf("%s is full at %d levels", f.Name, f.Levels)
		}
	}
	if !owner.AtCap() {
		t.Error("the tile's land could be built")
	}
}

// A tenant pays for the land it holds and works, not for what it rented
// that day: while land changes hands, a farm that is giving land up works
// acres it would otherwise not pay for, and a growing user pays for acres it
// has yet to take up. Over a day of a sudden boom in demand, what the
// land's owner earns is the land's price times the land its tenants held.
func TestTenantsPayForWhatTheyHold(t *testing.T) {
	w, r, owner := landTile(t)
	for _, p := range r.Pops {
		p.Size *= 2
		p.Cash *= 2
	}
	land := G("land")
	for range 30 {
		w.Step()
	}
	h := held(r)
	w.Step()
	price := r.prices[land]
	if got, want := owner.TickRevenue, price*h; math.Abs(got-want) > 1e-6*want {
		t.Errorf("the owner earned £%.4f for the day, the land held is worth £%.4f", got, want)
	}
	if rented := r.Markets[land].Demand; math.Abs(h-rented) < 0.01 {
		t.Errorf("held %.3f, rented %.3f: the test needs land to be changing hands", h, rented)
	}
}
