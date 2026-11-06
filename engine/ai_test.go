package engine

import (
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

// Left alone, computer nations grow: they occupy land, build roads, expand
// firms and research, and the world stays consistent.
func TestAINationsDevelop(t *testing.T) {
	if testing.Short() {
		t.Skip("long")
	}
	w := newAIGame(t)
	money, pop := w.Money(), w.Population()
	for i := 0; i < 1500; i++ {
		w.Step()
	}
	if relDiff(w.Money(), money) > 1e-10 || relDiff(w.Population(), pop) > 1e-10 {
		t.Fatal("money or people not conserved")
	}
	for _, n := range w.Nations {
		roads, public, known := 0, 0, 0
		for _, r := range n.Regions {
			roads += r.Infra
			for _, f := range r.Firms {
				if f.Levels > 1 {
					public++
				}
			}
		}
		for _, k := range n.Known {
			if k {
				known++
			}
		}
		t.Logf("%-9s tiles %2d army %6.1f roads %d grown firms %d techs %d budget %v", n.Name, len(n.Regions), n.Army, roads, public, known, n.Budget)
		if len(n.Regions) <= 4 || roads == 0 || known == 0 {
			t.Errorf("%s did not develop: %d tiles, %d road levels, %d techs", n.Name, len(n.Regions), roads, known)
		}
	}
}

func TestAIIsDeterministic(t *testing.T) {
	a, b := newAIGame(t), newAIGame(t)
	for i := 0; i < 400; i++ {
		a.Step()
		b.Step()
	}
	for i, n := range a.Nations {
		m := b.Nations[i]
		if n.Army != m.Army || n.Treasury != m.Treasury || len(n.Regions) != len(m.Regions) {
			t.Fatalf("%s differs between identical runs", n.Name)
		}
	}
}

func TestAIArmsWhenThreatened(t *testing.T) {
	w := newNeighbourGame(t)
	a, b := w.Nations[0], w.Nations[1]
	a.AI = true
	b.Army = 1000
	w.think(a)
	if a.Budget != aiWarBudget {
		t.Errorf("AI facing a much bigger neighbour kept budget %v", a.Budget)
	}
	b.Army = 0
	a.Army = 10
	w.think(a)
	if a.Budget == aiWarBudget {
		t.Error("AI stayed on a war footing with no threat")
	}
}

func TestPlayersNationsAreLeftAlone(t *testing.T) {
	w := newAIGame(t)
	human := w.Nations[1]
	human.AI = false
	budget := human.Budget
	for i := 0; i < 300; i++ {
		w.Step()
	}
	if human.Budget != budget || human.Campaign != nil {
		t.Error("the AI acted for a player's nation")
	}
	for _, r := range human.Regions {
		if r.Project != nil {
			t.Error("the AI started a project for a player's nation")
		}
	}
}
