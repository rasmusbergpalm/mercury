package engine

import (
	"strings"
	"testing"
)

// The ladder: worlds of rising complexity, each run for years and checked
// day by day by a healthWatch (see health_test.go). A rung's world adds one
// layer to the rung below: one tile on its own; a map of tiles trading and
// migrating; the same map with its firms investing, as every game opens; a
// nation that runs the whole map; three nations side by side; a big
// generated map. Each fails if any of its numbers leaves bounds, and prints
// them all.
//
// The bounds are signals, not laws. They are there to catch someone
// accidentally breaking the economy: a price cycle, goods piling up, a tile
// emptying, a budget swinging, a collapse. They are not there to keep the
// economy from ever changing. When a rung fails, look at what moved and
// why (the rung prints every number; the cmd/probe and century runs show
// the long view):
//
//   - if a change broke something by accident, fix the change;
//   - if the economy now behaves differently on purpose, and the new
//     behaviour is sound (checked against history where there is one,
//     against common sense where there isn't), move the bound, and say
//     why next to it.
//
// Each bound below carries the reason it sits where it does, so the next
// person can tell a deliberate allowance from a guess.

// The bounds. Only a world that doesn't grow reaches a fixed point: markets
// left alone with no investment and no births (rungs 0 and 1) are the
// reference, prices there wiggle 0.1% around their two-month average and
// nobody moves. Firms building capital (rung 2) move prices, wages and
// people as the capital comes in, but smoothly. A nation adds real shocks
// (taxes, building, research), so its bounds are looser, but the budget
// must be steady and nothing may cycle, pile up, empty or collapse. The
// reasons for each:
var (
	// markets alone: settled means settled
	marketBounds = healthLimits{
		PriceWiggle: 0.005, PriceWiggleWorst: 0.015, WageWiggle: 0.005, WageWiggleWorst: 0.015,
		PriceStep: 0.001, PriceDriftYear: 0.02,
		FullSpeed: 0.01,              // only thin markets, now and then
		StockDays: 30, StockSlack: 5, // nothing piles up unsold
		MinTilePeople: 25, // a tile at the migration floor (24) has emptied
		TileLossYear:  0.05, PeopleDropMonth: 0.001,
		MinEmployment: 0.9, LivingDropYear: 0.01,
	}
	// markets with firms investing: capital still coming in moves prices,
	// wages and people a little, but nothing cycles or empties
	investingBounds = healthLimits{
		PriceWiggle: 0.01, PriceWiggleWorst: 0.03, WageWiggle: 0.015, WageWiggleWorst: 0.03,
		PriceStep: 0.002, PriceDriftYear: 0.05,
		FullSpeed: 0.01,
		StockDays: 30, StockSlack: 5,
		MinTilePeople: 25, TileLossYear: 0.15, PeopleDropMonth: 0.001,
		MinEmployment: 0.85, LivingDropYear: 0.02,
		LevelGrowthYear: 0.15,
	}
	// a nation in peace
	nationBounds = healthLimits{
		// a couple of percent of noise from taxes and building; more is a
		// cycle people would see in every price
		PriceWiggle: 0.02, PriceWiggleWorst: 0.05, WageWiggle: 0.02, WageWiggleWorst: 0.05,
		PriceStep: 0.005,
		// fixed money and growing output make prices fall, a few percent a
		// year over a century (8-17x); a quarter in a year is a shock
		PriceDriftYear: 0.25,
		FullSpeed:      0.015,
		StockDays:      30, StockSlack: 5,
		MinTilePeople: 25, TileLossYear: 0.25, PeopleDropMonth: 0.01,
		MinEmployment: 0.8, LivingDropYear: 0.05,
		StateFrom: 90,
		ShortDays: 0, // a state that can't pay its bills lays off soldiers, teachers and doctors
		TaxMin:    0.01, TaxMax: 0.25,
		// within a point or so of its trend: a budget, not a cycle (a small
		// nation's swings with lumpy items: a new tile, an army, a project)
		TaxWiggle: 0.0125,
		// a treasury near its reserve; a small nation's jumps to twice or a
		// bit more when it takes new land, before its spending catches up
		ReserveLow: 0.5, ReserveHigh: 2.5,
		GDPDropMonth: 0.1,
		ArmySupply:   0.95, ArmySupplyWorst: 0.5, // in peace, at home: fed and equipped
		// capital grows two or three percent a year over a century; half
		// again in a year is a boom, then a bust
		LevelGrowthYear: 0.15,
	}
)

// withTechs is l requiring the nation that learns most to learn a tech
// every three years: the pace of a century's early decades (four or five
// in the first, about fifty by 1920).
func withTechs(l healthLimits, years int) healthLimits {
	l.TechsLeader = max(1, years/3)
	return l
}

