package engine

import (
	"errors"
	"fmt"
	"math"
)

// Doctrine is how an army fights and what it consumes. Needs are per soldier
// per day; an army short of any of them fights worse.
type Doctrine struct {
	Name  string
	Tech  string // tech that unlocks it ("" = available from the start)
	Power float64
	Needs map[GoodID]float64
}

func needs(pairs ...any) map[GoodID]float64 {
	m := map[GoodID]float64{}
	for i := 0; i < len(pairs); i += 2 {
		m[G(pairs[i].(string))] = pairs[i+1].(float64)
	}
	return m
}

// Doctrines, from late-medieval levies to combined arms.
var Doctrines = []Doctrine{
	{"Levies", "", 0.7, needs("bread", 0.08, "clothes", 0.015, "arms", 0.016)},
	{"Line infantry", "Drill", 1.0, needs("bread", 0.08, "clothes", 0.02, "arms", 0.032, "artillery", 0.004)},
	{"Napoleonic corps", "Mass conscription", 1.5, needs("bread", 0.08, "clothes", 0.025, "arms", 0.048, "artillery", 0.012)},
	{"Rifle infantry", "Rifled muskets", 2.3, needs("bread", 0.09, "clothes", 0.025, "arms", 0.072, "artillery", 0.016)},
	{"Industrial army", "Breech-loading rifles", 3.5, needs("bread", 0.09, "clothes", 0.03, "arms", 0.08, "artillery", 0.024, "munitions", 0.04)},
	{"Machine-gun infantry", "Machine guns", 5.5, needs("bread", 0.1, "clothes", 0.03, "arms", 0.08, "artillery", 0.032, "munitions", 0.1)},
	{"Combined arms", "Tanks", 9.0, needs("bread", 0.1, "clothes", 0.03, "arms", 0.08, "artillery", 0.032, "munitions", 0.12,
		"vehicles", 0.016, "aircraft", 0.008, "oil", 0.048)},
}

// supplyWeight is how much each supplied good counts towards effectiveness:
// food matters most, then clothing; the rest share what's left.
func supplyWeight(d *Doctrine, g GoodID) float64 {
	switch Goods[g].Name {
	case "bread":
		return 0.35
	case "clothes":
		return 0.1
	}
	return 0.55 / float64(len(d.Needs)-2)
}

// Army is a body of soldiers on a tile.
type Army struct {
	ID       int
	Nation   *Nation
	Region   int     // where it stands
	Men      float64 // soldiers
	Target   float64 // soldiers it recruits up to (or demobilizes down to) in home tiles
	Doctrine int
	// Supply is, per good, the smoothed share of the army's daily needs it
	// could use (0..1).
	Supply []float64
	// Stock is the equipment and provisions the army carries, per good: it
	// uses them up day by day and the depot refills them, so a bad day at
	// market doesn't weaken it at once.
	Stock []float64
	Path  []int   // tiles still to march through
	March float64 // days marched towards Path[0]
	// Coming are recruits marching out to the army from home.
	Coming []Draft

	// this tick's supply order
	depot    *Region   // its nearest home tile, where recruits set out from
	homeDays float64   // days' march from there
	need     []float64 // what it uses today
	order    []float64 // what it buys today: today's use plus refilling its stock
	source   []*Region // where it buys each good today
	tau      []float64
	got      []float64
	transit  float64 // transport needed this tick
	gotT     float64
}

// Army and war rules.
const (
	recruitRate     = 0.002  // share of each tile's people that can be drafted per day, nation-wide
	demobilizeRate  = 0.01   // share of an army sent home per day when over its target
	baseAttrition   = 0.0002 // share of men lost per day even when well supplied
	hungerAttrition = 0.004  // extra share lost per day when food is entirely missing
	marchDaysPerHop = 8.0    // days to march one hop of cost 1 (roads and rails shorten it)
	battleRate      = 0.04   // men lost per day per unit of enemy strength
	defenderHome    = 1.2    // defending one's own tile
	retreatRatio    = 0.3    // on a battle's first day, a side weaker than this fraction of its enemy breaks
	occupationDays  = 180.0  // days an unopposed army needs to take unowned land (techs change the pace)
	ownedTileFactor = 2.0    // taking another nation's tile takes this many times longer
	minOccupiers    = 1.0    // an army needs at least this many men to occupy or fight
	stockDays       = 30.0   // days of supplies an army keeps in stock
	refillDays      = 10.0   // an empty stock is refilled over this many days (at most doubling the order)
	// Occupying is costly: over a full occupation an army loses about 40% of
	// its men taking another nation's tile and about 15% taking free land,
	// however fast it goes. siegeLoss is -ln(share kept).
	siegeLossOwned = 0.51
	siegeLossFree  = 0.16
	survivorsHome  = 0.5 // share of a destroyed army's men who find their way home
	// In battle an army loses this many times as large a share of its stock
	// as of its men (ammunition spent, guns and wagons abandoned); otherwise
	// the men lost take their own kit with them.
	battleKitLoss = 3.0
)

