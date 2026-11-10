package engine

import (
	"math"
	"testing"
)

// Every tile gains from its size: no tile starts as a city, and a tile that
// fills with people becomes more productive in industry and services.
func TestSizeBonusGrowsWithPopulation(t *testing.T) {
	w := newTestGame(t)
	r := regionByName(w, "11")
	for _, x := range w.Regions {
		if !x.Urban || w.IsCity(x) {
			t.Fatalf("tile %s: urban %v, city %v at the start", x.Name, x.Urban, w.IsCity(x))
		}
	}
	if b := r.SizeBonus(); math.Abs(b-math.Pow(r.Population()/sizeBonusPop, sizeBonusExp)) > 1e-9 {
		t.Errorf("bonus %v for %v people", b, r.Population())
	}
	small := r.SizeBonus()
	addPeople(r, 600)
	r.updateAgglomeration()
	if big := r.SizeBonus(); big <= small {
		t.Errorf("bigger tile, bonus %v <= %v", big, small)
	}
	if !w.IsCity(r) {
		t.Errorf("%v people is not a city", r.Population())
	}
	for _, f := range r.Firms {
		want := r.SizeBonus()
		if f.Sector == Primary {
			want = 1
		}
		if f.Agglom != want {
			t.Errorf("%s (%v) agglomeration %v, want %v", f.Name, f.Sector, f.Agglom, want)
		}
	}
}

func TestPopulationFollowsFood(t *testing.T) {
	w := newTestGame(t)
	r := regionByName(w, "22")
	for _, p := range r.Pops {
		for g := range p.Consumed {
			p.Consumed[g] = 0
		}
	}
	if g := r.GrowthRate(); g >= 0 {
		t.Errorf("starving tile grows %v a year", g)
	}
	for _, p := range r.Pops {
		for _, nd := range Needs {
			for _, g := range nd.Goods {
				p.Consumed[g] = p.Size
			}
		}
	}
	if g := r.GrowthRate(); g <= 0 || g > popGrowthMax {
		t.Errorf("well-fed tile grows %v a year", g)
	}
	w.Migration = false
	pop := w.Population()
	for i := 0; i < int(TicksPerYear); i++ {
		w.Step()
	}
	if w.Population() <= pop {
		t.Errorf("population %v after a year, started with %v", w.Population(), pop)
	}
}

func laborShares(w *World) [3]float64 {
	var l [3]float64
	tot := 0.0
	for _, r := range w.Regions {
		for _, f := range r.Firms {
			l[f.Sector] += f.Labor
			tot += f.Labor
		}
	}
	for i := range l {
		l[i] /= tot
	}
	return l
}

// Industrial technology moves workers off the land into town jobs (industry
// and services).
func TestIndustrializationMovesLabor(t *testing.T) {
	if testing.Short() {
		t.Skip("long")
	}
	run := func(industrial bool) [3]float64 {
		w := newTestGame(t)
		w.Migration, w.Demography = false, false
		if industrial {
			for _, n := range w.Nations {
				for i := range Techs {
					n.Known[i] = true
				}
				n.applyTech()
			}
			w.refreshAllHops()
		}
		for i := 0; i < 1500; i++ {
			w.Step()
		}
		return laborShares(w)
	}
	pre, ind := run(false), run(true)
	t.Logf("labor shares (industry, primary, service): pre-industrial %.2f, industrial %.2f", pre, ind)
	// (by 2 points or more: it was 5 when the pre-industrial world kept
	// workers making clay nobody bought, before firms counted their unsold
	// stock; with every tech, kilns buy it, and those workers stay)
	if !(ind[Primary] < pre[Primary]-0.02 && ind[Industry]+ind[Service] > pre[Industry]+pre[Service]) {
		t.Errorf("industrial techs should move labor off the land")
	}
}

