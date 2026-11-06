package engine

import (
	"math"
	"testing"
)

func newMarket() *Market { return &Market{Price: 2, Speed: 0.1, MinVolume: 0.1} }

func TestClearRationsLongSide(t *testing.T) {
	m := newMarket()
	b1, b2, s1 := 100.0, 100.0, 0.0
	bf, af := m.Clear([]Order{{&b1, 3}, {&b2, 1}}, []Order{{&s1, 2}})
	if math.Abs(bf[0]-1.5) > 1e-12 || math.Abs(bf[1]-0.5) > 1e-12 || af[0] != 2 {
		t.Errorf("excess demand: fills %v %v, want proportional rationing of buyers", bf, af)
	}
	if b1+b2+s1 != 200 || s1 != 4 {
		t.Errorf("cash not conserved or wrong: %v %v %v", b1, b2, s1)
	}
	if m.Price <= 2 {
		t.Errorf("price should rise on excess demand, got %v", m.Price)
	}

	m = newMarket()
	bf, af = m.Clear([]Order{{&b1, 1}}, []Order{{&s1, 3}, {&b2, 1}})
	if bf[0] != 1 || math.Abs(af[0]-0.75) > 1e-12 || math.Abs(af[1]-0.25) > 1e-12 {
		t.Errorf("excess supply: fills %v %v, want proportional rationing of sellers", bf, af)
	}
	if m.Price >= 2 {
		t.Errorf("price should fall on excess supply, got %v", m.Price)
	}
}

func TestClearWithoutTradeKeepsPrice(t *testing.T) {
	m := newMarket()
	m.Clear(nil, nil)
	if m.Price != 2 {
		t.Errorf("no orders: price moved to %v", m.Price)
	}
	// a trickle of demand nobody can meet moves the price slowly, not at full speed
	c := 1.0
	m.Clear([]Order{{&c, 0.001}}, nil)
	if r := m.Price / 2; r <= 1 || r > math.Exp(0.1*0.011) {
		t.Errorf("trickle of demand: price ratio %v", r)
	}
}