// Army errors.
var (
	ErrNoSuchArmy  = errors.New("no such army")
	ErrNotYours    = errors.New("that army belongs to another nation")
	ErrNoPath      = errors.New("the army can't get there")
	ErrMobilized   = errors.New("that exceeds how many people the nation can mobilize")
	ErrArmySize    = errors.New("that is more men than one army can field (techs raise it)")
	ErrTileTaken   = errors.New("another of the nation's armies already holds or is heading for that tile")
	ErrNoUpgrade   = errors.New("the nation knows no better doctrine")
	ErrAtWar       = errors.New("already at war")
	ErrAtPeace     = errors.New("not at war")
	ErrNotOwnTile  = errors.New("armies are raised and disbanded in the nation's own tiles")
	ErrNotBordered = errors.New("the nations don't border each other")
)

// Effectiveness is the army's fighting power per soldier: its doctrine's
// power, cut by whatever supplies are short.
func (a *Army) Effectiveness() float64 {
	d := &Doctrines[a.Doctrine]
	e := d.Power
	for _, g := range sortedGoods(d.Needs) {
		e *= math.Pow(math.Max(a.Supply[g], 0.05), supplyWeight(d, g))
	}
	return e
}

// Strength is the army's fighting power: men times effectiveness times its
// nation's attack bonus.
func (a *Army) Strength() float64 {
	return a.Men * a.Effectiveness() * a.Nation.attackMult()
}

// FullStrength is the strength the army would have at its full size, fully
// supplied.
func (a *Army) FullStrength() float64 {
	return math.Max(a.Men, a.Target) * Doctrines[a.Doctrine].Power * a.Nation.attackMult()
}

func (w *World) army(id int) (*Army, error) {
	for _, a := range w.Armies {
		if a.ID == id {
			return a, nil
		}
	}
	return nil, ErrNoSuchArmy
}

// ArmyByID finds an army.
func (w *World) ArmyByID(id int) *Army {
	a, _ := w.army(id)
	return a
}

// ArmiesOf lists a nation's armies.
func (w *World) ArmiesOf(n *Nation) []*Army {
	var out []*Army
	for _, a := range w.Armies {
		if a.Nation == n {
			out = append(out, a)
		}
	}
	return out
}

// Mobilized is how many soldiers a nation has, and its targets add up to.
func (w *World) Mobilized(n *Nation) (men, target float64) {
	for _, a := range w.ArmiesOf(n) {
		men += a.Men
		target += a.Target
	}
	return
}

// MobilizationCap is how many soldiers the nation may have.
func (w *World) MobilizationCap(n *Nation) float64 {
	return n.Mobilization() * (n.population() + func() float64 { m, _ := w.Mobilized(n); return m }())
}

// bestDoctrine is the most powerful doctrine a nation knows.
func (n *Nation) bestDoctrine() int {
	best := 0
	for i, d := range Doctrines {
		if n.Has(d.Tech) {
			best = i
		}
	}
	return best
}

// RaiseArmy forms a new army in one of the nation's tiles; it recruits up to
// target soldiers from the tile it stands in.
func (w *World) RaiseArmy(nation, region int, target float64) (*Army, error) {
	n, r := w.Nations[nation], w.Regions[region]
	if r.Nation != n {
		return nil, ErrNotOwnTile
	}
	if w.tileTaken(n, region, nil) {
		return nil, ErrTileTaken
	}
	if err := w.checkMobilization(n, nil, target); err != nil {
		return nil, err
	}
	w.NextArmyID++
	a := &Army{ID: w.NextArmyID, Nation: n, Region: region, Target: target,
		Doctrine: n.bestDoctrine(), Supply: make([]float64, NumGoods), Stock: make([]float64, NumGoods)}
	for g := range Doctrines[a.Doctrine].Needs {
		a.Supply[g] = 1
	}
	w.Armies = append(w.Armies, a)
	return a, nil
}

func (w *World) checkMobilization(n *Nation, except *Army, target float64) error {
	if !(target >= 0) {
		return ErrMobilized
	}
	total := target
	for _, a := range w.ArmiesOf(n) {
		if a != except {
			total += a.Target
		}
	}
	if total > w.MobilizationCap(n)+1e-9 {
		return ErrMobilized
	}
	if target > n.MaxArmySize()+1e-9 {
		return ErrArmySize
	}
	return nil
}

// SetArmyTarget changes how many soldiers an army recruits up to.
func (w *World) SetArmyTarget(id int, target float64) error {
	a, err := w.army(id)
	if err != nil {
		return err
	}
	if err := w.checkMobilization(a.Nation, a, target); err != nil {
		return err
	}
	a.Target = target
	return nil
}

