package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/rasmusbergpalm/mercury/game"
)

type env struct {
	t   *testing.T
	ts  *httptest.Server
	srv *Server
}

func newEnv(t *testing.T) *env {
	t.Helper()
	static := fstest.MapFS{"index.html": {Data: []byte("<h1>mercury</h1>")}}
	srv := New(static, t.TempDir())
	t.Cleanup(srv.Wait) // runs before the temp dir is removed
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	return &env{t, ts, srv}
}

// do sends a request with an optional player token and decodes the reply.
func (e *env) do(method, path, tok string, body []byte, want int, out any) {
	e.t.Helper()
	req, _ := http.NewRequest(method, e.ts.URL+path, bytes.NewReader(body))
	if tok != "" {
		req.Header.Set("X-Player-Token", tok)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != want {
		e.t.Fatalf("%s %s: status %d (%s), want %d", method, path, resp.StatusCode, bytes.TrimSpace(raw), want)
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			e.t.Fatalf("%s %s: %v", method, path, err)
		}
	}
}

func (e *env) newGame(name string) GameSummary {
	var g GameSummary
	e.do("POST", "/api/games?name="+name, "", nil, 201, &g)
	return g
}

func (e *env) join(gid string, nation int) string {
	var out struct {
		Token  string `json:"token"`
		Nation int    `json:"nation"`
	}
	e.do("POST", fmt.Sprintf("/api/games/%s/join?nation=%d", gid, nation), "", nil, 200, &out)
	if out.Token == "" || out.Nation != nation {
		e.t.Fatalf("join: %+v", out)
	}
	return out.Token
}

func TestLobby(t *testing.T) {
	e := newEnv(t)
	a, b := e.newGame("Alpha"), e.newGame("Beta")
	if a.ID == b.ID || len(a.Nations) != 3 || a.Nations[0].Controller != "ai" {
		t.Fatalf("games %+v %+v", a, b)
	}
	var list []GameSummary
	e.do("GET", "/api/games", "", nil, 200, &list)
	if len(list) != 2 || list[0].Name != "Alpha" {
		t.Fatalf("lobby %+v", list)
	}
	e.do("POST", "/api/games?name=%3Cscript%3E", "", nil, 400, nil)
	e.do("POST", "/api/games?name=x&cost=-1", "", nil, 400, nil)
	e.do("GET", "/api/games/nope/state", "", nil, 404, nil)
	e.do("DELETE", "/api/games/"+b.ID, "", nil, 204, nil)
	e.do("GET", "/api/games", "", nil, 200, &list)
	if len(list) != 1 {
		t.Errorf("%d games after delete", len(list))
	}
}

func TestGamesAreIndependent(t *testing.T) {
	e := newEnv(t)
	a, b := e.newGame("A"), e.newGame("B")
	var st State
	e.do("POST", "/api/games/"+a.ID+"/step?n=30", "", nil, 200, &st)
	e.do("GET", "/api/games/"+b.ID+"/state", "", nil, 200, &st)
	if st.Tick != Warmup {
		t.Errorf("stepping one game moved another to tick %d", st.Tick)
	}
}

func TestPlayersControlTheirNationOnly(t *testing.T) {
	e := newEnv(t)
	g := e.newGame("G")
	base := "/api/games/" + g.ID
	tok := e.join(g.ID, 1)
	e.do("POST", base+"/join?nation=1", "", nil, 409, nil) // taken
	other := e.join(g.ID, 2)

	var st State
	e.do("GET", base+"/state", tok, nil, 200, &st)
	if st.You != 1 || st.Nations[1].Controller != "human" || st.Nations[0].Controller != "ai" {
		t.Fatalf("you %d, controllers %s %s", st.You, st.Nations[1].Controller, st.Nations[0].Controller)
	}
	// with players in the game, only they run the clock
	e.do("POST", base+"/step?n=50", "", nil, 403, nil)
	e.do("POST", base+"/step?n=50", tok, nil, 200, &st)
	e.do("POST", base+"/nations/1/tax?rate=0.2", "", nil, 403, nil)
	e.do("POST", base+"/nations/1/tax?rate=0.2", other, nil, 403, nil)
	e.do("POST", base+"/nations/1/tax?rate=0.2", tok, nil, 200, &st)
	if st.Nations[1].TaxRate != 0.2 {
		t.Errorf("tax %v", st.Nations[1].TaxRate)
	}
	verdaniaTile := -1 // a Verdania tile with nothing built or being built yet
	for _, r := range st.Regions {
		if r.Nation == 1 && r.Infra == 0 && r.Project == nil {
			verdaniaTile = r.ID
			break
		}
	}
	if verdaniaTile < 0 {
		t.Fatal("no unbuilt Verdania tile")
	}
	e.do("POST", fmt.Sprintf("%s/regions/%d/build", base, verdaniaTile), other, nil, 403, nil)
	e.do("POST", fmt.Sprintf("%s/regions/%d/build", base, verdaniaTile), tok, nil, 200, &st)
	if st.Regions[verdaniaTile].Project == nil {
		t.Error("no project after build")
	}
	e.do("POST", base+"/regions/2/build", tok, nil, 409, nil) // unowned
	e.do("POST", base+"/leave", tok, nil, 200, &st)
	if st.Nations[1].Controller != "ai" {
		t.Error("nation not handed back to the computer")
	}
	e.do("POST", base+"/nations/1/tax?rate=0.3", tok, nil, 403, nil)
}

