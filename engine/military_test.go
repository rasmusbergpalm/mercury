package engine

import (
	"errors"
	"math"
	"testing"
)

// a game with only the military in motion: no migration or population growth
func newQuietGame(t *testing.T) *World {
	t.Helper()
	w := newTestGame(t)
	w.Migration, w.Demography = false, false
	return w
}

func TestRaiseArmyRecruitsFromItsTile(t *testing.T) {
	w := newQuietGame(t)
	m := nationByName(w, "Montara")
	home := regionByName(w, "11") // Montara's capital
	if _, err := w.RaiseArmy(m.ID, regionByName(w, "22").ID, 1); !errors.Is(err, ErrNotOwnTile) {
		t.Errorf("raising an army abroad: %v", err)
	}
	cap := w.MobilizationCap(m)
	if _, err := w.RaiseArmy(m.ID, home.ID, cap*1.5); !errors.Is(err, ErrMobilized) {
		t.Errorf("raising beyond the mobilization cap: %v", err)
	}
	pop := home.Population()
	a, err := w.RaiseArmy(m.ID, home.ID, cap)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 60; i++ {
		w.Step()
	}
	if a.Men <= 0 || a.Men > cap+1e-9 {
		t.Fatalf("army has %v men (cap %v)", a.Men, cap)
	}
	if drafted := pop - home.Population(); drafted < a.Men-1e-9 {
		t.Errorf("tile lost %v people but the army has %v", drafted, a.Men)
	}
	if err := w.SetArmyTarget(a.ID, cap*2); !errors.Is(err, ErrMobilized) {
		t.Errorf("target beyond the cap: %v", err)
	}
	w.GrantTech(m.ID, "Drill")
	w.GrantTech(m.ID, "Mass conscription")
	if w.MobilizationCap(m) <= cap*1.9 {
		t.Errorf("Mass conscription: cap %v, was %v", w.MobilizationCap(m), cap)
	}
	before := home.Population()
	men := a.Men
	if err := w.DisbandArmy(a.ID); err != nil {
		t.Fatal(err)
	}
	if math.Abs(home.Population()-(before+men)) > 1e-9 || len(w.ArmiesOf(m)) != 0 {
		t.Errorf("disbanding didn't return %v men home", men)
	}
}

func TestSupplyShortagesHurt(t *testing.T) {
	run := func(military float64) float64 {
		w := newQuietGame(t)
		m := nationByName(w, "Montara")
		m.SetBudget([NumBudgets]float64{0.4, 0.2, military})
		a, _ := w.RaiseArmy(m.ID, regionByName(w, "11").ID, w.MobilizationCap(m))
		for i := 0; i < 200; i++ {
			w.Step()
		}
		return a.Effectiveness()
	}
	funded, unfunded := run(0.3), run(0)
	if unfunded >= funded || unfunded > 0.5*Doctrines[0].Power {
		t.Errorf("effectiveness %v funded, %v unfunded", funded, unfunded)
	}
}

func TestSuppliesAbroadNeedTransport(t *testing.T) {
	w := newQuietGame(t)
	m := nationByName(w, "Montara")
	a, _ := w.RaiseArmy(m.ID, regionByName(w, "11").ID, 5)
	for i := 0; i < 30; i++ {
		w.Step()
	}
	if a.transit != 0 || a.depot.ID != a.Region {
		t.Errorf("at home: depot %v, transport %v", a.depot.ID, a.transit)
	}
	if err := w.MoveArmy(a.ID, regionByName(w, "22").ID); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 60 && len(a.Path) > 0; i++ {
		w.Step()
	}
	w.Step()
	if a.Region != regionByName(w, "22").ID || a.transit <= 0 || a.depot.Nation != m {
		t.Errorf("abroad: at %d, depot %v, transport %v", a.Region, a.depot, a.transit)
	}
}

