package game

import "testing"

// hugeSpec is a generated map of about a thousand tiles.
func hugeSpec(tb testing.TB) GameSpec {
	m := MapSizes[len(MapSizes)-1]
	spec, err := GenerateGame(MapOptions{Rows: m.Rows, Cols: m.Cols, Nations: m.Nations, Seed: 1})
	if err != nil {
		tb.Fatal(err)
	}
	spec.AI = true
	return spec
}

// BenchmarkStepHuge steps a thousand-tile world (unsettled: settling it
// takes minutes; cmd/probe -map huge measures a settled one).
func BenchmarkStepHuge(b *testing.B) {
	w, err := buildGame(hugeSpec(b), false, nil)
	if err != nil {
		b.Fatal(err)
	}
	for i := 0; i < 50; i++ {
		w.Step()
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.Step()
	}
}
