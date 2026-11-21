package main

import "testing"

// Aether gets dearer the further a system is from Cinder, its one source;
// cut Cinder off and trade between the systems stops.
func TestStarlanes(t *testing.T) {
	before, after, err := run(4, 2)
	if err != nil {
		t.Fatal(err)
	}
	near, far := 0.0, 0.0
	for _, s := range before {
		switch s.Hops {
		case 1:
			near = max(near, s.Aether)
		case 4:
			far = max(far, s.Aether)
		}
	}
	if far <= near {
		t.Errorf("aether four lanes from Cinder costs %.2f, one lane away %.2f", far, near)
	}
	for i, s := range after {
		if s.Exports > 0.1*before[i].Exports {
			t.Errorf("%s still exports %.1f a day of %.1f under the blockade", s.Name, s.Exports, before[i].Exports)
		}
	}
}
