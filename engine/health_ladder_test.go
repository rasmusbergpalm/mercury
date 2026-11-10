package engine

import (
	"strings"
	"testing"
)

// The ladder: worlds of rising complexity, each run for years and checked
// day by day by a healthWatch (see health_test.go). A rung's world adds one
// layer to the rung below: one tile on its own; a map of tiles trading and
// migrating; a nation that runs the whole map; three nations side by side;
// a war; a big generated map. Each fails if any of its numbers leaves
// bounds, and prints them all.

// The bounds. Markets left alone (rung 1) are the reference: prices there
// wiggle 0.1% around their two-month average and nobody moves. A nation
// adds real shocks (taxes, building, research), so its bounds are looser,
// but the budget must be steady and nothing may cycle, pile up, empty or
// collapse. The reasons for each:
var (
	// markets alone: settled means settled
	marketBounds = healthLimits{
		PriceWiggle: 0.005, PriceWiggleWorst: 0.015, WageWiggle: 0.005, WageWiggleWorst: 0.015,
		PriceStep: 0.001, PriceDriftYear: 0.02,
		FullSpeed: 0.01,              // only thin markets, now and then
		StockDays: 30, StockSlack: 5, // nothing piles up unsold
		MinTilePeople: 30, // a tile at the migration floor (24) has emptied
		TileLossYear:  0.05, PeopleDropMonth: 0.001,
		MinEmployment: 0.9, LivingDropYear: 0.01,
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
		MinTilePeople: 30, TileLossYear: 0.25, PeopleDropMonth: 0.01,
		MinEmployment: 0.8, LivingDropYear: 0.05,
		StateFrom: 90,
		ShortDays: 0, // a state that can't pay its bills lays off soldiers, teachers and doctors
		TaxMin:    0.01, TaxMax: 0.25,
		TaxWiggle:  0.0075, // within about a point of its trend: a budget, not a cycle
		ReserveLow: 0.5, ReserveHigh: 2,
		GDPDropMonth: 0.1,
		ArmySupply:   0.95, ArmySupplyWorst: 0.5, // in peace, at home: fed and equipped
		// capital grows a few percent a year over a century (about 1,100
		// levels to 4,400); half again in a year is a boom, then a bust
		LevelGrowthYear: 0.15,
	}
)

// warBounds are a nation's bounds at war: prices, people and output may
// move more, armies may be cut off for a while, but nothing collapses.
func warBounds() healthLimits {
	l := nationBounds
	l.PriceWiggle, l.PriceWiggleWorst, l.WageWiggle, l.WageWiggleWorst = 0.04, 0.1, 0.04, 0.1
	l.PriceStep, l.PriceDriftYear = 0.01, 0.4
	l.TileLossYear, l.PeopleDropMonth, l.MinEmployment, l.LivingDropYear = 0.5, 0.03, 0.6, 0.15
	l.ReserveLow, l.ReserveHigh, l.GDPDropMonth = 0.25, 3, 0.2
	l.ArmySupply, l.ArmySupplyWorst = 0.9, 0.25
	return l
}

// withTechs is l requiring each nation to learn a tech every two years.
func withTechs(l healthLimits, years int) healthLimits {
	l.Techs = max(1, years/2)
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

// Rung 1: the classic map's tiles trading and migrating, no nations, after
// the settling every game starts from. Markets, merchants and migration on
// their own must hold prices and people steady.
func TestHealthMarkets(t *testing.T) {
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
	h.report(t, marketBounds)
}

// Rung 2: one computer nation holding the whole map, from the day the game
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

// Rung 3: the same, run by a player who keeps the tax automated (as a new
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

// Rung 4: the classic game, three computer nations, eight years: before
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
	h.report(t, withTechs(nationBounds, 8))
}

// Rung 5: war. The classic map shared out among its three nations (so they
// border each other); two of them go to war the day the game opens. For two
// years their armies must be supplied and the economy must carry the war
// without collapsing.
func TestHealthWar(t *testing.T) {
	if testing.Short() {
		t.Skip("long")
	}
	t.Parallel()
	spec, err := DivideLand(DefaultGame)
	if err != nil {
		t.Fatal(err)
	}
	spec.AI, spec.Seed = true, 1
	w, err := BuildGame(spec)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.DeclareWar(0, 1); err != nil {
		t.Fatal(err)
	}
	h := newHealthWatch(w)
	h.run(int(2*TicksPerYear), 90)
	h.report(t, warBounds())
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
	h.report(t, withTechs(nationBounds, 3))
}
