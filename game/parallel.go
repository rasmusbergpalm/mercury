package game

import (
	"runtime"
	"sync"
)

// Parallel days: the tiles' own work (firms planning, the labour and goods
// markets, merchants planning and shipping, production, dividends) runs on
// every core at once. Each tile touches only its own state while it does;
// what it owes others (taxes and purchases against its nation's treasury,
// merchants' sales on behalf of another tile's trading house) is written
// into ledgers that are settled one by one, in a fixed order, afterwards.
// So a day's result doesn't depend on how the work was shared out, and the
// simulation stays deterministic.

// parallelMin is the fewest tiles worth spreading over several cores.
const parallelMin = 64

// eachRegion runs f for every tile, on all cores in big worlds.
func (w *World) eachRegion(f func(r *Region)) {
	n := len(w.Regions)
	workers := runtime.GOMAXPROCS(0)
	if n < parallelMin || workers < 2 {
		for _, r := range w.Regions {
			f(r)
		}
		return
	}
	// small chunks, handed out in turn, keep the cores evenly busy
	chunk := max(1, n/(workers*8))
	var next int
	var mu sync.Mutex
	var wg sync.WaitGroup
	for k := 0; k < workers; k++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				mu.Lock()
				lo := next
				next += chunk
				mu.Unlock()
				if lo >= n {
					return
				}
				for _, r := range w.Regions[lo:min(lo+chunk, n)] {
					f(r)
				}
			}
		}()
	}
	wg.Wait()
}

// ledger is what a tile owes its nation from today's parallel work.
type ledger struct {
	tax     float64 // income tax collected from its people
	paid    float64 // the nation's purchases here: what it paid, as a negative sum
	tariffs float64 // import duties on goods sold here
}

// settleLedgers pays every tile's ledger to its nation, and every route's
// sales and duties to its trading house, in tile and route order.
func (w *World) settleLedgers() {
	for _, r := range w.Regions {
		l := &r.ledger
		if n := r.Nation; n != nil {
			n.Treasury += l.tax + l.paid + l.tariffs
			n.Revenue += l.tax
			n.Spending -= l.paid
			n.TariffRevenue += l.tariffs
		}
		*l = ledger{}
	}
	for _, r := range w.Regions {
		for _, t := range r.soldHere {
			t.House.Cash += t.proceeds
			t.House.Spent += t.duty
			t.proceeds, t.duty = 0, 0
		}
		r.soldHere = r.soldHere[:0]
	}
}

// settleTaxes pays the nation the taxes in its tiles' ledgers.
func (n *Nation) settleTaxes() {
	for _, r := range n.Regions {
		n.Treasury += r.ledger.tax
		n.Revenue += r.ledger.tax
		r.ledger.tax = 0
	}
}
