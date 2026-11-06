package engine

import "math"

// The computer's nations think every aiInterval ticks. The AI is part of the
// simulation: deterministic, so saved games and replays continue exactly.
const aiInterval = 10

// AI budgets: (economy, science, military).
var (
	aiPeaceBudget  = [NumBudgets]float64{0.45, 0.25, 0.15}
	aiExpandBudget = [NumBudgets]float64{0.4, 0.2, 0.3}
	aiWarBudget    = [NumBudgets]float64{0.3, 0.1, 0.5}
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

func (w *World) think(n *Nation) {
	threat, attacked := 0.0, false
	for _, o := range w.Nations {
		if o == n || o.Eliminated() || !w.nationsBorder(n, o) {
			continue
		}
		threat = math.Max(threat, o.Army)
		if o.Campaign != nil && w.Regions[o.Campaign.Target].Nation == n {
			attacked = true
		}
	}
	targets := w.campaignTargets(n)
	switch {
	case attacked || threat > 1.2*n.Army+1:
		n.SetBudget(aiWarBudget)
	case len(targets) > 0:
		n.SetBudget(aiExpandBudget)
	default:
		n.SetBudget(aiPeaceBudget)
	}
	w.aiBuild(n)
	w.aiExpandFirm(n)
	if n.Campaign == nil && !attacked {
		w.aiCampaign(n, targets)
	}
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

// aiBuild keeps one infrastructure project going, on the tile where better
// movement matters most: many people and much trade passing through.
func (w *World) aiBuild(n *Nation) {
	for _, r := range n.Regions {
		if r.Project != nil {
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
	var region, firm = -1, -1
	best := 0.0
	for _, r := range n.Regions {
		for i, f := range r.Firms {
			if f.Expansion != nil || f.Levels >= f.MaxLevels {
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

type campaignTarget struct {
	region *Region
	score  float64
}

// campaignTargets lists the bordering tiles the nation could take: unowned
// tiles, valued by their people and wellbeing, and other nations' tiles only
// when the army clearly outmatches their defence.
func (w *World) campaignTargets(n *Nation) []campaignTarget {
	var out []campaignTarget
	seen := make([]bool, len(w.Regions))
	for _, own := range n.Regions {
		for _, nb := range HexNeighbors(own.Row, own.Col, w.Rows, w.Cols) {
			r := w.Regions[nb[0]*w.Cols+nb[1]]
			if seen[r.ID] || r.Nation == n {
				continue
			}
			seen[r.ID] = true
			score := r.Population() * math.Max(r.UtilityEMA, 1)
			if r.Nation != nil {
				if n.Army < 2.5*defenderAdvantage*r.Nation.Defense() {
					continue
				}
				score *= 0.5 // conquest makes enemies; prefer free land
			}
			out = append(out, campaignTarget{r, score})
		}
	}
	return out
}

func (w *World) aiCampaign(n *Nation, targets []campaignTarget) {
	var best *Region
	bestScore := 0.0
	for _, t := range targets {
		// only march if the army can make real progress
		if n.Army < 0.3*occupyNeedPerHead*t.region.Population() {
			continue
		}
		if t.score > bestScore {
			best, bestScore = t.region, t.score
		}
	}
	if best != nil {
		w.StartCampaign(n.ID, best.ID)
	}
}
