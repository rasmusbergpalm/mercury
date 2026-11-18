package economy

import (
	"math"
	"testing"
)

func newTestWorld(t *testing.T, cost float64) *World {
	t.Helper()
	w, err := BuildWorld(cost, DefaultMap)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func TestMoneyIsConserved(t *testing.T) {
	w := newTestWorld(t, 1)
	m0 := w.Money()
	for i := 0; i < 300; i++ {
		w.Step()
		if d := relDiff(w.Money(), m0); d > 1e-10 {
			t.Fatalf("tick %d: money %v, started with %v", w.Tick, w.Money(), m0)
		}
	}
}

func TestStepIsDeterministic(t *testing.T) {
	a, b := newTestWorld(t, 1), newTestWorld(t, 1)
	for i := 0; i < 300; i++ {
		a.Step()
		b.Step()
	}
	for i := range a.Regions {
		ra, rb := a.Regions[i], b.Regions[i]
		if ra.Wage != rb.Wage {
			t.Fatalf("region %s: wages differ", ra.Name)
		}
		for g := range ra.Markets {
			if ra.Markets[g].Price != rb.Markets[g].Price {
				t.Fatalf("region %s: %s prices differ", ra.Name, Goods[g].Name)
			}
		}
	}
	for i := range a.Traders {
		if a.Traders[i].Shipped != b.Traders[i].Shipped {
			t.Fatalf("trader %s differs", a.Traders[i].Name())
		}
	}
}

func TestStateStaysValid(t *testing.T) {
	w := newTestWorld(t, 1)
	for i := 0; i < 500; i++ {
		w.Step()
	}
	finite := func(x float64) bool { return !math.IsNaN(x) && !math.IsInf(x, 0) }
	for _, r := range w.Regions {
		if !finite(r.Wage) || r.Wage <= 0 || !finite(r.Utility()) {
			t.Errorf("region %s: wage %v utility %v", r.Name, r.Wage, r.Utility())
		}
		for g, m := range r.Markets {
			if !finite(m.Price) || m.Price <= 0 {
				t.Errorf("region %s: %s price %v", r.Name, Goods[g].Name, m.Price)
			}
		}
		for _, p := range r.Pops {
			if p.Cash < -1e-9 || p.Employed() > p.Size+1e-9 {
				t.Errorf("region %s: pop cash %v employed %v of %v", r.Name, p.Cash, p.Employed(), p.Size)
			}
		}
		for _, f := range r.Firms {
			if f.Inventory < -1e-9 || !finite(f.Cash) {
				t.Errorf("region %s %s: inventory %v cash %v", r.Name, f.Name, f.Inventory, f.Cash)
			}
		}
	}
	for _, tr := range w.Traders {
		if tr.StockDest < -1e-9 || tr.StockOrigin < -1e-9 {
			t.Errorf("trader %s: stocks %v %v", tr.Name(), tr.StockOrigin, tr.StockDest)
		}
	}
	for _, r := range w.Regions {
		if r.House.Cash < -1e-9 {
			t.Errorf("trading house %s: cash %v", r.Name, r.House.Cash)
		}
	}
}

func TestHexGrid(t *testing.T) {
	rows, cols := 6, 6
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			nbs := HexNeighbors(r, c, rows, cols)
			interior := r > 0 && r < rows-1 && c > 0 && c < cols-1
			if interior && len(nbs) != 6 {
				t.Errorf("interior hex (%d,%d) has %d neighbours", r, c, len(nbs))
			}
			for _, nb := range nbs {
				back := false
				for _, x := range HexNeighbors(nb[0], nb[1], rows, cols) {
					back = back || x == [2]int{r, c}
				}
				if !back {
					t.Errorf("(%d,%d) -> %v is not symmetric", r, c, nb)
				}
			}
		}
	}
	if n := len(HexNeighbors(0, 0, rows, cols)); n != 2 {
		t.Errorf("corner (0,0) has %d neighbours, want 2", n)
	}
}

// Tiles on the map's edge are coastal whatever their terrain, and ships make
// passing them cheap.
func TestCoastIsCheapToCross(t *testing.T) {
	w := newTestWorld(t, 1)
	want := map[[2]bool]float64{{true, true}: 0.25, {true, false}: 0.625, {false, true}: 0.625, {false, false}: 1}
	seen := map[[2]bool]bool{}
	for _, l := range w.Links {
		a, b := w.Regions[l.A], w.Regions[l.B]
		k := [2]bool{a.Coastal, b.Coastal}
		seen[k] = true
		if l.Hop != want[k] {
			t.Errorf("%s-%s hop %v, want %v", a.Name, b.Name, l.Hop, want[k])
		}
	}
	if len(seen) < 3 {
		t.Errorf("map lacks some link types: %v", seen)
	}
	coastal := 0
	for _, r := range w.Regions {
		edge := r.Row == 0 || r.Col == 0 || r.Row == w.Rows-1 || r.Col == w.Cols-1
		if r.Coastal != edge {
			t.Errorf("tile %s coastal %v", r.Name, r.Coastal)
		}
		if r.Coastal {
			coastal++
		}
		fish := false
		for _, f := range r.Firms {
			fish = fish || f.Name == "Fishery"
		}
		if fish != r.Coastal {
			t.Errorf("tile %s (coastal %v) has a fishery: %v", r.Name, r.Coastal, fish)
		}
	}
	if coastal != 20 {
		t.Errorf("%d coastal tiles, want 20", coastal)
	}
}

func TestBuildWorldRejectsBadMaps(t *testing.T) {
	if _, err := BuildWorld(1, []string{"PX"}); err == nil {
		t.Error("unknown terrain symbol accepted")
	}
	if _, err := BuildWorld(1, []string{"PP", "P"}); err == nil {
		t.Error("ragged map accepted")
	}
	w, err := BuildWorld(math.Inf(1), DefaultMap)
	if err != nil || len(w.Traders) != 0 {
		t.Errorf("autarky world should have no traders (err %v)", err)
	}
}

// Economic sanity: cheaper transport means more trade, and trade beats autarky.
func TestTransportCostShapesTrade(t *testing.T) {
	if testing.Short() {
		t.Skip("long")
	}
	run := func(cost float64) (volume, utility float64) {
		w := newTestWorld(t, cost)
		const ticks, window = 600, 50
		for i := 0; i < ticks; i++ {
			w.Step()
			if i >= ticks-window {
				for _, tr := range w.Traders {
					volume += tr.Shipped / window
				}
				for _, r := range w.Regions {
					utility += r.Utility() / window / float64(len(w.Regions))
				}
			}
		}
		return volume, utility
	}
	vCheap, uCheap := run(0.5)
	vDear, uDear := run(2)
	_, uAutarky := run(math.Inf(1))
	t.Logf("volume %.1f (cost 0.5) vs %.1f (cost 2); utility %.3f / %.3f / %.3f (autarky)",
		vCheap, vDear, uCheap, uDear, uAutarky)
	if vCheap <= vDear {
		t.Errorf("cheaper transport should mean more trade: %v vs %v", vCheap, vDear)
	}
	if !(uCheap > uDear && uDear > uAutarky) {
		t.Errorf("utility should fall with transport cost: %v, %v, autarky %v", uCheap, uDear, uAutarky)
	}
}

func BenchmarkStep(b *testing.B) {
	w, _ := BuildWorld(1, DefaultMap)
	for i := 0; i < 200; i++ {
		w.Step()
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.Step()
	}
}
