package game

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
	// Thalassa can call up twice the share of its people, at a better
	// doctrine: its potential might dwarfs the others'
	strong := []string{"Percussion caps", "Mass conscription", "Universal conscription"}
	for _, tech := range strong {
		w.GrantTech(th.ID, tech)
	}
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
	// the others catch up: the alliance holds for its minimum term, then
	// breaks up
	for _, n := range []*Nation{m, v} {
		for _, tech := range strong {
			w.GrantTech(n.ID, tech)
		}
	}
	w.updateAlliances()
	if m.Coalition == 0 {
		t.Error("alliance broke up within its first years")
	}
	w.Tick += allianceYears * TicksPerYear
	w.updateAlliances()
	if m.Coalition != 0 || v.Coalition != 0 {
		t.Errorf("alliance outlived its reason: %d %d", m.Coalition, v.Coalition)
	}
}

// Regression: nations ally against the one that could raise the most
// (potential might), not the one that happens to keep the most under arms. Thalassa, the smallest
// nation, kept the most men under arms, and the other two allied against
// it at once.
func TestNoAllianceAgainstTheArmedButWeak(t *testing.T) {
	w := newQuietGame(t)
	m, v, th := nationByName(w, "Montara"), nationByName(w, "Verdania"), nationByName(w, "Thalassa")
	raise := func(n *Nation, men float64) {
		a, err := w.RaiseArmy(n.ID, n.Regions[0].ID, 0)
		if err != nil {
			t.Fatal(err)
		}
		a.Target, a.Men = men, men
	}
	raise(m, 5)
	raise(v, 5)
	raise(th, 30) // under arms, far beyond anyone else
	if w.ArmyStrength(th) <= 2*math.Max(w.ArmyStrength(m), w.ArmyStrength(v)) {
		t.Fatal("set-up: Thalassa isn't the most armed")
	}
	if w.ArmyPotential(th) > 1.5*math.Max(w.ArmyPotential(m), w.ArmyPotential(v)) {
		t.Fatal("set-up: Thalassa's potential dominates")
	}
	w.updateAlliances()
	if w.AlliedAgainst(m) == th || w.AlliedAgainst(v) == th {
		t.Error("allied against Thalassa for the men it keeps under arms")
	}
}

// A tile's allegiance: a conquered tile starts at nothing and free land at
// freeLandAllegiance; both grow towards loyalty, and until then the tile
// pays less tax and gives fewer recruits. A conquered tile also produces
// less (its people stand by another nation); free land taken over doesn't.
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
	if math.Abs(free.Allegiance()-freeLandAllegiance) > 1e-12 || free.Firms[0].Loyal != 1 {
		t.Errorf("settled free land: allegiance %v (want %v), firms at %v (want 1)", free.Allegiance(), freeLandAllegiance, free.Firms[0].Loyal)
	}
	for i := 0; i < int(allegianceYears*TicksPerYear); i++ {
		w.serviceEffects()
	}
	if a := home.Allegiance(); math.Abs(a-(1-math.Exp(-1))) > 0.01 {
		t.Errorf("after %v years: allegiance %v, want about %v", allegianceYears, a, 1-math.Exp(-1))
	}
}

// Loyalties are kept per nation: a tile lost and soon retaken still mostly
// stands by the nation that retook it.
func TestRetakenTilesStayLoyal(t *testing.T) {
	w := newQuietGame(t)
	m, v := nationByName(w, "Montara"), nationByName(w, "Verdania")
	home := m.Regions[0]
	w.transfer(home, v)
	for i := 0; i < int(TicksPerYear); i++ { // a year under Verdania
		w.serviceEffects()
	}
	w.transfer(home, m)
	if a, want := home.Allegiance(), math.Exp(-1/allegianceYears); math.Abs(a-want) > 0.01 {
		t.Errorf("retaken after a year: allegiance %v, want about %v", a, want)
	}
	sum := 0.0
	for _, x := range home.Loyalties {
		sum += x
	}
	if math.Abs(sum-1) > 1e-9 {
		t.Errorf("loyalties add up to %v", sum)
	}
}

