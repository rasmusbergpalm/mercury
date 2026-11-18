package game

import (
	"errors"
	"math"
	"testing"
)

func TestTechTreeIsWellFormed(t *testing.T) {
	branches := map[string]int{}
	seen := map[string]bool{}
	firms := map[string]bool{}
	for _, rc := range append(append(append(append([]Recipe(nil), RawRecipes...), ProcessingRecipes...), FullRecipes...), FullRawRecipes...) {
		firms[rc.Firm] = true
	}
	for k := Public(0); k < NumServices; k++ {
		firms[services[k].Firm] = true
	}
	for _, m := range MethodRecipes {
		if !firms[m.Base] {
			t.Errorf("method %s of unknown firm %q", m.Firm, m.Base)
		}
		firms[m.Firm] = true
	}
	for _, tech := range Techs {
		if seen[tech.Name] {
			t.Errorf("tech %q listed twice", tech.Name)
		}
		for _, req := range tech.Requires {
			if !seen[req] {
				t.Errorf("%s requires %q, which isn't listed before it", tech.Name, req)
			}
		}
		for f := range tech.FirmTFP {
			if !firms[f] {
				t.Errorf("%s boosts unknown firm %q", tech.Name, f)
			}
		}
		for _, f := range tech.EnableFirms {
			if !firms[f] {
				t.Errorf("%s enables unknown firm %q", tech.Name, f)
			}
		}
		if tech.Effect == "" || tech.Cost <= 0 {
			t.Errorf("%s lacks an effect or a cost", tech.Name)
		}
		seen[tech.Name] = true
		branches[tech.Branch]++
	}
	for _, b := range Branches {
		if branches[b] < 4 {
			t.Errorf("branch %s has %d techs, want at least 4", b, branches[b])
		}
	}
	if len(branches) != len(Branches) {
		t.Errorf("techs use branches %v", branches)
	}
	for _, l := range InfraLevels {
		if l.Tech != "" {
			TechID(l.Tech) // panics if unknown
		}
	}
}

func firmIn(r *Region, name string) *Firm {
	for _, f := range r.Firms {
		if f.Name == name {
			return f
		}
	}
	return nil
}

func TestGatedFirmsNeedTheirTech(t *testing.T) {
	w := newTestGame(t)
	m := nationByName(w, "Montara")
	own, unowned := regionByName(w, "00"), regionByName(w, "22")
	for i := 0; i < 50; i++ {
		w.Step()
	}
	for _, r := range []*Region{own, unowned} {
		k := firmIn(r, "Fertilizer works")
		if k.Enabled || k.Labor != 0 || k.LastOutput != 0 {
			t.Errorf("tile %s fertilizer works work without Fertilizers: labor %v output %v", r.Name, k.Labor, k.LastOutput)
		}
		if err := w.Expand(own.ID, firmIndex(own, "Fertilizer works")); !errors.Is(err, ErrFirmLocked) {
			t.Errorf("expanding a locked firm: %v", err)
		}
	}
	w.GrantTech(m.ID, "Fertilizers")
	for i := 0; i < 150; i++ { // its coal and clay may have to be imported
		w.Step()
	}
	if k := firmIn(own, "Fertilizer works"); !k.Enabled || k.LastOutput <= 0 {
		t.Errorf("fertilizer works idle after Fertilizers: enabled %v output %v", k.Enabled, k.LastOutput)
	}
	if firmIn(unowned, "Fertilizer works").Enabled {
		t.Error("an unowned tile's fertilizer works were unlocked by another nation's tech")
	}
	// the classic world has no gating
	classic := newTestWorld(t, 1)
	if f := firmIn(classic.Regions[0], "Kiln"); !f.Enabled {
		t.Error("classic world kiln is locked")
	}
	if firmIn(classic.Regions[0], "Textile factory") != nil {
		t.Error("classic world has industrial firms")
	}
}

func firmIndex(r *Region, name string) int {
	for i, f := range r.Firms {
		if f.Name == name {
			return i
		}
	}
	return -1
}

func TestConquestTransfersFirmGating(t *testing.T) {
	w := newNeighbourGame(t)
	w.GrantTech(0, "Fertilizers")
	tile := regionByName(w, "01")
	if firmIn(tile, "Fertilizer works").Enabled {
		t.Fatal("Beta's fertilizer works work without Fertilizers")
	}
	w.Annex(tile.ID, 0)
	if !firmIn(tile, "Fertilizer works").Enabled {
		t.Error("conquered tile's fertilizer works didn't get the conqueror's tech")
	}
}

