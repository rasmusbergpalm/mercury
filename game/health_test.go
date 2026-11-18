package game

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"testing"
)

// Economic health. The tests in health_ladder_test.go run worlds of rising
// complexity for years and watch every important number every day with a
// healthWatch; at the end each check compares what it measured with its
// limit, prints the whole table (so the tests are also the probes to read
// when tuning the economy) and fails on any number out of bounds.

// healthLimits are the bounds a world must keep. A zero limit is not
// checked (a rung leaves out what doesn't apply to it).
type healthLimits struct {
	// Prices and wages: the wiggle is the standard deviation of a tile's
	// log price level (or log wage) around its own 61-day centred average:
	// short swings, not the slow drift of a growing economy. Median over
	// the tiles, and the worst tile.
	PriceWiggle, PriceWiggleWorst float64
	WageWiggle, WageWiggleWorst   float64
	// PriceStep is the median day's move of a tile's log price level.
	PriceStep float64
	// PriceDriftYear is the most the world's (log) price level may move
	// in any 365 days, either way.
	PriceDriftYear float64
	// FullSpeed is the most market-days a price may move at its full daily
	// speed (a market that can't find its price).
	FullSpeed float64
	// StockDays: a firm's stock may grow by at most this many days of its
	// own output over the window, and StockSlack units beyond that: goods
	// piling up unsold. (Stock left over from before, that neither grows
	// nor sells, is reported but not failed.)
	StockDays, StockSlack float64
	// MinTilePeople and TileLossYear: no tile emptying, and none losing
	// more than this share of its people in a year. With EmptyShare, up to
	// that share of the tiles may fall below MinTilePeople (on a big
	// generated map a barren upland or two lies at the migration floor).
	MinTilePeople, TileLossYear, EmptyShare float64
	// PeopleDropMonth: the most the world's people may fall in a month.
	PeopleDropMonth float64
	// MinEmployment: the least share of workers in work, on the worst
	// tile-day.
	MinEmployment float64
	// LivingDropYear: the most average living standard may fall in a year.
	LivingDropYear float64

	// The state (when there are nations).
	StateFrom int // days after which the state checks apply
	// ShortDays: days a nation couldn't pay all its bills.
	ShortDays int
	// TaxMin and TaxMax bound the tax rate; TaxWiggle bounds its standard
	// deviation around its own yearly centred average: a budget cycling
	// rather than a rate following a slow trend.
	TaxMin, TaxMax, TaxWiggle float64
	// ReserveLow and ReserveHigh bound the treasury against the reserve
	// the tax rule aims for (aiTreasuryDays of spending).
	ReserveLow, ReserveHigh float64
	// TreasuryShare: the most of all the money the treasuries may hold.
	TreasuryShare float64
	// GDPDropMonth: the most a nation's real GDP may fall in a month
	// (unless its territory changed).
	GDPDropMonth float64
	// ArmySupply: the least mean supply (share of daily needs, per good)
	// of the armies over the window, and ArmySupplyWorst on any army-day.
	ArmySupply, ArmySupplyWorst float64
	// TechsLeader: the least techs the nation that learns most learns in
	// the window (research works); every surviving nation learns at least
	// one (none is stuck), when the window asks for any.
	TechsLeader int
	// LevelGrowthYear: the most a world's building levels (firms' and
	// services') may grow in any year, as a share of what it had.
	LevelGrowthYear float64
}

// A tile that changes hands is a shock from outside its economy (its
// productivity jumps with its people's allegiance to the new owner): its
// employment counts again once it has had handOverDays to recover.
const handOverDays = 30

