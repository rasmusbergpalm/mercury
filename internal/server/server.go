// Package server hosts Mercury games over a JSON HTTP API: a lobby of games,
// players who each take a nation, server-side clocks, and saves.
package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rasmusbergpalm/mercury/engine"
)

// Server hosts many games.
type Server struct {
	mu      sync.Mutex
	games   map[string]*Game
	dataDir string // where save slots live; "" disables them
	mux     *http.ServeMux
}

// Limits.
const (
	MaxStep    = 1000
	MaxGames   = 50
	maxImport  = 64 << 20
	clockEvery = 100 * time.Millisecond
)

// New makes a server. static, if non-nil, is served at /; dataDir holds saves.
func New(static fs.FS, dataDir string) *Server {
	s := &Server{games: map[string]*Game{}, dataDir: dataDir, mux: http.NewServeMux()}
	m := s.mux
	m.HandleFunc("GET /api/games", s.handleListGames)
	m.HandleFunc("POST /api/games", s.handleCreateGame)
	m.HandleFunc("POST /api/games/import", s.handleImport)
	m.HandleFunc("DELETE /api/games/{gid}", s.game(s.handleDeleteGame))
	m.HandleFunc("GET /api/games/{gid}/world", s.game(s.handleWorld))
	m.HandleFunc("GET /api/games/{gid}/state", s.game(s.handleState))
	m.HandleFunc("GET /api/games/{gid}/regions/{id}", s.game(s.handleRegion))
	m.HandleFunc("POST /api/games/{gid}/step", s.game(s.handleStep))
	m.HandleFunc("POST /api/games/{gid}/clock", s.game(s.handleClock))
	m.HandleFunc("POST /api/games/{gid}/join", s.game(s.handleJoin))
	m.HandleFunc("POST /api/games/{gid}/leave", s.game(s.handleLeave))
	m.HandleFunc("POST /api/games/{gid}/save", s.game(s.handleSave))
	m.HandleFunc("GET /api/games/{gid}/export", s.game(s.handleExport))
	m.HandleFunc("POST /api/games/{gid}/end-turn", s.game(s.handleEndTurn))
	m.HandleFunc("GET /api/games/{gid}/events", s.game(s.handleEvents))
	m.HandleFunc("GET /api/games/{gid}/history/nations/{id}", s.game(s.handleNationHistory))
	m.HandleFunc("GET /api/games/{gid}/history/regions/{id}", s.game(s.handleRegionHistory))
	m.HandleFunc("GET /api/games/{gid}/history/armies/{id}", s.game(s.handleArmyHistory))
	for _, cmd := range []string{"move", "target", "upgrade", "disband"} {
		m.HandleFunc("POST /api/games/{gid}/armies/{aid}/"+cmd, s.game(s.armyCommand(cmd)))
	}
	for _, cmd := range []string{"tax", "tariff", "embargo", "research", "queue", "budget", "drill", "war", "peace", "armies"} {
		m.HandleFunc("POST /api/games/{gid}/nations/{nid}/"+cmd, s.game(s.nationCommand(cmd)))
	}
	for _, cmd := range []string{"build", "cancel", "port"} {
		m.HandleFunc("POST /api/games/{gid}/regions/{id}/"+cmd, s.game(s.regionCommand(cmd)))
	}
	m.HandleFunc("POST /api/games/{gid}/regions/{id}/firms/{firm}/expand", s.game(s.regionCommand("expand")))
	m.HandleFunc("GET /api/saves", s.handleListSaves)
	m.HandleFunc("POST /api/saves/{slot}/load", s.handleLoad)
	if static != nil {
		m.Handle("GET /", http.FileServerFS(static))
	}
	s.loadAutosaves()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.mux.ServeHTTP(w, r) }

// Warmup is how many days a new game runs before anyone sees it, so that it
// opens on a working economy rather than on starting prices.
const Warmup = 30

// CreateGame adds a game on the default map, run in by Warmup days.
// turnDays > 0 makes it turn-based.
func (s *Server) CreateGame(name string, transportCost float64, turnDays int) (*Game, error) {
	spec := engine.DefaultGame
	spec.TransportCost = transportCost
	spec.AI = true
	spec.Seed = time.Now().UnixNano() // each game's rivals have their own temperaments
	w, err := engine.BuildGame(spec)
	if err != nil {
		return nil, err
	}
	for i := 0; i < Warmup; i++ {
		w.Step()
	}
	return s.add(name, w, turnDays)
}