// UpgradeArmy re-equips an army with the best doctrine its nation knows. New
// kit arrives gradually: supplies of goods it didn't use before start low.
func (w *World) UpgradeArmy(id int) error {
	a, err := w.army(id)
	if err != nil {
		return err
	}
	best := a.Nation.bestDoctrine()
	if best <= a.Doctrine {
		return ErrNoUpgrade
	}
	old := Doctrines[a.Doctrine].Needs
	a.Doctrine = best
	for g := range Doctrines[best].Needs {
		if _, had := old[g]; !had {
			a.Supply[g] = 0.2
		}
	}
	return nil
}

// DisbandArmy sends an army's soldiers home to the tile it stands in, which
// must be the nation's own.
func (w *World) DisbandArmy(id int) error {
	a, err := w.army(id)
	if err != nil {
		return err
	}
	r := w.Regions[a.Region]
	if r.Nation != a.Nation {
		return ErrNotOwnTile
	}
	addPeople(r, a.Men)
	w.removeArmy(a)
	return nil
}

func (w *World) removeArmy(a *Army) {
	for i, x := range w.Armies {
		if x == a {
			w.Armies = append(w.Armies[:i:i], w.Armies[i+1:]...)
			return
		}
	}
}

// passable reports whether a nation's armies may enter a tile: their own,
// unowned land, or the land of a nation they are at war with.
func (w *World) passable(n *Nation, r *Region) bool {
	return r.Nation == nil || r.Nation == n || n.AtWar[r.Nation.ID]
}

// neighbours lists (region, hop cost) for each link of a tile.
func (w *World) neighbours(id int) [][2]float64 {
	if w.adj == nil {
		w.adj = make([][][2]int, len(w.Regions))
		for i, l := range w.Links {
			w.adj[l.A] = append(w.adj[l.A], [2]int{l.B, i})
			w.adj[l.B] = append(w.adj[l.B], [2]int{l.A, i})
		}
	}
	var out [][2]float64
	for _, x := range w.adj[id] {
		out = append(out, [2]float64{float64(x[0]), w.Links[x[1]].Hop})
	}
	return out
}

// shortestPaths runs Dijkstra over land links from a tile, through tiles
// that ok allows (the start is always allowed). It returns the cost to reach
// each tile (+Inf if unreachable) and each tile's predecessor.
func (w *World) shortestPaths(from int, ok func(r *Region) bool) ([]float64, []int) {
	n := len(w.Regions)
	dist, prev, done := make([]float64, n), make([]int, n), make([]bool, n)
	for i := range dist {
		dist[i], prev[i] = math.Inf(1), -1
	}
	dist[from] = 0
	for {
		u := -1
		for i := range dist { // tiny graphs: a linear scan is fine and deterministic
			if !done[i] && !math.IsInf(dist[i], 1) && (u < 0 || dist[i] < dist[u]) {
				u = i
			}
		}
		if u < 0 {
			return dist, prev
		}
		done[u] = true
		for _, nb := range w.neighbours(u) {
			v := int(nb[0])
			if !done[v] && ok(w.Regions[v]) && dist[u]+nb[1] < dist[v] {
				dist[v], prev[v] = dist[u]+nb[1], u
			}
		}
	}
}

func pathTo(prev []int, from, to int) []int {
	var path []int
	for v := to; v != from; v = prev[v] {
		if v < 0 {
			return nil
		}
		path = append([]int{v}, path...)
	}
	return path
}

// MoveArmy orders an army to march to a tile along the cheapest route its
// nation may use. Arriving in enemy or unowned land starts a siege.
func (w *World) MoveArmy(id, dest int) error {
	a, err := w.army(id)
	if err != nil {
		return err
	}
	if dest < 0 || dest >= len(w.Regions) {
		return ErrNoPath
	}
	if dest == a.Region {
		a.Path, a.March = nil, 0
		return nil
	}
	if !w.passable(a.Nation, w.Regions[dest]) {
		return ErrNoPath
	}
	if w.tileTaken(a.Nation, dest, a) {
		return ErrTileTaken
	}
	dist, prev := w.shortestPaths(a.Region, func(r *Region) bool { return w.passable(a.Nation, r) })
	if math.IsInf(dist[dest], 1) {
		return ErrNoPath
	}
	a.Path, a.March = pathTo(prev, a.Region, dest), 0
	return nil
}

// tileTaken reports whether another of a nation's armies (not except)
// stands in a tile or is marching there: each tile holds one army per nation.
func (w *World) tileTaken(n *Nation, region int, except *Army) bool {
	for _, a := range w.Armies {
		if a == except || a.Nation != n {
			continue
		}
		if dest := a.Region; len(a.Path) > 0 {
			if a.Path[len(a.Path)-1] == region {
				return true
			}
		} else if dest == region {
			return true
		}
	}
	return false
}