// healthWatch records a world day by day.
type healthWatch struct {
	w      *World
	days   int
	basket [][]float64 // per tile: the fixed basket its price level is priced at

	logPrice, logWage [][]float64 // per tile, per day
	people            [][]float64 // per tile, per day
	employment        [][]float64 // per tile, per day: the share of workers in work (NaN just after it changed hands)
	owner             []*Nation   // each tile's owner, and the day it last changed hands
	ownerSince        []int
	living            []float64 // people-weighted utility, per day
	worldPeople       []float64
	money0            float64
	moneyDrift        float64
	marketDays        float64
	fullSpeed         float64
	floorDays         float64 // markets that trade at the price floor
	notFinite         []string

	// firms: summed output over the window, the stock they started with
	// and the most they held
	output     [][]float64
	making     [][]int // days on which it made anything
	startStock [][]float64
	maxStock   [][]float64

	// nations, per day
	tax, treasury, reserve [][]float64
	short                  []int
	gdp                    [][]float64 // RealGDP, sampled every 30 days
	gdpTiles               [][]int     // the nation's tiles at each sample
	known0                 []int
	treasuryShare          float64
	armyDays               float64
	armySum, armyWorst     float64
	armyNeeds              map[*Army][]int // the day each good was first needed by an army (-1: not yet)
	levels                 []float64       // all building levels, per day
}

// newHealthWatch starts watching w. The price baskets are what each tile's
// people buy today.
func newHealthWatch(w *World) *healthWatch {
	h := &healthWatch{w: w, money0: w.Money(), armyWorst: 1, armyNeeds: map[*Army][]int{}}
	n := len(w.Regions)
	h.basket = make([][]float64, n)
	h.logPrice, h.logWage, h.people = make([][]float64, n), make([][]float64, n), make([][]float64, n)
	for i, r := range w.Regions {
		h.basket[i] = make([]float64, NumGoods)
		for _, p := range r.Pops {
			for g, q := range p.Consumed {
				h.basket[i][g] += q
			}
		}
		total := 0.0
		for _, q := range h.basket[i] {
			total += q
		}
		if total == 0 { // nothing bought yet: a basket of one of each consumer good
			for _, g := range ConsumerGoods {
				h.basket[i][g] = 1
			}
		}
	}
	h.employment = make([][]float64, n)
	h.owner, h.ownerSince = make([]*Nation, n), make([]int, n)
	for i, r := range w.Regions {
		h.owner[i], h.ownerSince[i] = r.Nation, -handOverDays
	}
	h.output, h.startStock, h.maxStock = make([][]float64, n), make([][]float64, n), make([][]float64, n)
	h.making = make([][]int, n)
	for i, r := range w.Regions {
		h.output[i], h.startStock[i], h.maxStock[i] = make([]float64, len(r.Firms)), make([]float64, len(r.Firms)), make([]float64, len(r.Firms))
		h.making[i] = make([]int, len(r.Firms))
		for j, f := range r.Firms {
			h.startStock[i][j] = f.Inventory
		}
	}
	k := len(w.Nations)
	h.tax, h.treasury, h.reserve = make([][]float64, k), make([][]float64, k), make([][]float64, k)
	h.short, h.gdp, h.gdpTiles, h.known0 = make([]int, k), make([][]float64, k), make([][]int, k), make([]int, k)
	for j, nat := range w.Nations {
		h.known0[j] = knownTechs(nat)
	}
	return h
}

func knownTechs(n *Nation) int {
	k := 0
	for _, x := range n.Known {
		if x {
			k++
		}
	}
	return k
}

// run steps the world for days, recording each; stateFrom is the day from
// which the state's bills count (the first weeks of a game settle its
// budget in).
func (h *healthWatch) run(days, stateFrom int) {
	w := h.w
	prev := make([][]float64, len(w.Regions))
	for i, r := range w.Regions {
		prev[i] = r.Prices()
	}
	for d := 0; d < days; d++ {
		w.Step()
		h.days++
		h.record(d, prev, d >= stateFrom)
		for i, r := range w.Regions {
			for g, m := range r.Markets {
				prev[i][g] = m.Price
			}
		}
	}
}

