// Run runs the star-lanes economy (package starlanes) for ten years, then
// riots on Cinder, the one system with aether wells, cut the wells'
// productivity by 85% for two years: what happens to the price of the fuel
// all trade runs on, to the trade, and to the systems that live by it.
//
//	go run ./examples/starlanes/run
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/rasmusbergpalm/mercury/economy"
	"github.com/rasmusbergpalm/mercury/examples/starlanes"
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
	years := flag.Int("years", 10, "years before the riots")
	riot := flag.Int("riots", 2, "years of riots")
	flag.Parse()
	before, during, err := run(*years, *riot)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("%d, after %d years (a year's averages):\n", starlanes.StartYear+*years, *years)
	print(before, nil)
	fmt.Printf("\n%d, after %d years of riots on Cinder, against %d:\n", starlanes.StartYear+*years+*riot, *riot, starlanes.StartYear+*years)
	print(during, before)
}

// run builds the economy, runs it for years, then has riots on Cinder for
// riot years, and returns the last year's averages before and during them.
func run(years, riot int) (before, during []system, err error) {
	if err := starlanes.Load(); err != nil {
		return nil, nil, err
	}
	w, err := starlanes.Build()
	if err != nil {
		return nil, nil, err
	}
	before = observe(w, years)
	starlanes.Riot(w, 0.15)
	during = observe(w, riot)
	return before, during, nil
}

// observe runs the world for years and averages the last of them.
func observe(w *economy.World, years int) []system {
	aether, freight := economy.G("aether"), economy.Transport
	hops := starlanes.Hops()
	out := make([]system, len(w.Regions))
	days := starlanes.TicksPerYear
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
