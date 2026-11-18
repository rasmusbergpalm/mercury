package game

import (
	"math"
	"testing"
)

// Administration collects the taxes: unfunded, a tile pays only what tax
// farmers bring in.
func TestAdministrationCollectsTaxes(t *testing.T) {
	run := func(funded bool) float64 {
		w := newQuietGame(t)
		n := w.Nations[0]
		if !funded {
			for _, r := range n.Regions {
				w.SetServiceFunding(r.ID, Administration, 0)
			}
		}
		for i := 0; i < 60; i++ {
			w.Step()
		}
		return n.IncomeEMA
	}
	with, without := run(true), run(false)
	if without > (taxFarmed+0.15)*with || without < (taxFarmed-0.1)*with { // (its would-be officials earn taxable wages instead)
		t.Errorf("income with administration %v, without %v", with, without)
	}
}

// Far tiles need more officials than the capital.
func TestDistantTilesNeedMoreOfficials(t *testing.T) {
	w := newQuietGame(t)
	n := w.Nations[0]
	c := n.capital()
	r := regionByName(w, "22")
	w.Annex(r.ID, n.ID)
	perHead := func(x *Region) float64 { return w.ServiceNeed(x, Administration) / x.Population() }
	if hops := w.hopsFromCapital(r); hops < 2 || perHead(r) <= perHead(c) {
		t.Errorf("tile %d hexes away needs %v officials a head, the capital %v", hops, perHead(r), perHead(c))
	}
}

// Schools raise the educated share towards enrolment, and with it the
// productivity of industry and services.
func TestSchoolsEducate(t *testing.T) {
	w := newQuietGame(t)
	r := regionByName(w, "11")
	f := r.Service(Schools)
	f.Levels = 4
	w.SetServiceFunding(r.ID, Schools, 4)
	e0 := r.Educated
	for i := 0; i < 3*365; i++ {
		w.Step()
	}
	if r.Educated <= e0+0.05 {
		t.Errorf("three years of schools: educated %v -> %v", e0, r.Educated)
	}
	for _, x := range r.Firms {
		if x.Sector == Industry && math.Abs(x.EduMult-(1+EducationBonus*r.Educated)) > 1e-3 { // set the morning before
			t.Errorf("%s: schooling bonus %v for educated share %v", x.Name, x.EduMult, r.Educated)
		}
	}
}

// Crowding slows growth unless public health covers it.
func TestPublicHealthCoversCrowding(t *testing.T) {
	w := newQuietGame(t)
	r := regionByName(w, "11")
	addPeople(r, 400)
	w.Step()
	if r.Crowding() <= 0 || r.CrowdingPenalty() <= 0 {
		t.Fatalf("a tile of %v people: crowding %v, penalty %v", r.Population(), r.Crowding(), r.CrowdingPenalty())
	}
	f := r.Service(Health)
	f.Levels = serviceMaxLevels
	w.SetServiceFunding(r.ID, Health, f.Levels)
	for i := 0; i < 5; i++ {
		w.Step()
	}
	if r.Healthy < 0.99 || r.CrowdingPenalty() > 1e-9 {
		t.Errorf("with health staff: covered %v, penalty %v", r.Healthy, r.CrowdingPenalty())
	}
	if r.HealthBonus() < 0.99*HealthCover {
		t.Errorf("a covered tile's growth bonus %v, want %v", r.HealthBonus(), HealthCover)
	}
}

// Nations at war don't trade with each other.
func TestWarStopsTrade(t *testing.T) {
	w := newNeighbourGame(t)
	a, b := regionByName(w, "00"), regionByName(w, "01")
	w.DeclareWar(0, 1)
	for _, tr := range tradersBetween(w, a, b) {
		if _, blocked := tr.Border(); !blocked {
			t.Errorf("%s trades across the front", tr.Name())
		}
	}
}

