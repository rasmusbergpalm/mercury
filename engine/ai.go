package engine

import "math"

// The computer's nations think every aiInterval days. The AI is part of the
// simulation: deterministic, so saved games and replays continue exactly.
const aiInterval = 30

// AI budgets: (economy, science, military).
var (
	aiPeaceBudget  = [NumBudgets]float64{0.45, 0.3, 0.15}
	aiExpandBudget = [NumBudgets]float64{0.4, 0.25, 0.25}
	aiWarBudget    = [NumBudgets]float64{0.25, 0.15, 0.55}
)

// AI war policy.
const (
	aiGraceYears    = 15  // no wars of choice before this
	aiWarAdvantage  = 2.0 // declare war on a neighbour this much weaker
	aiWarYears      = 3   // seek peace after this long
	aiLosingRatio   = 0.6 // seek peace when this much weaker than the enemy
	aiPeaceMobilize = 0.5 // share of the mobilization cap kept under arms in peace
)

func (w *World) runAI() {
	if w.Tick%aiInterval != 0 {
		return
	}
	for _, n := range w.Nations {
		if n.AI && !n.Eliminated() {
			w.think(n)
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
	switch {
	case war || invaded || threat > 1.3*own+1:
		n.SetBudget(aiWarBudget)
	case len(targets) > 0:
		n.SetBudget(aiExpandBudget)
	default:
		n.SetBudget(aiPeaceBudget)
	}
	w.aiDiplomacy(n, own)
	w.aiBuild(n)
	w.aiExpandFirm(n)
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
	if w.atWar(n) || float64(w.Tick) < aiGraceYears*TicksPerYear || len(w.expansionTargets(n)) > 0 {
		return
	}
	var victim *Nation
	for _, o := range w.Nations {
		if o == n || o.Eliminated() || !w.nationsBorder(n, o) || w.atWar(o) {
			continue // no ganging up on a nation already at war
		}
		if s := w.ArmyStrength(o); own > aiWarAdvantage*s+1 && (victim == nil || s < w.ArmyStrength(victim)) {
			victim = o
		}
	}
	if victim != nil {
		w.DeclareWar(n.ID, victim.ID)
	}
}

// aiArmies keeps a field army of the right size and doctrine, and gives idle
// armies orders: defend invaded tiles, attack the enemy's weakest border
// tile, or occupy free land.
func (w *World) aiArmies(n *Nation, targets []expansionTarget) {
	war := w.atWar(n)
	share := aiPeaceMobilize
	if war {
		share = 1
	}
	want := share * w.MobilizationCap(n)
	armies := w.ArmiesOf(n)
	if len(armies) == 0 {
		home := n.Regions[0]
		for _, r := range n.Regions {
			if r.Population() > home.Population() {
				home = r
			}
		}
		if a, err := w.RaiseArmy(n.ID, home.ID, want); err == nil {
			armies = append(armies, a)
		}
	} else {
		w.SetArmyTarget(armies[0].ID, want)
	}
	for _, a := range armies {
		w.UpgradeArmy(a.ID)
		if len(a.Path) > 0 || a.Retreating || a.Men < 1 {
			continue
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

// aiDestination picks where an idle army should go, or -1.
func (w *World) aiDestination(n *Nation, a *Army, targets []expansionTarget) int {
	// defend: an enemy army on our soil
	for _, e := range w.Armies {
		if e.Nation != n && n.AtWar[e.Nation.ID] && w.Regions[e.Region].Nation == n {
			return e.Region
		}
	}
	// attack: the enemy border tile with the least enemy strength, then most people
	best, bestScore := -1, math.Inf(-1)
	for _, r := range w.Regions {
		if r.Nation == nil || r.Nation == n || !n.AtWar[r.Nation.ID] || !w.borders(n, r) {
			continue
		}
		enemy := 0.0
		for _, e := range w.Armies {
			if e.Region == r.ID && e.Nation == r.Nation {
				enemy += e.Strength()
			}
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

// aiExpandFirm keeps one state-funded firm expansion going, on the firm with
// the best profit per building level.
func (w *World) aiExpandFirm(n *Nation) {
	for _, r := range n.Regions {
		for _, f := range r.Firms {
			if f.Expansion != nil && f.Expansion.Public {
				return
			}
		}
	}
	region, firm := -1, -1
	best := 0.0
	for _, r := range n.Regions {
		for i, f := range r.Firms {
			if f.Expansion != nil || !f.Enabled || f.Levels >= f.MaxLevels {
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
