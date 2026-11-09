package engine

import (
	"math"
	"testing"
)

func TestFundByMargin(t *testing.T) {
	origin := NewRegion(0, "x", nil, 1, 1, 1)
	for _, m := range origin.Markets {
		m.Supply = 1e9
	}
	routes := func(margins ...float64) []*Trader {
		var out []*Trader
		for _, m := range margins {
			out = append(out, &Trader{Origin: origin, Active: true, Q: 10, unit: 2, RelMargin: m})
		}
		return out
	}
	spent := func(rs []*Trader) float64 {
		s := 0.0
		for _, r := range rs {
			s += r.Q * r.unit
		}
		return s
	}
	// enough money: every order stands
	rs := routes(0.3, 0.1)
	fundByMargin(rs, 100)
	if rs[0].Q != 10 || rs[1].Q != 10 {
		t.Errorf("orders cut with money to spare: %v %v", rs[0].Q, rs[1].Q)
	}
	// short of money: the budget is spent exactly, better margins get a
	// bigger share, a route that doesn't pay today the least, but some
	rs = routes(0.4, 0.2, 0.1, -0.05)
	fundByMargin(rs, 30)
	if math.Abs(spent(rs)-30) > 1e-9 {
		t.Errorf("spent %v of 30", spent(rs))
	}
	for i := 1; i < 4; i++ {
		if rs[i].Q > rs[i-1].Q || rs[i].Q <= 0 {
			t.Errorf("shares %v %v %v %v", rs[0].Q, rs[1].Q, rs[2].Q, rs[3].Q)
		}
	}
	// a little short: everyone keeps most of their order
	rs = routes(0.4, 0.2, 0.1, -0.05)
	fundByMargin(rs, 76)
	for _, r := range rs {
		if r.Q < 7.5 {
			t.Errorf("a 5%% shortfall cut an order to %v of 10", r.Q)
		}
	}
	// a big margin is funded in full, the rest share what's left
	rs = routes(5, 0.1, 0.1)
	fundByMargin(rs, 30)
	if rs[0].Q != 10 || math.Abs(spent(rs)-30) > 1e-9 || math.Abs(rs[1].Q-rs[2].Q) > 1e-12 {
		t.Errorf("funded %v %v %v", rs[0].Q, rs[1].Q, rs[2].Q)
	}
	// margins passing each other move the funding a little, not all of it
	a, b := routes(0.2, 0.2001), routes(0.2001, 0.2)
	fundByMargin(a, 20)
	fundByMargin(b, 20)
	if math.Abs(a[0].Q-b[1].Q) > 1e-9 || math.Abs(a[0].Q-a[1].Q) > 0.01 {
		t.Errorf("two near-equal routes: %v %v and %v %v", a[0].Q, a[1].Q, b[0].Q, b[1].Q)
	}
}
