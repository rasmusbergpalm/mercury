package engine

import (
	"fmt"
	"strings"
	"sync"
)

// SettleYears is how long the world runs before a game starts: no nations,
// no technology, no construction and no population growth, only markets,
// trade and migration, so that the game opens on prices that have found
// their level and people who have moved to where the work is, instead of
// every price at £1.
const SettleYears = 20

// settled caches the settled world per map and transport cost: the run is
// deterministic, so it is done once per process and copied into each game.
var settled = struct {
	sync.Mutex
	worlds map[string]*settledWorld
}{worlds: map[string]*settledWorld{}}

type settledWorld struct {
	once sync.Once
	w    *World
	err  error
}

// settledBase runs (or recalls) the settling of a game's map.
func settledBase(spec GameSpec) (*World, error) {
	key := fmt.Sprintf("%g|%s", spec.TransportCost, strings.Join(spec.Map, "/"))
	settled.Lock()
	s, ok := settled.worlds[key]
	if !ok {
		s = &settledWorld{}
		settled.worlds[key] = s
	}
	settled.Unlock()
	s.once.Do(func() {
		w, err := baseGameWorld(spec)
		if err != nil {
			s.err = err
			return
		}
		w.Migration = true
		for d := 0; d < SettleYears*365; d++ {
			w.Step()
		}
		s.w = w
	})
	return s.w, s.err
}

// settleFrom copies a settled world's markets, people, firms and traders
// into a freshly built one of the same map (the same state a save keeps).
func (w *World) settleFrom(b *World) {
	for i, r := range w.Regions {
		br := b.Regions[i]
		r.Wage, r.UtilityEMA, r.House.Cash = br.Wage, br.UtilityEMA, br.House.Cash
		for g, m := range br.Markets {
			r.Markets[g].Price, r.Markets[g].Supply, r.Markets[g].Demand, r.Markets[g].Trend = m.Price, m.Supply, m.Demand, m.Trend
		}
		for j, bp := range br.Pops {
			p := r.Pops[j]
			p.Size, p.Cash, p.Jobs = bp.Size, bp.Cash, append([]Job(nil), bp.Jobs...)
			copy(p.Consumed, bp.Consumed)
		}
		for j, bf := range br.Firms {
			f := r.Firms[j]
			f.Cash, f.Inventory, f.Labor, f.LastOutput, f.ProfitEMA = bf.Cash, bf.Inventory, bf.Labor, bf.LastOutput, bf.ProfitEMA
		}
		copy(w.TradePrices[i], b.TradePrices[i])
		r.updateAgglomeration()
	}
	for i, bt := range b.Traders {
		t := w.Traders[i]
		t.Q, t.StockOrigin, t.StockDest, t.RelMargin, t.Shipped, t.Margin = bt.Q, bt.StockOrigin, bt.StockDest, bt.RelMargin, bt.Shipped, bt.Margin
	}
}
