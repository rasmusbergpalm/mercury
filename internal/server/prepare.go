package server

import (
	"fmt"
	"strings"
	"time"

	"github.com/rasmusbergpalm/mercury/engine"
)

// A game on a generated map is prepared in the background: its world is
// settled (engine.SettleDays) before it opens, which takes a while on a big
// map. Meanwhile the lobby lists it with its progress, under the ID the game
// will have.

// preparing is a game whose world is being settled.
type preparing struct {
	id, name string
	created  time.Time
	mapName  string
	tiles    int
	nations  []engine.NationSpec
	turnDays int
	done     int // days settled so far (guarded by Server.mu)
	total    int
	err      error     // why it failed, if it did
	failed   time.Time // when (failures stay listed for a while)
}

// MaxPreparing is how many games may be prepared at once.
const MaxPreparing = 2

// failedShown is how long a game that failed to prepare stays in the lobby.
const failedShown = time.Minute

// MapChoice is what kind of map a new game is played on.
type MapChoice struct {
	Size    string // "classic" (the hand-made map) or one of engine.MapSizes
	Nations int    // 0: the size's default
	Seed    int64  // 0: a fresh one
	// Divided shares out all the land among the nations from the start, so
	// that there is no free land to take (engine.DivideLand).
	Divided bool
}

// PrepareGame starts a game on a generated map and returns its ID; it
// opens once its world has settled. The classic map opens at once.
func (s *Server) PrepareGame(name string, transportCost float64, turnDays int, m MapChoice) (string, error) {
	if m.Size == "" || m.Size == "classic" {
		g, err := s.createClassic(name, transportCost, turnDays, m.Divided)
		if err != nil {
			return "", err
		}
		return g.ID, nil
	}
	var spec engine.GameSpec
	var err error
	if m.Size == "world" { // the world in 1820, with its nations
		spec = engine.WorldGame()
	} else {
		var size *engine.MapSize
		for i := range engine.MapSizes {
			if engine.MapSizes[i].Name == m.Size {
				size = &engine.MapSizes[i]
			}
		}
		if size == nil {
			return "", fmt.Errorf("no map size %q", m.Size)
		}
		if m.Nations == 0 {
			m.Nations = size.Nations
		}
		if m.Seed == 0 {
			m.Seed = time.Now().UnixNano()
		}
		if spec, err = engine.GenerateGame(engine.MapOptions{Rows: size.Rows, Cols: size.Cols, Nations: m.Nations, Seed: m.Seed}); err != nil {
			return "", err
		}
	}
	if m.Divided {
		if spec, err = engine.DivideLand(spec); err != nil {
			return "", err
		}
	}
	spec.TransportCost, spec.AI = transportCost, true
	tiles := 0
	for _, row := range spec.Map {
		tiles += len(row) - strings.Count(row, string(rune(engine.Sea)))
	}
	p := &preparing{id: randomHex(4), name: name, created: time.Now(), mapName: m.Size, tiles: tiles,
		nations: spec.Nations, turnDays: turnDays, total: engine.PrepareDays(tiles)}
	s.mu.Lock()
	busy := 0
	for _, q := range s.preparing {
		if q.err == nil {
			busy++
		}
	}
	switch {
	case busy >= MaxPreparing:
		s.mu.Unlock()
		return "", fmt.Errorf("%d games are being prepared already: try again in a minute", busy)
	case len(s.games)+len(s.preparing) >= MaxGames:
		s.mu.Unlock()
		return "", fmt.Errorf("the server already hosts %d games", MaxGames)
	}
	s.preparing[p.id] = p
	s.mu.Unlock()

	s.preps.Add(1)
	go func() {
		defer s.preps.Done()
		w, err := engine.BuildGameProgress(spec, func(done, total int) {
			s.mu.Lock()
			p.done, p.total = done, total
			s.mu.Unlock()
		})
		if err == nil {
			for i := 0; i < Warmup; i++ {
				w.Step()
			}
			_, err = s.addAs(p.id, p.name, w, p.turnDays)
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if err != nil {
			p.err, p.failed = err, time.Now()
			return
		}
		delete(s.preparing, p.id)
	}()
	return p.id, nil
}

// preparingSummaries lists the games being prepared (and recent failures,
// dropping older ones). The caller holds s.mu.
func (s *Server) preparingSummaries() []GameSummary {
	var out []GameSummary
	for id, p := range s.preparing {
		if p.err != nil && time.Since(p.failed) > failedShown {
			delete(s.preparing, id)
			continue
		}
		share := 0.0
		if p.total > 0 {
			share = float64(p.done) / float64(p.total)
		}
		g := GameSummary{ID: p.id, Name: p.name, Turn: p.turnDays, Date: "being prepared",
			Map: p.mapName, Tiles: p.tiles, Preparing: &share, created: p.created}
		if p.err != nil {
			g.Error = p.err.Error()
		}
		for i, n := range p.nations {
			g.Nations = append(g.Nations, NationSummary{ID: i, Name: n.Name, Color: n.Color, Controller: "ai", Tiles: 0})
		}
		out = append(out, g)
	}
	return out
}