func TestNationCommands(t *testing.T) {
	e := newEnv(t)
	g := e.newGame("G")
	base := "/api/games/" + g.ID + "/nations/0/"
	tok := e.join(g.ID, 0)
	var st State
	e.do("POST", "/api/games/"+g.ID+"/step?n=150", tok, nil, 200, &st)

	e.do("POST", base+"auto?on=true", tok, nil, 200, &st)
	if !st.Nations[0].AutoBuild {
		t.Error("automated construction not on")
	}
	// universities: fund fewer levels, build one, close one
	home := 0                                              // tile 00 is Montara's
	uni := "/api/games/" + g.ID + "/regions/0/services/0/" // universities
	e.do("POST", uni+"fund?levels=1", tok, nil, 200, &st)
	if u := st.Regions[home].Services[0]; u == nil || u.Funded != 1 || u.Levels != 2 {
		t.Fatalf("university after funding 1: %+v", u)
	}
	e.do("POST", uni+"fund?levels=9", tok, nil, 400, nil)
	e.do("POST", uni+"close", tok, nil, 200, &st)
	if u := st.Regions[home].Services[0]; u.Levels != 1 || u.Funded != 1 {
		t.Errorf("university after closing a level: %+v", u)
	}
	e.do("POST", uni+"expand", tok, nil, 200, &st)
	if u := st.Regions[home].Services[0]; u.Building == nil {
		t.Errorf("no university level being built: %+v", u)
	}
	e.do("POST", "/api/games/"+g.ID+"/regions/0/services/9/expand", tok, nil, 404, nil)
	e.do("POST", base+"pension?level=2", tok, nil, 200, &st)
	e.do("POST", base+"auto-tax?on=true", tok, nil, 200, &st)
	if st.Nations[0].Pension != 2 || !st.Nations[0].AutoTax {
		t.Errorf("pension %d, auto tax %v", st.Nations[0].Pension, st.Nations[0].AutoTax)
	}
	e.do("POST", base+"pension?level=5", tok, nil, 400, nil)
	e.do("POST", base+"tariff?rate=0.3", tok, nil, 200, &st)
	e.do("POST", base+"tariff?rate=0.5&good=tea", tok, nil, 200, &st)
	if st.Nations[0].Tariff[0] != 0.3 || st.Nations[0].Tariff[game.G("tea")] != 0.5 {
		t.Errorf("tariffs %v", st.Nations[0].Tariff)
	}
	e.do("POST", base+"tariff?rate=0.5&good=military", tok, nil, 400, nil)
	e.do("POST", base+"embargo?target=2", tok, nil, 200, &st)
	if !st.Nations[0].Embargo[2] {
		t.Error("embargo not set")
	}
	e.do("POST", base+"embargo?target=0", tok, nil, 409, nil)
	masonry, railways := game.TechID("Macadam roads"), game.TechID("Railways")
	e.do("POST", fmt.Sprintf("%sresearch?tech=%d", base, masonry), tok, nil, 200, &st)
	if st.Nations[0].Researching != masonry {
		t.Errorf("researching %d", st.Nations[0].Researching)
	}
	e.do("POST", fmt.Sprintf("%sresearch?tech=%d", base, railways), tok, nil, 409, nil) // needs prerequisites
	// armies: raise one in the capital (tile 11), march it to free land next door
	// (the computer raised Montara's first army before we joined; it stays ours)
	e.do("POST", base+"armies?region=7&target=0", tok, nil, 200, &st)
	var army *ArmyState
	for i := range st.Armies {
		if st.Armies[i].Nation == 0 {
			army = &st.Armies[i]
		}
	}
	if army == nil || army.Target != 0 || army.Region != 7 {
		t.Fatalf("armies after raising: %+v", st.Armies)
	}
	e.do("POST", base+"armies?region=7&target=1e6", tok, nil, 409, nil) // beyond the mobilization cap
	apath := fmt.Sprintf("/api/games/%s/armies/%d/", g.ID, army.ID)
	e.do("POST", apath+"move?to=8", tok, nil, 200, &st)  // tile 12, unowned
	e.do("POST", apath+"move?to=10", tok, nil, 409, nil) // tile 14: Verdania, at peace
	e.do("POST", apath+"target?men=0", tok, nil, 200, &st)
	other := e.join(g.ID, 1)
	e.do("POST", apath+"move?to=7", other, nil, 403, nil)
	e.do("POST", fmt.Sprintf("/api/games/%s/armies/999/move?to=1", g.ID), tok, nil, 404, nil)
	// war and peace with Verdania (borders aren't shared yet: refused)
	e.do("POST", base+"war?target=1", tok, nil, 409, nil)
	e.do("POST", base+"peace?target=1", tok, nil, 409, nil)
	e.do("POST", base+"queue?techs=1,2", tok, nil, 200, &st)
	if q := st.Nations[0].Queue; len(q) != 2 || q[0] != 1 {
		t.Errorf("queue %v", q)
	}
	e.do("POST", base+"queue?techs=x", tok, nil, 400, nil)
	if len(st.Nations[0].TechCosts) != len(game.Techs) || st.Date == "" {
		t.Errorf("state lacks tech costs or date")
	}

	var d RegionDetail
	e.do("GET", "/api/games/"+g.ID+"/regions/0", "", nil, 200, &d)
	fi := -1
	for i, f := range d.Firms {
		if f.Expansion == nil && f.Enabled && (f.MaxLevels == 0 || f.Levels < f.MaxLevels) {
			fi = i
			break
		}
	}
	path := fmt.Sprintf("/api/games/%s/regions/0/firms/%d/expand", g.ID, fi)
	e.do("POST", path, tok, nil, 200, &st)
	e.do("POST", path, tok, nil, 409, nil)
	e.do("POST", fmt.Sprintf("/api/games/%s/regions/0/firms/999/expand", g.ID), tok, nil, 404, nil)
	e.do("GET", "/api/games/"+g.ID+"/regions/99", "", nil, 404, nil)
}

