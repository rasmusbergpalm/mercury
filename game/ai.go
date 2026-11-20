package game

import "math"

// The computer's nations think every aiInterval days. The AI is part of the
// simulation: deterministic, so saved games and replays continue exactly.
const aiInterval = 30

// aiArmyEvery is how often (in days) the computer's armies get new orders
// while it is at war or has free land to take.
const aiArmyEvery = 7

// AI war policy.
const (
	aiGraceYears    = 15  // no wars of choice before this
	aiWarAdvantage  = 2.0 // declare war on a neighbour this much weaker
	aiWarYears      = 3   // seek peace after this long
	aiLosingRatio   = 0.6 // seek peace when this much weaker than the enemy
	aiPeaceMobilize = 0.5 // share of the mobilization cap kept under arms in peace
	aiTruceYears    = 5   // no new war of choice on a nation this soon after peace
)

func (w *World) runAI() {
	if w.Tick%aiTaxEvery == 0 { // the tax, weekly, for the computer's nations and automated players'
		for _, n := range w.Nations {
			if !n.Eliminated() && (n.AI || n.AutoTax) {
				w.aiTax(n)
			}
		}
	}
	if w.Tick%aiInterval != 0 {
		// between the monthly reviews, armies get new orders weekly while
		// there is fighting or free land to take: a month is long in a
		// campaign, and a siege or a raid can't wait for the next review
		if w.Tick%aiArmyEvery == 0 && !w.burning {
			for _, n := range w.Nations {
				if !n.Eliminated() && n.AI {
					if targets := w.expansionTargets(n); w.atWar(n) || len(targets) > 0 {
						w.aiArmies(n, targets)
					}
				}
			}
		}
		return
	}
	if !w.burning { // (no alliances in the burn-in)
		w.updateAlliances()
	}
	for _, n := range w.Nations {
		switch {
		case n.Eliminated():
		case n.AI:
			w.think(n)
		default: // a player's nation: only what they left to the computer
			if n.AutoBuild {
				w.aiBuild(n)
				w.aiExpandFirm(n)
				w.aiServices(n, w.atWar(n), false, false) // builds services; funding stays the player's
			}
		}
	}
}

// ArmyStrength is the total strength of a nation's armies.
func (w *World) ArmyStrength(n *Nation) float64 {
	s := 0.0
	for _, a := range w.ArmiesOf(n) {
		s += a.Men * a.Effectiveness() * n.attackMult()
	}
	return s
}

func (w *World) atWar(n *Nation) bool {
	for _, x := range n.AtWar {
		if x {
			return true
		}
	}
	return false
}

func (w *World) think(n *Nation) {
	own := w.ArmyStrength(n)
	// how many to keep under arms: judged on what each side could field at
	// full mobilization (its armies at full size and supply), not on what
	// it has under arms now, which moves with the very decision; enough
	// that its standing army is at least a third of the strongest
	// neighbour's full strength, and everyone at war or when invaded
	ownPot, threatPot, invaded := w.ArmyFullStrength(n), 0.0, false
	for _, o := range w.Nations {
		if o == n || o.Eliminated() || !w.nationsBorder(n, o) {
			continue
		}
		threatPot = math.Max(threatPot, w.ArmyFullStrength(o))
	}
	for _, a := range w.Armies {
		if a.Nation != n && w.Regions[a.Region].Nation == n {
			invaded = true
		}
	}
	var targets []expansionTarget
	if !w.burning { // in the burn-in armies stay home
		targets = w.expansionTargets(n)
	}
	war := w.atWar(n)
	t := n.Temper
	standing := math.Max(MinStanding, DefaultStanding*t.Militarism)
	if ownPot > 0 {
		standing = math.Max(standing, aiStandingVsThreat*threatPot/ownPot)
	} else if threatPot > 0 {
		standing = MaxStanding // nothing to stand against an armed neighbour with
	}
	if war || invaded {
		standing = MaxStanding // call up the reserves
	}
	n.SetStanding(math.Min(MaxStanding, standing))
	if !w.burning { // the burn-in runs the peacetime economy only
		w.aiDiplomacy(n, own)
		w.aiBuild(n)
		w.aiExpandFirm(n)
	}
	w.aiServices(n, war || invaded, true, true)
	w.aiArmies(n, targets)
}

// The computer keeps at least this share of its strongest neighbour's full
// strength under arms in peace.
const aiStandingVsThreat = 1.0 / 3

// ArmyFullStrength is what a nation's armies, as raised, could field: each
// at its full (wartime) size and fully supplied. Unlike its might, it
// doesn't move when the nation calls up or stands down its reserves. The
// computer sizes its standing army against its neighbours' (think).
func (w *World) ArmyFullStrength(n *Nation) float64 {
	s := 0.0
	for _, a := range w.ArmiesOf(n) {
		s += math.Max(a.Men, a.Target) * n.doctrinePower(a.Doctrine) * n.attackMult()
	}
	return s
}

// ArmyPotential is the might a nation could raise: everyone it can call up
// (MobilizationCap: the loyal share of its people its techs allow, and
// those already serving), at its best doctrine, fully supplied. It is what
// its people, techs and loyalty make possible, whatever armies it has
// chosen to raise so far: what rivals arm against.
func (w *World) ArmyPotential(n *Nation) float64 {
	return w.MobilizationCap(n) * n.doctrinePower(n.bestDoctrine()) * n.attackMult()
}

