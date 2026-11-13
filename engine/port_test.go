package engine

import (
	"encoding/json"
	"errors"
	"testing"
)

// A nation may have one port for every TilesPerPort tiles, and at least
// one; ports being built count.
func TestPortAllowance(t *testing.T) {
	spec, err := DivideLand(generated(t, 3, 2)) // ten nations of about 99 tiles
	if err != nil {
		t.Fatal(err)
	}
	w, err := buildGame(spec, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	n := w.Nations[0]
	w.GrantTech(n.ID, "Ocean sailing")
	if want := len(n.Regions) / TilesPerPort; n.PortAllowance() != want || want < 2 {
		t.Fatalf("%d tiles allow %d ports, want %d", len(n.Regions), n.PortAllowance(), want)
	}
	built := 0
	for _, r := range n.Regions {
		if !r.Coastal {
			continue
		}
		err := w.BuildPort(r.ID)
		if built < n.PortAllowance() {
			if err != nil {
				t.Fatalf("port %d: %v", built+1, err)
			}
			built++
		} else if !errors.Is(err, ErrPortLimit) {
			t.Fatalf("a port beyond the allowance: %v", err)
		}
	}
	small := &Nation{Regions: n.Regions[:3]}
	if small.PortAllowance() != 1 {
		t.Errorf("a nation of 3 tiles may have %d ports, want 1", small.PortAllowance())
	}
}

// Pulling a port down closes its sea routes and leaves the rest as they
// would be had it never been built: a save restores the same world, and it
// carries on the same.
func TestRemovePort(t *testing.T) {
	spec, err := DivideLand(generated(t, 1, 3))
	if err != nil {
		t.Fatal(err)
	}
	w, err := buildGame(spec, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	var ports []*Region
	for _, n := range w.Nations {
		w.GrantTech(n.ID, "Ocean sailing")
		for _, r := range n.Regions {
			if r.Coastal && len(ports) < 4 {
				r.Port = true
				ports = append(ports, r)
				break
			}
		}
	}
	if len(ports) < 3 {
		t.Fatalf("only %d ports", len(ports))
	}
	for i := 0; i < 40; i++ {
		w.Step()
	}
	land := 0
	for _, x := range w.Traders {
		if !x.Sea {
			land++
		}
	}
	gone := ports[1]
	if err := w.RemovePort(gone.ID); err != nil {
		t.Fatal(err)
	}
	if err := w.RemovePort(gone.ID); !errors.Is(err, ErrNoPort) {
		t.Errorf("removed twice: %v", err)
	}
	left := len(ports) - 1
	if want := land + left*(left-1)*len(GameTradable); len(w.Traders) != want {
		t.Errorf("%d routes after the removal, want %d", len(w.Traders), want)
	}
	for _, x := range w.Traders {
		if x.Origin == gone && x.Sea || x.Dest == gone && x.Sea {
			t.Fatal("a sea route of the removed port is left")
		}
	}
	for _, r := range w.Regions {
		for _, x := range r.House.Routes {
			if x.Sea && (x.Origin == gone || x.Dest == gone) {
				t.Fatal("a house still funds a route of the removed port")
			}
		}
	}
	money := heldMoney(w)
	for i := 0; i < 20; i++ {
		w.Step()
	}
	if relDiff(heldMoney(w), money) > 1e-9 {
		t.Error("money changed after the removal")
	}
	snap, err := w.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(snap)
	var back Snapshot
	json.Unmarshal(raw, &back)
	w2, err := Restore(&back)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		w.Step()
		w2.Step()
	}
	a, _ := w.Snapshot()
	b, _ := w2.Snapshot()
	ra, _ := json.Marshal(a)
	rb, _ := json.Marshal(b)
	if string(ra) != string(rb) {
		t.Error("the restored world went its own way")
	}
}
