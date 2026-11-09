package engine

import "testing"

// Bug: war materiel nobody buys in peacetime sank to the price floor, so when
// a nation raised an army its foundries took the best part of a year to start
// up again (the price had to climb back by 10% a day from almost nothing),
// and the army went without artillery.
func TestWarGoodsKeepAPriceInPeacetime(t *testing.T) {
	w := newQuietGame(t)
	art := G("artillery")
	for i := 0; i < 200; i++ { // peace: nobody buys artillery
		w.Step()
	}
	for _, r := range w.Regions {
		var sold []float64
		for _, m := range r.Markets {
			if m.Supply > 0 {
				sold = append(sold, m.Price)
			}
		}
		if p, low := r.Markets[art].Price, median(sold)/unsuppliedPriceCap; p < 0.999*low {
			t.Fatalf("tile %s: artillery price %v sank below a tenth of the typical price (%v)", r.Name, p, low)
		}
	}
	m := nationByName(w, "Montara")
	w.GrantTech(m.ID, "Drill") // Line infantry wants artillery
	a, err := w.RaiseArmy(m.ID, m.Regions[0].ID, 0.5*w.MobilizationCap(m))
	if err != nil {
		t.Fatal(err)
	}
	fill := 0.0
	for i := 0; i < 120; i++ {
		w.Step()
		if i >= 90 {
			fill += a.tierFill(0) / 30
		}
	}
	if fill < 0.8 {
		t.Errorf("four months after raising it, the army's kit (artillery included) is %.0f%% filled", 100*fill)
	}
}

// Bug: merchants bid for oil (and aircraft, vehicles...) long before anyone
// could make it, so tiles showed a market and a price for goods that didn't
// exist yet. Such goods stay out of the economy until a nation can make them.
func TestNoMarketForGoodsNobodyCanMake(t *testing.T) {
	w := newQuietGame(t)
	oil := G("oil")
	before := make([]float64, len(w.Regions))
	for i, r := range w.Regions {
		before[i] = r.Markets[oil].Price
	}
	for i := 0; i < 30; i++ {
		w.Step()
	}
	if w.Made(oil) {
		t.Fatal("oil counts as made before anyone knows Petroleum")
	}
	for i, r := range w.Regions {
		m := r.Markets[oil]
		if m.Demand > 0 || m.Supply > 0 {
			t.Errorf("tile %s: a market for oil (supply %v, demand %v), which nobody can make", r.Name, m.Supply, m.Demand)
		}
		if m.Price != before[i] {
			t.Errorf("tile %s: oil's price moved from %v to %v", r.Name, before[i], m.Price)
		}
	}
	// once a nation with an oil field learns to drill, there is a market
	var driller *Nation
	for _, r := range w.Regions {
		for _, f := range r.Firms {
			if f.Output == oil && r.Nation != nil {
				driller = r.Nation
			}
		}
	}
	if driller == nil {
		t.Skip("no nation has an oil field on this map")
	}
	w.GrantTech(driller.ID, "Petroleum")
	w.Step()
	if !w.Made(oil) {
		t.Error("oil still counts as unmade once a nation can drill for it")
	}
}