func TestRailwaysSpeedMarches(t *testing.T) {
	days := func(infra int) int {
		w := newQuietGame(t)
		m := nationByName(w, "Montara")
		for _, name := range []string{"11", "21", "31"} { // inland, where ships don't help
			w.SetInfra(regionByName(w, name).ID, infra)
		}
		a, _ := w.RaiseArmy(m.ID, regionByName(w, "11").ID, 1)
		a.Men = 1
		if err := w.MoveArmy(a.ID, regionByName(w, "31").ID); err != nil {
			t.Fatal(err)
		}
		n := 0
		for ; len(a.Path) > 0 && n < 500; n++ {
			w.Step()
		}
		return n
	}
	if dirt, rail := days(0), days(3); rail >= dirt {
		t.Errorf("march took %d days by rail, %d without", rail, dirt)
	}
}

// Occupation is linear in time: it grows a fixed amount each day an
// unopposed army holds the tile, falls back the same amount otherwise, and
// the tile changes hands at 100%.
func TestOccupationIsLinear(t *testing.T) {
	w := newQuietGame(t)
	m := nationByName(w, "Montara")
	target := regionByName(w, "12") // unowned, next to Montara
	a, _ := w.RaiseArmy(m.ID, regionByName(w, "11").ID, 5)
	a.Men = 5
	a.Region = target.ID // place it directly
	for i := 0; i < 50; i++ {
		w.sieges()
	}
	if s := target.Siege; s == nil || math.Abs(s.Progress-50/occupationDays) > 1e-9 {
		t.Fatalf("after 50 days: %+v", target.Siege)
	}
	a.Region = regionByName(w, "11").ID // leave
	for i := 0; i < 20; i++ {
		w.sieges()
	}
	if s := target.Siege; s == nil || math.Abs(s.Progress-30/occupationDays) > 1e-9 {
		t.Fatalf("after leaving for 20 days: %+v", target.Siege)
	}
	for i := 0; i < 40; i++ {
		w.sieges()
	}
	if target.Siege != nil || target.Nation != nil {
		t.Fatalf("occupation should have faded away: %+v, owner %v", target.Siege, target.Nation)
	}
	a.Region = target.ID
	for i := 0; i < int(occupationDays)-1; i++ {
		w.sieges()
	}
	if target.Nation != nil {
		t.Fatal("tile changed hands early")
	}
	w.sieges()
	if target.Nation != m {
		t.Fatalf("tile not taken after %v days", occupationDays)
	}
}

func TestEnemyTilesNeedWarAndHoldOutLonger(t *testing.T) {
	w := newNeighbourGame(t)
	w.Migration, w.Demography = false, false
	a0, _ := w.RaiseArmy(0, regionByName(w, "00").ID, 0)
	a0.Men = 5
	enemy := regionByName(w, "01")
	if err := w.MoveArmy(a0.ID, enemy.ID); !errors.Is(err, ErrNoPath) {
		t.Fatalf("marching into a neighbour at peace: %v", err)
	}
	if err := w.DeclareWar(0, 1); err != nil {
		t.Fatal(err)
	}
	a0.Region = enemy.ID
	for i := 0; i < int(occupationDays); i++ {
		w.sieges()
	}
	if p := enemy.Siege.Progress; math.Abs(p-1/ownedTileFactor) > 1e-9 {
		t.Fatalf("an owned tile should take %v times as long: progress %v", ownedTileFactor, p)
	}
	// a defending army stops it gaining
	d, _ := w.RaiseArmy(1, enemy.ID, 1)
	d.Men = 1
	p := enemy.Siege.Progress
	w.sieges()
	if enemy.Siege.Progress >= p {
		t.Error("occupation advanced although the owner's army is there")
	}
}

func TestBattleAndRetreat(t *testing.T) {
	w := newNeighbourGame(t)
	w.Migration, w.Demography = false, false
	w.DeclareWar(0, 1)
	field := regionByName(w, "02")
	big, _ := w.RaiseArmy(0, regionByName(w, "00").ID, 0)
	small, _ := w.RaiseArmy(1, regionByName(w, "01").ID, 0)
	big.Men, small.Men = 50, 12
	big.Region, small.Region = field.ID, field.ID
	for i := 0; i < 100 && !small.Retreating; i++ {
		w.battles()
	}
	if !small.Retreating || big.Retreating {
		t.Fatalf("retreating: big %v small %v", big.Retreating, small.Retreating)
	}
	if big.Men >= 50 || small.Men >= 12 {
		t.Errorf("no casualties: %v, %v", big.Men, small.Men)
	}
	if len(small.Path) == 0 || small.Path[len(small.Path)-1] != regionByName(w, "01").ID {
		t.Errorf("retreat path %v", small.Path)
	}
}

