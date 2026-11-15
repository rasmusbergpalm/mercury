package engine

import (
	"fmt"
	"math"
	"strings"
	"sync"
)

// SettleYears is how long the world runs before a game starts: no nations,
// no technology and no population growth, only markets, trade, migration
// and firms' investment, so that the game opens on prices that have found
// their level, people who have moved to where the work is and the capital
// its returns justify, instead of every price at £1 and every firm at one
// level. (Opening without that capital, the first decade spent a third of
// its output building it.)
const SettleYears = 20

// Big maps settle for fewer years, so that a new game doesn't take minutes
// to prepare: prices find their level within a couple of years; it is the
// slow drift of people to where the work is that the long run is for, and
// on a big map that goes on in play. A map of settleFullTiles tiles or
// fewer settles for SettleYears; above it, the years shrink with the
// tiles, down to minSettleYears.
const (
	settleFullTiles = 100
	minSettleYears  = 4
)

// After settling, each game runs a short burn-in with its starting nations
// in place (burnIn): taxes, public services and standing armies come in,
// and prices (war materiel's above all) find their level with them, rather
// than lurching in the first months of play. Big maps burn in for a year.
const (
	burnInYears    = 2
	burnInBigYears = 1
)

// BurnInDays is how many days a game on a map of the given number of tiles
// burns in with its nations.
func BurnInDays(tiles int) int {
	if tiles > settleFullTiles {
		return burnInBigYears * TicksPerYear
	}
	return burnInYears * TicksPerYear
}

// PrepareDays is how many days a new game runs before it opens: settling
// (unless the map's settled world is cached) and the burn-in.
func PrepareDays(tiles int) int { return SettleDays(tiles) + BurnInDays(tiles) }

// SettleDays is how many days a map of the given number of tiles settles.
func SettleDays(tiles int) int {
	years := float64(SettleYears)
	if tiles > settleFullTiles {
		years = math.Max(minSettleYears, years*settleFullTiles/float64(tiles))
	}
	return int(years * TicksPerYear)
}

// maxSettled is how many settled worlds are kept: the classic map's, and a
// few small generated ones (each new big map is settled afresh).
const (
	maxSettled      = 8
	maxSettledTiles = 100
)

// settled caches the settled world per map and transport cost: the run is
// deterministic, so it is done once per process and copied into each game.
var settled = struct {
	sync.Mutex
	worlds map[string]*settledWorld
	order  []string // keys, oldest first
}{worlds: map[string]*settledWorld{}}

type settledWorld struct {
	once sync.Once
	w    *World
	err  error
}

// settledBase runs (or recalls) the settling of a game's map, reporting
// progress (days done of the total) if asked.
func settledBase(spec GameSpec, progress func(done, total int)) (*World, error) {
	key := fmt.Sprintf("%g|%s", spec.TransportCost, strings.Join(spec.Map, "/"))
	tiles := 0
	for _, row := range spec.Map {
		tiles += len(row) - strings.Count(row, string(rune(Sea)))
	}
	if tiles > maxSettledTiles { // too big to keep: settle it for this game alone
		return settle(spec, progress)
	}
	settled.Lock()
	s, ok := settled.worlds[key]
	if !ok {
		s = &settledWorld{}
		settled.worlds[key] = s
		settled.order = append(settled.order, key)
		if len(settled.order) > maxSettled {
			delete(settled.worlds, settled.order[0])
			settled.order = settled.order[1:]
		}
	}
	settled.Unlock()
	s.once.Do(func() { s.w, s.err = settle(spec, progress) })
	return s.w, s.err
}

// settle builds a game's map and runs it without nations until it settles.
func settle(spec GameSpec, progress func(done, total int)) (*World, error) {
	w, err := baseGameWorld(spec)
	if err != nil {
		return nil, err
	}
	w.Migration, w.Investment = true, true
	days := SettleDays(len(w.Regions))
	for d := 0; d < days; d++ {
		w.Step()
		if progress != nil && d%30 == 0 {
			progress(d, days)
		}
	}
	return w, nil
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
			f.Levels, f.Age, f.Wear, f.Expansion = bf.Levels, bf.Age, bf.Wear, nil
			f.Urge, f.Projects = bf.Urge, bf.Projects
			for k, m := range f.Methods {
				m.levels = bf.Methods[k].levels
			}
			if bf.Expansion != nil {
				e := *bf.Expansion
				f.Expansion = &e
			}
		}
		copy(w.TradePrices[i], b.TradePrices[i])
		r.updateAgglomeration()
	}
	for i, bt := range b.Traders {
		t := w.Traders[i]
		t.Q, t.StockOrigin, t.StockDest, t.RelMargin, t.Shipped, t.Margin = bt.Q, bt.StockOrigin, bt.StockDest, bt.RelMargin, bt.Shipped, bt.Margin
	}
}

// burnIn runs a new game for some days with its nations in place, the
// computer running each computer nation's peacetime economy (in a server
// game every nation is the computer's until a player joins): taxes, public
// services and standing armies. Nothing that is the game's
// to play happens: no wars or alliances, no research, no state building,
// no taking of free land and no births and deaths (firms do invest). Then
// the clock is set back to the start, with no history or events.
func (w *World) burnIn(days int, progress func(done int)) {
	// a nation the computer doesn't run spends nothing in the burn-in, so
	// it levies nothing either (its taxes would only pile up unspent)
	tax := make([]float64, len(w.Nations))
	for i, n := range w.Nations {
		tax[i] = n.TaxRate
		if !n.AI {
			n.TaxRate = 0
		}
	}
	inv, demo := w.Investment, w.Demography
	w.Demography, w.burning = false, true
	real := make([]GDPChain, len(w.Regions))
	for i, r := range w.Regions {
		real[i] = r.Real.clone()
	}
	for d := 0; d < days; d++ {
		w.Step()
		for _, n := range w.Nations {
			n.ResearchPoints = 0 // no research yet
		}
		if progress != nil && d%30 == 0 {
			progress(d)
		}
	}
	for i, n := range w.Nations {
		if !n.AI {
			n.TaxRate = tax[i]
		}
		n.Researching, n.ResearchPoints, n.RealGDP = -1, 0, 0
		n.Copying, n.CopyPoints = -1, 0
	}
	for i, r := range w.Regions {
		r.Real = real[i]
	}
	w.Investment, w.Demography, w.burning = inv, demo, false
	w.Tick, w.Events, w.History = 0, nil, History{}
}