func TestTechTreeAndPorts(t *testing.T) {
	e := newEnv(t)
	g := e.newGame("G")
	var info WorldInfo
	e.do("GET", "/api/games/"+g.ID+"/world", "", nil, 200, &info)
	if len(info.Branches) != 5 || info.Techs[0].Branch == "" || info.Port.Cost <= 0 {
		t.Fatalf("world: branches %v, first tech %+v, port %+v", info.Branches, info.Techs[0], info.Port)
	}
	tok := e.join(g.ID, 2) // Thalassa: tile 54 (index 34) is coastal
	var st State
	e.do("POST", "/api/games/"+g.ID+"/regions/34/port", tok, nil, 409, nil) // needs Clipper ships
	game := e.srv.games[g.ID]
	game.mu.Lock()
	game.w.GrantTech(2, "Hydrographic charts")
	game.w.GrantTech(2, "Clipper ships")
	if p := game.w.Regions[34].Project; p != nil { // the computer may have started a road while warming up
		game.w.CancelBuild(34)
	}
	game.mu.Unlock()
	e.do("POST", "/api/games/"+g.ID+"/regions/34/port", tok, nil, 200, &st)
	if p := st.Regions[34].Project; p == nil || !p.Port {
		t.Errorf("project %+v", p)
	}
	e.do("POST", "/api/games/"+g.ID+"/regions/27/port", tok, nil, 409, nil) // 43 is tropical, not coast
	var d RegionDetail
	e.do("GET", "/api/games/"+g.ID+"/regions/0", "", nil, 200, &d)
	locked := 0
	for _, f := range d.Firms {
		if !f.Enabled {
			locked++
			if f.Requires == "" {
				t.Errorf("locked firm %s names no tech", f.Name)
			}
		}
	}
	if locked == 0 {
		t.Error("no locked firms at the start")
	}
}

