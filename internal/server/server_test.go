package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/rasmusbergpalm/mercury/engine"
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

	e.do("POST", base+"budget?economy=0.3&science=0.3&military=0.4", tok, nil, 200, &st)
	if b := st.Nations[0].Budget; b[0] != 0.3 || b[1] != 0.3 || b[2] != 0.4 {
		t.Errorf("budget %v", b)
	}
	e.do("POST", base+"budget?economy=0.5&science=0.5&military=0.5", tok, nil, 400, nil)
	e.do("POST", base+"tariff?rate=0.3", tok, nil, 200, &st)
	e.do("POST", base+"tariff?rate=0.5&good=tea", tok, nil, 200, &st)
	if st.Nations[0].Tariff[0] != 0.3 || st.Nations[0].Tariff[engine.G("tea")] != 0.5 {
		t.Errorf("tariffs %v", st.Nations[0].Tariff)
	}
	e.do("POST", base+"tariff?rate=0.5&good=military", tok, nil, 400, nil)
	e.do("POST", base+"embargo?target=2", tok, nil, 200, &st)
	if !st.Nations[0].Embargo[2] {
		t.Error("embargo not set")
	}
	e.do("POST", base+"embargo?target=0", tok, nil, 409, nil)
	e.do("POST", base+"research?tech=2", tok, nil, 200, &st)
	if st.Nations[0].Researching != 2 {
		t.Errorf("researching %d", st.Nations[0].Researching)
	}
	e.do("POST", base+"research?tech=6", tok, nil, 409, nil) // Railways needs prerequisites
	e.do("POST", base+"campaign?target=2", tok, nil, 200, &st)
	if c := st.Nations[0].Campaign; c == nil || c.Target != 2 {
		t.Errorf("campaign %+v", c)
	}
	e.do("POST", base+"campaign?target=35", tok, nil, 409, nil) // not adjacent
	e.do("POST", base+"campaign/cancel", tok, nil, 200, &st)
	if st.Nations[0].Campaign != nil {
		t.Error("campaign not cancelled")
	}
	e.do("POST", base+"campaign/cancel", tok, nil, 409, nil)

	var d RegionDetail
	e.do("GET", "/api/games/"+g.ID+"/regions/0", "", nil, 200, &d)
	fi := -1
	for i, f := range d.Firms {
		if f.Expansion == nil && f.Levels < f.MaxLevels {
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

func TestClock(t *testing.T) {
	e := newEnv(t)
	g := e.newGame("G")
	var st State
	e.do("POST", "/api/games/"+g.ID+"/clock?running=true&speed=20", "", nil, 200, &st)
	if !st.Running || st.Speed != 20 {
		t.Fatalf("clock %v %v", st.Running, st.Speed)
	}
	e.do("POST", "/api/games/"+g.ID+"/clock?speed=7", "", nil, 400, nil)
	game := e.srv.games[g.ID]
	game.mu.Lock()
	game.advance(0.5) // half a second at 20 ticks per second
	tick := game.w.Tick
	game.running = false
	game.advance(10)
	stopped := game.w.Tick
	game.mu.Unlock()
	if tick != Warmup+10 || stopped != Warmup+10 {
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
	if a.Money != b.Money || a.Regions[5].Wage != b.Regions[5].Wage || a.Nations[1].Army != b.Nations[1].Army {
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
	g, _ := srv.CreateGame("x", 1)
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
	e.do("POST", "/api/games/"+g.ID+"/clock?running=true&speed=50", "", nil, 200, nil)
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