// nationsBorder reports whether any tiles of a and b are neighbours.
func (w *World) nationsBorder(a, b *Nation) bool {
	for _, r := range a.Regions {
		if w.borders(b, r) {
			return true
		}
	}
	return false
}

// aiDiplomacy makes peace in wars that drag on or go badly, and starts a war
// on a much weaker neighbour once the grace period is over.
func (w *World) aiDiplomacy(n *Nation, own float64) {
	for _, o := range w.Nations {
		if o == n || !n.AtWar[o.ID] {
			continue
		}
		long := float64(w.Tick-n.WarSince[o.ID]) > aiWarYears*TicksPerYear
		switch {
		case o.PeaceOffer[n.ID] && w.acceptsPeace(n, o):
			w.makePeace(n.ID, o.ID)
		case long || own < aiLosingRatio*w.ArmyStrength(o):
			w.OfferPeace(n.ID, o.ID)
		}
	}
	// wars of choice only once the grace period is over and there's no free
	// land left to take
	// (belligerent nations start sooner, and don't wait for the land to run out)
	t := n.Temper
	if w.atWar(n) || float64(w.Tick) < aiGraceYears/t.Aggression*TicksPerYear ||
		(len(w.expansionTargets(n)) > 0 && t.Aggression < 1.25) {
		return
	}
	advantage := aiWarAdvantage / t.Aggression * (1 + 0.15*w.noise(n, 1, w.Tick/TicksPerYear)) // this year's resolve
	var victim *Nation
	for _, o := range w.Nations {
		if o == n || o.Eliminated() || !w.nationsBorder(n, o) || w.atWar(o) {
			continue // no ganging up on a nation already at war
		}
		if since := float64(w.Tick - n.WarSince[o.ID]); n.WarSince[o.ID] > 0 && since < aiTruceYears/t.Aggression*TicksPerYear {
			continue // a truce after the last peace
		}
		if s := w.ArmyStrength(o); own > advantage*s+1 && (victim == nil || s < w.ArmyStrength(victim)) {
			victim = o
		}
	}
	if victim != nil {
		w.DeclareWar(n.ID, victim.ID)
	}
}

// aiArmies keeps enough armies to field the soldiers it wants (one army can
// only be so big), at the best doctrine, and gives idle armies orders: defend
// invaded tiles, attack the enemy's weakest border tile, or occupy free land.
func (w *World) aiArmies(n *Nation, targets []expansionTarget) {
	war := w.atWar(n)
	share := aiPeaceMobilize * n.Temper.Militarism
	if len(targets) > 0 {
		share *= n.Temper.Expansion // land-hungry nations field more men to take it
	}
	share = clamp01(share)
	if war {
		share = 1
	}
	want := share * w.MobilizationCap(n)
	if war {
		want = math.Min(want, w.sustainableSoldiers(n))
	}
	size := n.MaxArmySize()
	armies := w.ArmiesOf(n)
	k := int(math.Ceil(want/size - 1e-9))
	if war {
		// at war, as many armies as can each be aiArmyEdge times the
		// enemy's strongest: one army can only be in one place, and a
		// handful of raiders shouldn't draw a host off every siege it lays
		strongest := 0.0
		for _, e := range w.Armies {
			if n.AtWar[e.Nation.ID] {
				strongest = math.Max(strongest, math.Max(e.Men, e.Target)*e.Nation.doctrinePower(e.Doctrine)*e.Nation.attackMult())
			}
		}
		if perMan := n.doctrinePower(n.bestDoctrine()) * n.attackMult(); strongest > 0 {
			split := int(want * perMan / (aiArmyEdge * strongest))
			k = max(k, min(split, len(n.Regions)/2))
		}
	}
	for len(armies) < k {
		home := w.aiArmyHome(n)
		if home == nil {
			break
		}
		a, err := w.RaiseArmy(n.ID, home.ID, 0)
		if err != nil {
			break
		}
		armies = append(armies, a)
	}
	if war && len(armies) > max(k, 1) {
		// fewer men than armies to field them in: the smallest army at home
		// is disbanded (its men go home, and the rest fill up), rather than
		// every army left too weak to fight
		var small *Army
		for _, a := range armies {
			if w.Regions[a.Region].Nation == n && len(a.Path) == 0 && !a.Retreating && (small == nil || a.Men < small.Men) {
				small = a
			}
		}
		if small != nil && w.DisbandArmy(small.ID) == nil {
			for i, a := range armies {
				if a == small {
					armies = append(armies[:i:i], armies[i+1:]...)
					break
				}
			}
		}
	}
	if len(armies) > 0 {
		per := math.Min(size, want/float64(len(armies)))
		for _, a := range armies { // lower first, so the total never exceeds the cap
			if a.Target > per {
				w.SetArmyTarget(a.ID, per)
			}
		}
		for _, a := range armies {
			if a.Target < per {
				w.SetArmyTarget(a.ID, per)
			}
		}
	}
	w.aiPlan(n, armies, targets)
}

// At war the computer keeps no more soldiers than it can feed and arm. A
// state calls up what its treasury can pay and its farms and works can
// supply; calling up every man the law allows with neither leaves them all
// hungry and unarmed (as the computer did: its armies at a fifth of their
// strength within a month, prices of bread and arms bid up fivefold). So
// while its armies get at least aiFedEnough of their rations it calls up
// freely; when they go short, it keeps only as many as it is feeding.
const aiFedEnough = 0.8

