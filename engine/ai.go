package engine

import "math"

// The computer's nations think every aiInterval days. The AI is part of the
// simulation: deterministic, so saved games and replays continue exactly.
const aiInterval = 30

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
	ownPot, threatPot, invaded := w.ArmyPotential(n), 0.0, false
	for _, o := range w.Nations {
		if o == n || o.Eliminated() || !w.nationsBorder(n, o) {
			continue
		}
		threatPot = math.Max(threatPot, w.ArmyPotential(o))
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

// ArmyPotential is what a nation's armies could field: each at its full
// (wartime) size and fully supplied. Unlike its might, it doesn't move when
// the nation calls up or stands down its reserves.
func (w *World) ArmyPotential(n *Nation) float64 {
	s := 0.0
	for _, a := range w.ArmiesOf(n) {
		s += math.Max(a.Men, a.Target) * Doctrines[a.Doctrine].Power * n.attackMult()
	}
	return s
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
	advantage := aiWarAdvantage / t.Aggression * (1 + 0.15*w.noise(n, 1))
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
	size := n.MaxArmySize()
	armies := w.ArmiesOf(n)
	for k := int(math.Ceil(want/size - 1e-9)); len(armies) < k; {
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
	for _, a := range armies {
		if a.Retreating {
			continue
		}
		// fights come first, even for an army on the march or besieging:
		// defend our soil, then engage enemies it can beat
		if fight := w.aiFight(n, a); fight >= 0 {
			if going := a.Region; len(a.Path) > 0 {
				going = a.Path[len(a.Path)-1]
				if going == fight {
					continue
				}
			} else if going == fight {
				continue
			}
			w.MoveArmy(a.ID, fight)
			continue
		}
		if len(a.Path) > 0 {
			continue
		}
		atHome := w.Regions[a.Region].Nation == n
		if atHome && a.Men < aiRefill*w.UnderArms(a) && !w.underAttack(n) {
			continue // let a new army fill up (to what it keeps under arms now) before it marches; field armies are reinforced where they stand
		}
		here := w.Regions[a.Region]
		if here.Nation != n && here.Siege != nil && here.Siege.Nation == n && a.canOccupy() {
			continue // besieging
		}
		if dest := w.aiDestination(n, a, targets); dest >= 0 && dest != a.Region {
			w.MoveArmy(a.ID, dest)
		}
	}
}

// The computer only seeks battle when its side is this many times as strong.
const aiFightOdds = 1.2

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
		odds: aiFightOdds / math.Sqrt(n.Temper.Aggression) * (1 + 0.1*w.noise(n, 100+a.ID))}
}