func (s *Server) add(name string, w *engine.World, turnDays int) (*Game, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.games) >= MaxGames {
		return nil, fmt.Errorf("the server already hosts %d games", MaxGames)
	}
	g := newGame(name, w, turnDays)
	s.games[g.ID] = g
	g.mu.Lock()
	s.autosave(g) // a new game is on disk from the start
	g.mu.Unlock()
	return g, nil
}

// Run drives every running game's clock until stop is closed.
func (s *Server) Run(stop <-chan struct{}) {
	t := time.NewTicker(clockEvery)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			for _, g := range s.list() {
				g.mu.Lock()
				g.advance(clockEvery.Seconds())
				s.autosave(g)
				g.mu.Unlock()
			}
		}
	}
}

func (s *Server) list() []*Game {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*Game, 0, len(s.games))
	for _, g := range s.games {
		out = append(out, g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created.Before(out[j].Created) })
	return out
}

// ---- helpers ----------------------------------------------------------------

type httpError struct {
	status int
	err    error
}

func (e httpError) Error() string { return e.err.Error() }

func fail(status int, err error) error { return httpError{status, err} }

func badRequest(format string, a ...any) error {
	return fail(http.StatusBadRequest, fmt.Errorf(format, a...))
}

// statusOf maps errors to HTTP statuses: explicit ones as given, permission
// errors to 403, and anything the game refuses to 409 Conflict.
func statusOf(err error) int {
	var he httpError
	switch {
	case errors.As(err, &he):
		return he.status
	case errors.Is(err, ErrNotPlayer), errors.Is(err, ErrNotInGame):
		return http.StatusForbidden
	}
	return http.StatusConflict
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func token(r *http.Request) string {
	if t := r.Header.Get("X-Player-Token"); t != "" {
		return t
	}
	return r.URL.Query().Get("token")
}

func queryFloat(r *http.Request, key string) (float64, error) {
	v, err := strconv.ParseFloat(r.URL.Query().Get(key), 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, badRequest("%s must be a number", key)
	}
	return v, nil
}

func queryIndex(r *http.Request, key string, n int) (int, error) {
	v, err := strconv.Atoi(r.URL.Query().Get(key))
	if err != nil || v < 0 || v >= n {
		return 0, badRequest("%s must be an index below %d", key, n)
	}
	return v, nil
}

func pathIndex(r *http.Request, key string, n int) (int, error) {
	v, err := strconv.Atoi(r.PathValue(key))
	if err != nil || v < 0 || v >= n {
		return 0, fail(http.StatusNotFound, fmt.Errorf("no such %s", key))
	}
	return v, nil
}

type gameHandler func(g *Game, w http.ResponseWriter, r *http.Request) error

// game resolves {gid}, locks the game for the handler and turns errors into
// HTTP responses.
func (s *Server) game(h gameHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		g := s.games[r.PathValue("gid")]
		s.mu.Unlock()
		if g == nil {
			http.Error(w, "no such game", http.StatusNotFound)
			return
		}
		g.mu.Lock()
		defer g.mu.Unlock()
		if err := h(g, w, r); err != nil {
			http.Error(w, err.Error(), statusOf(err))
		}
		s.autosave(g) // the handler may have advanced the game
	}
}

// recentEvents is how many recent events the state carries.
const recentEvents = 40

func (g *Game) state(r *http.Request) State {
	st := worldState(g.w)
	st.Game, st.Running, st.Speed, st.You = g.ID, g.running, g.speed, g.nationOf(token(r))
	if g.turnDays > 0 {
		st.Turn = &TurnState{Days: g.turnDays, Ready: []int{}}
		for n := range g.w.Nations {
			if g.ready[n] {
				st.Turn.Ready = append(st.Turn.Ready, n)
			}
		}
	}
	ev := g.w.EventsSince(0, st.You)
	if len(ev) > recentEvents {
		ev = ev[len(ev)-recentEvents:]
	}
	st.Events = ev
	return st
}