func TestMilitaryAndPoliticsTechs(t *testing.T) {
	// occupation pace: Statistical bureaus speed it, the owner's Polygonal forts slow it
	progress := func(attacker, defender []string) float64 {
		w := newNeighbourGame(t)
		for _, g := range attacker {
			w.GrantTech(0, g)
		}
		for _, g := range defender {
			w.GrantTech(1, g)
		}
		w.DeclareWar(0, 1)
		a, _ := w.RaiseArmy(0, regionByName(w, "00").ID, 0)
		a.Men, a.Region = 5, regionByName(w, "01").ID
		for i := 0; i < 10; i++ {
			w.sieges()
		}
		return regionByName(w, "01").Siege.Progress
	}
	base := progress(nil, nil)
	if admin := progress([]string{"Statistical bureaus"}, nil); math.Abs(admin-1.5*base) > 1e-9 {
		t.Errorf("Administration: progress %v, want %v", admin, 1.5*base)
	}
	if fort := progress(nil, []string{"Polygonal forts"}); math.Abs(fort-base/1.5) > 1e-9 {
		t.Errorf("Polygonal forts: progress %v, want %v", fort, base/1.5)
	}
	// attack bonuses raise strength; desertion techs cut hunger losses
	w := newNeighbourGame(t)
	a, _ := w.RaiseArmy(0, regionByName(w, "00").ID, 0)
	a.Men = 10
	s0 := a.Strength()
	w.GrantTech(0, "Percussion caps")
	if got := a.Strength(); math.Abs(got-1.1*s0) > 1e-9 {
		t.Errorf("Percussion caps: strength %v, want %v", got, 1.1*s0)
	}
	s1 := a.Strength()
	w.GrantTech(0, "Shrapnel shells")
	if got := a.Strength(); math.Abs(got-1.1*s1) > 1e-9 {
		t.Errorf("Shrapnel shells: strength %v, want %v", got, 1.1*s1)
	}
	if m := w.Nations[0].desertionMult(); m != 1 {
		t.Errorf("desertion multiplier %v before Military logistics", m)
	}
	w.GrantTech(0, "Mass conscription")
	w.GrantTech(0, "Military logistics")
	if m := w.Nations[0].desertionMult(); m != 0.7 {
		t.Errorf("Military logistics: desertion multiplier %v", m)
	}
	if mob := w.Nations[0].Mobilization(); mob != 0.04 {
		t.Errorf("Mass conscription: mobilization %v", mob)
	}
}

func TestBureaucracyBuildsFaster(t *testing.T) {
	ticks := func(grant bool) int {
		w := newTestGame(t)
		for i := 0; i < 150; i++ {
			w.Step()
		}
		r := regionByName(w, "13")
		if grant {
			w.GrantTech(r.Nation.ID, "Statistical bureaus")
			w.GrantTech(r.Nation.ID, "Civil service reform")
		}
		w.Build(r.ID)
		start := w.Tick
		for r.Infra == 0 && w.Tick < start+500 {
			w.Step()
		}
		return w.Tick - start
	}
	if slow, fast := ticks(false), ticks(true); fast >= slow {
		t.Errorf("Bureaucracy: road took %d ticks, %d without", fast, slow)
	}
}

func TestScienceTechsSpeedResearch(t *testing.T) {
	points := func(grant bool) float64 {
		w := newTestGame(t)
		if grant {
			w.GrantTech(0, "Steam printing")
			w.GrantTech(0, "Research universities")
		}
		for i := 0; i < 300; i++ {
			w.Step()
		}
		n := w.Nations[0]
		total := n.ResearchPoints
		for i, k := range n.Known {
			if k && !(grant && (Techs[i].Name == "Steam printing" || Techs[i].Name == "Research universities")) {
				total += Techs[i].Cost
			}
		}
		return total
	}
	if base, boosted := points(false), points(true); boosted <= base {
		t.Errorf("science techs: %v research, %v without", boosted, base)
	}
}

func TestHexDistance(t *testing.T) {
	for _, c := range []struct{ r1, c1, r2, c2, want int }{
		{0, 0, 0, 0, 0}, {0, 0, 0, 5, 5}, {0, 0, 5, 0, 5}, {5, 0, 5, 5, 5}, {4, 0, 3, 5, 6}, {2, 2, 3, 2, 1},
	} {
		if got := HexDistance(c.r1, c.c1, c.r2, c.c2); got != c.want {
			t.Errorf("distance (%d,%d)-(%d,%d) = %d, want %d", c.r1, c.c1, c.r2, c.c2, got, c.want)
		}
	}
	for r := 0; r < 6; r++ {
		for c := 0; c < 6; c++ {
			for _, nb := range HexNeighbors(r, c, 6, 6) {
				if d := HexDistance(r, c, nb[0], nb[1]); d != 1 {
					t.Fatalf("neighbours (%d,%d)-%v at distance %d", r, c, nb, d)
				}
			}
		}
	}
}