func TestResearchQueueAndDiffusion(t *testing.T) {
	w := newTestGame(t)
	n, o := w.Nations[0], w.Nations[1]
	vit, cer := TechID("Viticulture"), TechID("Ceramics")
	if err := n.SetQueue([]int{vit, cer}); err != nil {
		t.Fatal(err)
	}
	n.Researching = -1
	n.pickResearch()
	if n.Researching != vit || len(n.Queue) != 1 {
		t.Fatalf("researching %d, queue %v", n.Researching, n.Queue)
	}
	n.Known[vit] = true
	n.pickResearch()
	if n.Researching != cer {
		t.Errorf("after Viticulture, researching %d", n.Researching)
	}
	if err := n.SetQueue([]int{vit}); err == nil {
		t.Error("queueing a known tech accepted")
	}
	// ideas travel with trade
	n.IncomeEMA = 100
	if d := n.Discount(cer, w.Nations); d != 0 {
		t.Errorf("discount %v with no partner knowing it", d)
	}
	o.Known[cer] = true
	if d := n.Discount(cer, w.Nations); d != 0 {
		t.Errorf("discount %v with no trade", d)
	}
	n.TradeWith[o.ID] = 50 // half the income: full discount
	if d := n.Discount(cer, w.Nations); math.Abs(d-maxDiffusion) > 1e-12 {
		t.Errorf("discount %v, want %v", d, maxDiffusion)
	}
	if c := n.TechCost(cer, w.Nations); math.Abs(c-Techs[cer].Cost*(1-maxDiffusion)) > 1e-9 {
		t.Errorf("cost %v", c)
	}
}

func TestTradeIsTrackedBetweenNations(t *testing.T) {
	w := newNeighbourGame(t)
	for i := 0; i < 200; i++ {
		w.Step()
	}
	if w.Nations[0].TradeWith[1] <= 0 || w.Nations[0].TradeWith[1] != w.Nations[1].TradeWith[0] {
		t.Errorf("trade between neighbours %v / %v", w.Nations[0].TradeWith[1], w.Nations[1].TradeWith[0])
	}
}

func TestEventsAreRecorded(t *testing.T) {
	w := newNeighbourGame(t)
	n := w.Nations[0]
	n.SetResearch(TechID("Wagons"))
	n.ResearchPoints = Techs[TechID("Wagons")].Cost
	w.Step()
	w.DeclareWar(0, 1)
	kinds := map[string]bool{}
	for _, e := range w.EventsSince(0, 0) {
		kinds[e.Kind] = true
	}
	if !kinds["tech"] || !kinds["war"] {
		t.Errorf("events %+v", w.Events)
	}
	if got := w.EventsSince(w.Tick+1, -1); len(got) != 0 {
		t.Errorf("future events %v", got)
	}
}

func TestCalendar(t *testing.T) {
	if d := Date(0); d.Year() != 1820 || d.YearDay() != 1 {
		t.Errorf("tick 0 is %v", d)
	}
	if d := Date(365); d.Year() != 1821 || d.YearDay() != 1 {
		t.Errorf("tick 365 is %v", d)
	}
	if d := Date(364); d.Month() != 12 || d.Day() != 31 { // 1820 was a leap year
		t.Errorf("tick 364 is %v", d)
	}
	if d := Date(59); d.Month() != 3 || d.Day() != 1 {
		t.Errorf("tick 59 is %v, want 1 March (no 29 February)", d)
	}
}

// A game in the middle of a war, with armies marching and occupying,
// continues exactly after save and load.
func TestSaveLoadMidWar(t *testing.T) {
	w := newNeighbourGame(t)
	for _, n := range w.Nations {
		n.AI = true
	}
	for i := 0; i < 100; i++ {
		w.Step()
	}
	w.DeclareWar(0, 1)
	var a *Army
	for _, x := range w.ArmiesOf(w.Nations[0]) { // the computer may have one there already
		if x.Region == regionByName(w, "00").ID {
			a = x
		}
	}
	if a == nil {
		var err error
		if a, err = w.RaiseArmy(0, regionByName(w, "00").ID, 0); err != nil {
			t.Fatal(err)
		}
	}
	a.Men = 30
	w.MoveArmy(a.ID, regionByName(w, "01").ID)
	for i := 0; i < 30; i++ {
		w.Step()
	}
	r := roundTrip(t, w)
	if len(r.Armies) != len(w.Armies) || r.NextArmyID != w.NextArmyID || len(r.Events) != len(w.Events) {
		t.Fatalf("restored %d armies, %d events", len(r.Armies), len(r.Events))
	}
	for i := 0; i < 300; i++ {
		w.Step()
		r.Step()
	}
	if w.Money() != r.Money() || w.Soldiers() != r.Soldiers() || len(w.Armies) != len(r.Armies) {
		t.Fatal("war diverged after load")
	}
	for i, reg := range w.Regions {
		if (reg.Siege == nil) != (r.Regions[i].Siege == nil) || reg.Population() != r.Regions[i].Population() {
			t.Fatalf("tile %s diverged after load", reg.Name)
		}
	}
}
