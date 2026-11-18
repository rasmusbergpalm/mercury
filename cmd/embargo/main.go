// Command embargo runs an AI game in which every nation embargoes every
// other, and reports how well armies are supplied (a diagnostic).
package main

import (
	"fmt"

	"github.com/rasmusbergpalm/mercury/game"
)

func main() {
	spec := game.DefaultGame
	spec.AI = true
	w, err := game.BuildGame(spec)
	if err != nil {
		panic(err)
	}
	for _, n := range w.Nations {
		for j := range n.Embargo {
			n.Embargo[j] = j != n.ID
		}
	}
	for day := 1; day <= 25*365; day++ {
		w.Step()
		if day%(365*2) != 0 {
			continue
		}
		fmt.Println(w.Date().Format("2006"))
		for _, a := range w.Armies {
			fmt.Printf("  %-9s army %2d men %5.1f/%5.1f eff %.2f", a.Nation.Name, a.ID, a.Men, a.Target, a.Effectiveness())
			for g := range game.Doctrines[a.Doctrine].Needs {
				src := a.Source(g)
				where, offered, price := "-", 0.0, 0.0
				if src >= 0 {
					r := w.Regions[src]
					where, offered, price = r.Name, r.Markets[g].Supply, r.Markets[g].Price
				}
				fmt.Printf(" | %s %.0f%% %2.0fd @%s offer %.2f p %.1f", game.Goods[g].Name, 100*a.Supply[g], a.StockDays(g), where, offered, price)
			}
			fmt.Println()
		}
		for _, n := range w.Nations {
			fmt.Printf("  %-9s spent %.0f income %.0f treasury %.0f tiles %d\n", n.Name, n.SpentOn, n.IncomeEMA, n.Treasury, len(n.Regions))
		}
	}
}
