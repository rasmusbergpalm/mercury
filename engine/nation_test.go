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
	if len(w.Nations) != 4 {
		t.Fatalf("%d nations", len(w.Nations))
	}
	total := 0
	for _, n := range w.Nations {
		total += len(n.Regions)
		for _, r := range n.Regions {
			if r.Nation != n {
				t.Errorf("tile %s listed under %s but owned by %v", r.Name, n.Name, r.Nation)
			}
		}
	}
	if total != len(w.Regions) {
		t.Errorf("nations own %d of %d tiles", total, len(w.Regions))
	}
	if r := w.Regions[0]; r.Nation.Name != "Montara" || r.Markets[Construction] == nil {
		t.Errorf("tile 00 owned by %s", r.Nation.Name)
	}
}

func TestBuildGameRejectsBadSpecs(t *testing.T) {
	spec := DefaultGame
	spec.NationMap = []string{"AAA"}
	if _, err := BuildGame(spec); err == nil {
		t.Error("nation map of the wrong shape accepted")
	}
	spec = DefaultGame
	spec.NationMap = append([]string{"ZAABBB"}, DefaultGame.NationMap[1:]...)
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
			for _, id := range []int{0, 9, 20, 33} {
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
	const id = 9 // tile "13", plains in Verdania
	r := w.Regions[id]
	if err := w.Build(id); err != nil {
		t.Fatal(err)
	}
	if err := w.Build(id); !errors.Is(err, ErrBuilding) {
		t.Errorf("second build while building: %v", err)
	}
	treasury := r.Nation.Treasury
	for i := 0; i < 300 && r.Infra == 0; i++ {
		w.Step()
	}
	if r.Infra != 1 || r.Project != nil {
		t.Fatalf("road not finished after 300 ticks: level %d, project %+v", r.Infra, r.Project)
	}
	if r.Nation.Treasury >= treasury+1e-9 && r.Nation.Spending == 0 {
		t.Error("building cost the treasury nothing")
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
			want := tr.TransportCost * (r.MoveCost() + map[bool]*Region{true: tr.Dest, false: tr.Origin}[tr.Origin == r].MoveCost()) / 2 * Goods[tr.Good].Bulk
			if math.Abs(tr.Tau-want) > 1e-12 {
				t.Errorf("trader %s tau %v, want %v", tr.Name(), tr.Tau, want)
			}
		}
	}

	// pave it, then there is nothing better to build
	if err := w.Build(id); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 400 && r.Infra == 1; i++ {
		w.Step()
	}
	if r.Infra != 2 {
		t.Fatalf("paved road not finished: level %d", r.Infra)
	}
	if err := w.Build(id); !errors.Is(err, ErrMaxLevel) {
		t.Errorf("build beyond the best level: %v", err)
	}
}

func TestCancelBuild(t *testing.T) {
	w := newTestGame(t)
	if err := w.CancelBuild(3); !errors.Is(err, ErrNoProject) {
		t.Errorf("cancel without project: %v", err)
	}
	w.Build(3)
	if err := w.CancelBuild(3); err != nil || w.Regions[3].Project != nil {
		t.Errorf("cancel: %v, project %+v", err, w.Regions[3].Project)
	}
}

func TestUnownedTilesCannotBuild(t *testing.T) {
	spec := DefaultGame
	spec.NationMap = append([]string{"..ABBB"}, DefaultGame.NationMap[1:]...)
	w, err := BuildGame(spec)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Build(0); !errors.Is(err, ErrNoNation) {
		t.Errorf("build on unowned tile: %v", err)
	}
	for i := 0; i < 50; i++ {
		w.Step() // unowned tiles must not break anything
	}
}

func TestNeglectedRoadsDecay(t *testing.T) {
	spec := DefaultGame
	spec.TaxRate = 0 // no revenue, so no upkeep can be paid
	w, _ := BuildGame(spec)
	w.SetInfra(9, 2)
	for i := 0; i < 200 && w.Regions[9].Infra == 2; i++ {
		w.Step()
	}
	if w.Regions[9].Infra != 1 {
		t.Errorf("unmaintained paved road still at level %d", w.Regions[9].Infra)
	}
}

func TestMaintainedRoadsLast(t *testing.T) {
	w := newTestGame(t)
	for i := 0; i < 100; i++ {
		w.Step()
	}
	w.SetInfra(9, 2)
	for i := 0; i < 400; i++ {
		w.Step()
	}
	if w.Regions[9].Infra != 2 {
		t.Errorf("maintained paved road decayed to level %d", w.Regions[9].Infra)
	}
}

// Economic sanity: infrastructure raises trade and welfare, mostly for the
// nation that builds it.
func TestInfrastructurePaysOff(t *testing.T) {
	if testing.Short() {
		t.Skip("long")
	}
	run := func(paved func(r *Region) bool) (utility, volume float64, byNation map[string]float64) {
		w := newTestGame(t)
		for _, r := range w.Regions {
			if paved(r) {
				w.SetInfra(r.ID, 2)
			}
		}
		byNation = map[string]float64{}
		const ticks, window = 600, 50
		for i := 0; i < ticks; i++ {
			w.Step()
			if i < ticks-window {
				continue
			}
			for _, r := range w.Regions {
				utility += r.Utility() / window / float64(len(w.Regions))
				byNation[r.Nation.Name] += r.Utility() / window / float64(len(r.Nation.Regions))
			}
			for _, tr := range w.Traders {
				volume += tr.Shipped / window
			}
		}
		return
	}
	uNone, vNone, nNone := run(func(*Region) bool { return false })
	uAll, vAll, _ := run(func(*Region) bool { return true })
	uM, _, nM := run(func(r *Region) bool { return r.Nation.Name == "Montara" })
	t.Logf("utility %.3f -> %.3f (all paved), volume %.0f -> %.0f; paving Montara: %.3f", uNone, uAll, vNone, vAll, uM)
	if !(uAll > uNone && vAll > vNone) {
		t.Errorf("paving everything should raise utility and trade")
	}
	gain := func(n string) float64 { return nM[n] - nNone[n] }
	for _, n := range []string{"Verdania", "Sudoria", "Thalassa"} {
		if gain("Montara") <= gain(n) {
			t.Errorf("paving Montara helped %s (%+.3f) at least as much as Montara (%+.3f)", n, gain(n), gain("Montara"))
		}
	}
	if gain("Montara") <= 0 {
		t.Errorf("paving Montara did not help Montara: %+.3f", gain("Montara"))
	}
}
