package game

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"testing"
)

func TestZZFingerprint(t *testing.T) {
	w, _ := baseGameWorld(DefaultGame)
	w.Migration, w.Investment, w.Demography = true, true, true
	h := sha256.New()
	put := func(x float64) { var b [8]byte; binary.LittleEndian.PutUint64(b[:], math.Float64bits(x)); h.Write(b[:]) }
	for d := 0; d < 2*365; d++ {
		w.Step()
	}
	for _, r := range w.Regions {
		put(r.Wage)
		put(r.Population())
		for _, m := range r.Markets {
			put(m.Price)
		}
		for _, f := range r.Firms {
			put(f.Cash)
			put(float64(f.Levels))
		}
	}
	for _, tr := range w.Traders {
		put(tr.Q)
	}
	fmt.Printf("FINGERPRINT %x\n", h.Sum(nil))
}
