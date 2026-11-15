package engine

import "testing"

// siegeScene sets up Alpha (the computer) besieging the free tile next to
// its home tile, with progress ours, while an army of Beta (at war with it)
// occupies that home tile, with progress theirs. ownMen and enemyMen are the
// two armies' sizes, both fully supplied.
func siegeScene(t *testing.T, ours, theirs, ownMen, enemyMen float64) (w *World, alpha *Nation, a *Army, home, free *Region) {
	t.Helper()
	w = newNeighbourGame(t)
	alpha, beta := w.Nations[0], w.Nations[1]
	alpha.AI = true
	home = alpha.Regions[0]
	for _, nb := range HexNeighbors(home.Row, home.Col, w.Rows, w.Cols) {
		if r := w.Regions[nb[0]*w.Cols+nb[1]]; r.Nation == nil && w.reachable(alpha, r) {
			free = r
			break
		}
	}
	if free == nil {
		t.Fatal("no free tile next to Alpha")
	}
	w.DeclareWar(beta.ID, alpha.ID)
	a = sceneArmy(t, w, alpha, home, ownMen)
	a.Region = free.ID
	free.Siege = &Siege{Nation: alpha, Progress: ours}
	if enemyMen > 0 {
		e := sceneArmy(t, w, beta, beta.Regions[0], enemyMen)
		e.Region = home.ID
		home.Siege = &Siege{Nation: beta, Progress: theirs}
	}
	return
}

func sceneArmy(t *testing.T, w *World, n *Nation, at *Region, men float64) *Army {
	t.Helper()
	a, err := w.RaiseArmy(n.ID, at.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	a.Target, a.Men = men, men
	for g := range a.Supply {
		a.Supply[g] = 1
	}
	return a
}

// 1) and 2): a besieger breaks off its siege to defend its home tile if and
// only if the enemy would take that tile before it takes its own and it can
// likely beat the enemy there; a siege that finishes first is kept.
func TestBesiegerBreaksOffIffBehindAndStronger(t *testing.T) {
	for _, tc := range []struct {
		name          string
		ours, theirs  float64 // progress: our siege abroad, theirs of our home tile
		ownMen, enemy float64
		breaks        bool
	}{
		{"ahead and stronger: keeps its siege", 0.9, 0.1, 10, 5, false},
		{"ahead and weaker: keeps its siege", 0.9, 0.1, 3, 20, false},
		{"behind and stronger: defends", 0.1, 0.9, 10, 5, true},
		{"behind and weaker: keeps its siege", 0.1, 0.9, 3, 20, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w, alpha, a, home, free := siegeScene(t, tc.ours, tc.theirs, tc.ownMen, tc.enemy)
			ourLeft := (1 - tc.ours) / w.siegeRate(free, alpha)
			theirLeft := (1 - tc.theirs) / w.siegeRate(home, w.Nations[1])
			if behind := theirLeft < ourLeft; behind != (tc.theirs > tc.ours) {
				t.Fatalf("set-up: days left %v ours, %v theirs", ourLeft, theirLeft)
			}
			w.aiPlan(alpha, []*Army{a}, w.expansionTargets(alpha))
			if broke := aiGoing(a) == home.ID; broke != tc.breaks {
				t.Errorf("marches home to defend: %v, want %v (path %v)", broke, tc.breaks, a.Path)
			}
			if !tc.breaks && (a.Region != free.ID || len(a.Path) > 0) {
				t.Errorf("left its siege for %v", a.Path)
			}
		})
	}
}

// 3) An idle army doesn't seek a battle it doesn't expect to win: not with
// an enemy army far stronger on enemy land, nor on its own soil.
func TestNoHopelessBattle(t *testing.T) {
	w := newNeighbourGame(t)
	alpha, beta := w.Nations[0], w.Nations[1]
	alpha.AI = true
	w.DeclareWar(beta.ID, alpha.ID)
	a := sceneArmy(t, w, alpha, alpha.Regions[0], 3)
	e := sceneArmy(t, w, beta, beta.Regions[0], 30) // on Beta's own tile, next door
	w.aiPlan(alpha, []*Army{a}, w.expansionTargets(alpha))
	if aiGoing(a) == e.Region {
		t.Errorf("marches on an army ten times its size at %s", w.Regions[e.Region].Name)
	}
	// on its own soil: a second tile of Alpha's, the enemy occupying it
	near := w.Regions[0]
	for _, nb := range HexNeighbors(alpha.Regions[0].Row, alpha.Regions[0].Col, w.Rows, w.Cols) {
		if r := w.Regions[nb[0]*w.Cols+nb[1]]; r.Nation == nil {
			near = r
			break
		}
	}
	w.Annex(near.ID, alpha.ID)
	e.Region = near.ID
	near.Siege = &Siege{Nation: beta, Progress: 0.5}
	a.Path = nil
	w.aiPlan(alpha, []*Army{a}, w.expansionTargets(alpha))
	if aiGoing(a) == near.ID {
		t.Errorf("throws itself at an army ten times its size on its own tile %s", near.Name)
	}
}