// Regression: allies at war with the same enemy don't both go for the same
// enemy tile (only one can occupy it): the second leaves it to the first.
func TestAlliesDontTakeTheSameTile(t *testing.T) {
	w := newQuietGame(t)
	m, v, th := nationByName(w, "Montara"), nationByName(w, "Verdania"), nationByName(w, "Thalassa")
	m.Coalition, v.Coalition = th.ID+1, th.ID+1
	for _, n := range []*Nation{m, v} {
		n.AtWar[th.ID], th.AtWar[n.ID] = true, true
	}
	// a tile of Thalassa's next to both allies
	target := regionByName(w, "12")
	w.Annex(target.ID, th.ID)
	if !w.borders(m, target) || !w.borders(v, target) {
		t.Fatal("set-up: tile 12 should border Montara and Verdania")
	}
	army := func(n *Nation) *Army {
		x, err := w.RaiseArmy(n.ID, n.Regions[0].ID, 0)
		if err != nil {
			t.Fatal(err)
		}
		x.Target, x.Men = 20, 20
		for g := range x.Supply {
			x.Supply[g] = 1
		}
		return x
	}
	a, b := army(m), army(v)
	if err := w.MoveArmy(a.ID, target.ID); err != nil { // Montara's army marches on it
		t.Fatal(err)
	}
	if dest := w.aiDestination(v, b, w.expansionTargets(v)); dest == target.ID {
		t.Errorf("Verdania's army heads for the enemy tile its ally is already marching on")
	}
}

// An alliance embargoes the nation it stands against, both ways, for as long
// as it lasts.
func TestAlliesEmbargoTheirFoe(t *testing.T) {
	w := newQuietGame(t)
	m, th := nationByName(w, "Montara"), nationByName(w, "Thalassa")
	route := func(from, to *Nation) *Trader {
		for _, tr := range w.Traders {
			if w.Regions[tr.Origin.ID].Nation == from && w.Regions[tr.Dest.ID].Nation == to {
				return tr
			}
		}
		return nil
	}
	w.Annex(regionByName(w, "12").ID, th.ID) // a Thalassan tile next to Montara
	there, back := route(m, th), route(th, m)
	if there == nil || back == nil {
		t.Fatal("set-up: no trade route between Montara and Thalassa")
	}
	if _, blocked := there.Border(); blocked {
		t.Fatal("trade blocked before any alliance")
	}
	m.Coalition = th.ID + 1
	for _, tr := range []*Trader{there, back} {
		if _, blocked := tr.Border(); !blocked {
			t.Errorf("%s: trade with the nation Montara is allied against", tr.Name())
		}
	}
	m.Coalition = 0
	if _, blocked := there.Border(); blocked {
		t.Error("trade still blocked after the alliance broke up")
	}
}

// Allies make peace together: peace between one ally and the nation they
// are allied against is peace for every ally at war with it.
func TestAlliesMakePeaceTogether(t *testing.T) {
	w := newQuietGame(t)
	m, v, th := nationByName(w, "Montara"), nationByName(w, "Verdania"), nationByName(w, "Thalassa")
	m.Coalition, v.Coalition = th.ID+1, th.ID+1
	for _, n := range []*Nation{m, v} {
		n.AtWar[th.ID], th.AtWar[n.ID] = true, true
	}
	if err := w.makePeace(m.ID, th.ID); err != nil {
		t.Fatal(err)
	}
	if v.AtWar[th.ID] || th.AtWar[v.ID] {
		t.Error("Montara made a separate peace: Verdania is still at war with Thalassa")
	}
	// and the other way round: Thalassa offering to Verdania ends both wars
	for _, n := range []*Nation{m, v} {
		n.AtWar[th.ID], th.AtWar[n.ID] = true, true
	}
	if err := w.makePeace(th.ID, v.ID); err != nil {
		t.Fatal(err)
	}
	if m.AtWar[th.ID] {
		t.Error("Thalassa made a separate peace with Verdania: Montara is still at war")
	}
}

// Regression: when the nation an alliance stands against declares war on one
// ally, the others join at once - even if they all made peace with it
// recently (allies make peace together, so no truce holds them back).
func TestAlliesJoinAWarOnTheirPartner(t *testing.T) {
	w := newQuietGame(t)
	m, v, th := nationByName(w, "Montara"), nationByName(w, "Verdania"), nationByName(w, "Thalassa")
	m.Coalition, v.Coalition = th.ID+1, th.ID+1
	w.Annex(regionByName(w, "12").ID, th.ID) // Thalassa borders Montara
	// a war and a peace a year ago
	for _, n := range []*Nation{m, v} {
		n.AtWar[th.ID], th.AtWar[n.ID] = true, true
	}
	w.Tick = 400
	w.makePeace(m.ID, th.ID)
	w.Tick += 365
	if err := w.DeclareWar(th.ID, m.ID); err != nil {
		t.Fatal(err)
	}
	if !v.AtWar[th.ID] || !th.AtWar[v.ID] {
		t.Error("Verdania didn't join the war Thalassa declared on its ally")
	}
}
