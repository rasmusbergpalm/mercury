package economy

import (
	"encoding/json"
	"testing"
)

// A world restored from a snapshot (written as JSON and read back) goes on
// exactly as the original does.
func TestSnapshotRestoresExactly(t *testing.T) {
	w, err := BuildFullWorld(1, DefaultMap, nil)
	if err != nil {
		t.Fatal(err)
	}
	w.Investment, w.Migration, w.Demography, w.TrackGDP = true, true, true, true
	w.Regions[3].IncomeTax, w.Traders[7].Tariff = 0.1, 0.2
	for d := 0; d < 200; d++ {
		w.Step()
	}
	b, err := json.Marshal(w.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	var s Snapshot
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	v, err := Restore(&s)
	if err != nil {
		t.Fatal(err)
	}
	for d := 0; d < 100; d++ {
		w.Step()
		v.Step()
	}
	a, _ := json.Marshal(w.Snapshot())
	c, _ := json.Marshal(v.Snapshot())
	if string(a) != string(c) {
		t.Error("the restored world went its own way")
	}
}
