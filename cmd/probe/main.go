// Command probe runs an AI game and prints yearly statistics (for
// calibration), and how long a day takes to simulate. -map picks a
// generated map (small, medium, large or huge) instead of the classic one.
package main

import (
	"flag"
	"fmt"
	"sort"
	"time"

	"github.com/rasmusbergpalm/mercury/game"
)

func main() {
	years := flag.Int("years", 20, "years to simulate")
	every := flag.Int("every", 2, "print every n years")
	mapSize := flag.String("map", "classic", "classic, or a generated map's size: small, medium, large or huge")
	nations := flag.Int("nations", 0, "nations on a generated map (0: the size's default)")
	seed := flag.Int64("seed", 1, "seed of a generated map and of the computer's choices")
	flag.Parse()
	spec := game.DefaultGame
	if *mapSize != "classic" {
		var err error
		if spec, err = generate(*mapSize, *nations, *seed); err != nil {
			panic(err)
		}
	}
	spec.AI, spec.Seed = true, *seed
	settling := time.Now()
	w, err := game.BuildGame(spec)
	if err != nil {
		panic(err)
	}
	fmt.Printf("%d tiles, settled %d days in %v\n", len(w.Regions), game.SettleDays(len(w.Regions)), time.Since(settling).Round(time.Millisecond))
	start := time.Now()
	for y := 1; y <= *years; y++ {
		for d := 0; d < 365; d++ {
			w.Step()
		}
		if y%*every != 0 {
			continue
		}
		fmt.Printf("%v a day so far\n", (time.Since(start) / time.Duration(365*y)).Round(time.Microsecond))
		pop := w.Population()
		var labor [3]float64
		for _, r := range w.Regions {
			for _, f := range r.Firms {
				labor[f.Sector] += f.Labor
			}
		}
		tot := labor[0] + labor[1] + labor[2]
		u := 0.0
		for _, r := range w.Regions {
			u += r.Utility() * r.Population() / pop
		}
		cityPop := 0.0
		for _, r := range w.Regions {
			if w.IsCity(r) {
				cityPop += r.Population()
			}
		}
		fmt.Printf("%d pop %.0f (cities %.0f%%, soldiers %.0f) U %.2f labor primary %.0f%% industry %.0f%% service %.0f%%\n",
			w.Date().Year(), pop+w.Soldiers(), 100*cityPop/pop, w.Soldiers(), u, 100*labor[1]/tot, 100*labor[0]/tot, 100*labor[2]/tot)
		big := append([]*game.Region(nil), w.Regions...)
		sort.Slice(big, func(i, j int) bool { return big[i].Population() > big[j].Population() })
		fmt.Print("   largest:")
		for _, r := range big[:6] {
			fmt.Printf(" %s %s %.0f (×%.2f)", r.Name, r.Terrain, r.Population(), r.SizeBonus())
		}
		fmt.Printf("  smallest: %.0f\n", big[len(big)-1].Population())
		for _, n := range w.Nations {
			known := 0
			for _, k := range n.Known {
				if k {
					known++
				}
			}
			war := ""
			for j, x := range n.AtWar {
				if x {
					war += " war:" + w.Nations[j].Name
				}
			}
			arm := ""
			for _, a := range w.ArmiesOf(n) {
				sup := ""
				for g := range game.Doctrines[a.Doctrine].Needs {
					sup += fmt.Sprintf(" %s:%.2f", game.Goods[g].Name, a.Supply[g])
				}
				arm += fmt.Sprintf(" [%.0f/%.0f %s eff %.2f @%s%s]", a.Men, a.Target, game.Doctrines[a.Doctrine].Name, a.Effectiveness(), w.Regions[a.Region].Name, sup)
			}
			fmt.Printf("   %-9s tiles %2d income %6.0f techs %2d res %.0f%s%s\n", n.Name, len(n.Regions), n.IncomeEMA, known, n.ResearchPoints, war, arm)
		}
	}
	fmt.Println("events:", len(w.Events), "elapsed", time.Since(start))
	for _, e := range w.EventsSince(0, -1) {
		if e.Kind == "war" || e.Kind == "peace" || e.Kind == "eliminated" || e.Kind == "taken" {
			fmt.Println("  ", game.Date(e.Tick).Format("2006-01-02"), e.Text)
		}
	}
}

func generate(size string, nations int, seed int64) (game.GameSpec, error) {
	for _, m := range game.MapSizes {
		if m.Name == size {
			if nations == 0 {
				nations = m.Nations
			}
			return game.GenerateGame(game.MapOptions{Rows: m.Rows, Cols: m.Cols, Nations: nations, Seed: seed})
		}
	}
	return game.GameSpec{}, fmt.Errorf("no map size %q", size)
}
