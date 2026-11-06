package engine

import (
	"encoding/json"
	"math"
	"os"
	"sort"
	"testing"
)

// The Python implementation in reference/ is the model's specification. The
// fixture is its trajectory (reference/export_reference.py).

type refFirm struct {
	Name                   string
	Cash, Inventory, Labor float64
}

type refRegion struct {
	Name    string
	Wage    float64
	Prices  []float64
	Utility float64
	PopCash []float64 `json:"pop_cash"`
	Firms   []refFirm
}

type refCheckpoint struct {
	Tick        int
	Money       float64
	Regions     []refRegion
	TradeVolume float64 `json:"trade_volume"`
}

type refFile struct {
	TransportCost float64 `json:"transport_cost"`
	Checkpoints   []refCheckpoint
}

func loadReference(t *testing.T) refFile {
	t.Helper()
	raw, err := os.ReadFile("testdata/reference.json")
	if err != nil {
		t.Fatal(err)
	}
	var ref refFile
	if err := json.Unmarshal(raw, &ref); err != nil {
		t.Fatal(err)
	}
	return ref
}

func relDiff(a, b float64) float64 {
	return math.Abs(a-b) / math.Max(math.Max(math.Abs(a), math.Abs(b)), 1e-6)
}

// maxDiff compares the world with a checkpoint and returns the largest
// relative difference and what it was in.
func maxDiff(w *World, cp refCheckpoint) (float64, string) {
	worst, where := 0.0, ""
	check := func(got, want float64, what string) {
		if d := relDiff(got, want); d > worst {
			worst, where = d, what
		}
	}
	for i, rr := range cp.Regions {
		r := w.Regions[i]
		check(r.Wage, rr.Wage, r.Name+" wage")
		check(r.Utility(), rr.Utility, r.Name+" utility")
		for g, p := range rr.Prices {
			check(r.Markets[g].Price, p, r.Name+" price "+Goods[g].Name)
		}
		for j, c := range rr.PopCash {
			check(r.Pops[j].Cash, c, r.Name+" pop cash")
		}
		for j, f := range rr.Firms {
			check(r.Firms[j].Cash, f.Cash, r.Name+" "+f.Name+" cash")
			check(r.Firms[j].Inventory, f.Inventory, r.Name+" "+f.Name+" inventory")
			check(r.Firms[j].Labor, f.Labor, r.Name+" "+f.Name+" labor")
		}
	}
	return worst, where
}

func median(xs []float64) float64 {
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	return s[len(s)/2]
}

func maxOf(xs []float64) float64 {
	m := 0.0
	for _, x := range xs {
		m = math.Max(m, x)
	}
	return m
}

// The Go engine must reproduce the Python reference. Up to tick 50 the whole
// state agrees to floating-point precision. After that, last-bit differences
// between Go's and C's pow/exp get amplified at the model's switch points
// (e.g. a trader turning on when its margin crosses zero), so the long run is
// compared on macro quantities instead.
func TestMatchesPythonReference(t *testing.T) {
	ref := loadReference(t)
	w, err := BuildWorld(ref.TransportCost, DefaultMap)
	if err != nil {
		t.Fatal(err)
	}
	for _, cp := range ref.Checkpoints {
		for w.Tick < cp.Tick {
			w.Step()
		}
		if d := relDiff(w.Money(), cp.Money); d > 1e-9 {
			t.Errorf("tick %d: money %v, want %v", cp.Tick, w.Money(), cp.Money)
		}
		if cp.Tick <= 50 {
			if d, where := maxDiff(w, cp); d > 1e-9 {
				t.Errorf("tick %d: relative difference %.2e in %s", cp.Tick, d, where)
			}
			continue
		}
		var logPrice, wage, util []float64
		for i, rr := range cp.Regions {
			r := w.Regions[i]
			for g, p := range rr.Prices {
				logPrice = append(logPrice, math.Abs(math.Log(r.Markets[g].Price/p)))
			}
			wage = append(wage, relDiff(r.Wage, rr.Wage))
			util = append(util, relDiff(r.Utility(), rr.Utility))
		}
		vol := 0.0
		for _, tr := range w.Traders {
			vol += tr.Shipped
		}
		checks := []struct {
			what       string
			got, limit float64
		}{
			{"median |log price ratio|", median(logPrice), 0.02},
			{"max wage difference", maxOf(wage), 0.05},
			{"max utility difference", maxOf(util), 0.05},
			{"trade volume difference", relDiff(vol, cp.TradeVolume), 0.02},
		}
		for _, c := range checks {
			if c.got > c.limit {
				t.Errorf("tick %d: %s = %.3g, limit %.3g", cp.Tick, c.what, c.got, c.limit)
			}
		}
	}
}
