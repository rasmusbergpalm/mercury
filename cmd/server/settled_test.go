package main

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/rasmusbergpalm/mercury/game"
)

// The shipped snapshots are what the engine makes now (with
// MERCURY_WRITE_SHIPPED=1 they are rewritten, after a change to the
// economy), and a game built on the shipped settled map is the game built
// by settling it.
func TestShipped(t *testing.T) {
	if testing.Short() {
		t.Skip("settles and burns in the classic map")
	}
	write := os.Getenv("MERCURY_WRITE_SHIPPED") != ""
	check := func(file string, shipped *[]byte, s *game.Snapshot) {
		b, err := encodeSnapshot(s)
		if err != nil {
			t.Fatal(err)
		}
		if write {
			if err := os.WriteFile(file, b, 0o644); err != nil {
				t.Fatal(err)
			}
			*shipped = b
		}
		if !bytes.Equal(b, *shipped) {
			t.Errorf("cmd/server/%s is stale: rewrite it with MERCURY_WRITE_SHIPPED=1 go test ./cmd/server", file)
		}
	}
	snap := func(w *game.World) *game.Snapshot {
		s, err := w.Snapshot()
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	fresh, err := freshThreeCrowns() // settles the map, and caches it
	if err != nil {
		t.Fatal(err)
	}
	check("three-crowns.json.gz", &threeCrowns, snap(fresh))
	settled, err := game.SettledSnapshot(settledSpec())
	if err != nil {
		t.Fatal(err)
	}
	check("settled-classic.json.gz", &settledClassic, settled)

	restored, err := shippedThreeCrowns()
	if err != nil {
		t.Fatal(err)
	}
	if err := preloadSettled(); err != nil {
		t.Fatal(err)
	}
	loaded, err := freshThreeCrowns() // now from the shipped settled map
	if err != nil {
		t.Fatal(err)
	}
	same := func(a, b *game.World) bool {
		x, _ := json.Marshal(snap(a))
		y, _ := json.Marshal(snap(b))
		return bytes.Equal(x, y)
	}
	if !same(fresh, loaded) {
		t.Error("a game built on the shipped settled map differs from one built by settling it")
	}
	for i := 0; i < 60; i++ { // and the shipped game plays on as the fresh one does
		fresh.Step()
		restored.Step()
	}
	if !same(fresh, restored) {
		t.Error("the shipped Three Crowns plays differently from a fresh one")
	}
}
