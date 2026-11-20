package game

import (
	"encoding/json"
	"fmt"
	"testing"
)

func flatJSON(prefix string, v any, out map[string]string) {
	switch x := v.(type) {
	case map[string]any:
		for k, y := range x {
			flatJSON(prefix+"."+k, y, out)
		}
	case []any:
		for i, y := range x {
			flatJSON(fmt.Sprintf("%s[%d]", prefix, i), y, out)
		}
	default:
		out[prefix] = fmt.Sprint(x)
	}
}

// A restored game plays on exactly as the game it was saved from: every
// field the next days depend on is in the snapshot.
func TestRestoredGamePlaysOn(t *testing.T) {
	w := newTestGame(t)
	for i := 0; i < 40; i++ {
		w.Step()
	}
	s, _ := w.Snapshot()
	b, _ := json.Marshal(s)
	var s2 Snapshot
	json.Unmarshal(b, &s2)
	r, err := Restore(&s2)
	if err != nil {
		t.Fatal(err)
	}
	for day := 0; day < 3; day++ {
		w.Step()
		r.Step()
		a, _ := w.Snapshot()
		c, _ := r.Snapshot()
		ja, _ := json.Marshal(a)
		jc, _ := json.Marshal(c)
		var ma, mc any
		json.Unmarshal(ja, &ma)
		json.Unmarshal(jc, &mc)
		fa, fc := map[string]string{}, map[string]string{}
		flatJSON("", ma, fa)
		flatJSON("", mc, fc)
		n := 0
		for k, v := range fa {
			if fc[k] != v && n < 15 {
				t.Logf("day %d %s: %s vs %s", day, k, v, fc[k])
				n++
			}
		}
		if n > 0 {
			t.Fatalf("differs on day %d", day)
		}
	}
}
