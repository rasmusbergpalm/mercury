package engine

import (
	"errors"
	"math"
	"testing"
)

// A nation half again as mighty as every other gets an alliance against it;
// allies can't fight each other, join each other's wars against it, and the
// alliance breaks up when the leader's edge shrinks below allianceBreak.
func TestAlliancesAgainstADominantNation(t *testing.T) {
	w := newQuietGame(t)
	m, v, th := nationByName(w, "Montara"), nationByName(w, "Verdania"), nationByName(w, "Thalassa")
	army := func(n *Nation, men float64) *Army {
		a, err := w.RaiseArmy(n.ID, n.Regions[0].ID, 0)
		if err != nil {
			t.Fatal(err)
		}
		a.Target, a.Men = men, men
		for g := range a.Supply {
			a.Supply[g] = 1
		}
		return a
	}
	big := army(th, 30)
	army(m, 10)
	army(v, 10)
	settle := func() { // might is judged over a year: start the averages where they are
		for _, n := range w.Nations {
			n.MightEMA = w.ArmyStrength(n)
		}
	}
	settle()
	w.updateAlliances()
	if !w.Allied(m, v) || w.AlliedAgainst(m) != th || w.Allied(m, th) {
		t.Fatalf("no alliance of Montara and Verdania against Thalassa: %d %d %d", m.Coalition, v.Coalition, th.Coalition)
	}
	if err := w.DeclareWar(m.ID, v.ID); !errors.Is(err, ErrAllied) && !errors.Is(err, ErrNotBordered) {
		t.Errorf("war between allies: %v", err)
	}
	// Thalassa attacks Verdania: Montara joins, border or not
	v.AtWar[th.ID], th.AtWar[v.ID] = true, true
	w.updateAlliances()
	if !m.AtWar[th.ID] || !th.AtWar[m.ID] {
		t.Error("Montara didn't join its ally's war on Thalassa")
	}
	// Thalassa's edge shrinks below allianceBreak: the alliance holds for
	// its minimum term, then breaks up
	big.Men = 11
	settle()
	w.updateAlliances()
	if m.Coalition == 0 {
		t.Error("alliance broke up within its first years")
	}
	w.Tick += allianceYears * TicksPerYear
	settle()
	w.updateAlliances()
	if m.Coalition != 0 || v.Coalition != 0 {
		t.Errorf("alliance outlived its reason: %d %d", m.Coalition, v.Coalition)
	}
}

// A tile's allegiance: a conquered tile starts at nothing and free land at
// freeLandAllegiance; both grow towards loyalty, and until then the tile
// produces less, pays less tax and gives fewer recruits.
func TestAllegianceGrowsAfterConquest(t *testing.T) {
	w := newQuietGame(t)
	m, v := nationByName(w, "Montara"), nationByName(w, "Verdania")
	home := m.Regions[0]
	if home.Allegiance() != 1 {
		t.Fatalf("a starting tile's allegiance %v", home.Allegiance())
	}
	capBefore := w.MobilizationCap(v)
	w.transfer(home, v)
	if home.Allegiance() != 0 || math.Abs(home.Firms[0].Loyal-0.5) > 1e-12 {
		t.Errorf("conquered: allegiance %v, firms at %v", home.Allegiance(), home.Firms[0].Loyal)
	}
	if got := w.MobilizationCap(v); math.Abs(got-capBefore) > 1e-9 {
		t.Errorf("conquered people count for the cap: %v -> %v", capBefore, got)
	}
	var free *Region
	for _, r := range w.Regions {
		if r.Nation == nil {
			free = r
			break
		}
	}
	w.transfer(free, m)
	if math.Abs(free.Allegiance()-freeLandAllegiance) > 1e-12 {
		t.Errorf("settled free land: allegiance %v, want %v", free.Allegiance(), freeLandAllegiance)
	}
	for i := 0; i < int(allegianceYears*TicksPerYear); i++ {
		w.serviceEffects()
	}
	if a := home.Allegiance(); math.Abs(a-(1-math.Exp(-1))) > 0.01 {
		t.Errorf("after %v years: allegiance %v, want about %v", allegianceYears, a, 1-math.Exp(-1))
	}
}