// 4) Absent anything else to do, an army that has filled up goes out to
// occupy free land.
func TestIdleArmiesStartSieges(t *testing.T) {
	w := newNeighbourGame(t)
	alpha := w.Nations[0]
	alpha.AI = true
	a := sceneArmy(t, w, alpha, alpha.Regions[0], 10)
	w.aiPlan(alpha, []*Army{a}, w.expansionTargets(alpha))
	dest := w.Regions[aiGoing(a)]
	if dest.Nation != nil || len(a.Path) == 0 {
		t.Fatalf("stays at %s instead of making for free land", dest.Name)
	}
	for i := 0; i < 60 && len(a.Path) > 0; i++ {
		w.Step()
	}
	if r := w.Regions[a.Region]; r.Siege == nil || r.Siege.Nation != alpha {
		t.Errorf("arrived at %s but isn't occupying it", r.Name)
	}
}

// freeNeighbour is a free tile next to r.
func freeNeighbour(t *testing.T, w *World, r *Region) *Region {
	t.Helper()
	for _, nb := range HexNeighbors(r.Row, r.Col, w.Rows, w.Cols) {
		if x := w.Regions[nb[0]*w.Cols+nb[1]]; x.Nation == nil {
			return x
		}
	}
	t.Fatal("no free tile next to", r.Name)
	return nil
}

// Regression: an army whose way is closed mid-march (the tile ahead changed
// hands) stopped where it was, even on a tile another of its armies held:
// two armies of one nation on one tile.
func TestClosedMarchDoesntDoubleUp(t *testing.T) {
	w := newNeighbourGame(t)
	alpha, beta := w.Nations[0], w.Nations[1]
	h2 := freeNeighbour(t, w, alpha.Regions[0])
	w.Annex(h2.ID, alpha.ID)
	x := freeNeighbour(t, w, h2)
	sceneArmy(t, w, alpha, h2, 10) // holds h2
	b := sceneArmy(t, w, alpha, alpha.Regions[0], 10)
	b.Region, b.Path = h2.ID, []int{x.ID} // passing through h2 on its way to x
	w.Annex(x.ID, beta.ID)                // x is Beta's now, at peace: closed
	w.march(b)
	if b.Region == h2.ID && len(b.Path) == 0 {
		t.Errorf("halted on %s, which another of its armies holds", h2.Name)
	}
}

// Regression: an army with nothing to do in another nation's land stayed
// there (the small bonus for keeping its plan made standing still worth
// something); it goes home.
func TestIdleArmyAbroadGoesHome(t *testing.T) {
	w := newNeighbourGame(t)
	alpha, beta := w.Nations[0], w.Nations[1]
	alpha.AI = true
	a := sceneArmy(t, w, alpha, alpha.Regions[0], 10)
	x := freeNeighbour(t, w, alpha.Regions[0])
	a.Region = x.ID        // took up a free tile ...
	w.Annex(x.ID, beta.ID) // ... that Beta settled, at peace
	w.aiPlan(alpha, []*Army{a}, nil)
	if aiGoing(a) == x.ID {
		t.Errorf("stays in Beta's tile %s with nothing to do", x.Name)
	}
}

// Armies at peace don't share a tile: a free tile another nation's army
// holds, or is marching to, is taken; neither the player nor the computer
// can send an army there.
func TestNoTwoNationsOnAFreeTile(t *testing.T) {
	w := newNeighbourGame(t)
	alpha, beta := w.Nations[0], w.Nations[1]
	alpha.AI = true
	x := freeNeighbour(t, w, alpha.Regions[0])
	if !w.borders(beta, x) {
		t.Skip("set-up: the free tile doesn't border Beta")
	}
	e := sceneArmy(t, w, beta, beta.Regions[0], 10)
	if err := w.MoveArmy(e.ID, x.ID); err != nil {
		t.Fatal(err)
	}
	a := sceneArmy(t, w, alpha, alpha.Regions[0], 30)
	if err := w.MoveArmy(a.ID, x.ID); err == nil {
		t.Errorf("sent to %s, which Beta's army is marching to", x.Name)
	}
	a.Path = nil
	w.aiPlan(alpha, []*Army{a}, w.expansionTargets(alpha))
	if aiGoing(a) == x.ID {
		t.Errorf("the computer sends its army to %s too", x.Name)
	}
}
