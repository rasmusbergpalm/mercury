package main

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"encoding/json"

	"github.com/rasmusbergpalm/mercury/engine"
	"github.com/rasmusbergpalm/mercury/internal/server"
)

// The server ships two snapshots so that it is ready at once:
//
//   - threeCrowns, the default game as it opens (server.ClassicWorld with
//     threeCrownsSeed): settled, burnt in and warmed up, restored at start;
//   - settledClassic, the classic map settled (engine.SettledSnapshot), so
//     a new game on it opens after only its burn-in.
//
// TestShipped fails when the engine has moved on from them, and rewrites
// them with MERCURY_WRITE_SHIPPED=1.
var (
	//go:embed three-crowns.json.gz
	threeCrowns []byte
	//go:embed settled-classic.json.gz
	settledClassic []byte
)

// threeCrownsSeed draws the default game's rivals' temperaments.
const threeCrownsSeed = 1820

// classicCost is the transport cost of the server's classic games.
const classicCost = 1

// settledSpec is the game the settled snapshot is of: the classic map at the
// server's transport cost.
func settledSpec() engine.GameSpec {
	spec := engine.DefaultGame
	spec.TransportCost = classicCost
	return spec
}

func encodeSnapshot(s *engine.Snapshot) ([]byte, error) {
	var buf bytes.Buffer
	z := gzip.NewWriter(&buf)
	if err := json.NewEncoder(z).Encode(s); err != nil {
		return nil, err
	}
	if err := z.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func decodeSnapshot(b []byte) (*engine.Snapshot, error) {
	z, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	var s engine.Snapshot
	if err := json.NewDecoder(z).Decode(&s); err != nil {
		return nil, err
	}
	return &s, nil
}

// preloadSettled hands the shipped settled map to the engine.
func preloadSettled() error {
	s, err := decodeSnapshot(settledClassic)
	if err != nil {
		return err
	}
	return engine.PreloadSettled(s)
}

// shippedThreeCrowns restores the shipped default game.
func shippedThreeCrowns() (*engine.World, error) {
	s, err := decodeSnapshot(threeCrowns)
	if err != nil {
		return nil, err
	}
	return engine.Restore(s)
}

// freshThreeCrowns builds the default game as the shipped one was built.
func freshThreeCrowns() (*engine.World, error) {
	return server.ClassicWorld(classicCost, false, threeCrownsSeed)
}
