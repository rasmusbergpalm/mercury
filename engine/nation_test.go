package engine

import (
	"errors"
	"math"
	"testing"
)

func newTestGame(t *testing.T) *World {
	t.Helper()
	w, err := BuildGame(DefaultGame)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func regionByName(w *World, name string) *Region {
	for _, r := range w.Regions {
		if r.Name == name {
			return r
		}
	}
	return nil
}

func nationByName(w *World, name string) *Nation {
	for _, n := range w.Nations {
		if n.Name == name {
			return n
		}
	}
	return nil
}

func TestBuildGameAssignsTiles(t *testing.T) {
	w := newTestGame(t)
	if len(w.Nations) != 3 {
		t.Fatalf("%d nations", len(w.Nations))
	}
	owned := 0
	for _, n := range w.Nations {
		if len(n.Regions) != 4 {
			t.Errorf("%s starts with %d tiles, want 4", n.Name, len(n.Regions))
		}
		owned += len(n.Regions)
		for _, r := range n.Regions {
			if r.Nation != n {
				t.Errorf("tile %s listed under %s but owned by %v", r.Name, n.Name, r.Nation)
			}
		}
	}
	unowned := 0
	for _, r := range w.Regions {
		if r.Nation == nil {
			unowned++
		}
	}
	if owned+unowned != len(w.Regions) || unowned != 24 {
		t.Errorf("%d owned and %d unowned tiles", owned, unowned)
	}
	if r := regionByName(w, "00"); r.Nation == nil || r.Nation.Name != "Montara" {
		t.Errorf("tile 00 should start in Montara")
	}
}

func TestBuildGameRejectsBadSpecs(t *testing.T) {
	spec := DefaultGame
	spec.NationMap = []string{"AAA"}
	if _, err := BuildGame(spec); err == nil {
		t.Error("nation map of the wrong shape accepted")
	}
	spec = DefaultGame
	spec.NationMap = append([]string{"ZA..BB"}, DefaultGame.NationMap[1:]...)
	if _, err := BuildGame(spec); err == nil {
		t.Error("unknown nation symbol accepted")
	}
	spec = DefaultGame
	spec.TaxRate = 1.5
	if _, err := BuildGame(spec); err == nil {
		t.Error("tax rate above 0.9 accepted")
	}
}

func TestIncomeIsTaxed(t *testing.T) {
	w := newTestGame(t)
	r := w.Regions[0]
	r.Nation.TaxRate = 0.2
	p := r.Pops[0]
	cash, treasury := p.Cash, r.Nation.Treasury
	r.payIncome(p, 100)
	if math.Abs(p.Cash-cash-80) > 1e-9 || math.Abs(r.Nation.Treasury-treasury-20) > 1e-9 {
		t.Errorf("pop got %v, treasury got %v; want 80 and 20", p.Cash-cash, r.Nation.Treasury-treasury)
	}
}

func TestTaxRateDrivesRevenue(t *testing.T) {
	revenue := func(rate float64) float64 {
		spec := DefaultGame
		spec.TaxRate = rate
		w, _ := BuildGame(spec)
		total := 0.0
		for i := 0; i < 100; i++ {
			w.Step()
			for _, n := range w.Nations {
				total += n.Revenue
			}
		}
		return total
	}
	if r := revenue(0); r != 0 {
		t.Errorf("zero tax raised %v", r)
	}
	lo, hi := revenue(0.05), revenue(0.2)
	if !(hi > 2*lo) {
		t.Errorf("revenue at 20%% (%v) should be well above revenue at 5%% (%v)", hi, lo)
	}
}

func TestRebateReturnsSurplusPerHead(t *testing.T) {
	w := newTestGame(t)
	n := w.Nations[0]
	n.Treasury = n.Reserve + 500
	before := make([]float64, 0)
	for _, r := range n.Regions {
		for _, p := range r.Pops {
			before = append(before, p.Cash)
		}
	}
	money := w.Money()
	n.rebate()
	want := n.RebateRate * 500 / n.population()
	i := 0
	for _, r := range n.Regions {
		for _, p := range r.Pops {
			if got := (p.Cash - before[i]) / p.Size; math.Abs(got-want) > 1e-9 {
				t.Errorf("rebate per head %v, want %v", got, want)
			}
			i++
		}
	}
	if relDiff(w.Money(), money) > 1e-12 {
		t.Errorf("rebate changed the money supply")
	}
}

func TestGameMoneyIsConserved(t *testing.T) {
	w := newTestGame(t)
	m0 := w.Money()
	for i := 0; i < 400; i++ {
		if i == 100 {
			for _, name := range []string{"00", "13", "44"} {
				id := regionByName(w, name).ID
				if err := w.Build(id); err != nil {
					t.Fatal(err)
				}
			}
		}
		w.Step()
		if d := relDiff(w.Money(), m0); d > 1e-10 {
			t.Fatalf("tick %d: money %v, started with %v", w.Tick, w.Money(), m0)
		}
	}
}

func TestGameIsDeterministic(t *testing.T) {
	a, b := newTestGame(t), newTestGame(t)
	for i := 0; i < 250; i++ {
		if i == 50 {
			a.Build(9)
			b.Build(9)
		}
		a.Step()
		b.Step()
	}
	for i, n := range a.Nations {
		if n.Treasury != b.Nations[i].Treasury {
			t.Fatalf("%s treasuries differ", n.Name)
		}
	}
	if a.Regions[9].Infra != b.Regions[9].Infra {
		t.Fatal("infrastructure differs")
	}
}

func TestBuildingRoadsLowersHopCosts(t *testing.T) {
	w := newTestGame(t)
	for i := 0; i < 200; i++ {
		w.Step()
	}
	r := regionByName(w, "13") // plains in Verdania
	id, n := r.ID, r.Nation
	if err := w.Build(id); err != nil {
		t.Fatal(err)
	}
	if err := w.Build(id); !errors.Is(err, ErrBuilding) {
		t.Errorf("second build while building: %v", err)
	}
	for i := 0; i < 300 && r.Infra == 0; i++ {
		w.Step()
	}
	if r.Infra != 1 || r.Project != nil {
		t.Fatalf("road not finished after 300 ticks: level %d, project %+v", r.Infra, r.Project)
	}
	for _, l := range w.Links {
		if l.A != id && l.B != id {
			continue
		}
		other := w.Regions[l.A+l.B-id]
		want := (InfraLevels[1].MoveMult*r.BaseMove + other.MoveCost()) / 2
		if math.Abs(l.Hop-want) > 1e-12 {
			t.Errorf("link %d-%d hop %v, want %v", l.A, l.B, l.Hop, want)
		}
	}
	for _, tr := range w.Traders {
		if tr.Origin == r || tr.Dest == r {
			want := tr.TransportCost * (tr.Origin.MoveCost() + tr.Dest.MoveCost()) / 2 * Goods[tr.Good].Bulk
			if math.Abs(tr.Tau-want) > 1e-12 {
				t.Errorf("trader %s tau %v, want %v", tr.Name(), tr.Tau, want)
			}
		}
	}

	// paving needs Masonry
	if err := w.Build(id); !errors.Is(err, ErrNeedTech) {
		t.Fatalf("paving without Masonry: %v", err)
	}
	n.Known[TechID("Masonry")] = true
	if err := w.Build(id); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 400 && r.Infra == 1; i++ {
		w.Step()
	}
	if r.Infra != 2 {
		t.Fatalf("paved road not finished: level %d", r.Infra)
	}
	// railways need the Railways tech, and iron
	if err := w.Build(id); !errors.Is(err, ErrNeedTech) {
		t.Errorf("railway without the tech: %v", err)
	}
	n.Known[TechID("Railways")] = true
	if err := w.Build(id); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 1500 && r.Infra == 2; i++ {
		w.Step()
	}
	if r.Infra != 3 {
		t.Fatalf("railway not finished: level %d, project %+v", r.Infra, r.Project)
	}
	if err := w.Build(id); !errors.Is(err, ErrMaxLevel) {
		t.Errorf("build beyond the best level: %v", err)
	}
}

func TestCancelBuild(t *testing.T) {
	w := newTestGame(t)
	id := regionByName(w, "13").ID
	if err := w.CancelBuild(id); !errors.Is(err, ErrNoProject) {
		t.Errorf("cancel without project: %v", err)
	}
	w.Build(id)
	if err := w.CancelBuild(id); err != nil || w.Regions[id].Project != nil {
		t.Errorf("cancel: %v, project %+v", err, w.Regions[id].Project)
	}
}

func TestUnownedTilesCannotBuild(t *testing.T) {
	w := newTestGame(t)
	id := regionByName(w, "22").ID
	if err := w.Build(id); !errors.Is(err, ErrNoNation) {
		t.Errorf("build on unowned tile: %v", err)
	}
	if err := w.Expand(id, 0); !errors.Is(err, ErrNoNation) {
		t.Errorf("expand on unowned tile: %v", err)
	}
}

func TestNeglectedRoadsDecay(t *testing.T) {
	spec := DefaultGame
	spec.TaxRate = 0 // no revenue, so no upkeep can be paid
	w, _ := BuildGame(spec)
	r := regionByName(w, "13")
	w.SetInfra(r.ID, 2)
	for i := 0; i < 200 && r.Infra == 2; i++ {
		w.Step()
	}
	if r.Infra != 1 {
		t.Errorf("unmaintained paved road still at level %d", r.Infra)
	}
}

func TestMaintainedRoadsLast(t *testing.T) {
	w := newTestGame(t)
	for i := 0; i < 100; i++ {
		w.Step()
	}
	r := regionByName(w, "13")
	w.SetInfra(r.ID, 2)
	for i := 0; i < 400; i++ {
		w.Step()
	}
	if r.Infra != 2 {
		t.Errorf("maintained paved road decayed to level %d", r.Infra)
	}
}

// Economic sanity: infrastructure raises trade and welfare. With migration,
// tiles that pave attract people, so their gain shows up as population
// rather than utility per head. (Inland tiles: on the coast, ships already
// beat a paved road.)
func TestInfrastructurePaysOff(t *testing.T) {
	if testing.Short() {
		t.Skip("long")
	}
	inland := map[string]bool{"22": true, "23": true, "32": true, "33": true}
	type result struct{ utility, volume, inlandPop float64 }
	run := func(paved func(r *Region) bool) (res result) {
		w := newTestGame(t)
		for _, r := range w.Regions {
			if paved(r) {
				w.SetInfra(r.ID, 2)
			}
		}
		const ticks, window = 1500, 200
		for i := 0; i < ticks; i++ {
			w.Step()
			if i < ticks-window {
				continue
			}
			for _, r := range w.Regions { // population-weighted utility
				res.utility += r.Utility() * r.Population() / w.Population() / window
				if inland[r.Name] {
					res.inlandPop += r.Population() / window
				}
			}
			for _, tr := range w.Traders {
				res.volume += tr.Shipped / window
			}
		}
		return
	}
	none := run(func(*Region) bool { return false })
	all := run(func(*Region) bool { return true })
	mid := run(func(r *Region) bool { return inland[r.Name] })
	t.Logf("none %+v\nall paved %+v\ninland paved %+v", none, all, mid)
	if !(all.utility > none.utility && all.volume > none.volume) {
		t.Errorf("paving everything should raise welfare and trade")
	}
	if mid.inlandPop <= none.inlandPop {
		t.Errorf("paving inland tiles should draw people to them: %v -> %v", none.inlandPop, mid.inlandPop)
	}
	// Paving a few tiles mostly relocates people: with every tile's industry
	// gaining from its size, the tiles they leave lose productivity that
	// migrants don't weigh, so world welfare needn't rise.
}