// sustainableSoldiers is how many soldiers the nation is keeping supplied:
// its men under arms times how well they are fed and armed (their
// strength over what it would be fully supplied), over aiFedEnough; no
// limit while they are fed well enough.
func (w *World) sustainableSoldiers(n *Nation) float64 {
	men, full, have := 0.0, 0.0, 0.0
	for _, a := range w.ArmiesOf(n) {
		men += a.Men
		full += a.Men * n.doctrinePower(a.Doctrine) * n.attackMult()
		have += a.Strength()
	}
	if men < 1 || full <= 0 {
		return math.Inf(1)
	}
	if fed := have / full; fed < aiFedEnough {
		return men * fed / aiFedEnough
	}
	return math.Inf(1)
}

// The computer only seeks battle when its side is this many times as strong.
const aiFightOdds = 1.2

// At war, the computer's armies are each at least this many times the
// enemy's strongest, if it has the men: enough to beat it on its own
// ground (the defender's bonus and the odds sought, with some to spare).
const aiArmyEdge = 2.0

// An army at home waits until it has this share of the men it keeps under
// arms before it marches (unless the nation is being invaded).
const aiRefill = 0.6

// underAttack reports whether an enemy army stands on the nation's soil.
func (w *World) underAttack(n *Nation) bool {
	for _, e := range w.Armies {
		if e.Nation != n && n.AtWar[e.Nation.ID] && w.Regions[e.Region].Nation == n {
			return true
		}
	}
	return false
}

// aiArmyHome is where to raise a new army: the most populous home tile with
// the fewest armies standing in it, and no enemy there.
func (w *World) aiArmyHome(n *Nation) *Region {
	var best *Region
	bestN := 0
	for _, r := range n.Regions {
		k, contested := 0, r.Siege != nil
		for _, a := range w.Armies {
			if a.Region == r.ID {
				k++
				contested = contested || a.Nation != n
			}
		}
		if contested || w.tileTaken(n, r.ID, nil) {
			continue // one army per tile; and one raised under the enemy's guns would be cut down at once
		}
		if best == nil || k < bestN || (k == bestN && r.Population() > best.Population()) {
			best, bestN = r, k
		}
	}
	return best
}

// aiDestination picks where an idle army should go, or -1.
// aiScope is what an army weighs its options with: the forces in a tile,
// its advantage there, the odds it seeks battle at, and how far tiles are.
type aiScope struct {
	w    *World
	n    *Nation
	a    *Army
	odds float64
	dist []float64
	best int
	top  float64
}

func (w *World) newAIScope(n *Nation, a *Army) *aiScope {
	dist, _ := w.allPaths(a.Region)
	return &aiScope{w: w, n: n, a: a, dist: dist, best: -1, top: math.Inf(-1),
		odds: aiFightOdds / math.Sqrt(n.Temper.Aggression) * (1 + 0.1*w.noise(n, 100+a.ID, w.Tick/TicksPerYear))}
}

// forces is the strength of the armies at war with n, and of n's other
// armies, in tile r.
func (s *aiScope) forces(r int) (enemy, own float64) {
	for _, e := range s.w.Armies {
		if e.Region != r || e == s.a {
			continue
		}
		if e.Nation == s.n {
			if len(e.Path) == 0 { // (not one ordered away)
				own += e.Strength()
			}
		} else if s.n.AtWar[e.Nation.ID] {
			enemy += e.Strength()
		}
	}
	return
}

// engages reports whether the army would march into tile r: an enemy army
// there is only sought out with the odds the nation's temper asks for (a
// little better than even); on its own soil, with a tile at stake, whenever
// it expects to win; an empty tile always.
func (s *aiScope) engages(r *Region) bool {
	if enemy, _ := s.forces(r.ID); enemy <= 0 {
		return true
	}
	if r.Nation == s.n {
		return s.advantage(r) > 1
	}
	return s.advantage(r) >= s.odds
}

// advantage is the army's edge in tile r, with our armies already there:
// our strength over theirs, the defender's bonus to whoever owns the tile
// (none on free land). It seeks battle from odds up.
func (s *aiScope) advantage(r *Region) float64 {
	enemy, own := s.forces(r.ID)
	mine, theirs := own+s.a.Strength(), enemy
	switch {
	case r.Nation == s.n:
		mine *= defenderHome * s.n.defenseMult()
	case r.Nation != nil:
		theirs *= defenderHome * r.Nation.defenseMult()
	}
	if theirs <= 0 {
		return math.Inf(1)
	}
	return mine / theirs
}

// The computer's armies weigh a few concrete moves by what they are likely
// to gain: tiles first, then the enemy's losses less their own.
//
// A tile of its own saved is worth 1. A tile taken is worth half that:
// newly conquered land starts disloyal (Allegiance), paying little tax and
// giving few recruits for years, while a home tile pays in full; free land
// settles slowly too. A battle's worth is the losses it is expected to
// cause the enemy less those it costs, relative to the army's own
// strength, times aiBattleWeight and the nation's Aggression: an enemy
// army broken while weak is worth hunting down, before it fills up and
// takes tiles.
//
// A move is rated by what it gains per day it takes (marching, then
// besieging), plus aiSettle days for the fighting and the unforeseen: an
// army's time is what a campaign spends, so a siege nearly done beats a
// long march to another, and a near tile beats a far one. A general who
// chases every raid across the country takes nothing. A defender that
// wins breaks the besieger and is free again once it has marched there;
// one that loses is tied down until the tile would have fallen, so a
// doubtful defence of a siege a year from done calls for no haste.
const (
	aiOwnTile   = 1.0
	aiOtherTile = 0.5
	aiSettle    = 30.0
	// an army standing still watches this far ahead for an enemy that
	// could come for it
	aiWatch = 60.0
	// an army marches to save a tile once the enemy's siege of it would end
	// within this many days of its getting there
	aiDefendSlack  = 90.0
	aiBattleWeight = 1.0
	// the move already under way is preferred by a quarter: orders that
	// flip with every small change march armies to and fro and finish
	// nothing
	aiKeepPlan = 0.25
)

