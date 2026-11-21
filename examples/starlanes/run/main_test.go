package main

import "testing"

// Aether gets dearer the further a system is from Cinder, its one source;
// riots on Cinder make it dearer everywhere, and cut the trade.
func TestStarlanes(t *testing.T) {
	before, during, err := run(4, 1)
	if err != nil {
		t.Fatal(err)
	}
	near, far := 0.0, 0.0
	trade, tradeRiot := 0.0, 0.0
	for i, s := range before {
		switch s.Hops {
		case 1:
			near = max(near, s.Aether)
		case 4:
			far = max(far, s.Aether)
		}
		trade += s.Exports
		tradeRiot += during[i].Exports
		if during[i].Aether <= s.Aether {
			t.Errorf("%s: aether %.2f in the riots, %.2f before", s.Name, during[i].Aether, s.Aether)
		}
	}
	if far <= near {
		t.Errorf("aether four lanes from Cinder costs %.2f, one lane away %.2f", far, near)
	}
	if tradeRiot >= trade {
		t.Errorf("trade %.0f a day in the riots, %.0f before", tradeRiot, trade)
	}
}
