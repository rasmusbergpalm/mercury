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
	Militarism float64 // military spending and men kept under arms
	Learning   float64 // science spending
	Aggression float64 // how readily it starts wars and seeks battle
	Expansion  float64 // how hard it pushes into free land
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

// budget leans a base budget towards the nation's temperament (and a little
// noise), keeping the total share spent.
func (t Temperament) budget(base [NumBudgets]float64, noise func(k int) float64) [NumBudgets]float64 {
	total := 0.0
	for _, x := range base {
		total += x
	}
	b := base
	b[Science] *= t.Learning
	b[Defense] *= t.Militarism
	sum := 0.0
	for i := range b {
		b[i] *= 1 + 0.1*noise(10+i)
		sum += b[i]
	}
	for i := range b {
		b[i] *= total / sum
	}
	return b
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
