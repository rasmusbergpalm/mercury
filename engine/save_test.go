package engine

import (
	"encoding/json"
	"testing"
)

func roundTrip(t *testing.T, w *World) *World {
	t.Helper()
	snap, err := w.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}
	var back Snapshot
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	r, err := Restore(&back)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// A restored game continues bit for bit like the original.
func TestSaveLoadContinuesExactly(t *testing.T) {
	w := newAIGame(t)
	for i := 0; i < 400; i++ {
		w.Step()
	}
	w.Nations[1].SetTariff(-1, 0.2)
	w.Nations[0].Embargo[2] = true
	w.Build(w.Nations[2].Regions[0].ID)
	r := roundTrip(t, w)
	for i := 0; i < 300; i++ {
		w.Step()
		r.Step()
	}
	for i, a := range w.Regions {
		b := r.Regions[i]
		if a.Wage != b.Wage || a.Population() != b.Population() || a.Infra != b.Infra {
			t.Fatalf("region %s differs after load", a.Name)
		}
		for g := range a.Markets {
			if a.Markets[g].Price != b.Markets[g].Price {
				t.Fatalf("region %s %s price differs after load", a.Name, Goods[g].Name)
			}
		}
		for j, f := range a.Firms {
			if f.Levels != b.Firms[j].Levels || f.Cash != b.Firms[j].Cash {
				t.Fatalf("region %s %s differs after load", a.Name, f.Name)
			}
		}
	}
	for i, n := range w.Nations {
		m := r.Nations[i]
		if n.Treasury != m.Treasury || len(n.Regions) != len(m.Regions) || n.ResearchPoints != m.ResearchPoints {
			t.Fatalf("%s differs after load", n.Name)
		}
	}
	if w.Money() != r.Money() {
		t.Fatal("money differs after load")
	}
}

func TestRestoreRejectsBadSnapshots(t *testing.T) {
	w := newTestGame(t)
	snap, _ := w.Snapshot()
	snap.Version = 99
	if _, err := Restore(snap); err == nil {
		t.Error("wrong version accepted")
	}
	snap, _ = w.Snapshot()
	snap.Regions = snap.Regions[1:]
	if _, err := Restore(snap); err == nil {
		t.Error("truncated snapshot accepted")
	}
	snap, _ = w.Snapshot()
	snap.Nations[1].Regions = append(snap.Nations[1].Regions, snap.Nations[0].Regions[0])
	if _, err := Restore(snap); err == nil {
		t.Error("a tile owned twice accepted")
	}
	if _, err := newTestWorld(t, 1).Snapshot(); err == nil {
		t.Error("classic world snapshotted")
	}
}
