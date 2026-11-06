package engine

import (
	"errors"
	"math"
	"testing"
)

func TestArmiesNeedFunding(t *testing.T) {
	w := newTestGame(t)
	paid, unpaid := w.Nations[0], w.Nations[1]
	unpaid.SetBudget([NumBudgets]float64{0.4, 0.2, 0})
	for i := 0; i < 200; i++ {
		w.Step()
	}
	if unpaid.Army != 0 || paid.Army <= 0 {
		t.Fatalf("armies: funded %v, unfunded %v", paid.Army, unpaid.Army)
	}
	army := paid.Army
	paid.SetBudget([NumBudgets]float64{0.4, 0.2, 0})
	for i := 0; i < 50; i++ {
		w.Step()
	}
	if want := army * math.Pow(1-armyDecay, 50); math.Abs(paid.Army-want) > 1e-9*army {
		t.Errorf("unpaid army %v after 50 ticks, want %v", paid.Army, want)
	}
}

func TestBarracksConsumeGoods(t *testing.T) {
	w := newTestGame(t)
	for i := 0; i < 100; i++ {
		w.Step()
	}
	for _, r := range w.Nations[0].Regions {
		for _, f := range r.Firms {
			if f.Name == "Barracks" && f.LastOutput > 0 {
				return // output needs ironware and clothes by construction of the recipe
			}
		}
	}
	t.Error("no barracks produced anything for a funded army")
}

func TestCampaignErrors(t *testing.T) {
	w := newTestGame(t)
	m := nationByName(w, "Montara")
	if err := w.StartCampaign(m.ID, regionByName(w, "02").ID); !errors.Is(err, ErrNoArmy) {
		t.Errorf("campaign without an army: %v", err)
	}
	m.Army = 50
	if err := w.StartCampaign(m.ID, regionByName(w, "00").ID); !errors.Is(err, ErrOwnTile) {
		t.Errorf("campaign on own tile: %v", err)
	}
	if err := w.StartCampaign(m.ID, regionByName(w, "25").ID); !errors.Is(err, ErrNotAdjacent) {
		t.Errorf("campaign on a distant tile: %v", err)
	}
	if err := w.CancelCampaign(m.ID); !errors.Is(err, ErrNoCampaign) {
		t.Errorf("cancel without a campaign: %v", err)
	}
	if err := w.StartCampaign(m.ID, regionByName(w, "02").ID); err != nil {
		t.Fatal(err)
	}
	if err := w.CancelCampaign(m.ID); err != nil || m.Campaign != nil {
		t.Errorf("cancel: %v", err)
	}
}

// Occupation is a slow process: an undefended tile takes at least 100 ticks.
func TestUndefendedTilesFallSlowly(t *testing.T) {
	w := newTestGame(t)
	m := nationByName(w, "Montara")
	for i := 0; i < 200; i++ {
		w.Step()
	}
	target := regionByName(w, "02")
	if err := w.StartCampaign(m.ID, target.ID); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 99; i++ {
		w.Step()
		if target.Nation == m {
			t.Fatalf("tile fell after only %d ticks", i+1)
		}
	}
	for i := 0; i < 400 && target.Nation != m; i++ {
		w.Step()
	}
	if target.Nation != m || len(m.Regions) != 5 || m.Campaign != nil {
		t.Fatalf("tile 02 not occupied: owner %v, %d tiles", target.Nation, len(m.Regions))
	}
}

func TestDefendedTilesResist(t *testing.T) {
	progress := func(defender float64) (p, attackerLeft, defenderLeft float64) {
		w := newNeighbourGame(t)
		a, b := w.Nations[0], w.Nations[1]
		a.Army, b.Army = 100, defender
		if err := w.StartCampaign(a.ID, regionByName(w, "01").ID); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 20; i++ {
			w.campaigns()
		}
		return a.Campaign.Progress, a.Army, b.Army
	}
	free, a0, _ := progress(0)
	held, a1, d1 := progress(40)
	_, _, _ = progress(100)
	if held >= free || a1 >= a0 || d1 >= 40 {
		t.Errorf("a defended tile should slow occupation and cost both armies: progress %v vs %v, armies %v %v", held, free, a1, d1)
	}
	if stalled, _, _ := progress(100); stalled != 0 {
		t.Errorf("an equal defender (with its advantage) should stop the attack, progress %v", stalled)
	}
}

func TestConquestEliminatesANation(t *testing.T) {
	w := newNeighbourGame(t)
	a, b := w.Nations[0], w.Nations[1]
	for i := 0; i < 50; i++ {
		w.Step()
	}
	w.GrantTech(a.ID, "Wagons")
	a.Army, b.Army = 500, 0
	b.SetBudget([NumBudgets]float64{0.4, 0.2, 0})
	tile := regionByName(w, "01")
	if err := w.StartCampaign(a.ID, tile.ID); err != nil {
		t.Fatal(err)
	}
	money := w.Money()
	for i := 0; i < 300 && !b.Eliminated(); i++ {
		w.Step()
	}
	if !b.Eliminated() || tile.Nation != a {
		t.Fatalf("Beta not eliminated: %d tiles", len(b.Regions))
	}
	for _, f := range tile.Firms {
		if f.Name == "Transport" && math.Abs(f.TFP-1.3*f.BaseTFP) > 1e-12 {
			t.Errorf("conquered tile lacks the conqueror's tech: %v", f.TFP)
		}
	}
	for i := 0; i < 50; i++ {
		w.Step() // an eliminated nation must not break anything
	}
	if relDiff(w.Money(), money) > 1e-10 {
		t.Error("money not conserved through a conquest")
	}
	if err := w.StartCampaign(b.ID, tile.ID); !errors.Is(err, ErrEliminated) {
		t.Errorf("eliminated nation campaigning: %v", err)
	}
}
