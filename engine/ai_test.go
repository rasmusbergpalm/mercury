package engine

import (
	"math"
	"testing"
)

func newAIGame(t *testing.T) *World {
	t.Helper()
	spec := DefaultGame
	spec.AI = true
	w, err := BuildGame(spec)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

// Left alone for 30 years, computer nations grow: they occupy land, build,
// research and keep armies, and the world stays consistent.
func TestAINationsDevelop(t *testing.T) {
	if testing.Short() {
		t.Skip("long")
	}
	w := newAIGame(t)
	money := w.Money()
	for i := 0; i < 30*int(TicksPerYear); i++ {
		w.Step()
	}
	if relDiff(w.Money(), money) > 1e-9 {
		t.Fatal("money not conserved")
	}
	unowned, roads, armies := 0, 0, 0
	for _, r := range w.Regions {
		if r.Nation == nil {
			unowned++
		}
		roads += r.Infra
		if math.IsNaN(r.Utility()) {
			t.Fatalf("tile %s utility NaN", r.Name)
		}
	}
	for _, n := range w.Nations {
		known := 0
		for _, k := range n.Known {
			if k {
				known++
			}
		}
		t.Logf("%-9s tiles %2d techs %2d armies %d", n.Name, len(n.Regions), known, len(w.ArmiesOf(n)))
		if !n.Eliminated() && known == 0 {
			t.Errorf("%s researched nothing in 30 years", n.Name)
		}
		armies += len(w.ArmiesOf(n))
	}
	branches := map[string]bool{}
	for _, n := range w.Nations {
		for i, k := range n.Known {
			if k {
				branches[Techs[i].Branch] = true
			}
		}
	}
	if len(branches) < 3 {
		t.Errorf("research reached only branches %v", branches)
	}
	if unowned > 6 || roads == 0 || armies == 0 {
		t.Errorf("%d tiles still unowned, %d road levels, %d armies", unowned, roads, armies)
	}
}

func TestAIIsDeterministic(t *testing.T) {
	a, b := newAIGame(t), newAIGame(t)
	for i := 0; i < 800; i++ {
		a.Step()
		b.Step()
	}
	for i, n := range a.Nations {
		m := b.Nations[i]
		if n.Treasury != m.Treasury || len(n.Regions) != len(m.Regions) || a.Soldiers() != b.Soldiers() {
			t.Fatalf("%s differs between identical runs", n.Name)
		}
	}
}

func TestAIArmsWhenThreatened(t *testing.T) {
	w := newNeighbourGame(t)
	a, b := w.Nations[0], w.Nations[1]
	a.AI = true
	big, _ := w.RaiseArmy(b.ID, regionByName(w, "01").ID, 0)
	big.Men = 1000
	w.think(a)
	if a.Drill != MaxDrill || a.Budget[Defense] < 0.4 { // a war footing, leaned by temperament
		t.Errorf("AI facing a much bigger neighbour kept budget %v, drill %v", a.Budget, a.Drill)
	}
	if len(w.ArmiesOf(a)) == 0 {
		t.Error("AI raised no army")
	}
	w.removeArmy(big)
	w.think(a)
	if a.Drill == MaxDrill {
		t.Error("AI stayed on a war footing with no threat")
	}
}

func TestPlayersNationsAreLeftAlone(t *testing.T) {
	w := newAIGame(t)
	human := w.Nations[1]
	human.AI = false
	budget := human.Budget
	for i := 0; i < 400; i++ {
		w.Step()
	}
	if human.Budget != budget || len(w.ArmiesOf(human)) != 0 {
		t.Error("the AI acted for a player's nation")
	}
	for _, r := range human.Regions {
		if r.Project != nil {
			t.Error("the AI started a project for a player's nation")
		}
	}
}

// Computer nations differ in temperament, by nation and by game, but a given
// game's are fixed.
func TestTemperamentsVary(t *testing.T) {
	spec := DefaultGame
	a, b := drawTemperament(1, 0), drawTemperament(1, 1)
	if a == b {
		t.Error("two nations of a game share a temperament")
	}
	if drawTemperament(2, 0) == a {
		t.Error("two games draw the same temperament")
	}
	spec.Seed = 1
	w, err := BuildGame(spec)
	if err != nil {
		t.Fatal(err)
	}
	if w.Nations[0].Temper != a {
		t.Error("the game's first nation doesn't have its seed's temperament")
	}
	if d := a.Describe(); d == "" {
		t.Error("no description")
	}
	// the AI's whims are deterministic: same tick, same answer
	if w.noise(w.Nations[0], 3) != w.noise(w.Nations[0], 3) || w.noise(w.Nations[0], 3) == w.noise(w.Nations[1], 3) {
		t.Error("noise is not a deterministic function of nation and decision")
	}
}