func (h *healthWatch) record(day int, prev [][]float64, state bool) {
	w := h.w
	finite := func(what string, x float64, low float64) {
		if math.IsNaN(x) || math.IsInf(x, 0) || x < low {
			if len(h.notFinite) < 10 {
				h.notFinite = append(h.notFinite, fmt.Sprintf("day %d: %s = %v", day, what, x))
			}
		}
	}
	utility, pop, levels := 0.0, 0.0, 0.0
	for i, r := range w.Regions {
		cost := 0.0
		for g, q := range h.basket[i] {
			cost += q * r.Markets[g].Price
		}
		h.logPrice[i] = append(h.logPrice[i], math.Log(cost))
		h.logWage[i] = append(h.logWage[i], math.Log(r.Wage))
		people := r.Population()
		h.people[i] = append(h.people[i], people)
		finite(r.Name+" wage", r.Wage, minPrice)
		finite(r.Name+" house cash", r.House.Cash, -1e-6)
		workers, employed := 0.0, 0.0
		for _, p := range r.Pops {
			finite(r.Name+" pop size", p.Size, 0)
			finite(r.Name+" pop cash", p.Cash, -1e-6)
			if p.Class == Workers {
				workers += p.Size
				employed += p.Employed()
			}
		}
		if r.Nation != h.owner[i] {
			h.owner[i], h.ownerSince[i] = r.Nation, h.days
		}
		e := math.NaN()
		if workers > 0 && h.days-h.ownerSince[i] >= handOverDays {
			e = employed / workers
		}
		h.employment[i] = append(h.employment[i], e)
		utility += r.Utility() * people
		pop += people
		for g, m := range r.Markets {
			finite(r.Name+" "+Goods[g].Name+" price", m.Price, minPrice)
			if IsService(GoodID(g)) && g != int(Construction) {
				continue
			}
			if m.Supply > 0 || m.Demand > 0 {
				h.marketDays++
				if math.Abs(math.Log(m.Price/prev[i][g])) >= 0.999*m.Speed {
					h.fullSpeed++
				}
				if m.Price <= 10*minPrice {
					h.floorDays++
				}
			}
		}
		for j, f := range r.Firms {
			levels += float64(f.Levels)
			finite(r.Name+" "+f.Name+" cash", f.Cash, -1e-6)
			finite(r.Name+" "+f.Name+" stock", f.Inventory, -1e-9)
			if !f.State {
				h.output[i][j] += f.LastOutput
				if f.LastOutput > 0 {
					h.making[i][j]++
				}
				h.maxStock[i][j] = math.Max(h.maxStock[i][j], f.Inventory)
			}
		}
	}
	h.levels = append(h.levels, levels)
	h.living = append(h.living, utility/pop)
	h.worldPeople = append(h.worldPeople, pop)
	money := w.Money()
	finite("money", money, 0)
	h.moneyDrift = math.Max(h.moneyDrift, relDiff(money, h.money0))
	treasuries := 0.0
	for j, n := range w.Nations {
		finite(n.Name+" treasury", n.Treasury, -1e-6)
		treasuries += math.Max(0, n.Treasury)
		if n.Eliminated() {
			continue
		}
		if state {
			h.tax[j] = append(h.tax[j], n.TaxRate)
			h.treasury[j] = append(h.treasury[j], n.Treasury)
			h.reserve[j] = append(h.reserve[j], aiTreasuryDays*n.SpendEMA)
			if n.payScale < 0.999 {
				h.short[j]++
			}
		}
		if (day+1)%30 == 0 {
			h.gdp[j] = append(h.gdp[j], n.RealGDP)
			h.gdpTiles[j] = append(h.gdpTiles[j], len(n.Regions))
		}
	}
	if state && money > 0 {
		h.treasuryShare = math.Max(h.treasuryShare, treasuries/money)
	}
	for _, a := range w.Armies {
		needs := Doctrines[a.Doctrine].Needs
		since := h.armyNeeds[a]
		if since == nil {
			since = make([]int, NumGoods)
			for g := range since {
				since[g] = -1
			}
			h.armyNeeds[a] = since
		}
		// an army stocks a good over the weeks after it first needs it (a
		// new army, or a new doctrine's kit, which its makers may have to
		// start making): count each good from two months on
		sum, worst, counted := 0.0, 1.0, 0
		for g := range needs {
			if since[g] < 0 {
				since[g] = day
			}
			if day-since[g] < 60 || a.Men < MinOccupiers {
				continue
			}
			sum += a.Supply[g]
			worst = math.Min(worst, a.Supply[g])
			counted++
		}
		if counted == 0 {
			continue
		}
		h.armyDays++
		h.armySum += sum / float64(counted)
		if state {
			h.armyWorst = math.Min(h.armyWorst, worst)
		}
	}
}

