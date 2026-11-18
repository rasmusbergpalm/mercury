package game

import (
	"math"
	"math/rand"
	"strings"
)

// Temperament shapes how a computer-run nation plays, so that rivals don't
// all pursue the same strategy. Each nation draws one from the game's seed
// when the game is built; every trait is a multiplier around 1.
type Temperament struct {
	Militarism float64 // men kept under arms, and how many of them in peace
	Learning   float64 // how much it invests in universities
	Aggression float64 // how readily it starts wars and seeks battle
	Expansion  float64 // how many men it fields to take free land
}

func drawTemperament(seed int64, nation int) Temperament {
	r := rand.New(rand.NewSource(seed*7919 + int64(nation)*104729 + 1))
	trait := func(lo, hi float64) float64 { return lo + (hi-lo)*r.Float64() }
	return Temperament{
		Militarism: trait(0.7, 1.4),
		Learning:   trait(0.7, 1.4),
		Aggression: trait(0.6, 1.5),
		Expansion:  trait(0.6, 1.4),
	}
}

// Describe sums the temperament up in a few words.
func (t Temperament) Describe() string {
	var w []string
	pick := func(x float64, low, high string) {
		switch {
		case x >= 1.15:
			w = append(w, high)
		case x <= 0.85:
			w = append(w, low)
		}
	}
	pick(t.Aggression, "cautious", "belligerent")
	pick(t.Militarism, "unmilitary", "militarist")
	pick(t.Learning, "unlettered", "scholarly")
	pick(t.Expansion, "settled", "expansionist")
	if len(w) == 0 {
		return "even-tempered"
	}
	return strings.Join(w, ", ")
}

// noise is a deterministic pseudo-random number in [-1, 1] for a nation's
// k-th kind of decision, the key-th time: the AI varies its choices a little
// without breaking the simulation's determinism (saved games continue
// exactly). The key is a stable count (the year, the day of a battle), not
// the date: a change that moves some event by a day then doesn't re-roll
// every draw after it, so two versions of the economy stay comparable.
func (w *World) noise(n *Nation, k, key int) float64 {
	x := uint64(w.seed())*0x9E3779B97F4A7C15 ^ uint64(n.ID+1)*0xBF58476D1CE4E5B9 ^ uint64(key+1)*0x94D049BB133111EB ^ uint64(k+1)*0xD6E8FEB86659FD93
	x ^= x >> 31
	x *= 0x9E3779B97F4A7C15
	x ^= x >> 29
	return 2*float64(x>>11)/float64(1<<53) - 1
}

// clamp01 keeps x in [0, 1].
func clamp01(x float64) float64 { return math.Max(0, math.Min(1, x)) }

// drawResearch picks a computer nation's next tech at random among those it
// can research: cheaper ones are likelier (by the square of the cheapest's
// cost over theirs), and its temperament leans it towards a branch (science
// by Learning, military by Militarism, politics by Aggression, transport by
// Expansion, each squared). The draw is deterministic: the game's seed, the
// nation and how many techs it knows, so a seed fixes the order a nation
// learns its techs in, whatever day each comes (keyed on the day, a tech
// finished a day sooner drew another next tech, and the smallest change to
// the economy sent a game down another tree). -1 if nothing is left.
func (n *Nation) drawResearch(w *World) int {
	var ids []int
	cost := make([]float64, len(Techs))
	cheapest := math.Inf(1)
	for i := range Techs {
		if n.CanResearch(i) && i != n.Copying {
			ids = append(ids, i)
			cost[i] = n.TechCost(i, w) // what it costs this nation: less where its partners know it
			cheapest = math.Min(cheapest, cost[i])
		}
	}
	if len(ids) == 0 {
		return -1
	}
	t := n.Temper
	lean := map[string]float64{BranchScience: t.Learning, BranchMilitary: t.Militarism, BranchPolitics: t.Aggression, BranchTransport: t.Expansion}
	// a nation facing a neighbour or enemy that could field more than it
	// can arms itself: France's chassepot after Prussia's needle gun won in
	// 1866, the naval races before 1914
	threat := w.militaryThreat(n)
	weights, total := make([]float64, len(ids)), 0.0
	for k, i := range ids {
		x := math.Pow(cheapest/cost[i], 2)
		if l, ok := lean[Techs[i].Branch]; ok && l > 0 {
			x *= l * l
		}
		if Techs[i].Branch == BranchMilitary && threat > 1 {
			x *= threat * threat
		}
		weights[k] = x
		total += x
	}
	x := n.random(len(ids)) * total
	for k, w := range weights {
		if x < w {
			return ids[k]
		}
		x -= w
	}
	return ids[len(ids)-1]
}

// militaryThreat is how many times the nation's potential might the
// strongest nation it borders or is at war with could raise (ArmyPotential:
// everyone it can call up, at its best doctrine, not what it has under
// arms today). A man's worth of strength is added to each side.
func (w *World) militaryThreat(n *Nation) float64 {
	threat := 0.0
	for _, o := range w.Nations {
		if o != n && !o.Eliminated() && (n.AtWar[o.ID] || w.nationsBorder(n, o)) {
			threat = max(threat, w.ArmyPotential(o))
		}
	}
	return (threat + 1) / (w.ArmyPotential(n) + 1)
}

// random is a deterministic pseudo-random number in [0, 1) for the nation's
// choice among k techs, with the techs it knows now.
func (n *Nation) random(k int) float64 {
	known := 0
	for _, x := range n.Known {
		if x {
			known++
		}
	}
	x := uint64(n.seed)*0x9E3779B97F4A7C15 ^ uint64(n.ID+1)*0xBF58476D1CE4E5B9 ^ uint64(known)*0x94D049BB133111EB ^ uint64(k+1)*0xC2B2AE3D27D4EB4F
	x ^= x >> 31
	x *= 0x9E3779B97F4A7C15
	x ^= x >> 29
	return float64(x>>11) / float64(1<<53)
}