func TestTurnBasedGames(t *testing.T) {
	e := newEnv(t)
	var g GameSummary
	e.do("POST", "/api/games?name=T&turn=30", "", nil, 201, &g)
	if g.Turn != 30 {
		t.Fatalf("turn days %d", g.Turn)
	}
	e.do("POST", "/api/games?name=T&turn=13", "", nil, 400, nil)
	a, b := e.join(g.ID, 0), e.join(g.ID, 1)
	var st State
	e.do("POST", "/api/games/"+g.ID+"/clock?running=true", a, nil, 409, nil)
	e.do("POST", "/api/games/"+g.ID+"/step?n=5", a, nil, 409, nil)
	e.do("POST", "/api/games/"+g.ID+"/end-turn", a, nil, 200, &st)
	if st.Tick != Warmup || len(st.Turn.Ready) != 1 {
		t.Fatalf("after one player ended the turn: tick %d, ready %v", st.Tick, st.Turn.Ready)
	}
	e.do("POST", "/api/games/"+g.ID+"/end-turn", b, nil, 200, &st)
	if st.Tick != Warmup+30 || len(st.Turn.Ready) != 0 {
		t.Fatalf("after both ended the turn: tick %d, ready %v", st.Tick, st.Turn.Ready)
	}
	e.do("POST", "/api/games/"+g.ID+"/end-turn", "", nil, 403, nil)
	real := e.newGame("R")
	tok := e.join(real.ID, 0)
	e.do("POST", "/api/games/"+real.ID+"/end-turn", tok, nil, 409, nil)
}

func TestEvents(t *testing.T) {
	e := newEnv(t)
	g := e.newGame("E")
	e.do("POST", "/api/games/"+g.ID+"/step?n=400", "", nil, 200, nil)
	var ev []game.Event
	e.do("GET", "/api/games/"+g.ID+"/events?since=0", "", nil, 200, &ev)
	if len(ev) == 0 {
		t.Error("no events in 400 days")
	}
	e.do("GET", "/api/games/"+g.ID+"/events?since=x", "", nil, 400, nil)
}

func TestHistoryAndBreakdown(t *testing.T) {
	e := newEnv(t)
	g := e.newGame("H")
	var st State
	e.do("POST", "/api/games/"+g.ID+"/step?n=300", "", nil, 200, &st)
	var h struct {
		Ticks []int `json:"ticks"`
		Data  struct {
			GDP         []float64   `json:"gdp"`
			ClassIncome [][]float64 `json:"class_income"`
		} `json:"data"`
	}
	e.do("GET", "/api/games/"+g.ID+"/history/nations/0", "", nil, 200, &h)
	if len(h.Ticks) == 0 || len(h.Data.GDP) != len(h.Ticks) || len(h.Data.ClassIncome) != int(game.NumClasses) {
		t.Fatalf("nation history: %d ticks, %d gdp", len(h.Ticks), len(h.Data.GDP))
	}
	e.do("GET", "/api/games/"+g.ID+"/history/regions/7", "", nil, 200, nil)
	e.do("GET", "/api/games/"+g.ID+"/history/regions/99", "", nil, 404, nil)
	if len(st.Armies) == 0 || len(st.Armies[0].Attack) == 0 || len(st.Nations[0].Labor) != 3 {
		t.Fatalf("state lacks army breakdown or nation stats")
	}
	e.do("GET", fmt.Sprintf("/api/games/%s/history/armies/%d", g.ID, st.Armies[0].ID), "", nil, 200, nil)
	e.do("GET", "/api/games/"+g.ID+"/history/armies/999", "", nil, 404, nil)
	var d RegionDetail
	e.do("GET", "/api/games/"+g.ID+"/regions/7", "", nil, 200, &d)
	if len(d.Classes) != int(game.NumClasses) || d.GDP <= 0 {
		t.Errorf("region detail: %d classes, gdp %v", len(d.Classes), d.GDP)
	}
}

