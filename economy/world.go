package economy

import "math"

// World is a set of regions linked by traders that pay transport costs.
type World struct {
	Rows, Cols int
	Regions    []*Region
	Traders    []*Trader
	Links      []Link
	Tick       int
	// made is, per good, whether anyone in the world can make it or still
	// has some to sell; merchants leave the rest alone (refreshMade)
	made []bool
	// Rules, off unless switched on: firms invest in building levels,
	// people migrate between neighbouring tiles, and populations grow.
	Investment, Migration, Demography bool
	// TrackGDP keeps every tile's real GDP chain (Region.Real).
	TrackGDP bool
	// Seed keys the economy's random draws (owners' resolve to invest), so
	// that the same seed gives the same history.
	Seed int64
	// Plan is what the world was built from (BuildWorld), so
	// that a snapshot can rebuild it.
	Plan Plan
	// TradePrices are the prices trade happened at in the last tick, per region.
	TradePrices [][]float64

	adj  [][][2]int // per tile: (neighbour, link index)
	grid []int      // per cell of the map (row*Cols+col): its region, or -1 for sea
}

// Link is a lane between two regions and the cost of one hop along it: the
// lane's length times the average of the two tiles' movement costs.
type Link struct {
	A, B   int
	Hop    float64
	Length float64
}

// Each tile's trading house starts with this much cash per route.
const houseCashPerRoute = 1.0

// NewWorld wires regions and traders together: every route belongs to the
// trading house of the tile it leaves from.
func NewWorld(rows, cols int, regions []*Region, traders []*Trader, links []Link) *World {
	connect(traders)
	for _, r := range regions {
		h := r.House
		h.Cash = houseCashPerRoute * float64(len(h.Routes))
		h.CashBuffer = h.Cash
	}
	w := &World{Rows: rows, Cols: cols, Regions: regions, Traders: traders, Links: links}
	for _, r := range regions {
		w.TradePrices = append(w.TradePrices, r.Prices())
	}
	w.grid = make([]int, rows*cols)
	for i := range w.grid {
		w.grid[i] = -1
	}
	for _, r := range regions {
		if len(w.grid) > 0 { // a graph map (BuildGraphWorld) has no grid
			w.grid[r.Row*cols+r.Col] = r.ID
		}
	}
	w.adj = make([][][2]int, len(regions))
	for i, l := range links {
		w.adj[l.A] = append(w.adj[l.A], [2]int{l.B, i})
		w.adj[l.B] = append(w.adj[l.B], [2]int{l.A, i})
	}
	return w
}

// RegionAt is the tile at a map cell, or nil for sea (or off the map).
func (w *World) RegionAt(row, col int) *Region {
	if row < 0 || row >= w.Rows || col < 0 || col >= w.Cols {
		return nil
	}
	if id := w.grid[row*w.Cols+col]; id >= 0 {
		return w.Regions[id]
	}
	return nil
}

// landNeighbours calls f for each land tile next to r.
func (w *World) landNeighbours(r *Region, f func(nb *Region) bool) {
	for _, x := range w.adj[r.ID] {
		if !f(w.Regions[x[0]]) {
			return
		}
	}
}

// Money is all cash in the world: its people's and firms', its trading
// houses', and what the tiles' accounts and outside buyers hold. Money only
// changes hands, so it is conserved, unless someone outside the economy
// takes it from the accounts or funds a buyer.
func (w *World) Money() float64 {
	s := 0.0
	for _, r := range w.Regions {
		s += r.Money()
	}
	for _, r := range w.Regions {
		s += r.House.Cash
	}
	for _, r := range w.Regions {
		s += r.Withheld + r.Duties
		for _, b := range r.Buyers {
			s += b.Account
		}
	}
	return s
}

// Step advances the world one tick: BeginTick, PlanTrade, Markets, Ship,
// Produce and EndTick, in turn. A caller that acts within the tick (to buy in
// the markets, or to collect what the tick's work owes it) runs the steps
// itself and acts between them. The tiles' own work runs in parallel (see
// eachRegion); what they owe each other is settled in order after.
func (w *World) Step() {
	w.BeginTick()
	w.PlanTrade()
	w.Markets()
	w.Ship()
	w.Produce()
	w.EndTick()
}

// BeginTick opens the tick: the goods in play are noted, prices recorded,
// and firms and owners plan.
func (w *World) BeginTick() {
	w.refreshMade()
	w.eachRegion(func(r *Region) {
		copy(w.TradePrices[r.ID], r.refreshPrices())
		r.Plan()
		if w.Investment {
			r.planInvestment(w.Seed)
		}
	})
}

