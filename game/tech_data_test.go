package game

import "testing"

// The recipes that start switched off are exactly those a tech switches on.
func TestTechsSwitchOnWhatStartsOff(t *testing.T) {
	off := map[string]bool{}
	for _, rs := range [][]Recipe{RawRecipes, ProcessingRecipes} {
		for _, r := range rs {
			if r.StartsOff {
				off[r.Firm] = true
			}
		}
	}
	for _, m := range MethodRecipes {
		if m.StartsOff {
			off[m.Firm] = true
		}
	}
	for f := range FirmTech {
		if !off[f] {
			t.Errorf("%s is switched on by %s but starts on", f, FirmTech[f])
		}
	}
	for f := range off {
		if _, ok := FirmTech[f]; !ok {
			t.Errorf("%s starts off and no tech switches it on", f)
		}
	}
}