// ---- lobby ------------------------------------------------------------------

func (s *Server) handleListGames(w http.ResponseWriter, _ *http.Request) {
	out := []GameSummary{}
	for _, g := range s.list() {
		g.mu.Lock()
		out = append(out, g.summary())
		g.mu.Unlock()
	}
	writeJSON(w, out)
}

var nameRe = regexp.MustCompile(`^[\pL\pN _.,'!-]{1,40}$`)

func (s *Server) handleCreateGame(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		name = "New game"
	}
	if !nameRe.MatchString(name) {
		http.Error(w, "name must be 1-40 letters, digits, spaces or simple punctuation", http.StatusBadRequest)
		return
	}
	cost := 1.0
	if r.URL.Query().Has("cost") {
		var err error
		if cost, err = queryFloat(r, "cost"); err != nil || cost < 0 {
			http.Error(w, "cost must be a non-negative number", http.StatusBadRequest)
			return
		}
	}
	turn := 0
	if r.URL.Query().Has("turn") {
		var err error
		turn, err = strconv.Atoi(r.URL.Query().Get("turn"))
		ok := err == nil
		if ok {
			ok = false
			for _, t := range TurnLengths {
				ok = ok || t == turn
			}
		}
		if !ok {
			http.Error(w, fmt.Sprintf("turn must be one of %v days", TurnLengths), http.StatusBadRequest)
			return
		}
	}
	g, err := s.CreateGame(name, cost, turn)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, g.summary())
}