// aiOption is a move: march to dest (or stay, dest = the army's tile), and
// what it is worth.
type aiOption struct {
	dest  int
	value float64
}

// aiPlan gives the nation's armies their orders as one plan. Each round
// every free army's best move is weighed, and the best of all is ordered
// first; the rest are weighed again knowing where it goes (a tile holds
// one army of a nation, so no two chase the same siege or answer the same
// raid). An army filling up at home waits; one that can't take ground goes
// home.
func (w *World) aiPlan(n *Nation, armies []*Army, targets []expansionTarget) {
	// a new army fills up before it marches; when the nation is invaded it
	// goes sooner, at half that (a handful of recruits sent out alone is
	// only cut down)
	refill := aiRefill
	if w.underAttack(n) {
		refill /= 2
	}
	ctx := w.newAIContext(n)
	var free []*Army
	for _, a := range armies {
		// (at home: on its own land, and staying there)
		home := w.Regions[a.Region].Nation == n && w.Regions[aiGoing(a)].Nation == n
		switch {
		case a.Retreating:
		case home && a.Men < refill*w.UnderArms(a):
			// a new army fills up (to what it keeps under arms now) before it
			// marches; field armies are reinforced where they stand. At war
			// it keeps out of the enemy's reach meanwhile.
			if len(ctx.enemies) > 0 {
				ctx.filling[a] = true
				free = append(free, a)
			}
		case !a.canOccupy():
			if w.Regions[a.Region].Nation != n {
				w.sendHome(a)
			}
		default:
			// one sent out half filled because the nation is invaded, or worn
			// down by battle, only defends (or keeps the siege it holds): it
			// is no army to march into enemy land with; with nothing to
			// defend it goes home to fill up
			ctx.defendOnly[a] = a.Men < aiRefill*w.UnderArms(a)
			free = append(free, a)
		}
	}
	// each army's best move is kept from round to round: an order only
	// takes its destination off the others' hands, so only those whose best
	// was that tile weigh again (a big nation has dozens of armies, and
	// weighing every one each round over every tile was most of a day's work
	// on a world map)
	kept := map[*Army]aiOption{}
	for len(free) > 0 {
		best, pick := aiOption{-1, math.Inf(-1)}, -1
		for i, a := range free {
			o, ok := kept[a]
			if !ok {
				o = w.aiBest(n, a, targets, ctx)
				kept[a] = o
			}
			if pick < 0 || o.value > best.value {
				best, pick = o, i
			}
		}
		a := free[pick]
		free = append(free[:pick:pick], free[pick+1:]...)
		delete(kept, a)
		w.aiOrder(n, a, best)
		for b, o := range kept {
			if o.dest == best.dest {
				delete(kept, b)
			}
		}
	}
}

// aiOrder carries out an army's chosen move. One with nothing worth doing
// (every move costs more than it gains) draws back to a tile of its own:
// the safest it chose, or failing that the nearest.
func (w *World) aiOrder(n *Nation, a *Army, o aiOption) {
	if o.value <= 0 && w.Regions[o.dest].Nation != n && !w.besieging(n, a) {
		w.sendHome(a)
		return
	}
	if o.dest == aiGoing(a) {
		return
	}
	w.MoveArmy(a.ID, o.dest) // (to its own tile: it stops where it is)
}

// aiGoing is where an army is going: the end of its path, or where it is.
func aiGoing(a *Army) int {
	if len(a.Path) > 0 {
		return a.Path[len(a.Path)-1]
	}
	return a.Region
}

// besieging reports whether the army is occupying the (enemy or free) tile
// it stands on.
func (w *World) besieging(n *Nation, a *Army) bool {
	here := w.Regions[a.Region]
	return len(a.Path) == 0 && here.Nation != n && here.Siege != nil && here.Siege.Nation == n
}

// aiDestination is where the army would go now, or -1 to stay.
func (w *World) aiDestination(n *Nation, a *Army, targets []expansionTarget) int {
	if o := w.aiBest(n, a, targets, w.newAIContext(n)); o.value > 0 && o.dest != a.Region {
		return o.dest
	}
	return -1
}

// aiContext is what a nation's plan shares between its armies: the enemy's
// field armies and how many days each would take to reach every tile, and
// which of its own armies may only defend.
type aiContext struct {
	enemies    []*Army
	enemyDays  [][]float64
	besieging  []bool // each enemy army holding a siege
	defendOnly map[*Army]bool
	filling    map[*Army]bool // filling up at home: only keeping out of reach
	scopes     map[*Army]*aiScope
	days       map[*Army][]float64 // each army's march days to every tile
}

