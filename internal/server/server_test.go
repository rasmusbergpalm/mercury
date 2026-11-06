package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/rasmusbergpalm/mercury/engine"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	static := fstest.MapFS{"index.html": {Data: []byte("<h1>mercury</h1>")}}
	s, err := New(1, static)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(s)
	t.Cleanup(ts.Close)
	return ts
}

func call(t *testing.T, ts *httptest.Server, method, path string, want int, out any) {
	t.Helper()
	req, _ := http.NewRequest(method, ts.URL+path, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != want {
		t.Fatalf("%s %s: status %d, want %d", method, path, resp.StatusCode, want)
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
	}
}

func TestWorldInfo(t *testing.T) {
	ts := newTestServer(t)
	var info WorldInfo
	call(t, ts, "GET", "/api/world", 200, &info)
	if info.Rows != 6 || info.Cols != 6 || len(info.Regions) != 36 {
		t.Errorf("got %dx%d map with %d regions", info.Rows, info.Cols, len(info.Regions))
	}
	if len(info.Goods) != engine.NumGoods || len(info.Links) == 0 || len(info.Tiers) != 3 {
		t.Errorf("got %d goods, %d links, %d tiers", len(info.Goods), len(info.Links), len(info.Tiers))
	}
}

func TestStepAdvancesAndReports(t *testing.T) {
	ts := newTestServer(t)
	var st State
	call(t, ts, "POST", "/api/step?n=50", 200, &st)
	if st.Tick != 50 || len(st.Regions) != 36 || len(st.Flows) == 0 {
		t.Fatalf("after 50 steps: tick %d, %d regions, %d flows", st.Tick, len(st.Regions), len(st.Flows))
	}
	if len(st.Regions[0].Prices) != engine.NumGoods || len(st.Regions[0].TierSpend) != 3 {
		t.Errorf("region state has %d prices and %d tiers", len(st.Regions[0].Prices), len(st.Regions[0].TierSpend))
	}
	call(t, ts, "GET", "/api/state", 200, &st)
	if st.Tick != 50 {
		t.Errorf("state tick %d, want 50", st.Tick)
	}
	call(t, ts, "POST", "/api/step?n=0", 400, nil)
	call(t, ts, "POST", "/api/step?n=abc", 400, nil)
	call(t, ts, "GET", "/api/step", 404, nil) // only POST steps the world
}

func TestRegionDetail(t *testing.T) {
	ts := newTestServer(t)
	call(t, ts, "POST", "/api/step?n=100", 200, nil)
	var d RegionDetail
	call(t, ts, "GET", "/api/regions/25", 200, &d)
	if d.Info.Name != "41" || len(d.Firms) == 0 || len(d.Markets) != engine.NumGoods || len(d.Pops) == 0 {
		t.Errorf("region 25: name %q, %d firms, %d markets, %d pops", d.Info.Name, len(d.Firms), len(d.Markets), len(d.Pops))
	}
	if len(d.Imports)+len(d.Exports) == 0 {
		t.Error("region 25 has no trade after 100 ticks")
	}
	call(t, ts, "GET", "/api/regions/99", 404, nil)
	call(t, ts, "GET", "/api/regions/x", 404, nil)
}

func TestReset(t *testing.T) {
	ts := newTestServer(t)
	var st State
	call(t, ts, "POST", "/api/step?n=5", 200, nil)
	call(t, ts, "POST", "/api/reset?cost=inf", 200, &st)
	if st.Tick != 0 || !st.Autarky {
		t.Errorf("after reset: tick %d autarky %v", st.Tick, st.Autarky)
	}
	call(t, ts, "POST", "/api/step?n=20", 200, &st)
	if len(st.Flows) != 0 {
		t.Errorf("autarky world has %d flows", len(st.Flows))
	}
	call(t, ts, "POST", "/api/reset?cost=0.5", 200, &st)
	if st.TransportCost != 0.5 || st.Autarky {
		t.Errorf("reset cost: %v autarky %v", st.TransportCost, st.Autarky)
	}
	call(t, ts, "POST", "/api/reset?cost=-1", 400, nil)
}

func TestServesStaticFiles(t *testing.T) {
	ts := newTestServer(t)
	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("GET /: status %d", resp.StatusCode)
	}
}