func (s *Server) handleDeleteGame(g *Game, w http.ResponseWriter, r *http.Request) error {
	if !g.mayControlClock(token(r)) {
		return ErrNotInGame
	}
	s.mu.Lock()
	delete(s.games, g.ID)
	s.mu.Unlock()
	s.dropAutosave(g)
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// ---- viewing ----------------------------------------------------------------

func (s *Server) handleWorld(g *Game, w http.ResponseWriter, _ *http.Request) error {
	writeJSON(w, worldInfo(g.w))
	return nil
}

func (s *Server) handleState(g *Game, w http.ResponseWriter, r *http.Request) error {
	writeJSON(w, g.state(r))
	return nil
}

func (s *Server) handleRegion(g *Game, w http.ResponseWriter, r *http.Request) error {
	id, err := pathIndex(r, "id", len(g.w.Regions))
	if err != nil {
		return err
	}
	writeJSON(w, regionDetail(g.w, id))
	return nil
}

// ---- clock and players --------------------------------------------------------

func (s *Server) handleStep(g *Game, w http.ResponseWriter, r *http.Request) error {
	if !g.mayControlClock(token(r)) {
		return ErrNotInGame
	}
	if g.turnDays > 0 && len(g.players) > 0 {
		return ErrTurnBased
	}
	n := 1
	if r.URL.Query().Has("n") {
		var err error
		if n, err = strconv.Atoi(r.URL.Query().Get("n")); err != nil || n < 1 || n > MaxStep {
			return badRequest("n must be an integer in [1, %d]", MaxStep)
		}
	}
	for i := 0; i < n; i++ {
		g.w.Step()
	}
	writeJSON(w, g.state(r))
	return nil
}

func (s *Server) handleClock(g *Game, w http.ResponseWriter, r *http.Request) error {
	if !g.mayControlClock(token(r)) {
		return ErrNotInGame
	}
	if g.turnDays > 0 {
		return ErrTurnBased
	}
	q := r.URL.Query()
	if q.Has("speed") {
		speed, err := queryFloat(r, "speed")
		if err != nil {
			return err
		}
		ok := false
		for _, s := range Speeds {
			ok = ok || s == speed
		}
		if !ok {
			return fail(http.StatusBadRequest, ErrBadSpeed)
		}
		g.speed = speed
	}
	if q.Has("running") {
		g.running = q.Get("running") == "true"
		g.carry = 0
	}
	writeJSON(w, g.state(r))
	return nil
}

func (s *Server) handleJoin(g *Game, w http.ResponseWriter, r *http.Request) error {
	nation, err := queryIndex(r, "nation", len(g.w.Nations))
	if err != nil {
		return err
	}
	t, err := g.join(nation)
	if err != nil {
		return err
	}
	writeJSON(w, map[string]any{"token": t, "nation": nation})
	return nil
}

func (s *Server) handleLeave(g *Game, w http.ResponseWriter, r *http.Request) error {
	if err := g.leave(token(r)); err != nil {
		return err
	}
	writeJSON(w, g.state(r))
	return nil
}

// ---- commands ---------------------------------------------------------------

// nationCommand runs a command on {nid}; only that nation's player may.
func (s *Server) nationCommand(cmd string) gameHandler {
	return func(g *Game, w http.ResponseWriter, r *http.Request) error {
		nid, err := pathIndex(r, "nid", len(g.w.Nations))
		if err != nil {
			return err
		}
		if g.nationOf(token(r)) != nid {
			return ErrNotPlayer
		}
		n, q := g.w.Nations[nid], r.URL.Query()
		switch cmd {
		case "tax":
			rate, err := queryFloat(r, "rate")
			if err != nil {
				return err
			}
			if err := n.SetTaxRate(rate); err != nil {
				return fail(http.StatusBadRequest, err)
			}
		case "tariff":
			rate, err := queryFloat(r, "rate")
			if err != nil {
				return err
			}
			good := engine.GoodID(-1)
			if name := q.Get("good"); name != "" {
				good = -2
				for _, x := range engine.Tradable {
					if engine.Goods[x].Name == name {
						good = x
					}
				}
				if good == -2 {
					return badRequest("no tradable good %q", name)
				}
			}
			if err := n.SetTariff(good, rate); err != nil {
				return fail(http.StatusBadRequest, err)
			}
		case "embargo":
			target, err := queryIndex(r, "target", len(g.w.Nations))
			if err != nil {
				return err
			}
			if target == nid {
				return engine.ErrSelf
			}
			n.Embargo[target] = q.Get("on") != "false"
		case "research":
			tech, err := queryIndex(r, "tech", len(engine.Techs))
			if err != nil {
				return err
			}
			if err := n.SetResearch(tech); err != nil {
				return err
			}
		case "budget":
			var b [engine.NumBudgets]float64
			for i, name := range engine.BudgetNames {
				if b[i], err = queryFloat(r, name); err != nil {
					return err
				}
			}
			if err := n.SetBudget(b); err != nil {
				return fail(http.StatusBadRequest, err)
			}
		case "drill":
			level, err := queryFloat(r, "level")
			if err != nil {
				return err
			}
			if err := n.SetDrill(level); err != nil {
				return fail(http.StatusBadRequest, err)
			}
		case "queue":
			var q []int
			if v := q0(r, "techs"); v != "" {
				for _, part := range strings.Split(v, ",") {
					i, err := strconv.Atoi(part)
					if err != nil {
						return badRequest("techs must be comma-separated tech ids")
					}
					q = append(q, i)
				}
			}
			if err := n.SetQueue(q); err != nil {
				return fail(http.StatusBadRequest, err)
			}
		case "war":
			target, err := queryIndex(r, "target", len(g.w.Nations))
			if err != nil {
				return err
			}
			if err := g.w.DeclareWar(nid, target); err != nil {
				return err
			}
		case "peace":
			target, err := queryIndex(r, "target", len(g.w.Nations))
			if err != nil {
				return err
			}
			if _, err := g.w.OfferPeace(nid, target); err != nil {
				return err
			}
		case "armies": // raise a new army
			region, err := queryIndex(r, "region", len(g.w.Regions))
			if err != nil {
				return err
			}
			men, err := queryFloat(r, "target")
			if err != nil {
				return err
			}
			if _, err := g.w.RaiseArmy(nid, region, men); err != nil {
				return err
			}
		}
		writeJSON(w, g.state(r))
		return nil
	}
}

func q0(r *http.Request, key string) string { return r.URL.Query().Get(key) }

// armyCommand runs a command on army {aid}; only its nation's player may.
func (s *Server) armyCommand(cmd string) gameHandler {
	return func(g *Game, w http.ResponseWriter, r *http.Request) error {
		id, err := strconv.Atoi(r.PathValue("aid"))
		a := g.w.ArmyByID(id)
		if err != nil || a == nil {
			return fail(http.StatusNotFound, engine.ErrNoSuchArmy)
		}
		if g.nationOf(token(r)) != a.Nation.ID {
			return ErrNotPlayer
		}
		switch cmd {
		case "move":
			var to int
			if to, err = queryIndex(r, "to", len(g.w.Regions)); err == nil {
				err = g.w.MoveArmy(id, to)
			}
		case "target":
			var men float64
			if men, err = queryFloat(r, "men"); err == nil {
				err = g.w.SetArmyTarget(id, men)
			}
		case "upgrade":
			err = g.w.UpgradeArmy(id)
		case "disband":
			err = g.w.DisbandArmy(id)
		}
		if err != nil {
			return err
		}
		writeJSON(w, g.state(r))
		return nil
	}
}

func (s *Server) handleEndTurn(g *Game, w http.ResponseWriter, r *http.Request) error {
	if _, err := g.endTurn(token(r)); err != nil {
		return err
	}
	writeJSON(w, g.state(r))
	return nil
}

// HistoryReply is a series bundle with its sample ticks.
type HistoryReply struct {
	Ticks []int `json:"ticks"`
	Data  any   `json:"data"`
}

func (s *Server) handleNationHistory(g *Game, w http.ResponseWriter, r *http.Request) error {
	id, err := pathIndex(r, "id", len(g.w.Nations))
	if err != nil {
		return err
	}
	var data any
	if h := g.w.History; len(h.Nations) > 0 {
		data = h.Nations[id]
	}
	writeJSON(w, HistoryReply{append([]int{}, g.w.History.Ticks...), data})
	return nil
}

func (s *Server) handleRegionHistory(g *Game, w http.ResponseWriter, r *http.Request) error {
	id, err := pathIndex(r, "id", len(g.w.Regions))
	if err != nil {
		return err
	}
	var data any
	if h := g.w.History; len(h.Regions) > 0 {
		data = h.Regions[id]
	}
	writeJSON(w, HistoryReply{append([]int{}, g.w.History.Ticks...), data})
	return nil
}

func (s *Server) handleArmyHistory(g *Game, w http.ResponseWriter, r *http.Request) error {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		return fail(http.StatusNotFound, engine.ErrNoSuchArmy)
	}
	for _, a := range g.w.History.Armies {
		if a.ID == id {
			writeJSON(w, HistoryReply{append([]int{}, g.w.History.Ticks[a.From:]...), a})
			return nil
		}
	}
	return fail(http.StatusNotFound, engine.ErrNoSuchArmy)
}

