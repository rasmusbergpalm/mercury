package economy

import (
	"math"
	"testing"
)

// A month with twice the bread at a lower price: real GDP doubles, whatever
// the pounds. Prices moving with quantities unchanged leave it
// alone, and a good that starts trading at an absurd price doesn't distort it.
func TestRealGDPCountsGoodsNotPounds(t *testing.T) {
	bread, coal := ConsumerGoods[0], GoodID(goodIndex["coal"])
	r := NewRegion(0, "x", nil, 1, 10, 10)
	month := func(qty, price, coalQty, coalPrice float64) {
		c := &r.Real
		c.AccNet, c.AccValue, c.AccVolume = make([]float64, NumGoods+1), make([]float64, NumGoods+1), make([]float64, NumGoods+1)
		c.AccNet[bread], c.AccValue[bread], c.AccVolume[bread] = 30*qty, 30*qty*price, 30*qty
		if coalQty > 0 {
			c.AccNet[coal], c.AccValue[coal], c.AccVolume[coal] = 30*coalQty, 30*coalQty*coalPrice, 30*coalQty
		}
		c.Ticks = 30
		r.warm = true
		r.linkGDP()
	}
	month(10, 2, 0, 0)
	if math.Abs(r.Real.Level-20) > 1e-9 {
		t.Fatalf("base %v, want 20 (this month's pounds)", r.Real.Level)
	}
	month(20, 1.5, 0, 0)
	if math.Abs(r.Real.Level-40) > 1e-9 {
		t.Errorf("twice the bread, cheaper: real GDP %v, want 40", r.Real.Level)
	}
	month(20, 2.5, 0, 0)
	if math.Abs(r.Real.Level-40) > 1e-9 {
		t.Errorf("dearer bread, same amount: real GDP %v, want 40", r.Real.Level)
	}
	// coal appears: it doesn't count until its second month
	month(20, 2.5, 1, 100)
	if math.Abs(r.Real.Level-40) > 1e-9 {
		t.Errorf("new coal: real GDP %v, want 40", r.Real.Level)
	}
	// then twice the coal at the same price: bread £50 a day, coal £100 then
	// £200, so up by 5/3
	month(20, 2.5, 2, 100)
	if math.Abs(r.Real.Level-40*5.0/3) > 1e-6 {
		t.Errorf("coal doubles: real GDP %v, want %v", r.Real.Level, 40*5.0/3)
	}
	// coal's price falls tenfold as it doubles: not settled, so it counts
	// for nothing this month
	before := r.Real.Level
	month(20, 2.5, 4, 10)
	if math.Abs(r.Real.Level-before) > 1e-6 {
		t.Errorf("coal doubles while its price falls tenfold: real GDP %v, want %v", r.Real.Level, before)
	}
}