// sendHome sets an army marching to the nearest tile of its own nation that
// none of its other armies holds, through any land. It returns false if
// there is none.
func (w *World) sendHome(a *Army) bool {
	if w.Regions[a.Region].Nation == a.Nation && !w.tileTaken(a.Nation, a.Region, a) {
		a.Path = nil
		return true
	}
	dist, prev := w.shortestPaths(a.Region, func(*Region) bool { return true })
	best := -1
	for _, r := range a.Nation.Regions {
		if w.tileTaken(a.Nation, r.ID, a) {
			continue
		}
		if best < 0 || dist[r.ID] < dist[best] {
			best = r.ID
		}
	}
	if best < 0 {
		return false
	}
	a.Path, a.March = pathTo(prev, a.Region, best), 0
	return true
}

// DeclareWar puts two nations at war. Only neighbours can fight.
func (w *World) DeclareWar(a, b int) error {
	na, nb := w.Nations[a], w.Nations[b]
	switch {
	case a == b:
		return ErrSelf
	case na.Eliminated() || nb.Eliminated():
		return ErrEliminated
	case na.AtWar[b]:
		return ErrAtWar
	case !w.nationsBorder(na, nb):
		return ErrNotBordered
	}
	na.AtWar[b], nb.AtWar[a] = true, true
	na.WarSince[b], nb.WarSince[a] = w.Tick, w.Tick
	na.PeaceOffer[b], nb.PeaceOffer[a] = false, false
	w.emit(-1, "war", -1, "%s declares war on %s", na.Name, nb.Name)
	return nil
}

// OfferPeace has nation a offer peace to b. Peace is made at once if b has
// offered it too, or if b is run by the computer and willing (see
// acceptsPeace); otherwise the offer stands until b answers. It reports
// whether peace was made.
func (w *World) OfferPeace(a, b int) (bool, error) {
	na, nb := w.Nations[a], w.Nations[b]
	if a == b || !na.AtWar[b] {
		return false, ErrAtPeace
	}
	if nb.PeaceOffer[a] || (nb.AI && w.acceptsPeace(nb, na)) {
		return true, w.makePeace(a, b)
	}
	na.PeaceOffer[b] = true
	return false, nil
}

// War weariness: a computer-run nation accepts peace after peaceYears of
// war, or after a year if it isn't clearly winning.
const peaceYears = 3

func (w *World) acceptsPeace(n, enemy *Nation) bool {
	years := float64(w.Tick-n.WarSince[enemy.ID]) / TicksPerYear
	if years >= peaceYears {
		return true
	}
	return years >= 1 && w.ArmyStrength(n) < 1.5*w.ArmyStrength(enemy)
}

// makePeace ends a war; borders stay where they are and armies go home.
func (w *World) makePeace(a, b int) error {
	na, nb := w.Nations[a], w.Nations[b]
	if a == b || !na.AtWar[b] {
		return ErrAtPeace
	}
	na.AtWar[b], nb.AtWar[a] = false, false
	na.WarSince[b], nb.WarSince[a] = w.Tick, w.Tick // now: when peace was made
	na.PeaceOffer[b], nb.PeaceOffer[a] = false, false
	for _, ar := range w.Armies {
		r := w.Regions[ar.Region]
		if (ar.Nation == na && r.Nation == nb) || (ar.Nation == nb && r.Nation == na) {
			w.sendHome(ar)
		}
	}
	for _, r := range w.Regions {
		if s := r.Siege; s != nil && ((s.Nation == na && r.Nation == nb) || (s.Nation == nb && r.Nation == na)) {
			r.Siege = nil
		}
	}
	w.emit(-1, "peace", -1, "%s and %s make peace", na.Name, nb.Name)
	return nil
}

// Siege is an occupation: a nation's army working to take a tile.
type Siege struct {
	Nation   *Nation
	Progress float64 // 0..1; the tile changes hands at 1
}

