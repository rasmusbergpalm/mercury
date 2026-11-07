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
	if w.Tick%aiInterval != 0 {
		return
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
			if n.AutoTax {
				w.aiTax(n)
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
	threat, invaded := 0.0, false
	for _, o := range w.Nations {
		if o == n || o.Eliminated() || !w.nationsBorder(n, o) {
			continue
		}
		threat = math.Max(threat, w.ArmyStrength(o))
	}
	for _, a := range w.Armies {
		if a.Nation != n && w.Regions[a.Region].Nation == n {
			invaded = true
		}
	}
	targets := w.expansionTargets(n)
	war := w.atWar(n)
	t := n.Temper
	switch {
	case war || invaded || threat > 1.3*own+1:
		n.SetDrill(MaxDrill) // keep the arms works busy for what's coming
	default:
		n.SetDrill(math.Min(MaxDrill, math.Max(MinDrill, DefaultDrill*t.Militarism)))
	}
	w.aiDiplomacy(n, own)
	w.aiBuild(n)
	w.aiExpandFirm(n)
	w.aiServices(n, war || invaded, true, true)
	w.aiTax(n)
	w.aiArmies(n, targets)
}

// nationsBorder reports whether any tiles of a and b are neighbours.
func (w *World) nationsBorder(a, b *Nation) bool {
	for _, r := range a.Regions {
		for _, nb := range HexNeighbors(r.Row, r.Col, w.Rows, w.Cols) {
			if w.Regions[nb[0]*w.Cols+nb[1]].Nation == b {
				return true
			}
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
		w.UpgradeArmy(a.ID)
		if len(a.Path) > 0 {
			continue
		}
		atHome := w.Regions[a.Region].Nation == n
		if atHome && a.Men < aiRefill*a.Target && !w.underAttack(n) {
			continue // let a new army fill up before it marches (field armies are reinforced where they stand)
		}
		here := w.Regions[a.Region]
		if here.Nation != n && here.Siege != nil && here.Siege.Nation == n {
			continue // besieging
		}
		if dest := w.aiDestination(n, a, targets); dest >= 0 && dest != a.Region {
			w.MoveArmy(a.ID, dest)
		}
	}
}

// The computer only seeks battle when its side is this many times as strong.
const aiFightOdds = 1.2

// An army at home waits until it has this share of its target before it
// marches (unless the nation is being invaded).
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
func (w *World) aiDestination(n *Nation, a *Army, targets []expansionTarget) int {
	// strength of the armies of nations at war with n, and of n, in a tile
	forces := func(r int) (enemy, own float64) {
		for _, e := range w.Armies {
			if e.Region != r || e == a {
				continue
			}
			if e.Nation == n {
				own += e.Strength()
			} else if n.AtWar[e.Nation.ID] {
				enemy += e.Strength()
			}
		}
		return
	}
	// defend: an enemy army on our soil, if together with our armies there
	// we can beat it; if we can't, hold and recruit
	odds := aiFightOdds / math.Sqrt(n.Temper.Aggression) * (1 + 0.1*w.noise(n, 100+a.ID))
	invaded := false
	for _, e := range w.Armies {
		if e.Nation != n && n.AtWar[e.Nation.ID] && w.Regions[e.Region].Nation == n {
			invaded = true
			if w.tileTaken(n, e.Region, a) {
				continue // another of our armies is on it
			}
			enemy, own := forces(e.Region)
			if (own+a.Strength())*defenderHome*n.defenseMult() >= odds*enemy {
				return e.Region
			}
		}
	}
	if invaded {
		return -1
	}
	// attack: the enemy border tile with the least enemy strength, then most
	// people, among those the army can win
	best, bestScore := -1, math.Inf(-1)
	for _, r := range w.Regions {
		if r.Nation == nil || r.Nation == n || !n.AtWar[r.Nation.ID] || !w.borders(n, r) || w.tileTaken(n, r.ID, a) {
			continue
		}
		enemy, own := forces(r.ID)
		if own+a.Strength() < odds*enemy*defenderHome*r.Nation.defenseMult() {
			continue
		}
		if score := -enemy*1000 + r.Population(); score > bestScore {
			best, bestScore = r.ID, score
		}
	}
	if best >= 0 {
		return best
	}
	// expand: free land the army is strong enough to take in reasonable time
	bestScore = 0
	for _, t := range targets {
		if w.tileTaken(n, t.region.ID, a) {
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

// aiBuild keeps one infrastructure project going: a couple of ports once it
// can sail, else roads on the tile where better movement matters most (many
// people and much trade passing through).
func (w *World) aiBuild(n *Nation) {
	ports := 0
	for _, r := range n.Regions {
		if r.Project != nil {
			return
		}
		if r.Port {
			ports++
		}
	}
	if n.CanBuildPorts() && ports < 2 {
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
	if short || n.Treasury < 0.5*aiTreasuryDays*n.SpendEMA {
		return
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

// aiTax is a predictive controller: once a month it sets the tax rate whose
// predicted income covers the average day's spending and closes the gap to
// aiTreasuryDays of spending within aiTaxHorizon days. Income is predicted
// as proportional to the rate (today's income per point of tax). Computer
// nations do it always, players who ask for it (AutoTax) too.
const (
	aiTreasuryDays = 30.0
	aiTaxHorizon   = 30.0
)

func (w *World) aiTax(n *Nation) {
	if n.TaxRate <= 0 || n.IncomeEMA <= 0 {
		n.SetTaxRate(math.Max(n.TaxRate, 0.05)) // nothing to predict from yet
		return
	}
	perPoint := n.IncomeEMA / n.TaxRate
	want := n.SpendEMA + (aiTreasuryDays*n.SpendEMA-n.Treasury)/aiTaxHorizon
	n.SetTaxRate(math.Max(0.005, math.Min(0.6, want/perPoint)))
}

// aiExpandFirm keeps one state-funded firm expansion going, on the firm with
// the best profit per building level.
func (w *World) aiExpandFirm(n *Nation) {
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
		for _, nb := range HexNeighbors(own.Row, own.Col, w.Rows, w.Cols) {
			r := w.Regions[nb[0]*w.Cols+nb[1]]
			if seen[r.ID] || r.Nation != nil {
				continue
			}
			seen[r.ID] = true
			out = append(out, expansionTarget{r, r.Population() * math.Max(r.UtilityEMA, 1)})
		}
	}
	return out
}