// healthCheck is one row of the report.
type healthCheck struct {
	name     string
	measured float64
	limit    float64
	kind     checkKind
	note     string
}

type checkKind int

const (
	atMost  checkKind = iota // the measure must be at most the limit
	atLeast                  // ...at least the limit
	none                     // ...zero
	shown                    // not checked, only reported
)

func (c healthCheck) ok() bool {
	switch c.kind {
	case atMost:
		return c.measured <= c.limit
	case atLeast:
		return c.measured >= c.limit
	case none:
		return c.measured == 0
	}
	return true
}

// wiggles is each series' standard deviation around its 61-day centred
// moving average.
func wiggles(series [][]float64) []float64 { return wigglesOver(series, 30) }

// wigglesOver is each series' standard deviation around its centred moving
// average over 2*half+1 days.
func wigglesOver(series [][]float64, half int) []float64 {
	var out []float64
	for _, s := range series {
		if len(s) < 2*half+2 {
			continue
		}
		sum, sq, n := 0.0, 0.0, 0.0
		for t := half; t < len(s)-half; t++ {
			avg := 0.0
			for k := t - half; k <= t+half; k++ {
				avg += s[k]
			}
			avg /= float64(2*half + 1)
			d := s[t] - avg
			sum, sq, n = sum+d, sq+d*d, n+1
		}
		out = append(out, math.Sqrt(math.Max(0, sq/n-(sum/n)*(sum/n))))
	}
	sort.Float64s(out)
	return out
}

func medianOf(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	return s[len(s)/2]
}

func last(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	return xs[len(xs)-1]
}

