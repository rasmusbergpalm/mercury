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
