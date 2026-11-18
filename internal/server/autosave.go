package server

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rasmusbergpalm/mercury/game"
)

// AutosaveEvery is how often (in game days) the server saves each game, so
// that a restart (a redeploy, a crash) loses at most that much play.
const AutosaveEvery = 30

// A world of more than bigWorld tiles is saved at most every
// bigAutosaveGap: its save is tens of megabytes, and taking it holds the
// game up, which at 30 days a second would be every second.
const (
	bigWorld       = 100
	bigAutosaveGap = 30 * time.Second
)

// autosaveFile is a game as the server keeps it: a save plus what a restart
// needs to carry on where it was, players' seats included. It never leaves
// the server (exports are plain SaveFiles, without tokens).
type autosaveFile struct {
	SaveFile
	ID      string         `json:"id"`
	Created time.Time      `json:"created"`
	Players map[string]int `json:"players"` // token -> nation
	Running bool           `json:"running"`
	Speed   float64        `json:"speed"`
}

func (s *Server) autosaveDir() string { return filepath.Join(s.dataDir, "autosave") }

// autosave saves a game once it has passed its next save day. The caller
// holds g.mu; the file is written in the background.
func (s *Server) autosave(g *Game) {
	if s.dataDir == "" || g.w.Tick < g.nextAutosave {
		return
	}
	if len(g.w.Regions) > bigWorld && time.Since(g.lastAutosave) < bigAutosaveGap {
		return // a big world saves at most every so often: it takes a while
	}
	g.lastAutosave = time.Now()
	g.nextAutosave = (g.w.Tick/AutosaveEvery + 1) * AutosaveEvery
	f, err := g.saveFile()
	if err != nil {
		log.Printf("autosave %s: %v", g.ID, err)
		return
	}
	players := map[string]int{}
	for t, n := range g.players {
		players[t] = n
	}
	raw, err := json.Marshal(autosaveFile{*f, g.ID, g.Created, players, g.running, g.speed})
	if err != nil {
		log.Printf("autosave %s: %v", g.ID, err)
		return
	}
	tick, dir, path := g.w.Tick, s.autosaveDir(), filepath.Join(s.autosaveDir(), g.ID+".json")
	s.writes.Add(1)
	go func() {
		defer s.writes.Done()
		g.fileMu.Lock()
		defer g.fileMu.Unlock()
		if tick < g.savedTick || g.deleted {
			return // a newer save is already on disk, or the game is gone
		}
		if err := writeFileAtomic(dir, path, raw); err != nil {
			log.Printf("autosave %s: %v", g.ID, err)
			return
		}
		g.savedTick = tick
	}()
}

// Wait blocks until every game being prepared has opened (or failed) and
// every autosave in progress is on disk.
func (s *Server) Wait() {
	s.preps.Wait()
	s.writes.Wait()
}

// dropAutosave removes a deleted game's autosave.
func (s *Server) dropAutosave(g *Game) {
	if s.dataDir == "" {
		return
	}
	g.fileMu.Lock()
	defer g.fileMu.Unlock()
	g.deleted = true
	os.Remove(filepath.Join(s.autosaveDir(), g.ID+".json"))
}

// loadAutosaves brings back the games the server was running before it
// stopped, with their IDs, players, clocks and speeds.
func (s *Server) loadAutosaves() {
	if s.dataDir == "" {
		return
	}
	entries, err := os.ReadDir(s.autosaveDir())
	if err != nil {
		return
	}
	for _, e := range entries {
		id, ok := strings.CutSuffix(e.Name(), ".json")
		if !ok {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(s.autosaveDir(), e.Name()))
		if err != nil {
			continue
		}
		var f autosaveFile
		if err := json.Unmarshal(raw, &f); err != nil || f.ID != id || f.Snapshot == nil {
			log.Printf("autosave %s: unreadable, skipped", e.Name())
			continue
		}
		w, err := game.Restore(f.Snapshot)
		if err != nil {
			log.Printf("autosave %s: %v (skipped)", e.Name(), err)
			continue
		}
		g := newGame(f.Name, w, f.TurnDays)
		g.ID, g.Created, g.running = f.ID, f.Created, f.Running
		if speedOK(f.Speed) {
			g.speed = f.Speed
		}
		for t, n := range f.Players {
			if n >= 0 && n < len(w.Nations) && !w.Nations[n].Eliminated() {
				g.players[t] = n
				w.Nations[n].AI = false
			}
		}
		g.nextAutosave = (w.Tick/AutosaveEvery + 1) * AutosaveEvery
		g.savedTick = w.Tick
		s.mu.Lock()
		if len(s.games) < MaxGames {
			s.games[g.ID] = g
		}
		s.mu.Unlock()
		log.Printf("resumed %q (%s) on %s", g.Name, g.ID, w.Date().Format("2 January 2006"))
	}
}

func speedOK(x float64) bool {
	for _, s := range Speeds {
		if s == x {
			return true
		}
	}
	return false
}

// writeFileAtomic writes a file whole or not at all.
func writeFileAtomic(dir, path string, raw []byte) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// NumGames is how many games the server hosts.
func (s *Server) NumGames() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.games)
}