// checks turns what was recorded into the report's rows.
func (h *healthWatch) checks(l healthLimits) []healthCheck {
	w := h.w
	var c []healthCheck
	// a zero limit leaves a check out of this rung: it is only reported
	bound := func(limit float64, k checkKind) checkKind {
		if limit == 0 {
			return shown
		}
		return k
	}
	add := func(name string, measured, limit float64, most bool, note string) {
		k := atLeast
		if most {
			k = atMost
		}
		c = append(c, healthCheck{name, measured, limit, bound(limit, k), note})
	}
	never := func(name string, measured float64, note string) {
		c = append(c, healthCheck{name, measured, 0, none, note})
	}
	never("numbers not finite or below their floor", float64(len(h.notFinite)), strings.Join(h.notFinite, "; "))
	add("money: largest drift from the start", h.moneyDrift, 1e-9, true, "money is conserved")

	pw, ww := wiggles(h.logPrice), wiggles(h.logWage)
	add("price wiggle, median tile", medianOf(pw), l.PriceWiggle, true, "sd of log price level around its 61-day average")
	add("price wiggle, worst tile", last(pw), l.PriceWiggleWorst, true, "")
	add("wage wiggle, median tile", medianOf(ww), l.WageWiggle, true, "")
	add("wage wiggle, worst tile", last(ww), l.WageWiggleWorst, true, "")
	var steps []float64
	for _, s := range h.logPrice {
		for t := 1; t < len(s); t++ {
			steps = append(steps, math.Abs(s[t]-s[t-1]))
		}
	}
	add("price level's daily move, median tile-day", medianOf(steps), l.PriceStep, true, "")
	drift := 0.0
	level := make([]float64, h.days)
	for t := range level {
		for i := range h.logPrice {
			level[t] += h.logPrice[i][t] / float64(len(h.logPrice))
		}
	}
	year := int(TicksPerYear)
	for t := 0; t+year < len(level); t += 30 {
		drift = math.Max(drift, math.Abs(level[t+year]-level[t]))
	}
	add("world price level's largest move in a year (log)", drift, l.PriceDriftYear, true, "")
	add("market-days at full price speed", h.fullSpeed/math.Max(h.marketDays, 1), l.FullSpeed, true, "markets that can't find their price")
	never("market-days at the price floor", h.floorDays, "a market at the floor has stopped clearing")

	// unsold stock: how much a firm's stock grew beyond StockDays of its
	// output, and the biggest stock held
	stockDays := l.StockDays
	if stockDays == 0 {
		stockDays = 30
	}
	piled, piledName, held, heldName := 0.0, "", 0.0, ""
	for i, r := range w.Regions {
		for j, f := range r.Firms {
			if f.State {
				continue
			}
			// a day's output while it was making anything (a method that came
			// in late in the window holds a day's stock of a full day's output)
			perDay := h.output[i][j] / float64(max(1, h.making[i][j]))
			days := "no output"
			if perDay > 0 {
				days = fmt.Sprintf("%.0f days of output", h.maxStock[i][j]/perDay)
			}
			what := fmt.Sprintf("%s in %s: %.0f %s at most, %s", f.Name, r.Name, h.maxStock[i][j], Goods[f.Output].Name, days)
			if grew := h.maxStock[i][j] - h.startStock[i][j] - stockDays*perDay; grew > piled {
				piled, piledName = grew, what
			}
			if excess := h.maxStock[i][j] - stockDays*perDay; excess > held {
				held, heldName = excess, what
			}
		}
	}
	slack := l.StockSlack
	if l.StockDays == 0 {
		slack = 0
	}
	add(fmt.Sprintf("most unsold stock piled up beyond %.0f days of output", stockDays), piled, slack, true, piledName)
	if l.StockDays != 0 && slack == 0 {
		c[len(c)-1].kind = none
	}
	c = append(c, healthCheck{fmt.Sprintf("most stock held beyond %.0f days of output", stockDays), held, 0, shown, heldName})
	growth := 0.0
	for t := year; t < len(h.levels); t += 30 {
		growth = math.Max(growth, h.levels[t]/h.levels[t-year]-1)
	}
	add("most building levels grew in a year", growth, l.LevelGrowthYear, true, fmt.Sprintf("%.0f levels to %.0f", h.levels[0], last(h.levels)))

	minPeople, lossYear, emptied := math.Inf(1), 0.0, 0
	for _, s := range h.people {
		tileMin := math.Inf(1)
		for t, p := range s {
			tileMin = math.Min(tileMin, p)
			if t >= year {
				lossYear = math.Max(lossYear, 1-p/s[t-year])
			}
		}
		minPeople = math.Min(minPeople, tileMin)
		if tileMin < l.MinTilePeople {
			emptied++
		}
	}
	if l.EmptyShare > 0 {
		c = append(c, healthCheck{"fewest people on a tile", minPeople, l.MinTilePeople, shown, ""})
		add(fmt.Sprintf("share of tiles that fell below %.0f people", l.MinTilePeople), float64(emptied)/float64(len(h.people)), l.EmptyShare, true, "")
	} else {
		add("fewest people on a tile", minPeople, l.MinTilePeople, false, "")
	}
	add("most of a tile's people lost in a year", lossYear, l.TileLossYear, true, "")
	drop := 0.0
	for t := 30; t < len(h.worldPeople); t++ {
		drop = math.Max(drop, 1-h.worldPeople[t]/h.worldPeople[t-30])
	}
	add("most of the world's people lost in a month", drop, l.PeopleDropMonth, true, "")
	// a month's average: a tile's workers between jobs for a week or two
	// (when a big project there ends, say) is the labour market at work;
	// out of work for a month is not
	worstEmp := 1.0
	for _, es := range h.employment {
		for t := 30; t <= len(es); t++ {
			m := 0.0
			for _, e := range es[t-30 : t] {
				m += e
			}
			if m == m { // no day in the month just after a change of hands
				worstEmp = math.Min(worstEmp, m/30)
			}
		}
	}
	add("workers in work, worst tile-month", worstEmp, l.MinEmployment, false, "a month's average; but a month after a tile changes hands")
	livingDrop := 0.0
	for t := year; t < len(h.living); t += 30 {
		livingDrop = math.Max(livingDrop, 1-h.living[t]/h.living[t-year])
	}
	add("most living standard falls in a year", livingDrop, l.LivingDropYear, true, fmt.Sprintf("from %.2f to %.2f over the run", h.living[0], last(h.living)))

	if len(w.Nations) == 0 {
		return c
	}
	short, taxLo, taxHi := 0, 1.0, 0.0
	resLo, resHi := math.Inf(1), 0.0
	gdpDrop := 0.0
	techs, techsBest := math.MaxInt, 0
	for j, n := range w.Nations {
		short = max(short, h.short[j])
		for t, x := range h.tax[j] {
			taxLo, taxHi = math.Min(taxLo, x), math.Max(taxHi, x)
			if r := h.reserve[j][t]; r > 0 {
				resLo, resHi = math.Min(resLo, h.treasury[j][t]/r), math.Max(resHi, h.treasury[j][t]/r)
			}
		}
		for t := 1; t < len(h.gdp[j]); t++ {
			if h.gdp[j][t-1] > 0 && h.gdpTiles[j][t] == h.gdpTiles[j][t-1] {
				gdpDrop = math.Max(gdpDrop, 1-h.gdp[j][t]/h.gdp[j][t-1])
			}
		}
		if !n.Eliminated() {
			techs = min(techs, knownTechs(n)-h.known0[j])
			techsBest = max(techsBest, knownTechs(n)-h.known0[j])
		}
	}
	if l.ShortDays == 0 {
		never("days a nation couldn't pay its bills", float64(short), "")
	} else {
		add("days a nation couldn't pay its bills", float64(short), float64(l.ShortDays), true, "")
	}
	add("lowest tax rate", taxLo, l.TaxMin, false, "")
	add("highest tax rate", taxHi, l.TaxMax, true, "")
	add("tax rate's wiggle around its yearly average, worst nation", last(wigglesOver(h.tax, year/2)), l.TaxWiggle, true, "a budget cycling, not a trend")
	add("lowest treasury against its reserve", resLo, l.ReserveLow, false, fmt.Sprintf("reserve: %.0f days of spending", aiTreasuryDays))
	add("highest treasury against its reserve", resHi, l.ReserveHigh, true, "")
	add("most of all money in the treasuries", h.treasuryShare, l.TreasuryShare, true, "money held by the state is money people can't spend")
	add("largest fall of a nation's real GDP in a month", gdpDrop, l.GDPDropMonth, true, "unless its territory changed")
	armyMean := 1.0
	if h.armyDays > 0 {
		armyMean = h.armySum / h.armyDays
	}
	add("armies' supply, mean over army-days", armyMean, l.ArmySupply, false, fmt.Sprintf("%.0f army-days", h.armyDays))
	add("armies' supply, worst good on any army-day", h.armyWorst, l.ArmySupplyWorst, false, "")
	add("most techs a nation learned", float64(techsBest), float64(l.TechsLeader), false, "")
	add("fewest techs a nation learned", float64(techs), float64(min(1, l.TechsLeader)), false, "nations differ in size and in what they spend on research")
	return c
}