// planSupply works out what each army uses today (in peace only its
// nation's drill share of its wartime needs) and orders that plus a refill
// of its stock (up to stockDays of wartime needs). It buys each good where it
// lands cheapest, among the nation's tiles whose markets offered enough of it
// yesterday: arms come from the arms works, bread from the farms. The goods
// travel by the cheapest path, needing transport bought where they are
// loaded, in proportion to distance and bulk.
func (w *World) planSupply(n *Nation, add func(r *Region, c claim)) {
	drill := 1.0
	if !w.atWar(n) {
		drill = n.Drill
	}
	for _, a := range w.ArmiesOf(n) {
		if a.need == nil {
			a.need, a.order = make([]float64, NumGoods), make([]float64, NumGoods)
			a.tau, a.got = make([]float64, NumGoods), make([]float64, NumGoods)
			a.source = make([]*Region, NumGoods)
		}
		for g := range a.need {
			a.need[g], a.order[g], a.tau[g], a.got[g], a.source[g] = 0, 0, 0, 0, nil
		}
		for g, per := range Doctrines[a.Doctrine].Needs {
			a.need[g] = a.Men * per * drill
		}
		a.transit, a.gotT, a.depot = 0, 0, nil
		if len(n.Regions) == 0 {
			continue
		}
		dist, _ := w.shortestPaths(a.Region, func(*Region) bool { return true })
		for _, r := range n.Regions { // the nearest home tile, its base
			if a.depot == nil || dist[r.ID] < dist[a.depot.ID] {
				a.depot = r
			}
		}
		if math.IsInf(dist[a.depot.ID], 1) {
			a.depot = nil
			continue
		}
		a.homeDays = marchDaysPerHop * dist[a.depot.ID]
		perHop := w.Spec.TransportCost * n.supplyTransportMult()
		for _, g := range sortedGoods(Doctrines[a.Doctrine].Needs) {
			use, wartime := a.need[g], a.Men*Doctrines[a.Doctrine].Needs[g]
			if use <= 0 {
				continue
			}
			q := use + math.Min(wartime, math.Max(0, stockDays*wartime-a.Stock[g])/refillDays)
			src := w.supplySource(n, g, q, dist, perHop)
			a.source[g], a.order[g] = src, q
			a.tau[g] = perHop * dist[src.ID] * Goods[g].Bulk
			add(src, claim{kind: armyClaim, good: g, army: a, qty: q})
			if t := a.tau[g] * q; t > 0 {
				a.transit += t
				add(src, claim{kind: armyClaim, good: Transport, army: a, qty: t})
			}
		}
	}
}

// supplySource is the nation's tile where q of good g lands cheapest at an
// army dist away: price plus transport, among the tiles whose markets offered
// at least q yesterday; if none did, the tile that offered the most.
func (w *World) supplySource(n *Nation, g GoodID, q float64, dist []float64, perHop float64) *Region {
	var best, most *Region
	bestCost := math.Inf(1)
	for _, r := range n.Regions {
		if math.IsInf(dist[r.ID], 1) {
			continue
		}
		m := r.Markets[g]
		if most == nil || m.Supply > most.Markets[g].Supply {
			most = r
		}
		if m.Supply < q {
			continue
		}
		if c := m.Price + perHop*dist[r.ID]*Goods[g].Bulk*r.Markets[Transport].Price; c < bestCost {
			best, bestCost = r, c
		}
	}
	if best == nil {
		return most
	}
	return best
}

// settleSupply adds what the depot delivered to the army's stock (goods only
// arrive in proportion to the transport bought for them), then the army uses
// today's needs from its stock. A cut-off army lives off its stock.
func (a *Army) settleSupply() {
	carried := 0.0
	if a.depot != nil {
		carried = 1
		if a.transit > 0 {
			carried = math.Min(1, a.gotT/a.transit)
		}
	}
	for _, g := range sortedGoods(Doctrines[a.Doctrine].Needs) {
		need := a.Men * Doctrines[a.Doctrine].Needs[g]
		if a.need != nil {
			need = a.need[g] // drill share in peace
		}
		if a.got != nil {
			a.Stock[g] += a.got[g] * carried
		}
		if need <= 0 {
			continue
		}
		use := math.Min(a.Stock[g], need)
		a.Stock[g] -= use
		a.Supply[g] = 0.8*a.Supply[g] + 0.2*use/need
	}
}

// lose takes a share of the army's men, and kit times that share of its
// stock (at most all of it).
func (a *Army) lose(share, kit float64) {
	share = math.Max(0, math.Min(1, share))
	a.Men *= 1 - share
	keep := math.Max(0, 1-kit*share)
	for g := range a.Stock {
		a.Stock[g] *= keep
	}
}

// StockDays is how many days of a good the army carries at its current size.
func (a *Army) StockDays(g GoodID) float64 {
	need := a.Men * Doctrines[a.Doctrine].Needs[g]
	if need <= 0 {
		return 0
	}
	return a.Stock[g] / need
}

// armies runs a day of military life: recruitment, attrition, marching,
// battles and sieges.
func (w *World) armies() {
	w.recruitAll()
	for _, a := range append([]*Army(nil), w.Armies...) {
		w.demobilize(a)
		a.arrivals(w.Tick)
		hunger := 1 - a.Supply[G("bread")]
		loss := a.Men * (baseAttrition + hungerAttrition*hunger) * a.Nation.desertionMult()
		a.lose(loss/math.Max(a.Men, 1e-12), 1)
		w.march(a)
	}
	w.battles()
	w.sieges()
	for _, a := range append([]*Army(nil), w.Armies...) {
		switch {
		case a.Nation.Eliminated():
			w.removeArmy(a)
		case a.Men < 0.5 && w.Regions[a.Region].Nation != a.Nation:
			w.emit(a.Nation.ID, "army", a.Region, "%s's army %d has dwindled away", a.Nation.Name, a.ID)
			w.destroyArmy(a)
		}
	}
}