// PlanTrade has every trading house plan its routes.
func (w *World) PlanTrade() {
	w.eachRegion(func(r *Region) { r.House.Plan(w.Tick) })
}

// Markets clears the labour market and every goods market in every tile.
func (w *World) Markets() {
	w.eachRegion(func(r *Region) {
		r.LaborMarket()
		r.GoodsMarkets()
	})
	w.settleHouses()
}

// Ship sends what the merchants bought on its way.
func (w *World) Ship() {
	w.eachRegion(func(r *Region) { r.shipAll() })
}

// Produce has firms produce and pay their owners what they don't need.
func (w *World) Produce() {
	w.eachRegion(func(r *Region) {
		r.Produce()
		owners := r.owners()
		for _, f := range r.Firms {
			// keep cash for what the firm wants to spend, but no more than it
			// can grow into: twice what it actually spent today
			r.PayDividends(&f.Cash, f.CashBuffer, math.Min(f.DesiredSpend, 2*f.TickCost+f.CashBuffer), owners)
		}
		// like firms: hold what today's plans need, but no more than the
		// house can grow into (twice what it actually spent today)
		h := r.House
		r.PayDividends(&h.Cash, h.CashBuffer, math.Min(h.DesiredSpend, 2*h.Spent+h.CashBuffer), owners)
	})
	w.settleHouses()
}

// EndTick closes the tick: owners settle their projects, people move and
// multiply, and the tick advances.
func (w *World) EndTick() {
	if w.Investment {
		w.eachRegion((*Region).settleFirms)
	}
	if w.Migration {
		w.Migrate()
	}
	w.Tick++
	starving := hasSubsistence()
	w.eachRegion(func(r *Region) {
		if w.Demography {
			r.grow()
		}
		if starving {
			r.starve()
		}
		if w.TrackGDP {
			r.accumulateGDP()
		}
	})
}

// refreshMade notes which goods are in play: those some enabled firm on
// the map makes. The rest (oil before any oil well is switched on) are left out of the economy
// entirely: no market clears for them, no merchant or state buys them, and
// they have no price.
func (w *World) refreshMade() {
	if w.made == nil {
		w.made = make([]bool, NumGoods)
	}
	for g := range w.made {
		w.made[g] = false
	}
	for _, r := range w.Regions {
		for _, f := range r.Firms {
			if f.Enabled {
				w.made[f.Output] = true
			}
		}
	}
	for _, r := range w.Regions {
		r.made = w.made
	}
}

// Made reports whether good g is in play: someone can make it.
func (w *World) Made(g GoodID) bool {
	return w.made == nil || w.made[g]
}

// shipAll sends what each of the tile's routes can ship today.
func (r *Region) shipAll() {
	for _, t := range r.House.today { // the rest are idle: they ship nothing
		t.Ship()
	}
}

// Migration rules (when World.Migration is on).
const (
	// share of a tile's people who move per tick per unit of relative utility
	// gap: a 10% gap draws about 7% of a tile's people in 365 ticks (with a
	// hop of 0; at a tick a day, a year). Ten times faster, a building boom
	// on a tile emptied its neighbours in a few dozen ticks, and the people
	// left again when it ended.
	MigrationRate  = 0.002
	migrationStart = 50 // ticks before anyone moves (utility records need to settle)
	MinPopulation  = 20 // a tile never empties below this
)

// MigrationShare is the share of from's people who move to to in a tick,
// for a relative gap in living standards and the hop between them. A border
// holds no one back: in the nineteenth century some fifty million Europeans
// left for the Americas and elsewhere, and millions more crossed into the
// industrial regions next door (the Irish into Britain, Poles into the
// Ruhr, Italians into France), going where wages were. So a country that
// lives well draws its neighbours' people.
func MigrationShare(from, to *Region, gap, hop float64) float64 {
	return MigrationRate * gap / (1 + hop)
}

