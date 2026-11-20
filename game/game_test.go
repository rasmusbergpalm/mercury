package game

import (
	"errors"
	"math"
	"testing"
)

// two nations side by side on the default map: A owns 00, B owns 01
func newNeighbourGame(t *testing.T) *World {
	t.Helper()
	spec := DefaultGame
	spec.NationMap = []string{"AB....", "......", "......", "......", "......", "......"}
	spec.Nations = []NationSpec{{'A', "Alpha", "#c00"}, {'B', "Beta", "#00c"}}
	w, err := BuildGame(spec)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func tradersBetween(w *World, a, b *Region) []*Trader {
	var out []*Trader
	for _, tr := range w.Traders {
		if (tr.Origin == a.Region && tr.Dest == b.Region) || (tr.Origin == b.Region && tr.Dest == a.Region) {
			out = append(out, tr)
		}
	}
	return out
}

// ---- M3: tariffs and embargoes ------------------------------------------------

func TestTariffAppliesOnlyToImports(t *testing.T) {
	w := newTestGame(t)
	v := nationByName(w, "Verdania")
	if err := v.SetTariff(-1, 0.5); err != nil {
		t.Fatal(err)
	}
	for _, tr := range w.Traders {
		rate, blocked := w.duty(tr)
		into := w.Regions[tr.Dest.ID].Nation == v && w.Regions[tr.Origin.ID].Nation != v
		if blocked || (into && rate != 0.5) || (!into && rate != 0) {
			t.Fatalf("%s: tariff %v blocked %v", tr.Name(), rate, blocked)
		}
	}
	if err := v.SetTariff(G("bread"), -1); !errors.Is(err, ErrBadTariff) {
		t.Errorf("negative tariff: %v", err)
	}
}

func TestTariffsRaiseRevenueAndCutImports(t *testing.T) {
	if testing.Short() {
		t.Skip("long")
	}
	imports := func(rate float64) (volume, revenue float64) {
		w := newTestGame(t)
		v := nationByName(w, "Verdania")
		v.SetTariff(-1, rate)
		m0 := w.Money()
		for i := 0; i < 500; i++ {
			w.Step()
			if i >= 450 {
				revenue += v.TariffRevenue / 50
				for _, tr := range w.Traders {
					if w.Regions[tr.Dest.ID].Nation == v && w.Regions[tr.Origin.ID].Nation != v {
						volume += tr.Shipped / 50
					}
				}
			}
		}
		if relDiff(w.Money(), m0) > 1e-10 {
			t.Errorf("tariffs broke money conservation")
		}
		return
	}
	vFree, rFree := imports(0)
	vTax, rTax := imports(0.5)
	t.Logf("imports into Verdania %.2f -> %.2f with a 50%% tariff, raising %.2f/tick", vFree, vTax, rTax)
	if rFree != 0 || rTax <= 0 {
		t.Errorf("tariff revenue %v (free) and %v (taxed)", rFree, rTax)
	}
	if vTax >= vFree {
		t.Errorf("a 50%% tariff should cut imports: %v -> %v", vFree, vTax)
	}
}

func TestEmbargoStopsCrossBorderTrade(t *testing.T) {
	w := newNeighbourGame(t)
	a, b := regionByName(w, "00"), regionByName(w, "01")
	for i := 0; i < 150; i++ {
		w.Step()
	}
	flow := func() float64 {
		s := 0.0
		for _, tr := range tradersBetween(w, a, b) {
			s += tr.Shipped
		}
		return s
	}
	if flow() == 0 {
		t.Fatal("no trade between the neighbours to begin with")
	}
	w.Nations[0].Embargo[1] = true // Alpha embargoes Beta: both directions close
	for i := 0; i < 20; i++ {      // stock bought before the embargo ships out first
		w.Step()
	}
	for _, tr := range tradersBetween(w, a, b) {
		if _, blocked := w.duty(tr); !blocked {
			t.Errorf("%s not blocked", tr.Name())
		}
	}
	if f := flow(); f > 1e-6 {
		t.Errorf("trade across an embargoed border: %v", f)
	}
	// trade with unowned tiles goes on
	open := 0.0
	for _, tr := range w.Traders {
		if tr.Origin == a.Region && w.Regions[tr.Dest.ID].Nation == nil {
			open += tr.Shipped
		}
	}
	if open == 0 {
		t.Error("embargo stopped trade with unowned tiles too")
	}
}

// ---- M4: technology ---------------------------------------------------------

func TestResearchUnlocksTechInOrder(t *testing.T) {
	w := newTestGame(t)
	n := w.Nations[0]
	if err := n.SetResearch(TechID("Locomotives")); !errors.Is(err, ErrBadTech) {
		t.Errorf("researching Locomotives before Canals: %v", err)
	}
	if err := n.SetResearch(TechID("Canals")); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 50; i++ {
		w.Step()
	}
	if n.ResearchPoints <= 0 {
		t.Fatal("no research produced")
	}
	n.ResearchPoints = Techs[TechID("Canals")].Cost
	w.Step()
	if !n.Has("Canals") {
		t.Fatalf("Canals not researched with its full cost banked")
	}
	if err := n.SetResearch(TechID("Canals")); !errors.Is(err, ErrBadTech) {
		t.Errorf("researching a known tech: %v", err)
	}
	if err := n.SetResearch(TechID("Locomotives")); err != nil {
		t.Errorf("Steam engine after Canals: %v", err)
	}
	// its bonus (transport +20%) comes in over techAdoptDays: none yet, half
	// halfway, all of it at the end
	check := func(adopted float64) {
		t.Helper()
		for _, r := range w.Regions {
			f := firmIn(r, "Transport")
			want := f.Methods[0].BaseTFP
			switch {
			case r.Nation == n:
				want *= 1 + 0.2*adopted
			case r.Nation != nil:
				continue
			}
			if math.Abs(f.TFP()-want) > 1e-9 {
				t.Errorf("%.0f%% adopted: tile %s transport TFP %v, want %v", 100*adopted, r.Name, f.TFP(), want)
			}
		}
	}
	wagons := TechID("Canals")
	learned := n.LearnedAt[wagons]
	if a := n.Adoption(wagons); a > 0.01 {
		t.Errorf("Canals %v adopted the day after it was learned", a)
	}
	check(n.Adoption(wagons))
	for w.Tick-1-learned < techAdoptDays/2 {
		w.Step()
	}
	if a := n.Adoption(wagons); math.Abs(a-0.5) > 0.01 {
		t.Errorf("halfway: %v adopted", a)
	}
	check(n.Adoption(wagons))
	for w.Tick-1-learned < techAdoptDays+10 {
		w.Step()
	}
	check(1)
}

// Research comes from the universities the state funds: none funded, none
// made; more funded levels, more research, at more cost.
func TestFundedUniversitiesDriveResearch(t *testing.T) {
	run := func(levels int) (points, spent float64) {
		w := newTestGame(t)
		n := w.Nations[0]
		for _, r := range n.Regions {
			r.Service(University).Levels = levels
			if err := w.SetFunding(r.ID, levels); err != nil {
				t.Fatal(err)
			}
		}
		for i := 0; i < 300; i++ {
			w.Step()
			spent += n.SpentOn[Science]
		}
		points = n.ResearchPoints
		for i, k := range n.Known { // plus what was spent on techs
			if k {
				points += n.TechCost(i, w)
			}
		}
		return
	}
	if p, c := run(0); p != 0 || c > 1e-9 {
		t.Errorf("unfunded universities made %v points and cost %v", p, c)
	}
	lo, loCost := run(1)
	hi, hiCost := run(3)
	if hi <= lo || hiCost <= loCost {
		t.Errorf("more funded levels should mean more research at more cost: %v (%v) vs %v (%v)", lo, loCost, hi, hiCost)
	}
	w := newTestGame(t)
	r := w.Nations[0].Regions[0]
	if err := w.SetFunding(r.ID, r.Service(University).Levels+1); !errors.Is(err, ErrBadFunding) {
		t.Errorf("funding beyond the levels: %v", err)
	}
}

func TestSteamshipsCheapenCoastTiles(t *testing.T) {
	w := newTestGame(t)
	th := nationByName(w, "Thalassa")
	before := map[*Region]float64{}
	for _, r := range w.Regions {
		before[r] = r.MoveCost()
	}
	w.GrantTech(th.ID, "Steamships")
	for _, r := range w.Regions {
		want := before[r]
		if r.Nation == th && r.Coastal {
			want *= 0.6
		}
		if math.Abs(r.MoveCost()-want) > 1e-12 {
			t.Errorf("tile %s (%s, %v) move cost %v, want %v", r.Name, r.Terrain, r.Nation != nil, r.MoveCost(), want)
		}
	}
}

func TestRefrigerationCutsTransitLoss(t *testing.T) {
	w := newTestGame(t)
	th := nationByName(w, "Thalassa")
	var tr *Trader
	for _, x := range w.Traders {
		if w.Regions[x.Origin.ID].Nation == th && Goods[x.Good].Name == "fish" {
			tr = x
			break
		}
	}
	before := tr.Arriving
	w.GrantTech(th.ID, "Refrigeration")
	want := math.Pow(1-0.5*0.4, tr.Distance)
	if math.Abs(tr.Arriving-want) > 1e-12 || tr.Arriving <= before {
		t.Errorf("fish arriving %v -> %v, want %v", before, tr.Arriving, want)
	}
}

// ---- M5: firm investment, migration, annexation ----------------------------

func TestLevelsAreConstantReturns(t *testing.T) {
	rc := ProcessingRecipes[0]
	one := NewFirm(rc.Firm, G(rc.Output), rc.TFP, rc.Factors, 1e9)
	two := NewFirm(rc.Firm, G(rc.Output), rc.TFP, rc.Factors, 1e9)
	two.Levels = 2
	prices := testPrices()
	one.Plan(prices, 1)
	two.Plan(prices, 1)
	if math.Abs(two.PlanLabor-2*one.PlanLabor) > 1e-9*one.PlanLabor {
		t.Errorf("two levels plan %v labor, one level %v", two.PlanLabor, one.PlanLabor)
	}
	for _, f := range []*Firm{one, two} {
		f.Labor = f.PlanLabor
		copy(f.Inputs, f.PlanQty)
	}
	if q1, q2 := one.Produce(), two.Produce(); math.Abs(q2-2*q1) > 1e-9*q1 {
		t.Errorf("two levels with twice the inputs make %v, one level %v", q2, q1)
	}
}

func TestProfitableFirmsInvest(t *testing.T) {
	w := newTestGame(t)
	for i := 0; i < 600; i++ {
		w.Step()
	}
	grew := 0
	for _, r := range w.Regions {
		for _, f := range r.Firms {
			if (f.MaxLevels > 0 && f.Levels > f.MaxLevels) || (f.Levels < 1 && f.Enabled) { // a method not yet known has none
				t.Errorf("%s in %s has %d levels (max %d)", f.Name, r.Name, f.Levels, f.MaxLevels)
			}
			if f.Levels > 1 {
				grew++
			}
		}
	}
	if grew == 0 {
		t.Error("no firm expanded in 600 ticks")
	}
	still := newTestWorld(t, 1) // investment off
	for i := 0; i < 300; i++ {
		still.Step()
	}
	for _, r := range still.Regions {
		for _, f := range r.Firms {
			if f.Levels != 1 {
				t.Fatalf("with investment off, firm %s grew to %d levels", f.Name, f.Levels)
			}
		}
	}
}

func TestNationFundedExpansion(t *testing.T) {
	w := newTestGame(t)
	r := regionByName(w, "13")
	fi := 0
	for i, f := range r.Firms {
		if f.Name == "Bakery" {
			fi = i
		}
	}
	f := r.Firms[fi]
	f.MaxLevels = 99 // keep private investment from finishing first
	if err := w.Expand(r.ID, fi); err != nil {
		t.Fatal(err)
	}
	if err := w.Expand(r.ID, fi); !errors.Is(err, ErrExpanding) {
		t.Errorf("second expansion: %v", err)
	}
	levels, spent := f.Levels, 0.0
	for i := 0; i < 3000 && f.Levels == levels; i++ {
		w.Step()
		spent += r.Nation.Spending
	}
	if f.Levels != levels+1 || spent <= 0 {
		t.Errorf("bakery levels %d -> %d, nation spent %v", levels, f.Levels, spent)
	}
	f.Levels, f.Expansion = f.MaxLevels, nil
	if err := w.Expand(r.ID, fi); !errors.Is(err, ErrFirmMax) {
		t.Errorf("expansion beyond max: %v", err)
	}
}

func TestMovePeopleConserves(t *testing.T) {
	w := newTestGame(t)
	a, b := w.Regions[0], w.Regions[1]
	pop, money := w.Population(), w.Money()
	cashPerHead := a.Pops[0].Cash / a.Pops[0].Size
	MovePeople(a, b, 10)
	if relDiff(w.Population(), pop) > 1e-12 || relDiff(w.Money(), money) > 1e-12 {
		t.Errorf("moving people changed population or money")
	}
	if math.Abs(a.Population()-90) > 1e-9 || math.Abs(b.Population()-110) > 1e-9 {
		t.Errorf("populations %v, %v", a.Population(), b.Population())
	}
	if got := a.Pops[0].Cash / a.Pops[0].Size; math.Abs(got-cashPerHead) > 1e-9 {
		t.Errorf("stayers' cash per head changed %v -> %v", cashPerHead, got)
	}
}

func TestPeopleMoveTowardsHigherUtility(t *testing.T) {
	w := newTestGame(t)
	for i := 0; i < 60; i++ {
		w.Step()
	}
	target := regionByName(w, "22")
	for _, r := range w.Regions {
		r.UtilityEMA = 10
	}
	target.UtilityEMA = 30
	before := map[*Region]float64{}
	for _, r := range w.Regions {
		before[r] = r.Population()
	}
	w.Migrate()
	if target.Population() <= before[target] {
		t.Errorf("tile with the highest utility did not gain people")
	}
	for _, nb := range HexNeighbors(target.Row, target.Col, w.Rows, w.Cols) {
		r := w.Regions[nb[0]*w.Cols+nb[1]]
		if r.Population() >= before[r] {
			t.Errorf("neighbour %s did not lose people", r.Name)
		}
	}
}

func TestMigrationKeepsPeopleAndFloor(t *testing.T) {
	w := newTestGame(t)
	w.Demography = false
	pop := w.Population()
	for i := 0; i < 600; i++ {
		w.Step()
	}
	if relDiff(w.Population(), pop) > 1e-10 {
		t.Errorf("population %v, started with %v", w.Population(), pop)
	}
	lo, hi := math.Inf(1), 0.0
	for _, r := range w.Regions {
		lo, hi = math.Min(lo, r.Population()), math.Max(hi, r.Population())
	}
	if lo < minPopulation-1e-9 {
		t.Errorf("a tile emptied to %v people", lo)
	}
	if hi-lo < 10 {
		t.Errorf("nobody moved: populations %v..%v", lo, hi)
	}
}

func BenchmarkGameStep(b *testing.B) {
	w, _ := BuildGame(DefaultGame)
	for i := 0; i < 200; i++ {
		w.Step()
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.Step()
	}
}

// People cross borders as freely as they move within a nation, going where
// living is better; but slowly enough that a poorer nation isn't emptied
// into its neighbours within decades (a 10% gap once drew 15% of a tile's
// people a year): a few percent a year.
func TestEmigrationAsFreeAsMoving(t *testing.T) {
	w := newNeighbourGame(t)
	a, b := w.Nations[0].Regions[0], w.Nations[1].Regions[0]
	const gap, hop = 0.1, 1.0
	yearly := 1 - math.Pow(1-MigrationShare(a, b, gap, hop), TicksPerYear)
	if yearly > 0.06 || yearly < 0.01 {
		t.Errorf("a 10%% gap draws %.2f%% of a border tile's people a year, want 1-6%%", 100*yearly)
	}
	if home := MigrationShare(a, a, gap, hop); home != MigrationShare(a, b, gap, hop) {
		t.Errorf("moving within a nation (%v a day) and across a border (%v) differ", home, MigrationShare(a, b, gap, hop))
	}
}