func TestClock(t *testing.T) {
	e := newEnv(t)
	g := e.newGame("G")
	var st State
	e.do("POST", "/api/games/"+g.ID+"/clock?running=true&speed=30", "", nil, 200, &st)
	if !st.Running || st.Speed != 30 {
		t.Fatalf("clock %v %v", st.Running, st.Speed)
	}
	e.do("POST", "/api/games/"+g.ID+"/clock?speed=4", "", nil, 400, nil)
	game := e.srv.games[g.ID]
	game.mu.Lock()
	game.advance(0.5) // half a second at 30 days per second
	tick := game.w.Tick
	game.running = false
	game.advance(10)
	stopped := game.w.Tick
	game.mu.Unlock()
	if tick != Warmup+15 || stopped != Warmup+15 {
		t.Errorf("clock ran %d ticks, then %d when stopped; want 10 and 10", tick, stopped)
	}
}

func TestSaveAndLoad(t *testing.T) {
	e := newEnv(t)
	g := e.newGame("Saved")
	tok := e.join(g.ID, 0)
	var st State
	e.do("POST", "/api/games/"+g.ID+"/step?n=120", tok, nil, 200, &st)
	e.do("POST", "/api/games/"+g.ID+"/save?slot=slot-1", "", nil, 403, nil)
	var info SaveInfo
	e.do("POST", "/api/games/"+g.ID+"/save?slot=slot-1", tok, nil, 200, &info)
	if info.Tick != Warmup+120 || info.Name != "Saved" {
		t.Errorf("save info %+v", info)
	}
	e.do("POST", "/api/games/"+g.ID+"/save?slot=../evil", tok, nil, 400, nil)
	var saves []SaveInfo
	e.do("GET", "/api/saves", "", nil, 200, &saves)
	if len(saves) != 1 || saves[0].Slot != "slot-1" {
		t.Fatalf("saves %+v", saves)
	}
	var loaded GameSummary
	e.do("POST", "/api/saves/slot-1/load", "", nil, 201, &loaded)
	if loaded.ID == g.ID || loaded.Tick != Warmup+120 || loaded.Nations[0].Controller != "ai" {
		t.Errorf("loaded %+v", loaded)
	}
	e.do("POST", "/api/saves/missing/load", "", nil, 404, nil)

	// the loaded game continues exactly like the original
	var a, b State
	e.do("POST", "/api/games/"+g.ID+"/leave", tok, nil, 200, nil)
	e.do("POST", "/api/games/"+g.ID+"/step?n=30", "", nil, 200, &a)
	e.do("POST", "/api/games/"+loaded.ID+"/step?n=30", "", nil, 200, &b)
	if a.Money != b.Money || a.Regions[5].Wage != b.Regions[5].Wage || a.Nations[1].Soldiers != b.Nations[1].Soldiers {
		t.Error("loaded game diverged from the original")
	}
}