func TestPeaceNeedsBothSides(t *testing.T) {
	w := newNeighbourGame(t)
	w.DeclareWar(0, 1)
	made, err := w.OfferPeace(0, 1)
	if err != nil || made {
		t.Fatalf("one-sided offer to a player: made %v, %v", made, err)
	}
	a, _ := w.RaiseArmy(0, regionByName(w, "00").ID, 0)
	a.Men, a.Region = 3, regionByName(w, "01").ID
	made, err = w.OfferPeace(1, 0) // both have offered
	if err != nil || !made || w.Nations[0].AtWar[1] {
		t.Fatalf("mutual offers: made %v, %v", made, err)
	}
	if len(a.Path) == 0 {
		t.Error("army in enemy land not sent home at peace")
	}
	if _, err := w.OfferPeace(0, 1); !errors.Is(err, ErrAtPeace) {
		t.Errorf("peace offer at peace: %v", err)
	}
	// a computer opponent accepts after peaceYears
	w.DeclareWar(0, 1)
	w.Nations[1].AI = true
	w.Tick += int(peaceYears * TicksPerYear)
	if made, _ := w.OfferPeace(0, 1); !made {
		t.Error("computer nation refused peace after years of war")
	}
}

func TestUpgradeArmy(t *testing.T) {
	w := newQuietGame(t)
	m := nationByName(w, "Montara")
	a, _ := w.RaiseArmy(m.ID, regionByName(w, "11").ID, 5)
	if err := w.UpgradeArmy(a.ID); !errors.Is(err, ErrNoUpgrade) {
		t.Errorf("upgrade without a better doctrine: %v", err)
	}
	w.GrantTech(m.ID, "Drill")
	if err := w.UpgradeArmy(a.ID); err != nil {
		t.Fatal(err)
	}
	if a.Doctrine != 1 || a.Supply[G("artillery")] != 0.2 || a.Supply[G("bread")] != 1 {
		t.Errorf("after upgrade: doctrine %d, artillery %v, bread %v", a.Doctrine, a.Supply[G("artillery")], a.Supply[G("bread")])
	}
	if a.Effectiveness() >= Doctrines[1].Power {
		t.Error("new kit should take time to arrive")
	}
}

func TestConquestEliminatesANation(t *testing.T) {
	w := newNeighbourGame(t)
	w.GrantTech(0, "Wagons")
	w.DeclareWar(0, 1)
	beta := regionByName(w, "01")
	d, _ := w.RaiseArmy(1, beta.ID, 0)
	a, _ := w.RaiseArmy(0, regionByName(w, "00").ID, 0)
	a.Men, a.Region = 50, beta.ID
	money := w.Money()
	for i := 0; i < 2*int(occupationDays*ownedTileFactor) && !w.Nations[1].Eliminated(); i++ {
		w.Step()
	}
	if !w.Nations[1].Eliminated() || beta.Nation != w.Nations[0] {
		t.Fatalf("Beta not eliminated: owner of 01 is %v", beta.Nation)
	}
	if w.ArmyByID(d.ID) != nil || w.Nations[0].AtWar[1] {
		t.Error("fallen nation's army or war survived")
	}
	for _, f := range beta.Firms {
		if f.Name == "Transport" && math.Abs(f.TFP/f.BaseTFP-1.3) > 1e-12 {
			t.Errorf("conquered tile lacks the conqueror's tech: %v", f.TFP/f.BaseTFP)
		}
	}
	for i := 0; i < 30; i++ {
		w.Step()
	}
	if relDiff(w.Money(), money) > 1e-10 {
		t.Error("money not conserved through a conquest")
	}
}