// Without the rebate the state's money only leaves the treasury by spending,
// and money is still conserved.
func TestServicesConserveMoney(t *testing.T) {
	w := newAIGame(t)
	m := w.Money()
	for i := 0; i < 2*365; i++ {
		w.Step()
	}
	if relDiff(w.Money(), m) > 1e-9 {
		t.Errorf("money %v -> %v", m, w.Money())
	}
}

// Pensions pay the elderly a share of the average wage, straight to the
// people, in five steps.
func TestPensions(t *testing.T) {
	w := newQuietGame(t)
	n := w.Nations[0]
	if err := n.SetPension(MaxPension + 1); err == nil {
		t.Error("a pension level above the top accepted")
	}
	for i := 0; i < 30; i++ { // some taxes in the treasury first
		w.Step()
	}
	n.SetPension(2)                                                 // half the average wage
	want := n.population() * pensionerShare * n.averageWage() * 0.5 // at the day's opening wages, which the bill is reckoned at
	w.Step()
	if got := n.SpentOn[Pensions]; math.Abs(got-want) > 0.05*want {
		t.Errorf("pensions cost %v, want about %v", got, want)
	}
}

// Schooling spreads as schooled children replace unschooled adults: a tile
// that schools every child from 1820 is over half educated by 1850 (not
// nine in ten, as it was), and nine in ten only by about 1900.
func TestEducationSpreadsOverAGeneration(t *testing.T) {
	e := StartEducated
	at := map[int]float64{}
	for d := 1; d <= 80*TicksPerYear; d++ {
		e = educate(e, 1)
		if d%TicksPerYear == 0 {
			at[1820+d/TicksPerYear] = e
		}
	}
	if at[1850] < 0.45 || at[1850] > 0.65 {
		t.Errorf("educated in 1850: %.0f%%, want about half", 100*at[1850])
	}
	if at[1900] < 0.85 || at[1900] > 0.95 {
		t.Errorf("educated in 1900: %.0f%%, want about nine in ten", 100*at[1900])
	}
}

// Schooling matters far more to the universities than to the mills.
func TestEducationMattersMostToScience(t *testing.T) {
	w := newTestGame(t)
	r := w.Regions[0]
	mults := func(educated float64) (uni, mill float64) {
		r.Educated = educated
		r.UpdateAgglomeration()
		for _, f := range r.Firms {
			switch {
			case f.State && Public(f.Public) == University:
				uni = f.EduMult
			case f.Sector == Industry:
				mill = f.EduMult
			}
		}
		return
	}
	u0, m0 := mults(0)
	u1, m1 := mults(1)
	if u1/u0 < 2.5 {
		t.Errorf("a fully schooled tile's university does %.2f× an unschooled one's", u1/u0)
	}
	if m1/m0 > 1.2 || m1/m0 <= 1 {
		t.Errorf("a fully schooled tile's industry does %.2f× an unschooled one's", m1/m0)
	}
}

// Bug: the tile and State views showed a tile as crowded even when public
// health covered it. One figure now says what health does to growth: the
// bonus of coverage less uncovered crowding, negative when more is needed.
func TestHealthIsOneGrowthFigure(t *testing.T) {
	w := newTestGame(t)
	r := w.Regions[0]
	for i := 0; i < 20 && r.Crowding() < 1; i++ { // fully crowded
		for _, p := range r.Pops {
			p.Size *= 1.5
		}
	}
	if r.Crowding() < 0.999 {
		t.Fatalf("crowding %v", r.Crowding())
	}
	r.Healthy = 1
	if g := r.HealthGrowth(); math.Abs(g-HealthCover) > 1e-12 {
		t.Errorf("a crowded tile fully covered grows %v a year from health, want %v", g, HealthCover)
	}
	r.Healthy = 0
	if g := r.HealthGrowth(); g >= 0 {
		t.Errorf("a crowded tile with no public health grows %v a year from it, want less than nothing", g)
	}
}