func (s *Server) handleEvents(g *Game, w http.ResponseWriter, r *http.Request) error {
	since := 0
	if v := q0(r, "since"); v != "" {
		var err error
		if since, err = strconv.Atoi(v); err != nil {
			return badRequest("since must be a tick")
		}
	}
	writeJSON(w, g.w.EventsSince(since, g.nationOf(token(r))))
	return nil
}

// regionCommand runs a command on tile {id}; only its owner's player may.
func (s *Server) regionCommand(cmd string) gameHandler {
	return func(g *Game, w http.ResponseWriter, r *http.Request) error {
		id, err := pathIndex(r, "id", len(g.w.Regions))
		if err != nil {
			return err
		}
		owner := g.w.Regions[id].Nation
		if owner == nil {
			return engine.ErrNoNation
		}
		if g.nationOf(token(r)) != owner.ID {
			return ErrNotPlayer
		}
		switch cmd {
		case "build":
			err = g.w.Build(id)
		case "cancel":
			err = g.w.CancelBuild(id)
		case "port":
			err = g.w.BuildPort(id)
		case "expand":
			var firm int
			if firm, err = pathIndex(r, "firm", len(g.w.Regions[id].Firms)); err == nil {
				err = g.w.Expand(id, firm)
			}
		}
		if err != nil {
			return err
		}
		writeJSON(w, g.state(r))
		return nil
	}
}

// ---- saves ------------------------------------------------------------------

// SaveFile is a saved game.
type SaveFile struct {
	Name     string           `json:"name"`
	TurnDays int              `json:"turn_days"`
	SavedAt  time.Time        `json:"saved_at"`
	Snapshot *engine.Snapshot `json:"snapshot"`
}