func (w *World) newAIContext(n *Nation) *aiContext {
	ctx := &aiContext{defendOnly: map[*Army]bool{}, filling: map[*Army]bool{},
		scopes: map[*Army]*aiScope{}, days: map[*Army][]float64{}}
	for _, e := range w.Armies {
		if e.Nation != n && n.AtWar[e.Nation.ID] && !e.Retreating && e.canOccupy() {
			ctx.enemies = append(ctx.enemies, e)
			ctx.enemyDays = append(ctx.enemyDays, w.aiMarchDays(e.Nation, e))
			ctx.besieging = append(ctx.besieging, w.besieging(e.Nation, e))
		}
	}
	return ctx
}

// relief is the strongest enemy army not in tile r that could reach it
// within the given days: a siege of r is fought against it, if it is
// stronger than what stands there now.
func (c *aiContext) relief(r int, within float64) float64 {
	s := 0.0
	for i, e := range c.enemies {
		if e.Region != r && c.enemyDays[i][r] < within {
			s = math.Max(s, e.Strength())
		}
	}
	return s
}

// aiBest is the army's best move: staying (and keeping a siege), defending
// one of its nation's tiles an enemy army stands on, or marching on an enemy
// or free tile (fighting the army there, if any, and occupying it).
func (w *World) aiBest(n *Nation, a *Army, targets []expansionTarget, ctx *aiContext) aiOption {
	s, days := ctx.scopes[a], ctx.days[a]
	if s == nil { // (weighed once a plan: the map doesn't change meanwhile)
		s, days = w.newAIScope(n, a), w.aiMarchDays(n, a)
		ctx.scopes[a], ctx.days[a] = s, days
	}
	going := aiGoing(a)
	rate := func(gain, days float64) float64 { return gain / (days + aiSettle) }
	// the chance of winning a fight in tile r against an enemy of the given
	// strength, and what the fight is worth
	fightVs := func(r *Region, enemy, own float64) (p, worth float64) {
		if enemy <= 0 {
			return 1, 0
		}
		mine, theirs := own+a.Strength(), enemy
		switch {
		case r.Nation == n:
			mine *= defenderHome * n.defenseMult()
		case r.Nation != nil:
			theirs *= defenderHome * r.Nation.defenseMult()
		}
		adv := mine / theirs
		p = adv * adv / (1 + adv*adv)
		// the loser fights on until down to breakShare and loses
		// retreatLoss of the rest on the way back; each side loses men in
		// proportion to the other's strength, so the winner loses the
		// loser's battle losses times the odds against it
		lost := 1 - breakShare*(1-retreatLoss)
		mine = own + a.Strength()
		theirLoss := p*enemy*lost + (1-p)*mine*(1-breakShare)*adv
		ownLoss := p*enemy*(1-breakShare)/adv + (1-p)*mine*lost
		// in tiles: an enemy army as strong as this one destroyed is worth
		// aiBattleWeight of a tile (it would take or hold tiles for months
		// until replaced), times the nation's Aggression
		return p, aiBattleWeight * n.Temper.Aggression * (theirLoss - ownLoss) / a.Strength()
	}
	fight := func(r *Region) (p, worth float64) {
		enemy, own := s.forces(r.ID)
		return fightVs(r, enemy, own)
	}
	// danger is what the battle is expected to cost if an enemy army
	// stronger than this one, and that would seek it out, can reach tile r
	// within the given days: a weak army doesn't put itself where it can
	// be caught
	danger := func(r *Region, within float64) float64 {
		// (what could come within the next aiWatch days of arriving: over
		// a long siege, hold weighs the enemy that could relieve it)
		within = math.Min(within, days[r.ID]+aiWatch)
		worst := 0.0
		for i, e := range ctx.enemies {
			if ctx.enemyDays[i][r.ID] >= within || e.Region == r.ID || ctx.besieging[i] {
				continue // (one standing there is fought anyway; one holding a siege is busy)
			}
			_, own := s.forces(r.ID)
			mine, theirs := own+a.Strength(), e.Strength()
			switch {
			case r.Nation == n:
				mine *= defenderHome * n.defenseMult()
			case r.Nation == e.Nation:
				theirs *= defenderHome * e.Nation.defenseMult()
			}
			if theirs >= aiFightOdds*mine { // it would come for us
				// the nearer it is, the likelier it comes before anything
				// else draws it away
				_, worth := fightVs(r, e.Strength(), own)
				worst = math.Min(worst, worth*(1-ctx.enemyDays[i][r.ID]/within))
			}
		}
		return worst
	}
	// the chance of holding a siege of r for the given days: of beating
	// what stands there and whatever enemy army could come to its relief
	// (the stronger of the two; a siege is not kept while the enemy's field
	// army is undefeated nearby)
	hold := func(r *Region, within float64) float64 {
		p, _ := fight(r)
		enemy, own := s.forces(r.ID)
		if relief := ctx.relief(r.ID, within); relief > enemy {
			adv := (own + a.Strength()) / relief
			if r.Nation != nil && r.Nation != n {
				adv /= defenderHome * r.Nation.defenseMult()
			}
			p = math.Min(p, adv*adv/(1+adv*adv))
		}
		return p
	}
	// a siege's days left, at the occupier's pace, if it is taken up (again)
	// after the given days: progress left unheld wears away meanwhile, so
	// an old siege at the far end of the map is no head start
	leftAfter := func(r *Region, occupier *Nation, after float64) float64 {
		rate, prog := w.siegeRate(r, occupier), 0.0
		if sg := r.Siege; sg != nil && sg.Nation == occupier {
			prog = math.Max(0, sg.Progress-after/occupationDays)
		}
		return (1 - prog) / rate
	}
	left := func(r *Region, occupier *Nation) float64 { return leftAfter(r, occupier, 0) }
	// the enemy's sieges of our tiles, and when each would fall
	type threat struct {
		r    *Region
		left float64
	}
	var threats []threat
	for _, r := range n.Regions {
		for _, e := range w.Armies {
			if e.Region == r.ID && n.AtWar[e.Nation.ID] && !e.Retreating && e.canOccupy() && len(e.Path) == 0 {
				threats = append(threats, threat{r, left(r, e.Nation)})
				break
			}
		}
	}
	here := w.Regions[a.Region]
	best := aiOption{going, math.Inf(-1)} // (nothing better: carry on, or go home)
	consider := func(dest int, v float64) {
		if dest == going && v > 0 {
			v *= 1 + aiKeepPlan // (a move worth nothing gets no bonus: an army with nothing to do goes home)
		}
		if v > best.value {
			best = aiOption{dest, v}
		}
	}
	// draw back to a tile of its own, out of the enemy's reach if it can:
	// what an army with nothing better to do does (and one filling up does
	// nothing else)
	withdraw := func() {
		for _, r := range n.Regions {
			d := days[r.ID]
			if r.ID == a.Region || math.IsInf(d, 1) || w.tileTaken(n, r.ID, a) || r.Siege != nil {
				continue
			}
			// (among tiles as safe, the nearest)
			consider(r.ID, rate(danger(r, d+aiWatch), d)-1e-9*d)
		}
	}
	if ctx.filling[a] {
		if len(a.Path) == 0 || !w.tileTaken(n, a.Region, a) {
			_, worth := fight(here)
			consider(a.Region, rate(worth+danger(here, aiWatch), 0))
		}
		withdraw()
		return best
	}
	// stay: keep a siege (and defend afterwards whatever it can still reach in time), or hold
	gain, took := 0.0, 0.0
	if w.besieging(n, a) {
		ours := left(here, n)
		gain, took = aiOtherTile*hold(here, ours), ours
		for _, t := range threats {
			if d := days[t.r.ID]; !w.tileTaken(n, t.r.ID, a) && ours+d < t.left {
				p, _ := fight(t.r)
				// (busy till its siege is done and it has marched there; if
				// it then loses, till the tile would have fallen)
				gain, took = gain+aiOwnTile*p, ours+d+(1-p)*(t.left-ours-d)
				break
			}
		}
	}
	for _, t := range threats { // an enemy besieging the tile it stands on: holding saves it
		if t.r == here {
			p, _ := fight(here)
			gain, took = gain+aiOwnTile*p, math.Max(took, t.left)
		}
	}
	_, worth := fight(here) // an enemy army on its tile: the fight happens anyway
	if len(a.Path) == 0 || !w.tileTaken(n, a.Region, a) {
		// (not halting on a tile another of its armies holds)
		consider(a.Region, rate(gain+worth+danger(here, math.Max(took, aiWatch)), took))
	}
	// defend a tile of ours an enemy army stands on
	besieging := w.besieging(n, a)
	ours := left(here, n)
	for _, t := range threats {
		d := days[t.r.ID]
		if t.r.ID == a.Region || math.IsInf(d, 1) || w.tileTaken(n, t.r.ID, a) {
			continue
		}
		if !s.engages(t.r) {
			continue
		}
		// a besieger breaks off only if the enemy would take the tile
		// before it takes its own by more than the march there and back:
		// else it keeps its siege (and defends afterwards if still in
		// time). Two armies each marching across the map to save its own
		// tile from the other, and back, take nothing.
		if besieging && t.left+2*d >= ours {
			continue
		}
		// nor does an army march to save a tile that would hold out a
		// season after it got there: it does what it can meanwhile, and
		// answers when the siege nears its end (a besieger it can beat
		// close by it hunts down at once all the same)
		if t.left-d > aiDefendSlack {
			continue
		}
		p, worth := fight(t.r)
		took := d
		if d < t.left { // in time to save it
			// a won battle breaks the besieger and frees it after the
			// march; a lost one leaves it tied down till the tile would
			// have fallen
			worth, took = worth+aiOwnTile*p, d+(1-p)*(t.left-d)
		}
		consider(t.r.ID, rate(worth+danger(t.r, took), took))
	}
	// hunt an enemy army it can beat: the sooner it is caught, the less it
	// has grown (a new army filling up is the easiest prey) and the fewer
	// tiles it takes
	for _, e := range ctx.enemies {
		r := w.Regions[e.Region]
		d := days[r.ID]
		if ctx.defendOnly[a] || besieging || r.ID == a.Region || math.IsInf(d, 1) || !w.reachable(n, r) ||
			w.tileTaken(n, r.ID, a) || w.allyTaken(n, r.ID) || !s.engages(r) {
			continue
		}
		if _, worth := fight(r); worth > 0 {
			consider(r.ID, rate(worth+danger(r, d+aiSettle), d))
		}
	}
	// march on an enemy or free tile: fight what stands there, then occupy it
	free := map[int]bool{}
	for _, t := range targets {
		free[t.region.ID] = true
	}
	maxPop := 1.0
	for _, r := range w.Regions {
		maxPop = math.Max(maxPop, r.Population())
	}
	for _, r := range w.Regions {
		if r.ID == a.Region || r.Nation == n {
			continue
		}
		enemyLand := r.Nation != nil && n.AtWar[r.Nation.ID] && w.borders(n, r)
		if !enemyLand && !(free[r.ID] && !w.peaceTaken(n, r.ID)) {
			continue
		}
		d := days[r.ID]
		if ctx.defendOnly[a] || math.IsInf(d, 1) || !w.reachable(n, r) || w.tileTaken(n, r.ID, a) || w.allyTaken(n, r.ID) || !s.engages(r) {
			continue
		}
		_, worth := fight(r)
		took := d + leftAfter(r, n, d)
		p := hold(r, took)
		// a populous tile is worth a little more
		consider(r.ID, rate(aiOtherTile*p*(0.97+0.03*r.Population()/maxPop)+worth+danger(r, took), took))
	}
	withdraw()
	return best
}