// destroyArmy takes a defeated army off the map: some of its men find their
// way home to the nation's nearest tile, the rest are lost.
func (w *World) destroyArmy(a *Army) {
	dist, _ := w.shortestPaths(a.Region, func(*Region) bool { return true })
	var home *Region
	for _, r := range a.Nation.Regions {
		if home == nil || dist[r.ID] < dist[home.ID] {
			home = r
		}
	}
	if home != nil {
		if n := survivorsHome*a.Men + a.coming(); n > 0 {
			addPeople(home, n) // recruits on the way turn back too
		}
	}
	a.Men, a.Coming = 0, nil
	w.removeArmy(a)
}

// Draft is a group of recruits on their way to an army.
type Draft struct {
	Men    float64
	Arrive int // tick
}

// coming is how many recruits are on their way to the army.
func (a *Army) coming() float64 {
	s := 0.0
	for _, d := range a.Coming {
		s += d.Men
	}
	return s
}

// arrivals adds the recruits that have reached the army.
func (a *Army) arrivals(tick int) {
	kept := a.Coming[:0]
	for _, d := range a.Coming {
		if d.Arrive <= tick {
			a.Men += d.Men
		} else {
			kept = append(kept, d)
		}
	}
	a.Coming = kept
}

// recruitAll drafts recruits for each nation's armies that are below their
// targets, from all its tiles: each tile gives up to recruitRate of its
// people a day (never leaving fewer than minPopulation workers), shared
// among the armies by how short they are. An army at home gets them at once;
// one in the field when they have marched out to it. A cut-off army gets none.
func (w *World) recruitAll() {
	for _, n := range w.Nations {
		armies := w.ArmiesOf(n)
		short, need := make([]float64, len(armies)), 0.0
		for i, a := range armies {
			if a.depot == nil && w.Regions[a.Region].Nation != n {
				continue // cut off
			}
			if s := a.Target - a.Men - a.coming(); s > 1e-9 {
				short[i] = s
				need += s
			}
		}
		if need <= 0 {
			continue
		}
		avail, pool := make([]float64, len(n.Regions)), 0.0
		for i, r := range n.Regions {
			avail[i] = math.Max(0, math.Min(recruitRate*r.Population(), sizeOf(movable(r))-minPopulation))
			pool += avail[i]
		}
		if pool <= 0 {
			continue
		}
		draft := math.Min(pool, need)
		for i, r := range n.Regions {
			if avail[i] > 0 {
				removePeople(r, draft*avail[i]/pool)
			}
		}
		for i, a := range armies {
			if short[i] <= 0 {
				continue
			}
			men := draft * short[i] / need
			if w.Regions[a.Region].Nation == n || a.homeDays < 1 {
				a.Men += men
			} else {
				a.Coming = append(a.Coming, Draft{men, w.Tick + int(math.Ceil(a.homeDays))})
			}
		}
	}
}

// demobilize sends men above an army's target home while it stands on its
// own soil.
func (w *World) demobilize(a *Army) {
	r := w.Regions[a.Region]
	if r.Nation != a.Nation || len(a.Path) > 0 || a.Men <= a.Target {
		return
	}
	n := math.Min(a.Men-a.Target, math.Max(demobilizeRate*a.Men, 0.5))
	addPeople(r, n)
	a.Men -= n
}

// march moves an army a day along its path. Roads and railways make hops
// cheaper and so marches faster.
func (w *World) march(a *Army) {
	if len(a.Path) == 0 {
		return
	}
	next := a.Path[0]
	if !w.passable(a.Nation, w.Regions[next]) {
		a.Path, a.March = nil, 0 // the way is closed (peace, or the tile changed hands)
		return
	}
	a.March++
	if a.March >= marchDaysPerHop*w.hopBetween(a.Region, next) {
		a.Region, a.Path, a.March = next, a.Path[1:], 0
	}
}

func (w *World) hopBetween(a, b int) float64 {
	for _, nb := range w.neighbours(a) {
		if int(nb[0]) == b {
			return nb[1]
		}
	}
	return 1
}

// battles: wherever armies of nations at war share a tile, each side loses
// men in proportion to the other's strength; a side that falls far behind is
// destroyed.
func (w *World) battles() {
	for _, r := range w.Regions {
		var here []*Army
		for _, a := range w.Armies {
			if a.Region == r.ID && a.Men >= minOccupiers {
				here = append(here, a)
			}
		}
		fought := false
		if len(here) >= 2 {
			for i, x := range w.Nations {
				for _, y := range w.Nations[i+1:] {
					if x.AtWar[y.ID] && w.fight(r, x, y, here, r.Battle+1) {
						fought = true
					}
				}
			}
		}
		if fought {
			r.Battle++
		} else {
			r.Battle = 0
		}
	}
}

