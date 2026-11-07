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

func TestRaiseArmyRecruitsFromTheNation(t *testing.T) {
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
	people := func() float64 {
		s := 0.0
		for _, r := range m.Regions {
			s += r.Population()
		}
		return s
	}
	pop := people()
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
	if drafted := pop - people(); drafted < a.Men-1e-9 { // some have since deserted or died
		t.Errorf("the nation lost %v people but the army has %v", drafted, a.Men)
	}
	for _, r := range m.Regions { // every tile gave its share
		if r != home && r.Population() >= 100 {
			t.Errorf("tile %s gave no recruits", r.Name)
		}
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
	run := func(broke bool) float64 {
		w := newQuietGame(t)
		m := nationByName(w, "Montara")
		a, _ := w.RaiseArmy(m.ID, regionByName(w, "11").ID, w.MobilizationCap(m))
		for i := 0; i < 200; i++ {
			if broke {
				m.Treasury = 0 // nothing to pay with
			}
			w.Step()
		}
		return a.Effectiveness()
	}
	funded, unfunded := run(false), run(true)
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
	home := a.transit / a.Men // goods may come from other home tiles too
	if a.depot.ID != a.Region {
		t.Errorf("at home: base %v", a.depot.ID)
	}
	if err := w.MoveArmy(a.ID, regionByName(w, "22").ID); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 60 && len(a.Path) > 0; i++ {
		w.Step()
	}
	w.Step()
	if a.Region != regionByName(w, "22").ID || a.transit/a.Men <= home || a.depot.Nation != m {
		t.Errorf("abroad: at %d, base %v, transport per man %v (at home %v)", a.Region, a.depot, a.transit/a.Men, home)
	}
}

func TestRailwaysSpeedMarches(t *testing.T) {
	days := func(infra int) int {
		w := newQuietGame(t)
		m := nationByName(w, "Montara")
		for _, name := range []string{"11", "21", "31"} { // inland, where ships don't help
			w.Annex(regionByName(w, name).ID, m.ID) // its own, so its supply lines reach
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

// A battle bleeds both sides; the side that falls far behind falls back to
// its nearest home tile, out of the fight.
func TestBattleBeatenArmyFallsBack(t *testing.T) {
	w := newNeighbourGame(t)
	w.Migration, w.Demography = false, false
	w.DeclareWar(0, 1)
	field, home := regionByName(w, "02"), regionByName(w, "01")
	big, _ := w.RaiseArmy(0, regionByName(w, "00").ID, 0)
	small, _ := w.RaiseArmy(1, home.ID, 0)
	big.Men, small.Men = 50, 12
	big.Region, small.Region = field.ID, field.ID
	for i := 0; i < 100 && !small.Retreating; i++ {
		w.battles()
	}
	if w.ArmyByID(small.ID) == nil || !small.Retreating || big.Retreating {
		t.Fatalf("after the battle: small %v (retreating %v), big retreating %v", w.ArmyByID(small.ID), small.Retreating, big.Retreating)
	}
	if big.Men >= 50 || small.Men >= 12 {
		t.Errorf("no casualties: %v, %v", big.Men, small.Men)
	}
	if len(small.Path) == 0 || small.Path[len(small.Path)-1] != home.ID {
		t.Errorf("falls back along %v, want to %d", small.Path, home.ID)
	}
	men := small.Men
	w.battles() // it no longer fights
	if small.Men != men {
		t.Error("a retreating army fought")
	}
	if err := w.MoveArmy(small.ID, field.ID); !errors.Is(err, ErrRetreating) {
		t.Errorf("orders to a retreating army: %v", err)
	}
	for i := 0; i < 60 && len(small.Path) > 0; i++ {
		w.march(small)
	}
	w.march(small)
	if small.Region != home.ID || small.Retreating {
		t.Errorf("after the retreat: at %d, retreating %v", small.Region, small.Retreating)
	}
}

// A beaten army with no way home that isn't barred by the enemy is
// surrounded: it fights on until it is destroyed.
func TestSurroundedArmyFightsToTheEnd(t *testing.T) {
	w := newNeighbourGame(t)
	w.Migration, w.Demography = false, false
	w.DeclareWar(0, 1)
	field := regionByName(w, "02")
	big, _ := w.RaiseArmy(0, regionByName(w, "00").ID, 0)
	small, _ := w.RaiseArmy(1, regionByName(w, "01").ID, 0)
	big.Men, small.Men = 50, 12
	big.Region, small.Region = field.ID, field.ID
	// the enemy holds every tile around the field
	for _, nb := range HexNeighbors(field.Row, field.Col, w.Rows, w.Cols) {
		r := w.Regions[nb[0]*w.Cols+nb[1]]
		if w.tileTaken(big.Nation, r.ID, nil) {
			continue
		}
		g := &Army{ID: 900 + r.ID, Nation: big.Nation, Region: r.ID, Men: 5, Supply: make([]float64, NumGoods), Stock: make([]float64, NumGoods)}
		w.Armies = append(w.Armies, g)
	}
	for i := 0; i < 200 && w.ArmyByID(small.ID) != nil; i++ {
		w.battles()
		if small.Retreating {
			t.Fatal("a surrounded army fell back")
		}
	}
	if w.ArmyByID(small.ID) != nil {
		t.Errorf("the surrounded army survived with %v men", small.Men)
	}
}

// Armies only march into tiles their nation holds or borders: supply lines.
func TestArmiesKeepToTheFront(t *testing.T) {
	w := newQuietGame(t)
	m := nationByName(w, "Montara")
	a, _ := w.RaiseArmy(m.ID, regionByName(w, "11").ID, 1)
	if err := w.MoveArmy(a.ID, regionByName(w, "22").ID); err != nil {
		t.Errorf("into a tile next to home: %v", err)
	}
	if err := w.MoveArmy(a.ID, regionByName(w, "33").ID); !errors.Is(err, ErrNoPath) {
		t.Errorf("deep into free land: %v", err)
	}
}

// An army can only be so big; techs raise the limit.
func TestArmySizeLimit(t *testing.T) {
	w := newQuietGame(t)
	m := nationByName(w, "Montara")
	home := regionByName(w, "11")
	addPeople(home, 2000) // enough people to mobilize more than one army's worth
	if _, err := w.RaiseArmy(m.ID, home.ID, baseArmySize+1); !errors.Is(err, ErrArmySize) {
		t.Errorf("an army beyond the size limit: %v", err)
	}
	a, err := w.RaiseArmy(m.ID, home.ID, baseArmySize)
	if err != nil {
		t.Fatal(err)
	}
	w.GrantTech(m.ID, "Drill")
	if m.MaxArmySize() != 20 {
		t.Fatalf("Drill: armies of %v", m.MaxArmySize())
	}
	if err := w.SetArmyTarget(a.ID, 20); err != nil {
		t.Errorf("Drill should allow 20 men: %v", err)
	}
}

// Occupying another nation's tile costs the occupier about 40% of its men
// (30-50%), whatever its techs.
func TestOccupationCostsMen(t *testing.T) {
	for _, tech := range []string{"", "General staff"} {
		w := newNeighbourGame(t)
		w.DeclareWar(0, 1)
		if tech != "" {
			w.GrantTech(0, tech)
		}
		enemy := regionByName(w, "01")
		a, _ := w.RaiseArmy(0, regionByName(w, "00").ID, 0)
		a.Men, a.Region = 50, enemy.ID
		for i := 0; i < 2000 && enemy.Nation != w.Nations[0]; i++ {
			w.sieges()
		}
		if enemy.Nation != w.Nations[0] {
			t.Fatal("tile not taken")
		}
		if kept := a.Men / 50; kept < 0.5 || kept > 0.7 {
			t.Errorf("with %q the occupier kept %.0f%% of its men", tech, 100*kept)
		}
	}
}

// An army carries a stock of supplies, so a sudden shortage doesn't weaken it
// at once.
func TestSupplyStockBuffersShortages(t *testing.T) {
	w := newQuietGame(t)
	m := nationByName(w, "Montara")
	a, _ := w.RaiseArmy(m.ID, regionByName(w, "11").ID, w.MobilizationCap(m))
	for i := 0; i < 300; i++ {
		w.Step()
	}
	if d := a.StockDays(G("bread")); d < 10 {
		t.Fatalf("only %v days of bread in stock", d)
	}
	before := a.Effectiveness()
	for i := 0; i < 10; i++ {
		m.Treasury = 0 // nothing to pay with
		w.Step()
	}
	if after := a.Effectiveness(); after < 0.95*before {
		t.Errorf("ten days without supplies: effectiveness %v -> %v", before, after)
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
	a.Men, a.Region = 20, beta.ID // an army its taxes can pay
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

// Armies buy each good where it lands cheapest among tiles that offer
// enough, not just at the nearest tile.
func TestArmiesBuyWhereGoodsAreMade(t *testing.T) {
	w := newQuietGame(t)
	m := nationByName(w, "Montara")
	a, _ := w.RaiseArmy(m.ID, regionByName(w, "11").ID, 5)
	a.Men = 5
	arms := G("arms")
	for _, r := range m.Regions { // only tile 00 makes arms
		r.Markets[arms].Supply = 0
	}
	maker := regionByName(w, "00")
	maker.Markets[arms].Supply = 100
	var c []claim
	w.planSupply(m, func(r *Region, cl claim) { c = append(c, cl) })
	if a.Source(arms) != maker.ID {
		t.Errorf("arms bought at %d, want the arms maker %d", a.Source(arms), maker.ID)
	}
	if a.Source(G("bread")) < 0 {
		t.Error("no source for bread")
	}
}

// In peace armies use only the drill share of their wartime supplies; in war
// all of them.
func TestPeacetimeDrill(t *testing.T) {
	w := newNeighbourGame(t)
	n := w.Nations[0]
	a, _ := w.RaiseArmy(0, regionByName(w, "00").ID, 0)
	a.Men = 10
	bread := G("bread")
	wartime := a.Men * Doctrines[a.Doctrine].Needs[bread]
	if err := n.SetDrill(0.1); !errors.Is(err, ErrBadDrill) {
		t.Errorf("drill below 20%%: %v", err)
	}
	n.SetDrill(0.3)
	w.planSupply(n, func(*Region, claim) {})
	if got := a.need[bread]; math.Abs(got-0.3*wartime) > 1e-12 {
		t.Errorf("peace: uses %v bread, want %v", got, 0.3*wartime)
	}
	w.DeclareWar(0, 1)
	w.planSupply(n, func(*Region, claim) {})
	if got := a.need[bread]; math.Abs(got-wartime) > 1e-12 {
		t.Errorf("war: uses %v bread, want %v", got, wartime)
	}
}

// Workers only go to firms that can pay them the tile's wage.
func TestBrokeFirmsDontHire(t *testing.T) {
	w := newQuietGame(t)
	r := regionByName(w, "11")
	w.Step()
	var f *Firm
	for _, x := range r.Firms {
		if x.Enabled && x.Labor > 0.1 {
			f = x
			break
		}
	}
	f.Cash = 0.01 * r.Wage // can pay a hundredth of a worker
	r.Plan()
	f.PlanLabor = math.Max(f.PlanLabor, 1)
	r.LaborMarket()
	if f.Labor > 0.01+1e-9 {
		t.Errorf("a broke firm hired %v workers", f.Labor)
	}
}

// Losses take equipment as well as men: battles drain the stock three times
// as fast as the ranks; other losses keep the days of stock per man.
func TestLossesTakeEquipment(t *testing.T) {
	w := newNeighbourGame(t)
	w.DeclareWar(0, 1)
	field := regionByName(w, "02")
	x, _ := w.RaiseArmy(0, regionByName(w, "00").ID, 0)
	y, _ := w.RaiseArmy(1, regionByName(w, "01").ID, 0)
	x.Men, y.Men = 30, 25
	x.Region, y.Region = field.ID, field.ID
	bread := G("bread")
	x.Stock[bread] = 30 * x.Men * Doctrines[x.Doctrine].Needs[bread]
	days := x.StockDays(bread)
	men, stock := x.Men, x.Stock[bread]
	w.battles()
	lostMen, lostStock := 1-x.Men/men, 1-x.Stock[bread]/stock
	if lostMen <= 0 || math.Abs(lostStock-battleKitLoss*lostMen) > 1e-9 {
		t.Errorf("battle: lost %.1f%% of men and %.1f%% of stock", 100*lostMen, 100*lostStock)
	}
	if x.StockDays(bread) >= days {
		t.Errorf("battle didn't cut days of stock: %v -> %v", days, x.StockDays(bread))
	}
	before := x.StockDays(bread)
	x.lose(0.1, 1)
	if math.Abs(x.StockDays(bread)-before) > 1e-9 {
		t.Errorf("attrition changed days of stock: %v -> %v", before, x.StockDays(bread))
	}
}

// A nation holds each tile with one army at most.
func TestOneArmyPerTile(t *testing.T) {
	w := newQuietGame(t)
	m := nationByName(w, "Montara")
	home, other := regionByName(w, "11"), regionByName(w, "10")
	a, err := w.RaiseArmy(m.ID, home.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.RaiseArmy(m.ID, home.ID, 1); !errors.Is(err, ErrTileTaken) {
		t.Errorf("second army in the same tile: %v", err)
	}
	b, err := w.RaiseArmy(m.ID, other.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.MoveArmy(b.ID, home.ID); !errors.Is(err, ErrTileTaken) {
		t.Errorf("marching onto our own army: %v", err)
	}
	if err := w.MoveArmy(a.ID, regionByName(w, "22").ID); err != nil {
		t.Fatal(err)
	}
	if err := w.MoveArmy(b.ID, regionByName(w, "22").ID); !errors.Is(err, ErrTileTaken) {
		t.Errorf("two armies heading for the same tile: %v", err)
	}
}

// Battles between near-equal armies end within weeks, and while they last
// occupation stands still.
func TestBattlesAreQuickAndFreezeOccupation(t *testing.T) {
	w := newNeighbourGame(t)
	w.Migration, w.Demography = false, false
	w.DeclareWar(0, 1)
	enemy := regionByName(w, "01")
	x, _ := w.RaiseArmy(0, regionByName(w, "00").ID, 0)
	x.Men, x.Region = 40, enemy.ID
	for i := 0; i < 30; i++ {
		w.sieges()
	}
	p := enemy.Siege.Progress
	y, _ := w.RaiseArmy(1, enemy.ID, 0)
	y.Men = 30                           // with home ground, close to even
	w.Annex(regionByName(w, "02").ID, 1) // somewhere for the loser to fall back to
	decided := func(a *Army) bool { return w.ArmyByID(a.ID) == nil || a.Retreating }
	days := 0
	for ; !decided(x) && !decided(y) && days < 100; days++ {
		w.battles()
		w.sieges()
		if enemy.Battle > 0 && enemy.Siege != nil && enemy.Siege.Progress != p {
			t.Fatalf("occupation moved during battle: %v -> %v", p, enemy.Siege.Progress)
		}
	}
	if days > 20 {
		t.Errorf("an even battle lasted %d days", days)
	}
	t.Logf("battle decided in %d days", days)
}

// Armies in the field are reinforced: recruits drafted at home march out and
// join after the march; a cut-off army gets none.
func TestFieldArmiesAreReinforced(t *testing.T) {
	w := newQuietGame(t)
	m := nationByName(w, "Montara")
	a, _ := w.RaiseArmy(m.ID, regionByName(w, "11").ID, w.MobilizationCap(m))
	if err := w.MoveArmy(a.ID, regionByName(w, "22").ID); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 200 && len(a.Path) > 0; i++ {
		w.Step()
	}
	a.Men = 1 // heavy losses
	w.Step()
	if a.coming() <= 0 || len(a.Coming) == 0 || a.Coming[0].Arrive <= w.Tick {
		t.Fatalf("no recruits on the way: %+v", a.Coming)
	}
	for i := 0; i < 90; i++ {
		w.Step()
	}
	if a.Men < 0.8*a.Target {
		t.Errorf("after 90 days of reinforcements: %v of %v men", a.Men, a.Target)
	}
}

// Lowering an army's target, or raising an empty army, is always allowed,
// even when the nation is over its cap (e.g. after losing tiles).
func TestOverCapStillAllowsCuts(t *testing.T) {
	w := newQuietGame(t)
	m := nationByName(w, "Montara")
	a, _ := w.RaiseArmy(m.ID, regionByName(w, "11").ID, 5)
	a.Target = m.MaxArmySize() // over the cap of about 8, within the army size limit
	if a.Target <= w.MobilizationCap(m) {
		t.Fatalf("the cap %v is not below %v", w.MobilizationCap(m), a.Target)
	}
	if err := w.SetArmyTarget(a.ID, a.Target-1); err != nil {
		t.Errorf("lowering an over-cap target: %v", err)
	}
	if _, err := w.RaiseArmy(m.ID, regionByName(w, "10").ID, 0); err != nil {
		t.Errorf("raising an empty army while over the cap: %v", err)
	}
	if err := w.SetArmyTarget(a.ID, a.Target+0.5); !errors.Is(err, ErrMobilized) {
		t.Errorf("raising an over-cap target: %v", err)
	}
}

// The state presses harder for its armies' supplies than for construction:
// a shortage shows as a big excess demand, so the price rises until supply
// answers.
func TestArmiesBidHardForScarceSupplies(t *testing.T) {
	w := newQuietGame(t)
	m := nationByName(w, "Montara")
	m.Treasury = 1e4
	w.GrantTech(m.ID, "Drill") // line infantry needs artillery
	home := regionByName(w, "11")
	a, _ := w.RaiseArmy(m.ID, home.ID, 8)
	a.Men = 100 // an order (~0.6 a day) between construction's limit and the army's
	art := G("artillery")
	for _, r := range m.Regions {
		r.Markets[art].Supply = 0
	}
	home.Markets[art].Supply = 0.1
	m.plan(w)
	bid := home.publicBid[art]
	if limit := stateBuyMargin*0.1 + stateBuyFloor; bid <= limit {
		t.Errorf("artillery bid %v, no more than construction's limit %v", bid, limit)
	}
	if bid > armyBuyMargin*0.1+armyBuyFloor+1e-9 {
		t.Errorf("artillery bid %v above the army's limit", bid)
	}
}
