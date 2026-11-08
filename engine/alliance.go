package engine

import "errors"

// Balance of power: when one nation grows far stronger than all the others,
// they ally against it. An alliance forms when the strongest nation's might
// is at least allianceForm times every other's, and breaks up when it falls
// below allianceBreak times the next strongest (or the leader or a member
// falls). Allies can't make war on each other, and when one of them is at war
// with the nation they're allied against, the others join that war (without
// needing a border), unless they made peace with it within the truce.
//
// Might is judged over a year (a moving average, updated monthly), and an
// alliance holds for at least allianceYears unless its foe or a member falls:
// month-to-month swings in small early armies mustn't make and break it.
const (
	allianceForm  = 1.5
	allianceBreak = 1.2
	allianceYears = 3
	mightMonths   = 12 // the moving average of might spans about this many months
)

// ErrAllied is returned when a nation declares war on an ally.
var ErrAllied = errors.New("the nations are allies")

// Allied reports whether a and b are in the same alliance.
func (w *World) Allied(a, b *Nation) bool {
	return a != b && a.Coalition > 0 && a.Coalition == b.Coalition
}

// AlliedAgainst is the nation a's alliance stands against, or nil.
func (w *World) AlliedAgainst(a *Nation) *Nation {
	if a.Coalition <= 0 {
		return nil
	}
	return w.Nations[a.Coalition-1]
}

// updateAlliances forms and dissolves alliances, and brings allies into
// their partners' wars against the nation they're allied against.
func (w *World) updateAlliances() {
	var leader *Nation
	might := make([]float64, len(w.Nations))
	for i, n := range w.Nations {
		if n.Eliminated() {
			n.MightEMA = 0
			continue
		}
		n.MightEMA += (w.ArmyStrength(n) - n.MightEMA) / mightMonths
		might[i] = n.MightEMA
		if leader == nil || might[i] > might[leader.ID] {
			leader = n
		}
	}
	if leader == nil {
		return
	}
	second, others := 0.0, 0
	for i, n := range w.Nations {
		if n != leader && !n.Eliminated() {
			others++
			if might[i] > second {
				second = might[i]
			}
		}
	}
	// break up an alliance whose reason has passed
	for _, n := range w.Nations {
		h := w.AlliedAgainst(n)
		if h == nil {
			continue
		}
		fallen := n.Eliminated() || h.Eliminated() || w.members(h) < 2
		young := float64(w.Tick-n.AllianceSince) < allianceYears*TicksPerYear
		if fallen || (!young && (h != leader || might[h.ID] < allianceBreak*second)) {
			w.dissolve(h)
		}
	}
	// form one against a dominant nation
	if others >= 2 && might[leader.ID] >= allianceForm*second && second > 0 && w.members(leader) == 0 {
		var names []string
		for _, n := range w.Nations {
			if n != leader && !n.Eliminated() {
				n.Coalition, n.AllianceSince = leader.ID+1, w.Tick
				names = append(names, n.Name)
			}
		}
		w.emit(-1, "alliance", -1, "%s ally against %s", joinNames(names), leader.Name)
	}
	// allies join each other's wars against it
	for _, h := range w.Nations {
		if w.members(h) == 0 {
			continue
		}
		fighting := false
		for _, n := range w.Nations {
			if w.AlliedAgainst(n) == h && n.AtWar[h.ID] {
				fighting = true
			}
		}
		if !fighting {
			continue
		}
		for _, n := range w.Nations {
			if w.AlliedAgainst(n) != h || n.AtWar[h.ID] || n.Eliminated() {
				continue
			}
			if since := w.Tick - n.WarSince[h.ID]; n.WarSince[h.ID] > 0 && float64(since) < aiTruceYears*TicksPerYear {
				continue // a separate peace holds for the truce
			}
			n.AtWar[h.ID], h.AtWar[n.ID] = true, true
			n.WarSince[h.ID], h.WarSince[n.ID] = w.Tick, w.Tick
			n.PeaceOffer[h.ID], h.PeaceOffer[n.ID] = false, false
			w.emit(-1, "war", -1, "%s joins its allies' war on %s", n.Name, h.Name)
		}
	}
}

// members counts the nations allied against h.
func (w *World) members(h *Nation) int {
	k := 0
	for _, n := range w.Nations {
		if w.AlliedAgainst(n) == h && !n.Eliminated() {
			k++
		}
	}
	return k
}

// dissolve breaks up the alliance against h.
func (w *World) dissolve(h *Nation) {
	var names []string
	for _, n := range w.Nations {
		if w.AlliedAgainst(n) == h {
			n.Coalition = 0
			names = append(names, n.Name)
		}
	}
	w.emit(-1, "alliance", -1, "The alliance of %s against %s breaks up", joinNames(names), h.Name)
}

func joinNames(names []string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	}
	s := names[0]
	for _, x := range names[1 : len(names)-1] {
		s += ", " + x
	}
	return s + " and " + names[len(names)-1]
}
