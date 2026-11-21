// Starlanes runs an economy far from Mercury's 1820 example, to show the
// engine's range: twelve star systems joined by a sparse web of lanes, in
// which every ship between systems burns aether, a fuel found in only one
// of them. Nothing in the engine changes; the goods, needs, recipes and
// kinds of system are this directory's data files, and the map is a graph
// (economy.BuildWorld) rather than a hex grid.
//
// Freight, the transport merchants buy, is made from labour and aether.
// A system with no aether can still receive goods but can't send any, so
// the systems' trade, and with it their living standards, hang on the
// aether trade out of Cinder. After ten years Cinder is cut off for three:
// a blockade of the one source of the fuel all trade runs on.
//
//	go run ./examples/starlanes
package main

import (
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"os"

	"github.com/rasmusbergpalm/mercury/economy"
)

//go:embed data/*.json
var data embed.FS

// The systems and the lanes between them. Cinder, a barren world, has the
// only aether wells.
var (
	systems = []economy.Place{
		{Name: "Cinder", Terrain: "barren"},
		{Name: "Halcyon", Terrain: "garden"},
		{Name: "Tamsin", Terrain: "ocean"},
		{Name: "Vey", Terrain: "forge"},
		{Name: "Corran", Terrain: "ice"},
		{Name: "Pellam", Terrain: "garden"},
		{Name: "Drace", Terrain: "forge"},
		{Name: "Mirrow", Terrain: "ocean"},
		{Name: "Saltreach", Terrain: "ice"},
		{Name: "Umber", Terrain: "garden"},
		{Name: "Quillon", Terrain: "forge"},
		{Name: "Isk", Terrain: "ocean"},
	}
	lanes = []economy.Lane{
		{A: 0, B: 1, Length: 1.5}, {A: 0, B: 3, Length: 2},
		{A: 1, B: 2, Length: 1}, {A: 1, B: 5, Length: 2},
		{A: 2, B: 4, Length: 1.5}, {A: 3, B: 6, Length: 1},
		{A: 5, B: 6, Length: 1.5}, {A: 5, B: 7, Length: 1},
		{A: 4, B: 8, Length: 2}, {A: 7, B: 9, Length: 1.5},
		{A: 8, B: 9, Length: 1}, {A: 9, B: 10, Length: 2},
		{A: 6, B: 10, Length: 2.5}, {A: 10, B: 11, Length: 1.5},
	}
)

const source = 0 // Cinder

// The economy counts ticks. Its people and owners are calibrated for a
// tick of a day (they spend a twentieth of their savings a tick, and take
// 90 ticks to build), so here too a tick is a standard day, and the
// economy starts on the first day of 2300.
const (
	ticksPerYear = 365
	startYear    = 2300
)

// system is what a system looks like, averaged over a year.
type system struct {
	Name                  string
	Hops                  int
	Aether, Freight       float64 // prices
	Exports, People, Wage float64 // exports: value shipped out a day
	Living                float64
}

func main() {
	years := flag.Int("years", 10, "years before the blockade")
	cut := flag.Int("blockade", 3, "years of blockade")
	flag.Parse()
	before, after, err := run(*years, *cut)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("%d, after %d years (a year's averages):\n", startYear+*years, *years)
	print(before, nil)
	fmt.Printf("\n%d, after %d years of Cinder blockaded, against %d:\n", startYear+*years+*cut, *cut, startYear+*years)
	print(after, before)
}

// run builds the economy, runs it for years, then blockades Cinder for cut
// years, and returns the last year's averages before and during the
// blockade.
func run(years, cut int) (before, after []system, err error) {
	sub, err := fs.Sub(data, "data")
	if err != nil {
		return nil, nil, err
	}
	if err := economy.LoadData(sub); err != nil {
		return nil, nil, err
	}
	w, err := economy.BuildWorld(1, economy.Graph{Places: systems, Lanes: lanes})
	if err != nil {
		return nil, nil, err
	}
	w.Investment, w.Migration = true, true
	before = observe(w, years)
	for _, t := range w.Traders { // nothing in or out of Cinder
		if t.Origin.ID == source || t.Dest.ID == source {
			t.Closed = true
		}
	}
	after = observe(w, cut)
	return before, after, nil
}

// observe runs the world for years and averages the last of them.
func observe(w *economy.World, years int) []system {
	aether, freight := economy.G("aether"), economy.Transport
	hops := hopsFrom(source, len(w.Regions))
	out := make([]system, len(w.Regions))
	days := int(ticksPerYear)
	for d := 0; d < years*days; d++ {
		w.Step()
		if d < (years-1)*days {
			continue
		}
		for _, t := range w.Traders {
			out[t.Origin.ID].Exports += t.Shipped * t.Origin.Markets[t.Good].Price
		}
		for i, r := range w.Regions {
			s := &out[i]
			s.Aether += r.Markets[aether].Price
			s.Freight += r.Markets[freight].Price
			s.People += r.Population()
			s.Wage += r.Wage
			s.Living += r.Utility()
		}
	}
	for i, r := range w.Regions {
		s := &out[i]
		s.Name, s.Hops = r.Name, hops[i]
		for _, x := range []*float64{&s.Aether, &s.Freight, &s.Exports, &s.People, &s.Wage, &s.Living} {
			*x /= float64(days)
		}
	}
	return out
}

// hopsFrom counts the lanes between each system and src.
func hopsFrom(src, n int) []int {
	hops := make([]int, n)
	for i := range hops {
		hops[i] = -1
	}
	hops[src] = 0
	for queue := []int{src}; len(queue) > 0; queue = queue[1:] {
		a := queue[0]
		for _, l := range lanes {
			for _, e := range [][2]int{{l.A, l.B}, {l.B, l.A}} {
				if e[0] == a && hops[e[1]] < 0 {
					hops[e[1]] = hops[a] + 1
					queue = append(queue, e[1])
				}
			}
		}
	}
	return hops
}

func print(ss, base []system) {
	fmt.Printf("%-10s %4s %8s %8s %8s %7s %6s %7s\n", "system", "hops", "aether", "freight", "exports", "people", "wage", "living")
	for i, s := range ss {
		if base == nil {
			fmt.Printf("%-10s %4d %8.2f %8.2f %8.1f %7.0f %6.2f %7.2f\n", s.Name, s.Hops, s.Aether, s.Freight, s.Exports, s.People, s.Wage, s.Living)
			continue
		}
		b := base[i]
		pct := func(x, y float64) string { return fmt.Sprintf("%+.0f%%", 100*(x/y-1)) }
		fmt.Printf("%-10s %4d %8s %8s %8s %7s %6s %7s\n", s.Name, s.Hops, pct(s.Aether, b.Aether), pct(s.Freight, b.Freight), pct(s.Exports, b.Exports), pct(s.People, b.People), pct(s.Wage, b.Wage), pct(s.Living, b.Living))
	}
}