// Migrate moves people along each link from the tile with lower (smoothed)
// utility to the one with higher. Cheaper hops mean more migration. People
// take their cash with them.
func (w *World) Migrate() {
	w.eachRegion(func(r *Region) {
		u := r.Utility()
		if w.Tick == 0 {
			r.UtilityEMA = u
		}
		r.UtilityEMA = 0.9*r.UtilityEMA + 0.1*u
	})
	if w.Tick < migrationStart {
		return
	}
	for _, l := range w.Links {
		from, to := w.Regions[l.A], w.Regions[l.B]
		if to.UtilityEMA < from.UtilityEMA {
			from, to = to, from
		}
		if from.UtilityEMA <= 0 {
			continue
		}
		gap := math.Min((to.UtilityEMA-from.UtilityEMA)/from.UtilityEMA, 0.3)
		pop := from.Population()
		n := math.Min(MigrationShare(from, to, gap, l.Hop)*pop, SizeOf(Movable(from))-MinPopulation) // never below MinPopulation workers
		if n > 1e-9 {
			MovePeople(from, to, n)
		}
	}
}

// Movable are the pops that move, get drafted and come home: the workers
// (or everyone, where there are no classes).
func Movable(r *Region) []*Pop {
	if w := r.ClassPops(Workers); len(w) > 0 && len(w) < len(r.Pops) {
		return w
	}
	return r.Pops
}

func SizeOf(pops []*Pop) float64 {
	s := 0.0
	for _, p := range pops {
		s += p.Size
	}
	return s
}

// MovePeople moves n workers (and their share of cash) between tiles,
// taking from and adding to every worker pop in proportion to its size.
func MovePeople(from, to *Region, n float64) {
	src, dst := Movable(from), Movable(to)
	fromPop, toPop := SizeOf(src), SizeOf(dst)
	n = math.Min(n, fromPop*0.5)
	cash := 0.0
	for _, p := range src {
		k := n * p.Size / fromPop
		c := p.Cash * k / p.Size
		p.Size -= k
		p.Cash -= c
		cash += c
	}
	for _, p := range dst {
		share := p.Size / toPop
		p.Size += n * share
		p.Cash += cash * share
	}
}

// Population is the number of people in the world.
func (w *World) Population() float64 {
	s := 0.0
	for _, r := range w.Regions {
		s += r.Population()
	}
	return s
}

// RefreshHops recomputes the cost of every hop over land, and the
// distance of every land route, from the tiles' movement costs (after a
// tile's LandTransportMult changes, say). Routes of a fixed distance (by sea) keep theirs.
func (w *World) RefreshHops() {
	hop := func(a, b *Region) float64 { return (a.MoveCost() + b.MoveCost()) / 2 }
	for i, l := range w.Links {
		w.Links[i].Hop = l.Length * hop(w.Regions[l.A], w.Regions[l.B])
	}
	for _, t := range w.Traders {
		if !t.FixedDistance {
			t.SetDistance(t.length * hop(t.Origin, t.Dest))
		}
	}
}

// RefreshHopsAt recomputes the hops and land routes touching r.
func (w *World) RefreshHopsAt(r *Region) {
	hop := func(a, b *Region) float64 { return (a.MoveCost() + b.MoveCost()) / 2 }
	for i, l := range w.Links {
		if l.A == r.ID || l.B == r.ID {
			w.Links[i].Hop = l.Length * hop(w.Regions[l.A], w.Regions[l.B])
		}
	}
	for _, t := range w.Traders {
		if !t.FixedDistance && (t.Origin == r || t.Dest == r) {
			t.SetDistance(t.length * hop(t.Origin, t.Dest))
		}
	}
}

// AddTraders opens new routes (sea routes between ports, say): each
// belongs to the trading house of the tile it leaves from.
func (w *World) AddTraders(ts []*Trader) {
	connect(ts)
	w.Traders = append(w.Traders, ts...)
}

// RemoveTraders closes the routes gone reports, at the tiles touched
// reports (their ends): what they had bought or had in transit is lost.
func (w *World) RemoveTraders(gone func(t *Trader) bool, touched func(r *Region) bool) {
	keep := func(ts []*Trader) []*Trader {
		out := ts[:0]
		for _, t := range ts {
			if !gone(t) {
				out = append(out, t)
			}
		}
		return out
	}
	w.Traders = keep(w.Traders)
	for _, x := range w.Regions {
		if !touched(x) {
			continue
		}
		h := x.House
		h.Routes = keep(h.Routes)
		for i, t := range h.Routes {
			t.slot = i
		}
		h.busyKnown, h.today = false, h.today[:0]
		for g := range x.outbound {
			x.outbound[g] = keep(x.outbound[g])
			x.inbound[g] = keep(x.inbound[g])
			x.bidders[g] = x.bidders[g][:0]
		}
	}
}

// Adjacent lists a tile's neighbours over land: (neighbour, link index).
func (w *World) Adjacent(r int) [][2]int { return w.adj[r] }
