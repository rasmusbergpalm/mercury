package engine

import (
	"math"
	"testing"
)

// A new game opens on a settled world: prices have left their starting £1,
// people have moved towards the work (tiles differ), no one was born or
// died, and no money was made or lost. Its markets are calm: the first day
// moves prices little.
func TestGamesStartSettled(t *testing.T) {
	raw, err := buildGame(DefaultGame, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	w, err := BuildGame(DefaultGame)
	if err != nil {
		t.Fatal(err)
	}
	if w.Tick != 0 || len(w.Nations) != len(DefaultGame.Nations) {
		t.Fatalf("tick %d, %d nations", w.Tick, len(w.Nations))
	}
	if relDiff(w.Population(), raw.Population()) > 1e-9 || relDiff(w.Money(), raw.Money()) > 1e-9 {
		t.Errorf("settling changed population %v -> %v or money %v -> %v", raw.Population(), w.Population(), raw.Money(), w.Money())
	}
	lo, hi := math.Inf(1), 0.0
	for _, r := range w.Regions {
		lo, hi = math.Min(lo, r.Population()), math.Max(hi, r.Population())
	}
	if hi < 1.2*lo {
		t.Errorf("no one moved: tiles from %v to %v people", lo, hi)
	}
	bread := G("bread")
	before := make([]float64, len(w.Regions))
	moved := 0
	for i, r := range w.Regions {
		before[i] = r.Markets[bread].Price
		if math.Abs(before[i]-1) > 0.05 {
			moved++
		}
	}
	if moved == 0 {
		t.Error("bread is still at its starting price everywhere")
	}
	w.Step()
	for i, r := range w.Regions {
		if p := r.Markets[bread].Price; p > 1.15*before[i] || p < before[i]/1.15 {
			t.Errorf("tile %s: bread %v -> %v on the first day", r.Name, before[i], p)
		}
	}
}

// Bug: the world settled without nations, so a game opened with no state
// demand at all. When the nations' armies, taxes and services came in, the
// prices of what they buy (arms above all, which nobody had bought before)
// lurched for months. A burn-in with the nations in place lets them settle,
// and the game still opens on 1 January 1820 with nothing played.
func TestGameOpensBurntIn(t *testing.T) {
	if testing.Short() {
		t.Skip("settles a map")
	}
	spec := DefaultGame
	spec.AI = true // as in a server game, until players join
	w, err := BuildGame(spec)
	if err != nil {
		t.Fatal(err)
	}
	if w.Tick != 0 || len(w.Events) > 0 || len(w.History.Ticks) > 0 {
		t.Fatalf("the game opens at tick %d with %d events and %d history samples", w.Tick, len(w.Events), len(w.History.Ticks))
	}
	for _, n := range w.Nations {
		known := 0
		for _, k := range n.Known {
			if k {
				known++
			}
		}
		if known > 0 || n.ResearchPoints > 0 || n.Coalition != 0 || w.atWar(n) {
			t.Errorf("%s opens with %d techs, %v research points, coalition %d, at war %v", n.Name, known, n.ResearchPoints, n.Coalition, w.atWar(n))
		}
		if len(w.ArmiesOf(n)) == 0 {
			t.Errorf("%s opens with no standing army", n.Name)
		}
	}
	arms := G("arms")
	start := make([]float64, len(w.Nations))
	for i, n := range w.Nations {
		p, v := w.NationPrices(n)
		if v[arms] <= 0 {
			t.Fatalf("%s: no arms change hands at the start", n.Name)
		}
		start[i] = p[arms]
	}
	for d := 0; d < 120; d++ {
		w.Step()
		if d%30 != 29 {
			continue
		}
		for i, n := range w.Nations {
			if p, v := w.NationPrices(n); v[arms] > 0 && (p[arms] > 2*start[i] || p[arms] < start[i]/2) {
				t.Errorf("day %d: %s's arms price moved from %.2f to %.2f", d+1, n.Name, start[i], p[arms])
			}
		}
	}
}