// aiMarchDays is how many days the army would take to reach each tile,
// through land its nation may enter and around tiles an enemy army holds
// (into which it can only march as its destination).
func (w *World) aiMarchDays(n *Nation, a *Army) []float64 {
	barred := w.enemyHeld(n)
	dist, _ := w.shortestPaths(a.Region, func(r *Region) bool { return w.reachable(n, r) && !barred[r.ID] })
	days := make([]float64, len(w.Regions))
	for i := range days {
		days[i] = marchDaysPerHop * dist[i]
	}
	for i, r := range w.Regions { // a held tile: from the nearest open neighbour
		if !barred[i] || !w.reachable(n, r) {
			continue
		}
		for _, x := range w.World.Adjacent(i) {
			if v := marchDaysPerHop * (dist[x[0]] + w.Links[x[1]].Hop); v < days[i] {
				days[i] = v
			}
		}
	}
	days[a.Region] = 0
	return days
}

// enemyHeld marks the tiles where an army of a nation at war with n stands.
func (w *World) enemyHeld(n *Nation) []bool {
	held := make([]bool, len(w.Regions))
	for _, e := range w.Armies {
		if e.Nation != n && n.AtWar[e.Nation.ID] && !e.Retreating && e.Men >= minOccupiers {
			held[e.Region] = true
		}
	}
	return held
}

