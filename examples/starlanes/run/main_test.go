package main

import (
	"testing"

	"github.com/rasmusbergpalm/mercury/economy"
	"github.com/rasmusbergpalm/mercury/examples/starlanes"
)

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

// Freight burns aether: with Cinder's wells making none, once the aether
// already mined is burnt no ship moves and trade between the systems stops.
func TestNoAetherNoTrade(t *testing.T) {
	if err := starlanes.Load(); err != nil {
		t.Fatal(err)
	}
	w, err := starlanes.Build()
	if err != nil {
		t.Fatal(err)
	}
	for d := 0; d < 2*starlanes.TicksPerYear; d++ {
		w.Step()
	}
	aether := economy.G("aether")
	traded := func() (shipped, freight, stock float64) {
		for _, tr := range w.Traders {
			shipped += tr.Shipped
		}
		for _, r := range w.Regions {
			for _, f := range r.Firms {
				switch f.Output {
				case economy.Transport:
					freight += f.LastOutput
				case aether:
					stock += f.Inventory
				}
			}
		}
		return
	}
	before, freight0, _ := traded()
	starlanes.Riot(w, 0)
	stopped := -1
	for d := 0; d < starlanes.TicksPerYear; d++ {
		w.Step()
		if s, _, _ := traded(); stopped < 0 && s < 0.01*before {
			stopped = d + 1
		}
	}
	t.Logf("trade below 1%% of what it was after %d days", stopped)
	after, freight1, _ := traded()
	t.Logf("shipped a day %.1f -> %.4f, freight made %.1f -> %.4f", before, after, freight0, freight1)
	if before <= 0 || freight0 <= 0 {
		t.Fatalf("nothing shipped (%v) or no freight (%v) before", before, freight0)
	}
	if after > 1e-3*before || freight1 > 1e-3*freight0 {
		t.Errorf("a year after the last aether: shipped %.4f a day (%.1f before), freight made %.4f (%.1f before)", after, before, freight1, freight0)
	}
}
