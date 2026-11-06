package engine

import (
	"math"
	"testing"
)

func TestHistoryIsRecordedMonthly(t *testing.T) {
	w := newTestGame(t)
	for i := 0; i < 365; i++ {
		w.Step()
	}
	h := w.History
	if len(h.Ticks) != 365/HistoryEvery || h.Ticks[0] != HistoryEvery {
		t.Fatalf("ticks %v", h.Ticks)
	}
	n := len(h.Ticks)
	for i, nh := range h.Nations {
		if len(nh.Population) != n || len(nh.GDP) != n || len(nh.Labor[Primary]) != n || len(nh.ExportsGood[0]) != n {
			t.Fatalf("nation %d series lengths wrong", i)
		}
		if nh.GDP[n-1] <= 0 || nh.Population[n-1] <= 0 {
			t.Errorf("nation %d: gdp %v pop %v", i, nh.GDP[n-1], nh.Population[n-1])
		}
	}
	rh := h.Regions[0]
	if len(rh.Prices) != len(ConsumerGoods) || len(rh.Prices[0]) != n || len(rh.Wage) != n {
		t.Fatal("region series lengths wrong")
	}
	if w := newTestWorld(t, 1); func() bool { w.Step(); return len(w.History.Ticks) > 0 }() {
		t.Error("the classic world records history")
	}
}

func TestArmyHistoryAndBreakdown(t *testing.T) {
	w := newQuietGame(t)
	for i := 0; i < 40; i++ {
		w.Step()
	}
	m := nationByName(w, "Montara")
	w.GrantTech(m.ID, "Drill")
	w.GrantTech(m.ID, "Field artillery")
	a, _ := w.RaiseArmy(m.ID, regionByName(w, "11").ID, 5)
	for i := 0; i < 100; i++ {
		w.Step()
	}
	var ah *ArmyHistory
	for _, x := range w.History.Armies {
		if x.ID == a.ID {
			ah = x
		}
	}
	if ah == nil || len(ah.Men) != len(w.History.Ticks)-ah.From || ah.From == 0 {
		t.Fatalf("army history %+v", ah)
	}
	attack, defense, attrition := w.Breakdown(a)
	prod := 1.0
	for _, m := range attack {
		prod *= m.Value
	}
	if want := a.Effectiveness() * m.attackMult(); math.Abs(prod-want) > 1e-9 {
		t.Errorf("breakdown multiplies to %v, strength per man is %v", prod, want)
	}
	if len(defense) == 0 || len(attrition) < 2 {
		t.Error("missing defence or attrition breakdown")
	}
}

func TestHistorySurvivesSaveLoad(t *testing.T) {
	w := newTestGame(t)
	for i := 0; i < 100; i++ {
		w.Step()
	}
	r := roundTrip(t, w)
	for i := 0; i < 60; i++ {
		w.Step()
		r.Step()
	}
	if len(r.History.Ticks) != len(w.History.Ticks) || r.History.Nations[0].GDP[len(r.History.Ticks)-1] != w.History.Nations[0].GDP[len(w.History.Ticks)-1] {
		t.Error("history diverged after load")
	}
}