// report prints the checks and fails the test on any out of bounds.
func (h *healthWatch) report(t *testing.T, l healthLimits) {
	t.Helper()
	var b strings.Builder
	fmt.Fprintf(&b, "%d days, %d tiles, %d nations\n", h.days, len(h.w.Regions), len(h.w.Nations))
	var failed []string
	for _, c := range h.checks(l) {
		mark, limit := "  ", "-"
		switch c.kind {
		case atMost:
			limit = fmt.Sprintf("≤ %.4g", c.limit)
		case atLeast:
			limit = fmt.Sprintf("≥ %.4g", c.limit)
		case none:
			limit = "none"
		}
		if !c.ok() {
			mark = "✗ "
			failed = append(failed, c.name)
		}
		fmt.Fprintf(&b, "%s%-50s %12.4g   %-10s %s\n", mark, c.name, c.measured, limit, c.note)
	}
	sort.Strings(failed)
	t.Log("\n" + b.String())
	if len(failed) > 0 {
		t.Errorf("out of bounds: %s (a signal, not a verdict: see the ladder's note on bounds)", strings.Join(failed, "; "))
	}
}

// checkFinite fails the test if any price, wage, cash balance or population
// in the world is not a finite number, or is below its floor.
func checkFinite(t *testing.T, w *World) {
	t.Helper()
	h := newHealthWatch(w)
	prev := make([][]float64, len(w.Regions))
	for i, r := range w.Regions {
		prev[i] = r.Prices()
	}
	h.record(0, prev, false)
	if len(h.notFinite) > 0 {
		t.Errorf("numbers out of bounds: %v", h.notFinite)
	}
}
