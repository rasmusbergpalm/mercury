package engine

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
// k-th decision this tick: the AI varies its choices a little without
// breaking the simulation's determinism (saved games continue exactly).
func (w *World) noise(n *Nation, k int) float64 {
	seed := int64(0)
	if w.Spec != nil {
		seed = w.Spec.Seed
	}
	x := uint64(seed)*0x9E3779B97F4A7C15 ^ uint64(n.ID+1)*0xBF58476D1CE4E5B9 ^ uint64(w.Tick)*0x94D049BB133111EB ^ uint64(k+1)*0xD6E8FEB86659FD93
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
// nation and the day. -1 if nothing is left.
func (n *Nation) drawResearch() int {
	var ids []int
	cheapest := math.Inf(1)
	for i := range Techs {
		if n.CanResearch(i) {
			ids = append(ids, i)
			cheapest = math.Min(cheapest, Techs[i].Cost)
		}
	}
	if len(ids) == 0 {
		return -1
	}
	t := n.Temper
	lean := map[string]float64{BranchScience: t.Learning, BranchMilitary: t.Militarism, BranchPolitics: t.Aggression, BranchTransport: t.Expansion}
	weights, total := make([]float64, len(ids)), 0.0
	for k, i := range ids {
		w := math.Pow(cheapest/Techs[i].Cost, 2)
		if l, ok := lean[Techs[i].Branch]; ok && l > 0 {
			w *= l * l
		}
		weights[k] = w
		total += w
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

// random is a deterministic pseudo-random number in [0, 1) for the nation's
// k-th choice today.
func (n *Nation) random(k int) float64 {
	x := uint64(n.seed)*0x9E3779B97F4A7C15 ^ uint64(n.ID+1)*0xBF58476D1CE4E5B9 ^ uint64(n.now)*0x94D049BB133111EB ^ uint64(k+1)*0xC2B2AE3D27D4EB4F
	x ^= x >> 31
	x *= 0x9E3779B97F4A7C15
	x ^= x >> 29
	return float64(x>>11) / float64(1<<53)
}
