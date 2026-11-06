package engine

import (
	"math"
	"testing"
)

func TestTilesHaveOwnerClasses(t *testing.T) {
	w := newTestGame(t)
	r := regionByName(w, "22")
	var size [NumClasses]float64
	for _, p := range r.Pops {
		size[p.Class] += p.Size
	}
	pop := r.Population()
	if math.Abs(size[Owners]-ownerPeopleShare*pop) > 1e-9 {
		t.Errorf("class sizes %v of %v", size, pop)
	}
	classic := newTestWorld(t, 1)
	for _, p := range classic.Regions[0].Pops {
		if p.Class != Workers {
			t.Error("the classic world has owner classes")
		}
	}
}

// Workers work; owners live off dividends from land, works and trade.
func TestOwnersLiveOffDividends(t *testing.T) {
	w := newQuietGame(t)
	for i := 0; i < 200; i++ {
		w.Step()
	}
	var income, size [NumClasses]float64
	for _, r := range w.Regions {
		for _, p := range r.Pops {
			if p.Class != Workers && len(p.Jobs) > 0 {
				t.Fatalf("tile %s: %v have jobs", r.Name, p.Class)
			}
			income[p.Class] += p.Income
			size[p.Class] += p.Size
		}
	}
	perHead := func(c Class) float64 { return income[c] / size[c] }
	t.Logf("income per head: workers %.2f, owners %.2f", perHead(Workers), perHead(Owners))
	if perHead(Owners) <= perHead(Workers) {
		t.Error("owners should earn more per head than workers")
	}
}

func TestOnlyWorkersAreDraftedAndMove(t *testing.T) {
	w := newQuietGame(t)
	r := regionByName(w, "11")
	owners := func(r *Region) float64 {
		return sizeOf(r.ClassPops(Owners))
	}
	before := owners(r)
	removePeople(r, 10)
	addPeople(r, 3)
	movePeople(r, regionByName(w, "12"), 5)
	if math.Abs(owners(r)-before) > 1e-12 {
		t.Errorf("owners changed from %v to %v", before, owners(r))
	}
}