func buildPortNow(t *testing.T, w *World, name string) *Region {
	t.Helper()
	r := regionByName(w, name)
	if err := w.BuildPort(r.ID); err != nil {
		t.Fatalf("port on %s: %v", name, err)
	}
	for i := 0; i < 600 && !r.Port; i++ {
		w.Step()
	}
	if !r.Port {
		t.Fatalf("port on %s not finished: %+v", name, r.Project)
	}
	return r
}

func TestPortRules(t *testing.T) {
	w := newTestGame(t)
	th := nationByName(w, "Thalassa")
	coast, inland := regionByName(w, "54"), regionByName(w, "43")
	if err := w.BuildPort(coast.ID); !errors.Is(err, ErrNeedTech) {
		t.Errorf("port without Clipper ships: %v", err)
	}
	w.GrantTech(th.ID, "Hydrographic charts")
	w.GrantTech(th.ID, "Clipper ships")
	if err := w.BuildPort(inland.ID); !errors.Is(err, ErrNotCoast) {
		t.Errorf("port inland: %v", err)
	}
	if err := w.BuildPort(regionByName(w, "50").ID); !errors.Is(err, ErrNoNation) {
		t.Errorf("port on an unowned tile: %v", err)
	}
	if err := w.BuildPort(coast.ID); err != nil {
		t.Fatal(err)
	}
	if err := w.BuildPort(coast.ID); !errors.Is(err, ErrBuilding) {
		t.Errorf("second project: %v", err)
	}
	for i := 0; i < 600 && !coast.Port; i++ {
		w.Step()
	}
	if err := w.BuildPort(coast.ID); !errors.Is(err, ErrHasPort) {
		t.Errorf("second port: %v", err)
	}
}

// Ports link distant coast tiles directly by sea.
func TestPortsOpenSeaRoutes(t *testing.T) {
	w := newTestGame(t)
	th, mo := nationByName(w, "Thalassa"), nationByName(w, "Montara")
	for _, n := range []*Nation{th, mo} {
		w.GrantTech(n.ID, "Hydrographic charts")
		w.GrantTech(n.ID, "Clipper ships")
	}
	// a coast tile at the far end of the south coast, for a second nation
	// (each of these small ones may have one port)
	w.Annex(regionByName(w, "50").ID, mo.ID)
	for i := 0; i < 100; i++ {
		w.Step()
	}
	traders := len(w.Traders)
	a := buildPortNow(t, w, "54")
	if len(w.Traders) != traders {
		t.Fatal("a lone port opened routes")
	}
	b := buildPortNow(t, w, "50")
	if got := len(w.Traders) - traders; got != 2*len(FullTradable) {
		t.Fatalf("second port added %d traders, want %d", got, 2*len(FullTradable))
	}
	want := portHandling + seaPerHex*4*th.coastMult()
	sea := 0
	for _, tr := range w.Traders {
		if !tr.Sea {
			continue
		}
		sea++
		if !((tr.Origin == a.Region && tr.Dest == b.Region) || (tr.Origin == b.Region && tr.Dest == a.Region)) {
			t.Fatalf("sea trader on the wrong route: %s", tr.Name())
		}
		if math.Abs(tr.Distance-want) > 1e-12 {
			t.Fatalf("sea hop %v, want %v", tr.Distance, want)
		}
	}
	if sea != 2*len(FullTradable) {
		t.Fatalf("%d sea traders", sea)
	}
	money := w.Money()
	shipped := 0.0
	for i := 0; i < 300; i++ {
		w.Step()
		for _, tr := range w.Traders {
			if tr.Sea {
				shipped += tr.Shipped
			}
		}
	}
	if shipped <= 0 {
		t.Error("nothing shipped by sea")
	}
	if relDiff(w.Money(), money) > 1e-10 {
		t.Error("ports broke money conservation")
	}
	// steamships make sea routes cheaper
	w.GrantTech(th.ID, "Canals")
	w.GrantTech(th.ID, "Locomotives")
	w.GrantTech(th.ID, "Steamships")
	for _, tr := range w.Traders {
		if tr.Sea && tr.Distance >= want {
			t.Fatalf("sea hop %v after Steamships, was %v", tr.Distance, want)
		}
	}
}