// aiBuildMaxTax is the tax rate above which the computer (and automated
// construction) starts no new roads, ports or state firm expansions: a poor
// nation that builds anyway taxes its people away to its neighbours.
const aiBuildMaxTax = 0.15

// canAffordBuilding reports whether the nation can take on new building:
// light taxes. What it can spend on building each day is its building
// budget (Nation.BuildingBudget), which tapers as the treasury runs down,
// so there is no gate on the treasury here: a gate that opened and shut
// with the treasury made the state's spending, and the economy with it,
// cycle.
func (n *Nation) canAffordBuilding() bool {
	return n.TaxRate <= aiBuildMaxTax
}

// aiBuild keeps one infrastructure project going: a couple of ports once it
// can sail, else roads on the tile where better movement matters most (many
// people and much trade passing through).
func (w *World) aiBuild(n *Nation) {
	if !n.canAffordBuilding() {
		return
	}
	if !w.roomToBuild(n) {
		return
	}
	ports := 0
	for _, r := range n.Regions {
		if r.Port || (r.Project != nil && r.Project.Port) {
			ports++
		}
	}
	if n.CanBuildPorts() && ports < min(2, n.PortAllowance()) {
		var best *Region
		for _, r := range n.Regions {
			if r.Coastal && !r.Port && r.Project == nil && (best == nil || r.Population() > best.Population()) {
				best = r
			}
		}
		if best != nil {
			w.BuildPort(best.ID)
			return
		}
	}
	traffic := make([]float64, len(w.Regions))
	for _, t := range w.Traders {
		traffic[t.Origin.ID] += t.Shipped
		traffic[t.Dest.ID] += t.Shipped
	}
	var best *Region
	bestScore := 0.0
	for _, r := range n.Regions {
		if r.Project != nil {
			continue
		}
		next := r.Infra + 1
		if next >= len(InfraLevels) || !n.Has(InfraLevels[next].Tech) {
			continue
		}
		if score := r.Population() * (1 + traffic[r.ID]); score > bestScore {
			best, bestScore = r, score
		}
	}
	if best != nil {
		w.Build(best.ID)
	}
}

// roomToBuild reports whether the nation's building budget has room for
// another project beside those under way, each taking construction at the
// most a project can take a day. A state builds schools and roads in every
// province at once, as far as its money goes: a nation with four times the
// land had four times the ministry, not a queue four times as long.
func (w *World) roomToBuild(n *Nation) bool {
	price, k := 0.0, 0
	under := 0
	for _, r := range n.Regions {
		price += r.Markets[Construction].Price
		k++
		if r.Project != nil {
			under++
		}
		for _, f := range r.Firms {
			if e := f.Expansion; e != nil && e.External {
				under++
			}
		}
	}
	if k == 0 {
		return false
	}
	day := n.MaxBuildRate * n.buildRateMult() * price / float64(k)
	return float64(under+1)*day <= n.BuildingBudget()
}

// The computer's public services: it funds every level (universities only
// half when it can't pay its bills or is at war), and builds a level of a
// service where it falls furthest short: administration and public health
// to cover the need, schools to its Learning (aiSchooling of children at
// Learning 1), universities while they cost less than aiScience of income
// times its Learning. A level of each service at a time on a tile, as
// many tiles at once as its building budget runs to (roomToBuild).
const (
	aiScience   = 0.12
	aiSchooling = 0.6
)