func TestExportImport(t *testing.T) {
	e := newEnv(t)
	g := e.newGame("Exported")
	e.do("POST", "/api/games/"+g.ID+"/step?n=40", "", nil, 200, nil)
	req, _ := http.NewRequest("GET", e.ts.URL+"/api/games/"+g.ID+"/export", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.Header.Get("Content-Disposition") == "" {
		t.Error("export is not a download")
	}
	var imported GameSummary
	e.do("POST", "/api/games/import", "", raw, 201, &imported)
	if imported.Tick != Warmup+40 || imported.Name != "Exported" {
		t.Errorf("imported %+v", imported)
	}
	e.do("POST", "/api/games/import", "", []byte(`{"name":"x"}`), 400, nil)
	e.do("POST", "/api/games/import", "", []byte(`not json`), 400, nil)
}

func TestSavingDisabledWithoutDataDir(t *testing.T) {
	srv := New(nil, "")
	ts := httptest.NewServer(srv)
	defer ts.Close()
	g, _ := srv.CreateGame("x", 1, 0)
	resp, err := http.Post(ts.URL+"/api/games/"+g.ID+"/save?slot=a", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotImplemented {
		t.Errorf("save without a data dir: %d", resp.StatusCode)
	}
}

func TestServesStaticFiles(t *testing.T) {
	e := newEnv(t)
	resp, err := http.Get(e.ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("GET /: %d", resp.StatusCode)
	}
}

// The clock goroutine and requests share games; run with -race.
func TestClockRunsAlongsideRequests(t *testing.T) {
	e := newEnv(t)
	g := e.newGame("Busy")
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() { e.srv.Run(stop); close(done) }()
	e.do("POST", "/api/games/"+g.ID+"/clock?running=true&speed=30", "", nil, 200, nil)
	var st State
	for i := 0; i < 20; i++ {
		e.do("GET", "/api/games/"+g.ID+"/state", "", nil, 200, &st)
		e.do("GET", "/api/games", "", nil, 200, nil)
	}
	for st.Tick == 0 {
		e.do("GET", "/api/games/"+g.ID+"/state", "", nil, 200, &st)
	}
	close(stop)
	<-done
}

// Games are saved every AutosaveEvery days and come back, players and all,
// when the server restarts.
func TestAutosaveSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	s := New(nil, dir)
	t.Cleanup(s.Wait)
	g, err := s.CreateGame("Kept", 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	g.mu.Lock()
	tok, err := g.join(1)
	g.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	h := httptest.NewServer(s)
	defer h.Close()
	req, _ := http.NewRequest("POST", h.URL+"/api/games/"+g.ID+"/step?n=45", nil)
	req.Header.Set("X-Player-Token", tok)
	if res, err := http.DefaultClient.Do(req); err != nil || res.StatusCode != 200 {
		t.Fatalf("step: %v %v", err, res)
	}
	path := filepath.Join(dir, "autosave", g.ID+".json")
	var f autosaveFile
	for i := 0; i < 100; i++ { // written in the background
		if raw, err := os.ReadFile(path); err == nil && json.Unmarshal(raw, &f) == nil && f.Snapshot.Tick >= 75 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if f.Snapshot == nil || f.Snapshot.Tick != 75 {
		t.Fatal("no autosave at tick 75 (30 days of warm-up, then 45)")
	}
	// a new server on the same data picks the game up where it was saved
	s2 := New(nil, dir)
	t.Cleanup(s2.Wait)
	g2 := s2.games[g.ID]
	if g2 == nil || g2.w.Tick != 75 || g2.nationOf(tok) != 1 || g2.w.Nations[1].AI {
		t.Fatal("the game didn't resume as saved, with its player")
	}
	// deleting a game deletes its autosave
	req, _ = http.NewRequest("DELETE", h.URL+"/api/games/"+g.ID, nil)
	req.Header.Set("X-Player-Token", tok)
	http.DefaultClient.Do(req)
	g.fileMu.Lock()
	g.fileMu.Unlock()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("autosave of a deleted game still there: %v", err)
	}
}

// A game on a generated map is listed while it is being prepared, with its
// progress, then opens under the same ID; a bad choice is refused at once.
func TestGeneratedGameIsPrepared(t *testing.T) {
	if testing.Short() {
		t.Skip("settles a map")
	}
	e := newEnv(t)
	e.do("POST", "/api/games?name=Islands&map=nowhere", "", nil, http.StatusConflict, nil)
	e.do("POST", "/api/games?name=Islands&map=small&nations=13", "", nil, http.StatusBadRequest, nil)
	e.do("POST", "/api/games?name=Islands&map=small&nations=9", "", nil, http.StatusConflict, nil) // too little land
	var g GameSummary
	e.do("POST", "/api/games?name=Islands&map=small&nations=2&seed=4", "", nil, http.StatusAccepted, &g)
	if g.Preparing == nil || g.Map != "small" || len(g.Nations) != 2 {
		t.Fatalf("created %+v", g)
	}
	e.srv.Wait()
	var games []GameSummary
	e.do("GET", "/api/games", "", nil, http.StatusOK, &games)
	var got *GameSummary
	for i := range games {
		if games[i].ID == g.ID {
			got = &games[i]
		}
	}
	if got == nil || got.Preparing != nil || got.Map != "small" || got.Tiles != g.Tiles || got.Tick != Warmup {
		t.Fatalf("after preparing: %+v", got)
	}
	var w WorldInfo
	e.do("GET", "/api/games/"+g.ID+"/world", "", nil, http.StatusOK, &w)
	if len(w.Regions) != g.Tiles || w.Regions[0].Name == "00" {
		t.Errorf("world of %d tiles, first named %q", len(w.Regions), w.Regions[0].Name)
	}
}

// A game with no free land starts with every tile owned.
func TestDividedClassicGame(t *testing.T) {
	e := newEnv(t)
	var g GameSummary
	e.do("POST", "/api/games?name=Shared&divided=1", "", nil, http.StatusCreated, &g)
	total := 0
	for _, n := range g.Nations {
		total += n.Tiles
	}
	if total != g.Tiles {
		t.Errorf("nations hold %d of %d tiles", total, g.Tiles)
	}
}