func TestSaveLoadWithPorts(t *testing.T) {
	w := newTestGame(t)
	th := nationByName(w, "Thalassa")
	mo, ve := nationByName(w, "Montara"), nationByName(w, "Verdania")
	for _, n := range []*Nation{th, mo, ve} { // one port each: the most these small nations may have
		w.GrantTech(n.ID, "Hydrographic charts")
		w.GrantTech(n.ID, "Clipper ships")
	}
	w.GrantTech(ve.ID, "Portland cement") // its kilns work on 44, which it gets
	w.Annex(regionByName(w, "50").ID, mo.ID)
	w.Annex(regionByName(w, "44").ID, ve.ID)
	buildPortNow(t, w, "54")
	buildPortNow(t, w, "50")
	w.BuildPort(regionByName(w, "44").ID) // under way when saved
	for i := 0; i < 20; i++ {
		w.Step()
	}
	r := roundTrip(t, w)
	if len(r.Traders) != len(w.Traders) || len(r.Ports) != 2 {
		t.Fatalf("restored %d traders, %d ports", len(r.Traders), len(r.Ports))
	}
	for i := 0; i < 300; i++ {
		w.Step()
		r.Step()
	}
	if w.Money() != r.Money() || len(w.Traders) != len(r.Traders) || len(w.Ports) != len(r.Ports) {
		t.Fatal("game with ports diverged after load")
	}
	for i, tr := range w.Traders {
		if tr.Shipped != r.Traders[i].Shipped {
			t.Fatalf("trader %s diverged after load", tr.Name())
		}
	}
	if !firmIn(regionByName(r, "44"), "Kiln").Enabled {
		t.Error("restored tile lost its unlocked firm")
	}
}

func TestCoastMovementTakesTheCheaperWay(t *testing.T) {
	w := newTestGame(t)
	th := nationByName(w, "Thalassa")
	coast, land := regionByName(w, "54"), regionByName(w, "43")
	if coast.MoveCost() != 0.25 || land.MoveCost() != 1 {
		t.Fatalf("base costs %v %v", coast.MoveCost(), land.MoveCost())
	}
	w.SetInfra(coast.ID, 1) // road: 0.6 by land, still 0.25 by sea
	if coast.MoveCost() != 0.25 {
		t.Errorf("coast with a road: %v", coast.MoveCost())
	}
	w.SetInfra(coast.ID, 3) // railway: 0.15 by land beats 0.25 by sea
	if coast.MoveCost() != 0.15 {
		t.Errorf("coast with a railway: %v", coast.MoveCost())
	}
	w.GrantTech(th.ID, "Hydrographic charts") // sea: 0.25 * 0.8 = 0.2 > 0.15
	if coast.MoveCost() != 0.15 {
		t.Errorf("railway coast after Hydrographic charts: %v", coast.MoveCost())
	}
	w.SetInfra(coast.ID, 0)
	if got := coast.MoveCost(); math.Abs(got-0.2) > 1e-12 {
		t.Errorf("coast after Hydrographic charts: %v", got)
	}
}

// When a player's research finishes with nothing queued, the next tech is
// picked provisionally and the player is asked to choose; a queue, the
// computer, or the player's choice means no question.
func TestFinishedResearchAsksThePlayer(t *testing.T) {
	w := newTestGame(t)
	finish := func(n *Nation) {
		n.pickResearch(w)
		n.ResearchPoints = n.TechCost(n.Researching, w)
		w.Step()
	}
	human, ai := w.Nations[0], w.Nations[1]
	human.AI, ai.AI = false, true
	finish(human)
	if !human.ChooseResearch || human.Researching < 0 {
		t.Fatalf("no question after a finished tech (researching %d)", human.Researching)
	}
	next := -1
	for i := range Techs {
		if human.CanResearch(i) && i != human.Researching {
			next = i
			break
		}
	}
	if err := human.SetResearch(next); err != nil || human.ChooseResearch {
		t.Fatalf("choosing didn't answer the question: %v", err)
	}
	for i := range Techs {
		if human.CanResearch(i) && i != human.Researching {
			human.SetQueue([]int{i})
			break
		}
	}
	finish(human)
	if human.ChooseResearch {
		t.Error("asked although the queue had the next tech")
	}
	finish(ai)
	if ai.ChooseResearch {
		t.Error("asked the computer")
	}
}

