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
		low := median(sold) / unsuppliedPriceCap
		for _, g := range []GoodID{art, G("arms")} { // (arms: merchants' bids are a trickle)
			if p := r.Markets[g].Price; p < 0.999*low {
				t.Fatalf("tile %s: %s price %v sank below a tenth of the typical price (%v)", r.Name, Goods[g].Name, p, low)
			}
		}
	}
	m := nationByName(w, "Montara")
	w.GrantTech(m.ID, "Percussion caps") // the Napoleonic corps wants artillery
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

// The nation's price index weighs each tile's price by what changed hands
// there, and is kept in the nation's history.
func TestNationPriceIndex(t *testing.T) {
	w := newQuietGame(t)
	for i := 0; i < 2*HistoryEvery; i++ {
		w.Step()
	}
	n := nationByName(w, "Montara")
	prices, traded := w.NationPrices(n)
	bread := G("bread")
	sum, vol := 0.0, 0.0
	for _, r := range n.Regions {
		m := r.Markets[bread]
		v := min(m.Supply, m.Demand)
		sum += w.TradePrices[r.ID][bread] * v
		vol += v
	}
	if vol <= 0 || relDiff(prices[bread], sum/vol) > 1e-9 || relDiff(traded[bread], vol) > 1e-9 {
		t.Errorf("bread: index %v on %v traded, want %v on %v", prices[bread], traded[bread], sum/vol, vol)
	}
	if oil := G("oil"); prices[oil] != 0 || traded[oil] != 0 {
		t.Errorf("oil, which nobody trades, has price %v and volume %v", prices[oil], traded[oil])
	}
	h := w.History.Nations[n.ID]
	if len(h.Prices) != NumGoods || len(h.Prices[bread]) != len(w.History.Ticks) || h.Prices[bread][len(w.History.Ticks)-1] <= 0 {
		t.Errorf("bread's price isn't in the nation's history: %v samples for %v ticks", len(h.Prices[bread]), len(w.History.Ticks))
	}
}

// The armies keep ordering in full while a good's price climbs (up to ten
// times its usual level): before, they cut their orders once it doubled,
// which slowed the very rise that calls up supply.
func TestArmiesKeepOrderingAsPricesRise(t *testing.T) {
	w := newQuietGame(t)
	m := nationByName(w, "Montara")
	w.GrantTech(m.ID, "Percussion caps")
	if _, err := w.RaiseArmy(m.ID, m.Regions[0].ID, 0.5*w.MobilizationCap(m)); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 60; i++ {
		w.Step()
	}
	art := G("artillery")
	ordered := func(mult float64) float64 {
		for _, r := range w.Regions { // every tile alike, so none looks dear next to the rest
			mk := r.Markets[art]
			if mk.Trend <= 0 {
				mk.Trend = mk.Price
			}
			mk.Price = mult * mk.Trend
			w.TradePrices[r.ID][art] = mk.Price
		}
		m.plan(w)
		q := 0.0
		for _, r := range m.Regions {
			for _, c := range r.claims {
				if c.kind == armyClaim && c.good == art {
					q += c.qty
				}
			}
		}
		return q
	}
	usual := ordered(1)
	if usual <= 0 {
		t.Fatal("the army orders no artillery")
	}
	if dear := ordered(5); dear < 0.99*usual {
		t.Errorf("at 5× its usual price the army orders %v artillery a day, at the usual price %v", dear, usual)
	}
	if dearer := ordered(20); dearer > 0.6*usual {
		t.Errorf("at 20× its usual price the army still orders %v artillery a day (usual %v)", dearer, usual)
	}
}