func (w *World) aiServices(n *Nation, war, funding, universities bool) {
	short := n.payScale < 0.999
	if funding {
		for _, r := range n.Regions {
			for _, f := range r.Services {
				if f == nil {
					continue
				}
				funded := f.Levels
				if f.Kind == University && (short || war) {
					funded = (f.Levels + 1) / 2
				}
				w.SetServiceFunding(r.ID, f.Kind, funded)
			}
		}
	}
	if short || !n.canAffordBuilding() || w.burning {
		return // new levels only when it can afford them (and not in the burn-in)
	}
	for k := Public(0); k < NumServices; k++ {
		if k == University && (!universities || war || n.serviceBillOf(University) > aiScience*n.Temper.Learning*n.IncomeEMA) {
			continue
		}
		target := 1.0
		if k == Schools {
			target = math.Min(1, aiSchooling*n.Temper.Learning)
		}
		if !w.roomToBuild(n) {
			return
		}
		var best *Region
		bestCov := 2.0
		for _, r := range n.Regions {
			f := r.Service(k)
			if f == nil || f.Expansion != nil || f.Levels >= f.MaxLevels {
				continue
			}
			cov := 0.0
			if k == University {
				cov = -r.Population() // the biggest tile first
			} else if need := w.ServiceNeed(r, k); need > 0 {
				cov = float64(f.Levels) * ServiceStaff / need // what its levels could cover
				if cov >= target {
					continue
				}
			} else {
				continue
			}
			if cov < bestCov {
				best, bestCov = r, cov
			}
		}
		if best != nil {
			w.ExpandService(best.ID, k)
		}
	}
}

// serviceBillOf is what the nation's funded levels of one service cost today.
func (n *Nation) serviceBillOf(k Public) float64 {
	s := 0.0
	for _, r := range n.Regions {
		if f := r.Service(k); f != nil {
			s += float64(f.Funded) * ServiceStaff * r.Wage
		}
	}
	return s
}

// aiTax is a predictive controller: every aiTaxEvery days it sets the tax
// rate whose predicted income covers the average day's spending and closes
// the gap to the reserve (aiTreasuryDays of spending) over aiTaxHorizon
// days. Income is predicted as proportional to the rate (today's income per
// point of tax). The rate moves at most aiTaxStep at a time: a steep tax
// shrinks the economy it is predicted from, and in big monthly steps, read
// off averages weeks old, the rule overshot both ways and cycled. Computer
// nations do it always, players who ask for it (AutoTax) too.
const (
	aiTreasuryDays = 15.0
	aiTaxHorizon   = 90.0
	aiTaxStep      = 0.005
	aiWarTaxSpeed  = 4.0
	aiTaxEvery     = 7
)

func (w *World) aiTax(n *Nation) {
	if n.TaxRate <= 0 || n.IncomeEMA <= 0 {
		n.SetTaxRate(math.Max(n.TaxRate, 0.05)) // nothing to predict from yet
		return
	}
	perPoint := n.IncomeEMA / n.TaxRate
	want := n.SpendEMA + (n.reserve()-n.Treasury)/aiTaxHorizon
	// at war taxes go up four times as fast: states at war raised them
	// within months (Britain's income tax of 1798, the Union's of 1861-62),
	// or their armies went hungry
	step := aiTaxStep
	if w.atWar(n) {
		step *= aiWarTaxSpeed
	}
	rate := math.Max(n.TaxRate-step, math.Min(n.TaxRate+step, want/perPoint))
	n.SetTaxRate(math.Max(0.005, math.Min(0.6, rate)))
}

// aiExpandFirm keeps one state-funded firm expansion going, on the firm with
// the best profit per building level.
func (w *World) aiExpandFirm(n *Nation) {
	if !n.canAffordBuilding() {
		return
	}
	for _, r := range n.Regions {
		for _, f := range r.Firms {
			if f.Expansion != nil && f.Expansion.External {
				return
			}
		}
	}
	region, firm := -1, -1
	best := 0.0
	for _, r := range n.Regions {
		for i, f := range r.Firms {
			if f.Expansion != nil || !f.Enabled || f.AtCap() {
				continue
			}
			if f.Levels == 0 {
				continue // a method yet to be refitted to
			}
			if p := f.ProfitEMA / float64(f.Levels); p > best {
				region, firm, best = r.ID, i, p
			}
		}
	}
	if region >= 0 {
		w.Expand(region, firm)
	}
}

type expansionTarget struct {
	region *Region
	score  float64
}

// expansionTargets lists unowned tiles bordering the nation, valued by their
// people and wellbeing.
func (w *World) expansionTargets(n *Nation) []expansionTarget {
	var out []expansionTarget
	seen := make([]bool, len(w.Regions))
	for _, own := range n.Regions {
		w.landNeighbours(own, func(r *Region) bool {
			if !seen[r.ID] && r.Nation == nil {
				seen[r.ID] = true
				out = append(out, expansionTarget{r, r.Population() * math.Max(r.UtilityEMA, 1)})
			}
			return true
		})
	}
	return out
}

// allyTaken reports whether an army of one of n's allies holds tile r or is
// marching to it: only one can occupy it, so allies leave it to each other.
func (w *World) allyTaken(n *Nation, r int) bool {
	for _, o := range w.Nations {
		if w.Allied(n, o) && w.tileTaken(o, r, nil) {
			return true
		}
	}
	return false
}