// SaveInfo describes a save slot.
type SaveInfo struct {
	Slot    string    `json:"slot"`
	Name    string    `json:"name"`
	Tick    int       `json:"tick"`
	SavedAt time.Time `json:"saved_at"`
}

var slotRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,40}$`)

func (s *Server) slotPath(slot string) (string, error) {
	if s.dataDir == "" {
		return "", fail(http.StatusNotImplemented, errors.New("saving is disabled on this server"))
	}
	if !slotRe.MatchString(slot) {
		return "", badRequest("slot must be 1-40 letters, digits, - or _")
	}
	return filepath.Join(s.dataDir, slot+".json"), nil
}

func (g *Game) saveFile() (*SaveFile, error) {
	snap, err := g.w.Snapshot()
	if err != nil {
		return nil, err
	}
	return &SaveFile{Name: g.Name, TurnDays: g.turnDays, SavedAt: time.Now().UTC(), Snapshot: snap}, nil
}

func (s *Server) handleSave(g *Game, w http.ResponseWriter, r *http.Request) error {
	if !g.mayControlClock(token(r)) {
		return ErrNotInGame
	}
	path, err := s.slotPath(r.URL.Query().Get("slot"))
	if err != nil {
		return err
	}
	f, err := g.saveFile()
	if err != nil {
		return err
	}
	raw, err := json.Marshal(f)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.dataDir, 0o755); err != nil {
		return fail(http.StatusInternalServerError, err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return fail(http.StatusInternalServerError, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fail(http.StatusInternalServerError, err)
	}
	writeJSON(w, SaveInfo{r.URL.Query().Get("slot"), f.Name, f.Snapshot.Tick, f.SavedAt})
	return nil
}

func (s *Server) handleListSaves(w http.ResponseWriter, _ *http.Request) {
	out := []SaveInfo{}
	if s.dataDir != "" {
		entries, _ := os.ReadDir(s.dataDir)
		for _, e := range entries {
			slot, ok := cutSuffix(e.Name(), ".json")
			if !ok || !slotRe.MatchString(slot) {
				continue
			}
			f, err := readSave(filepath.Join(s.dataDir, e.Name()))
			if err != nil {
				continue
			}
			out = append(out, SaveInfo{slot, f.Name, f.Snapshot.Tick, f.SavedAt})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SavedAt.After(out[j].SavedAt) })
	writeJSON(w, out)
}

func cutSuffix(s, suffix string) (string, bool) {
	if len(s) > len(suffix) && s[len(s)-len(suffix):] == suffix {
		return s[:len(s)-len(suffix)], true
	}
	return s, false
}

func readSave(path string) (*SaveFile, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return decodeSave(raw)
}

func decodeSave(raw []byte) (*SaveFile, error) {
	var f SaveFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, err
	}
	if f.Snapshot == nil {
		return nil, errors.New("not a Mercury save")
	}
	if !nameRe.MatchString(f.Name) {
		f.Name = "Loaded game"
	}
	valid := false
	for _, t := range TurnLengths {
		valid = valid || t == f.TurnDays
	}
	if !valid {
		f.TurnDays = 0
	}
	return &f, nil
}

// restore starts a new game from a save; every nation starts computer-run.
func (s *Server) restore(w http.ResponseWriter, f *SaveFile) {
	world, err := engine.Restore(f.Snapshot)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	g, err := s.add(f.Name, world, f.TurnDays)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, g.summary())
}

func (s *Server) handleLoad(w http.ResponseWriter, r *http.Request) {
	path, err := s.slotPath(r.PathValue("slot"))
	if err != nil {
		http.Error(w, err.Error(), statusOf(err))
		return
	}
	f, err := readSave(path)
	if errors.Is(err, os.ErrNotExist) {
		http.Error(w, ErrNoSuchSlot.Error(), http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.restore(w, f)
}

func (s *Server) handleExport(g *Game, w http.ResponseWriter, _ *http.Request) error {
	f, err := g.saveFile()
	if err != nil {
		return err
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="mercury-%s-tick%d.json"`, g.ID, g.w.Tick))
	writeJSON(w, f)
	return nil
}

func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxImport))
	if err != nil {
		http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)
		return
	}
	f, err := decodeSave(raw)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.restore(w, f)
}