// forces is the strength of the armies at war with n, and of n's other
// armies, in tile r.
func (s *aiScope) forces(r int) (enemy, own float64) {
	for _, e := range s.w.Armies {
		if e.Region != r || e == s.a {
			continue
		}
		if e.Nation == s.n {
			own += e.Strength()
		} else if s.n.AtWar[e.Nation.ID] {
			enemy += e.Strength()
		}
	}
	return
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

// pick keeps the best-scoring tile (a little less for distance).
func (s *aiScope) pick(r *Region, score float64) {
	if score -= 0.01 * s.dist[r.ID]; score > s.top {
		s.best, s.top = r.ID, score
	}
}

// aiFight is where the army should fight, or -1: first defend (enemy armies
// on our soil it can beat, with our home bonus), then engage (enemy armies
// anywhere it can reach and beat, with their home bonus on their soil); in
// each, the one it beats by the widest margin. Fights come before any
// march or siege.
func (w *World) aiFight(n *Nation, a *Army) int {
	s := w.newAIScope(n, a)
	return s.fight()
}

func (s *aiScope) fight() int {
	w, n, a := s.w, s.n, s.a
	for _, home := range []bool{true, false} {
		for _, e := range w.Armies {
			r := w.Regions[e.Region]
			if e.Nation == n || !n.AtWar[e.Nation.ID] || e.Retreating || (r.Nation == n) != home ||
				w.tileTaken(n, r.ID, a) || !w.reachable(n, r) {
				continue
			}
			if adv := s.advantage(r); adv >= s.odds {
				s.pick(r, adv)
			}
		}
		if s.best >= 0 {
			return s.best
		}
	}
	return -1
}

// aiDestination picks where an idle army should go, or -1: a fight (see
// aiFight), else enemy land, else free land.
func (w *World) aiDestination(n *Nation, a *Army, targets []expansionTarget) int {
	s := w.newAIScope(n, a)
	if f := s.fight(); f >= 0 {
		return f
	}
	forces, advantage, odds := s.forces, s.advantage, s.odds
	pick := s.pick
	// too weak to take ground: home to refill
	if !a.canOccupy() {
		if w.Regions[a.Region].Nation != n {
			w.sendHome(a)
		}
		return -1
	}
	// then take land: enemy border tiles it can win, the weakest held and
	// then the most populous first
	for _, r := range w.Regions {
		if r.Nation == nil || r.Nation == n || !n.AtWar[r.Nation.ID] || !w.borders(n, r) || w.tileTaken(n, r.ID, a) || w.allyTaken(n, r.ID) || advantage(r) < odds {
			continue
		}
		enemy, _ := forces(r.ID)
		pick(r, -enemy*1000+r.Population())
	}
	if s.best >= 0 {
		return s.best
	}
	// expand: free land, where it is stronger than any other nation's armies
	// (the strongest there occupies it; a weaker army would only wait to be
	// beaten)
	best, bestScore := -1, 0.0
	for _, t := range targets {
		if w.tileTaken(n, t.region.ID, a) || w.allyTaken(n, t.region.ID) || !w.strongestOn(n, a, t.region.ID) {
			continue
		}
		if t.score > bestScore {
			best, bestScore = t.region.ID, t.score
		}
	}
	if best >= 0 {
		return best
	}
	if w.Regions[a.Region].Nation != n {
		w.sendHome(a)
	}
	return -1
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
	ports := 0
	for _, r := range n.Regions {
		if r.Project != nil {
			return
		}
		if r.Port {
			ports++
		}
	}
	if n.CanBuildPorts() && ports < min(2, n.PortAllowance()) {
		var best *Region
		for _, r := range n.Regions {
			if r.Coastal && !r.Port && (best == nil || r.Population() > best.Population()) {
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

// The computer's public services: it funds every level (universities only
// half when it can't pay its bills or is at war), and builds a level of a
// service where it falls furthest short: administration and public health
// to cover the need, schools to its Learning (aiSchooling of children at
// Learning 1), universities while they cost less than aiScience of income
// times its Learning. One level of each service at a time, and only while
// the treasury holds a month of spending.
const (
	aiScience   = 0.12
	aiSchooling = 0.6
)

func (w *World) aiServices(n *Nation, war, funding, universities bool) {
	short := n.payScale < 0.999
	if funding {
		for _, r := range n.Regions {
			for _, f := range r.Firms {
				if !f.State {
					continue
				}
				funded := f.Levels
				if f.Public == University && (short || war) {
					funded = (f.Levels + 1) / 2
				}
				w.SetServiceFunding(r.ID, f.Public, funded)
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
		var best *Region
		bestCov := 2.0
		busy := false
		for _, r := range n.Regions {
			f := r.Service(k)
			if f == nil || !f.Enabled {
				continue
			}
			if f.Expansion != nil {
				busy = true
				break
			}
			if f.Levels >= f.MaxLevels {
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
		if !busy && best != nil {
			w.ExpandService(best.ID, k)
		}
	}
}

// serviceBillOf is what the nation's funded levels of one service cost today.
func (n *Nation) serviceBillOf(k Public) float64 {
	s := 0.0
	for _, r := range n.Regions {
		if f := r.Service(k); f != nil && f.Enabled {
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
	aiTaxEvery     = 7
)

func (w *World) aiTax(n *Nation) {
	if n.TaxRate <= 0 || n.IncomeEMA <= 0 {
		n.SetTaxRate(math.Max(n.TaxRate, 0.05)) // nothing to predict from yet
		return
	}
	// what a point of tax brings in, and what the tax must bring in: the
	// spending and the gap to the reserve, less the mint's new money
	perPoint := math.Max(n.IncomeEMA-n.MintEMA, 1e-9) / n.TaxRate
	want := n.SpendEMA + (n.reserve()-n.Treasury)/aiTaxHorizon - n.MintEMA
	rate := math.Max(n.TaxRate-aiTaxStep, math.Min(n.TaxRate+aiTaxStep, want/perPoint))
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
			if f.Expansion != nil && f.Expansion.Public && !f.State {
				return
			}
		}
	}
	region, firm := -1, -1
	best := 0.0
	for _, r := range n.Regions {
		for i, f := range r.Firms {
			if f.Expansion != nil || f.State || !f.Enabled || f.Levels >= f.MaxLevels {
				continue
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

// strongestOn reports whether army a (with its nation's other armies there)
// would be stronger in tile r than every other nation's armies on it.
func (w *World) strongestOn(n *Nation, a *Army, r int) bool {
	ours, theirs := a.Strength(), map[*Nation]float64{}
	for _, e := range w.Armies {
		if e.Region != r || e == a {
			continue
		}
		if e.Nation == n {
			ours += e.Strength()
		} else {
			theirs[e.Nation] += e.Strength()
		}
	}
	for _, s := range theirs {
		if s >= ours {
			return false
		}
	}
	return true
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