// oneNationGame is the classic map held whole by one nation.
func oneNationGame(t *testing.T, ai bool) *World {
	t.Helper()
	spec := DefaultGame
	spec.NationMap = nil
	for range DefaultMap {
		spec.NationMap = append(spec.NationMap, strings.Repeat("A", len(DefaultMap[0])))
	}
	spec.Nations = DefaultGame.Nations[:1]
	spec.AI, spec.Seed = ai, 1
	w, err := BuildGame(spec)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

// Rung 0: one tile, no trade, no nation. Left alone it must settle to a
// fixed point: no price or wage moves once it has.
func TestHealthLoneTile(t *testing.T) {
	if testing.Short() {
		t.Skip("long")
	}
	t.Parallel()
	w, err := baseGameWorld(GameSpec{Map: []string{"P"}, TransportCost: 1})
	if err != nil {
		t.Fatal(err)
	}
	for d := 0; d < int(SettleYears*TicksPerYear); d++ {
		w.Step()
	}
	h := newHealthWatch(w)
	h.run(int(2*TicksPerYear), 0)
	h.report(t, healthLimits{
		PriceWiggle: 1e-4, PriceWiggleWorst: 1e-4, WageWiggle: 1e-4, WageWiggleWorst: 1e-4,
		PriceStep: 1e-5, PriceDriftYear: 1e-3, FullSpeed: 1e-9,
		StockDays: 3, StockSlack: 1, // a fixed point: what is made is sold
		MinEmployment: 0.9,
	})
}

// Rung 1: the classic map's tiles trading and migrating, no nations and no
// investment, settled as long as a game is. Markets, merchants and
// migration on their own must hold prices and people steady.
func TestHealthMarkets(t *testing.T) {
	if testing.Short() {
		t.Skip("long")
	}
	t.Parallel()
	w, err := baseGameWorld(DefaultGame)
	if err != nil {
		t.Fatal(err)
	}
	w.Migration = true
	for d := 0; d < SettleDays(len(w.Regions)); d++ {
		w.Step()
	}
	h := newHealthWatch(w)
	h.run(int(2*TicksPerYear), 0)
	h.report(t, marketBounds)
}

// Rung 2: the same with firms investing: the settled world every game
// opens on, run on for two years. Capital still comes in, smoothly.
func TestHealthInvesting(t *testing.T) {
	if testing.Short() {
		t.Skip("long")
	}
	t.Parallel()
	w, err := settle(DefaultGame, nil)
	if err != nil {
		t.Fatal(err)
	}
	h := newHealthWatch(w)
	h.run(int(2*TicksPerYear), 0)
	h.report(t, investingBounds)
}

// Rung 3: one computer nation holding the whole map, from the day the game
// opens, for ten years of peace: taxes, public services, a standing army,
// research, building and private investment on top of the markets.
func TestHealthOneNation(t *testing.T) {
	if testing.Short() {
		t.Skip("long")
	}
	t.Parallel()
	w := oneNationGame(t, true)
	h := newHealthWatch(w)
	h.run(int(10*TicksPerYear), 90)
	h.report(t, withTechs(nationBounds, 10))
}

// Rung 4: the same, run by a player who keeps the tax automated (as a new
// player starts) and builds nothing.
func TestHealthPlayerNation(t *testing.T) {
	if testing.Short() {
		t.Skip("long")
	}
	t.Parallel()
	w := oneNationGame(t, false)
	w.Nations[0].AutoTax = true
	h := newHealthWatch(w)
	h.run(int(10*TicksPerYear), 90)
	h.report(t, withTechs(nationBounds, 10))
}

// Rung 5: the classic game, three computer nations, eight years: before
// any of them may start a war (aiGraceYears over the most aggressive
// temperament), so in peace.
func TestHealthThreeNations(t *testing.T) {
	if testing.Short() {
		t.Skip("long")
	}
	t.Parallel()
	spec := DefaultGame
	spec.AI, spec.Seed = true, 1
	w, err := BuildGame(spec)
	if err != nil {
		t.Fatal(err)
	}
	h := newHealthWatch(w)
	h.run(int(8*TicksPerYear), 90)
	// research grows with a nation's land (each tile starts with its
	// universities): sharing the map three ways, a nation learns a tech
	// in four years at best, at first in ten
	l := nationBounds
	l.TechsLeader = 1
	h.report(t, l)
}

// Rung 6: a generated map of about 140 tiles, all of it one computer
// nation's, for three years from the day it opens.
func TestHealthBigMap(t *testing.T) {
	if testing.Short() {
		t.Skip("long")
	}
	t.Parallel()
	m := MapSizes[1]
	spec, err := GenerateGame(MapOptions{Rows: m.Rows, Cols: m.Cols, Nations: 1, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	if spec, err = DivideLand(spec); err != nil {
		t.Fatal(err)
	}
	spec.AI = true
	w, err := BuildGame(spec)
	if err != nil {
		t.Fatal(err)
	}
	h := newHealthWatch(w)
	h.run(int(3*TicksPerYear), 90)
	// a few barren uplands (this map's mountain tiles Caelifell, Duncrag,
	// Wesamont...) lie at the migration floor; with four times the tiles, the
	// worst tile's wages sit further out
	l := withTechs(nationBounds, 3)
	l.EmptyShare, l.WageWiggleWorst = 0.04, 0.06
	h.report(t, l)
}
