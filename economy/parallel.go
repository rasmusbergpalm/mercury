package economy

import (
	"runtime"
	"sync"
)

// Parallel ticks: the tiles' own work (firms planning, the labour and goods
// markets, merchants planning and shipping, production, dividends) runs on
// every core at once. Each tile touches only its own state while it does;
// what it owes others (merchants' sales on behalf of another tile's
// trading house) is written into ledgers that are settled one by one, in a
// fixed order, afterwards; what it collects in taxes and duties stays in
// its own accounts.
// So a tick's result doesn't depend on how the work was shared out, and the
// simulation stays deterministic.

// ParallelMin is the fewest tiles worth spreading over several cores.
const ParallelMin = 64

// eachRegion runs f for every tile, on all cores in big worlds.
func (w *World) eachRegion(f func(r *Region)) {
	n := len(w.Regions)
	workers := runtime.GOMAXPROCS(0)
	if n < ParallelMin || workers < 2 {
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

// settleHouses pays every route's sales and duties to its trading house,
// in tile and route order.
func (w *World) settleHouses() {
	for _, r := range w.Regions {
		for _, t := range r.soldHere {
			t.House.Cash += t.proceeds
			t.House.Spent += t.duty
			t.proceeds, t.duty = 0, 0
		}
		r.soldHere = r.soldHere[:0]
	}
}