// breakRatio is how far behind its enemy a side may fall before it breaks
// and is destroyed, on the day-th day of a battle: at first only a side
// badly outmatched breaks, but the longer the fight, the smaller the margin
// that decides it, so battles don't drag on.
func breakRatio(day int) float64 {
	return math.Min(0.95, retreatRatio+0.06*float64(day-1))
}

// fight is a day of battle between two nations' armies on a tile; it
// reports whether they fought.
func (w *World) fight(r *Region, x, y *Nation, here []*Army, day int) bool {
	side := func(n *Nation) ([]*Army, float64, float64) {
		var armies []*Army
		power, men := 0.0, 0.0
		for _, a := range here {
			if a.Nation == n && a.Men >= minOccupiers {
				armies = append(armies, a)
				power += a.Strength()
				men += a.Men
			}
		}
		if r.Nation == n {
			power *= defenderHome * n.defenseMult()
		}
		return armies, power, men
	}
	ax, px, mx := side(x)
	ay, py, my := side(y)
	if len(ax) == 0 || len(ay) == 0 {
		return false
	}
	lose := func(armies []*Army, men, losses float64) {
		if men <= 0 {
			return
		}
		for _, a := range armies {
			a.lose(losses/men, battleKitLoss)
		}
	}
	lose(ax, mx, math.Min(mx, battleRate*py))
	lose(ay, my, math.Min(my, battleRate*px))
	defeat := func(armies []*Army, n, enemy *Nation) {
		for _, a := range armies {
			w.destroyArmy(a)
		}
		w.emit(-1, "battle", r.ID, "%s destroys %s's army at tile %s", enemy.Name, n.Name, r.Name)
	}
	br := breakRatio(day)
	switch {
	case px < br*py:
		defeat(ax, x, y)
	case py < br*px:
		defeat(ay, y, x)
	}
	return true
}

// sieges advances occupations. While a nation's army stands on a tile it may
// occupy (unowned land, or an enemy's) and no army of the owner is there,
// occupation rises by a fixed amount each day; otherwise it falls back by
// the same amount until it is gone. At 100% the tile changes hands. Another
// nation can only start occupying once the previous occupation has fallen
// back to nothing. Size doesn't matter: holding the ground does. While a
// battle is fought on the tile, occupation stands still.
func (w *World) sieges() {
	for _, r := range w.Regions {
		if r.Battle > 0 {
			continue // no ground is gained or given up while the armies fight
		}
		var occupier *Nation
		biggest, defended := 0.0, false
		for _, a := range w.Armies {
			if a.Region != r.ID || len(a.Path) > 0 || a.Men < minOccupiers {
				continue
			}
			if a.Nation == r.Nation {
				defended = true
			} else if w.passable(a.Nation, r) && a.Men > biggest {
				occupier, biggest = a.Nation, a.Men
			}
		}
		holding := occupier != nil && !defended
		if s := r.Siege; s != nil && !(holding && occupier == s.Nation) {
			s.Progress -= 1 / occupationDays
			if s.Progress <= 0 {
				r.Siege = nil
			}
			continue
		}
		if !holding {
			continue
		}
		if r.Siege == nil {
			r.Siege = &Siege{Nation: occupier}
			if r.Nation != nil {
				w.emit(r.Nation.ID, "siege", r.ID, "%s is occupying tile %s", occupier.Name, r.Name)
			}
		}
		rate := w.siegeRate(r, occupier)
		loss := math.Exp(-siegeLoss(r) * rate)
		for _, a := range w.Armies {
			if a.Region == r.ID && a.Nation == occupier && len(a.Path) == 0 {
				a.lose(1-loss, 1)
			}
		}
		r.Siege.Progress = math.Min(1, r.Siege.Progress+rate)
		if r.Siege.Progress >= 1-1e-9 {
			old := r.Nation
			r.Siege = nil
			w.transfer(r, occupier)
			if old != nil {
				w.emit(old.ID, "lost", r.ID, "%s takes tile %s from %s", occupier.Name, r.Name, old.Name)
			}
			w.emit(occupier.ID, "taken", r.ID, "%s takes tile %s", occupier.Name, r.Name)
		}
	}
}

// siegeRate is how much of a tile an occupier takes per day.
func (w *World) siegeRate(r *Region, occupier *Nation) float64 {
	rate := occupier.occupyMult() / occupationDays
	if r.Nation != nil {
		rate /= ownedTileFactor * r.Nation.defenseMult()
	}
	return rate
}

