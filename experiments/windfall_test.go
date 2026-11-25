package experiments

import (
	"testing"

	. "github.com/rasmusbergpalm/mercury/economy"
)

// A windfall makes a tile's firms more productive, but never its land: the
// windfall experiments once made half again as much land along with
// everything else.
func TestWindfallLeavesTheLand(t *testing.T) {
	w := world()
	r := w.Regions[14]
	before := map[*Method]float64{}
	for _, f := range r.Firms {
		for _, m := range f.Methods {
			before[m] = m.TFP
		}
	}
	windfall(r, 1.5, nil)
	land := false
	for _, f := range r.Firms {
		for _, m := range f.Methods {
			switch want := before[m] * 1.5; {
			case f.Fixed:
				land = true
				if m.TFP != before[m] {
					t.Errorf("the windfall made more land: %s's TFP %v -> %v", f.Name, before[m], m.TFP)
				}
			case m.TFP != want:
				t.Errorf("%s's %s: TFP %v, want %v", f.Name, m.Name, m.TFP, want)
			}
		}
	}
	if !land {
		t.Fatal("no landowner in the tile")
	}
}