// A computer nation's tech order follows from the seed, not from the day a
// tech finishes: the same techs known draw the same next one any day.
func TestResearchDrawDoesNotDependOnTheDay(t *testing.T) {
	w := newTestGame(t)
	n := w.Nations[0]
	n.now = 100
	first := n.drawResearch(w)
	for _, day := range []int{101, 2000, 36500} {
		n.now = day
		if got := n.drawResearch(w); got != first {
			t.Errorf("day %d draws %s, day 100 %s", day, Techs[got].Name, Techs[first].Name)
		}
	}
}

// A nation catches up on a tech its neighbour knows in a second slot,
// alongside its own research, and learns it from the points that slot gets.
func TestCatchUpSlot(t *testing.T) {
	w := newNeighbourGame(t)
	a, b := w.Nations[0], w.Nations[1]
	masonry := TechID("Macadam roads")
	b.Known[masonry] = true
	a.SetResearch(TechID("Percussion caps"))
	a.pickCopy(w)
	if a.Copying != masonry {
		t.Fatalf("catching up on %d, want Macadam roads (%d)", a.Copying, masonry)
	}
	a.CopyPoints = 2 * Techs[masonry].Cost * float64(len(w.Regions)) / techCostTiles // more than it costs, whatever the discount
	w.Step()
	if !a.Known[masonry] {
		t.Error("Macadam roads not learned from the catch-up slot")
	}
	if a.Researching != TechID("Percussion caps") {
		t.Errorf("own research changed to %d", a.Researching)
	}
	b.pickCopy(w)
	if b.Copying >= 0 && !a.Known[b.Copying] {
		t.Errorf("Beta catches up on %s, which Alpha doesn't know", Techs[b.Copying].Name)
	}
	// an embargo (either way) closes the border to it
	b.Known[TechID("Percussion caps")] = true
	a.Researching, a.Copying = -1, -1
	a.Embargo[b.ID] = true
	if a.pickCopy(w); a.Copying >= 0 {
		t.Errorf("catching up on %s across an embargo", Techs[a.Copying].Name)
	}
	a.Embargo[b.ID], b.Embargo[a.ID] = false, true
	if a.pickCopy(w); a.Copying >= 0 {
		t.Errorf("catching up on %s across the neighbour's embargo", Techs[a.Copying].Name)
	}
}

// A nation facing a neighbour that could raise far more (its potential
// might: everyone it can call up, at its best doctrine) leans its research
// to military techs.
func TestMilitaryThreatLeansResearch(t *testing.T) {
	w := newNeighbourGame(t)
	a, b := w.Nations[0], w.Nations[1]
	share := func() float64 {
		mil := 0
		for k := 0; k < 200; k++ {
			a.seed = int64(k)
			if i := a.drawResearch(w); i >= 0 && Techs[i].Branch == BranchMilitary {
				mil++
			}
		}
		return float64(mil) / 200
	}
	even := share()
	// the neighbour can call up twice the share of its people, at a better doctrine
	w.GrantTech(b.ID, "Percussion caps")
	w.GrantTech(b.ID, "Mass conscription")
	if w.militaryThreat(a) < 2 {
		t.Fatalf("set-up: threat %v", w.militaryThreat(a))
	}
	if behind := share(); behind < 1.5*even {
		t.Errorf("military share of the draw %.2f behind, %.2f even: it should lean to arms", behind, even)
	}
}

// An embargo (either way) stops ideas at once: a catch-up already under way
// on a tech the embargoed neighbour knows stops, and the trade of before
// the embargo no longer cheapens that neighbour's techs.
func TestEmbargoStopsDiffusion(t *testing.T) {
	w := newNeighbourGame(t)
	a, b := w.Nations[0], w.Nations[1]
	masonry := TechID("Macadam roads")
	b.Known[masonry] = true
	a.TradeWith[b.ID] = 1e6 // a great deal of trade, before the embargo
	a.IncomeEMA = 1
	a.Researching = -1
	a.pickCopy(w)
	if a.Copying != masonry {
		t.Fatalf("set-up: catching up on %d, want Macadam roads", a.Copying)
	}
	if a.Discount(masonry, w.Nations) <= 0 {
		t.Fatal("set-up: no discount from a trading neighbour")
	}
	for _, embargo := range []func(bool){
		func(on bool) { a.Embargo[b.ID] = on },
		func(on bool) { b.Embargo[a.ID] = on },
	} {
		embargo(true)
		a.Copying = masonry
		a.pickCopy(w)
		if a.Copying == masonry {
			t.Error("still catching up on Macadam roads across an embargo")
		}
		if d := a.Discount(masonry, w.Nations); d != 0 {
			t.Errorf("discount %v across an embargo", d)
		}
		embargo(false)
	}
}