// siegeLoss is how hard occupying the tile is on the occupier's men.
func siegeLoss(r *Region) float64 {
	if r.Nation != nil {
		return siegeLossOwned
	}
	return siegeLossFree
}

// removePeople drafts n people from a tile, taking from every pop in
// proportion to its size; their cash stays with those who remain.
func removePeople(r *Region, n float64) {
	pops := movable(r)
	pop := sizeOf(pops)
	for _, p := range pops {
		p.Size -= n * p.Size / pop
	}
}

// addPeople adds n people (with no cash) to a tile.
func addPeople(r *Region, n float64) {
	pops := movable(r)
	pop := sizeOf(pops)
	for _, p := range pops {
		p.Size += n * p.Size / pop
	}
}

func (a *Army) String() string {
	return fmt.Sprintf("%s army %d (%.0f men, %s) at %d", a.Nation.Name, a.ID, a.Men, Doctrines[a.Doctrine].Name, a.Region)
}

// Modifier is one factor in an army's strength.
type Modifier struct {
	Name  string  `json:"name"`
	Value float64 `json:"value"` // a multiplier (or, for the first entry, the doctrine's base power)
	Note  string  `json:"note"`
	Add   bool    `json:"add"` // added, not multiplied (losses)
}

// Breakdown explains an army's strength per soldier: the doctrine's power,
// each supplied good's effect, and its nation's technology; then what applies
// when it defends its own tile, and its daily losses.
func (w *World) Breakdown(a *Army) (attack []Modifier, defense []Modifier, attrition []Modifier) {
	d := &Doctrines[a.Doctrine]
	attack = append(attack, Modifier{d.Name, d.Power, "doctrine's base power per soldier", false})
	for _, g := range sortedGoods(d.Needs) {
		s := math.Max(a.Supply[g], 0.05)
		wt := supplyWeight(d, g)
		attack = append(attack, Modifier{Goods[g].Name + " supply", math.Pow(s, wt),
			fmt.Sprintf("%.0f%% supplied, weight %.2f", 100*a.Supply[g], wt), false})
	}
	for i, t := range Techs {
		if a.Nation.Known[i] && t.Attack != 0 {
			attack = append(attack, Modifier{t.Name, t.Attack, "technology", false})
		}
	}
	defense = append(defense, Modifier{"Home ground", defenderHome, "fighting in its own tile", false})
	for i, t := range Techs {
		if a.Nation.Known[i] && t.Defense != 0 {
			defense = append(defense, Modifier{t.Name, t.Defense, "technology", false})
		}
	}
	hunger := 1 - a.Supply[G("bread")]
	attrition = append(attrition, Modifier{"Base", baseAttrition, "share of men lost a day", true})
	attrition = append(attrition, Modifier{"Hunger", hungerAttrition * hunger, fmt.Sprintf("bread %.0f%% supplied", 100*a.Supply[G("bread")]), true})
	for i, t := range Techs {
		if a.Nation.Known[i] && t.Desertion != 0 {
			attrition = append(attrition, Modifier{t.Name, t.Desertion, "multiplies the losses above", false})
		}
	}
	if s := w.sieging(a); s > 0 {
		attrition = append(attrition, Modifier{"Occupying", 1 - math.Exp(-s), "a full occupation costs about 40% of the army (15% on free land)", true})
	}
	return
}

// sieging is the occupation loss rate (-ln share kept per day) an army
// suffers today, or 0.
func (w *World) sieging(a *Army) float64 {
	r := w.Regions[a.Region]
	if r.Siege == nil || r.Siege.Nation != a.Nation || len(a.Path) > 0 {
		return 0
	}
	return siegeLoss(r) * w.siegeRate(r, a.Nation)
}

// DailyLoss is the share of its men the army loses today outside battle.
func (w *World) DailyLoss(a *Army) float64 {
	hunger := 1 - a.Supply[G("bread")]
	base := (baseAttrition + hungerAttrition*hunger) * a.Nation.desertionMult()
	return 1 - (1-base)*math.Exp(-w.sieging(a))
}

// SupplyLine describes where an army's supplies come from: the depot tile,
// how far away it is, and the transport that today's supplies needed.
func (a *Army) SupplyLine() (depot int, transport float64) {
	if a.depot == nil {
		return -1, 0
	}
	return a.depot.ID, a.transit
}

// Reinforcements are the recruits marching out to the army, and the days
// until the first of them arrive.
func (a *Army) Reinforcements(tick int) (men float64, days int) {
	days = -1
	for _, d := range a.Coming {
		men += d.Men
		if days < 0 || d.Arrive-tick < days {
			days = d.Arrive - tick
		}
	}
	return
}

// Source is the tile the army bought a good in today, or -1.
func (a *Army) Source(g GoodID) int {
	if a.source == nil || a.source[g] == nil {
		return -1
	}
	return a.source[g].ID
}
