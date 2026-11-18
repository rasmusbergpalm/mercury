package game

import (
	"math"

	"github.com/rasmusbergpalm/mercury/economy"
)

// World is a game: the economy (economy.World) of a map whose tiles nations
// own, tax, defend and fight over.
type World struct {
	*economy.World
	Regions []*Region // the economy's tiles, with what the game knows of them
	Nations []*Nation
	burning bool // in the burn-in before a game starts (settle.go)
	// Spec is the scenario a game world was built from (nil for a bare map).
	Spec *GameSpec
	// Ports lists tiles with ports in the order they opened (which fixes the
	// order of sea traders).
	Ports []int

	Armies     []*Army
	NextArmyID int
	Events     []Event
	History    History // game worlds only

	// shortest paths through any land, by start (see allPaths); cleared
	// when a hop changes
	paths map[int]cachedPaths
}

// Region is a tile of the game: the economy's tile, and who owns it, its
// roads and port, its loyalties and the war on it.
type Region struct {
	*economy.Region
	w       *World
	Nation  *Nation  // owner, or nil
	Infra   int      // index into InfraLevels
	Port    bool     // has a port: trades by sea with every other port
	Project *Project // infrastructure under construction, or nil
	Neglect float64  // accumulated unmet upkeep
	Siege   *Siege   // a foreign army working to take the tile, or nil
	Collect float64  // share of its taxes the administration collects
	// Loyalties is how the tile's people's allegiance is split: one share
	// per nation, and a last one for nobody; they add up to 1 (nil outside
	// games with nations). See Allegiance.
	Loyalties []float64
	Battle    int // days the current battle on the tile has lasted (0: none)

	claims    []claim        // what the nation buys here this tick
	buyer     *economy.Buyer // the nation, buying in the tile's markets
	exportsTo []float64      // value shipped to each other nation today (see trackTrade)
}

// wrap makes a game of an economy: a game tile for each of its tiles, with
// a buyer in its markets for whoever owns it.
func wrap(e *economy.World) *World {
	w := &World{World: e}
	for _, r := range e.Regions {
		g := &Region{Region: r, w: w, Collect: 1, buyer: economy.NewBuyer()}
		r.Buyers = append(r.Buyers, g.buyer)
		w.Regions = append(w.Regions, g)
	}
	return w
}

// setNation hands the tile to a nation (nil: free land), which taxes its
// people's incomes.
func (r *Region) setNation(n *Nation) {
	r.Nation = n
	if n == nil {
		r.TaxFn = nil
	} else {
		r.TaxFn = r.incomeTax
	}
}

// incomeTax is what the tile's administration collects of an income, from
// people who pay.
func (r *Region) incomeTax(amount float64) float64 {
	return amount * r.Nation.TaxRate * r.Collect * r.AllegianceMult()
}

// setInfra sets the tile's infrastructure level, and what it does to moving
// over land.
func (r *Region) setInfra(level int) {
	r.Infra = level
	r.RoadMult = InfraLevels[level].MoveMult
}

// Step plays a day: the economy's, with the nations' part in it. The
// computer plays first; the nations then set the economy's levers (their
// technology, tariffs and policies), buy in the markets once merchants
// have planned, take their taxes and duties when the markets and the
// firms are done, and settle their treasuries, services and wars.
func (w *World) Step() {
	w.runAI()
	w.setLevers()
	e := w.World
	e.BeginDay()
	e.PlanTrade()
	w.planDay()
	e.Markets()
	w.collect()
	e.Ship()
	w.afterShipping()
	e.Produce()
	w.collect()
	w.settleDay()
	w.setPolicies() // (what techs learned today do to today's growth)
	e.EndDay()
	if w.Spec != nil && w.Tick%HistoryEvery == 0 {
		w.record()
	}
}

// setLevers sets the economy's levers from the nations: the technologies
// known, each route's tariff and embargo, and each tile's policies.
func (w *World) setLevers() {
	if w.Known == nil {
		w.Known = map[string]bool{}
	}
	for i := range Techs {
		known := false
		for _, n := range w.Nations {
			if n.Known[i] {
				known = true
				break
			}
		}
		w.Known[Techs[i].Name] = known
	}
	for _, t := range w.Traders {
		t.Tariff, t.Closed = w.duty(t)
	}
	w.setPolicies()
}

// duty is the tariff on a route, set by the nation it enters: none from its
// own tiles, and an embargo on trade with its enemies.
func (w *World) duty(t *Trader) (float64, bool) {
	on, dn := w.Regions[t.Origin.ID].Nation, w.Regions[t.Dest.ID].Nation
	if dn == nil || dn == on {
		return 0, false
	}
	if on != nil && on.closedTo(dn) {
		return 0, true // embargoed, at war, or allied against: no trade with the enemy
	}
	return dn.Tariff[t.Good], false
}

// setPolicies sets what each tile's owner's technology does to its economy:
// the payback owners ask, its people's growth, and the cost and losses of
// shipping.
func (w *World) setPolicies() {
	type policy struct{ payback, growth, coast, spoilage float64 }
	of := make([]policy, len(w.Nations))
	for i, n := range w.Nations {
		of[i] = policy{n.paybackMult(), n.popGrowthBonus(), n.coastMult(), n.spoilageMult()}
	}
	for _, r := range w.Regions {
		p := policy{1, 0, 1, 1}
		if r.Nation != nil {
			p = of[r.Nation.ID]
		}
		r.PaybackMult, r.GrowthBonus, r.CoastMult, r.SpoilageMult = p.payback, p.growth, p.coast, p.spoilage
	}
}

// planDay: the universities' scholars, and the nations' purchases.
func (w *World) planDay() {
	for _, r := range w.Regions {
		if r.Urban {
			r.scholars()
		}
		if r.Nation == nil {
			for g := range r.buyer.Bid {
				r.buyer.Bid[g] = 0
			}
		}
	}
	for _, n := range w.Nations {
		n.plan(w)
	}
}

// collect pays each tile's taxes, duties and the nation's purchases there
// into its nation's treasury.
func (w *World) collect() {
	for _, r := range w.Regions {
		if n := r.Nation; n != nil {
			n.Treasury += r.Withheld + r.buyer.Account + r.Duties
			n.Revenue += r.Withheld
			n.Spending -= r.buyer.Account
			n.TariffRevenue += r.Duties
		}
		r.Withheld, r.buyer.Account, r.Duties = 0, 0, 0
	}
}

// afterShipping notes the trade between nations.
func (w *World) afterShipping() {
	for _, r := range w.Regions {
		r.sumExports(len(w.Nations))
	}
	w.trackTrade()
}

// settleDay settles the nations' day: their treasuries, research and
// projects, their services, new ports, and their armies.
func (w *World) settleDay() {
	changed := false
	for _, n := range w.Nations {
		changed = n.settle(w) || changed
	}
	if w.Spec != nil {
		w.serviceEffects()
	}
	w.openNewPorts()
	if changed {
		w.refreshAllHops()
	}
	if len(w.Nations) > 0 {
		w.armies()
	}
}

// Money is all cash in the world, the treasuries' with the rest; it is
// conserved.
func (w *World) Money() float64 {
	s := w.World.Money()
	for _, n := range w.Nations {
		s += n.Treasury
	}
	return s
}

// sumExports sums what the tile shipped today to every other nation (for
// trackTrade).
func (r *Region) sumExports(nations int) {
	if len(r.exportsTo) != nations {
		r.exportsTo = make([]float64, nations)
	}
	for j := range r.exportsTo {
		r.exportsTo[j] = 0
	}
	on := r.Nation
	for _, t := range r.House.Today() {
		if dn := r.w.Regions[t.Dest.ID].Nation; on != nil && dn != nil && on != dn && t.Shipped > 0 {
			r.exportsTo[dn.ID] += t.Shipped * r.Markets[t.Good].Price
		}
	}
}

func (w *World) refreshAllHops() {
	w.paths = nil
	w.setPolicies()
	w.World.RefreshHops()
	for _, t := range w.Traders {
		if t.FixedDistance {
			t.SetDistance(w.seaHop(t.Origin, t.Dest))
		}
	}
}

// refreshHops recomputes the hop cost of every link and route touching r.
func (w *World) refreshHops(r *Region) {
	w.paths = nil
	w.setPolicies()
	w.World.RefreshHopsAt(r.Region)
}

// seed is the game's seed (0 outside game worlds).
func (w *World) seed() int64 {
	if w.Spec == nil {
		return 0
	}
	return w.Spec.Seed
}

// Soldiers is how many people serve in armies.
func (w *World) Soldiers() float64 {
	s := 0.0
	for _, a := range w.Armies {
		s += a.Men
	}
	return s
}

// trackTrade updates each nation's moving average of trade with every other.
func (w *World) trackTrade() {
	if len(w.Nations) == 0 {
		return
	}
	k := len(w.Nations)
	flow := make([]float64, k*k)
	for _, r := range w.Regions { // each tile's exports, summed as it shipped them
		on := r.Nation
		for j, v := range r.exportsTo {
			if on != nil && v > 0 {
				flow[on.ID*k+j] += v
				flow[j*k+on.ID] += v
			}
		}
	}
	for _, n := range w.Nations {
		for j := range n.TradeWith {
			n.TradeWith[j] = 0.98*n.TradeWith[j] + 0.02*flow[n.ID*k+j]
		}
	}
}

// Build starts the next infrastructure level on a tile, paid for by its nation.
func (w *World) Build(region int) error {
	r := w.Regions[region]
	switch {
	case r.Nation == nil:
		return ErrNoNation
	case r.Project != nil:
		return ErrBuilding
	case r.Infra+1 >= len(InfraLevels):
		return ErrMaxLevel
	case !r.Nation.Has(InfraLevels[r.Infra+1].Tech):
		return ErrNeedTech
	}
	r.Project = &Project{Target: r.Infra + 1, Materials: map[GoodID]float64{}}
	return nil
}

// Expand has a tile's nation pay for one more building level of a firm.
func (w *World) Expand(region, firm int) error {
	r := w.Regions[region]
	f := r.Firms[firm]
	switch {
	case r.Nation == nil:
		return ErrNoNation
	case f.Expansion != nil:
		return ErrExpanding
	case !f.Enabled:
		return ErrFirmLocked
	case f.AtCap():
		return ErrFirmMax
	}
	best, _ := f.BestMethod(r.Prices(), r.Wage) // a new level runs the method it earns most on
	if best < 0 {
		return ErrFirmMax
	}
	f.Expansion = &Expansion{External: true, Method: best}
	return nil
}

// Refit has a tile's nation pay to refit one of a firm's levels to its
// method-th method: the level of its other methods that earns least.
func (w *World) Refit(region, firm, method int) error {
	r := w.Regions[region]
	f := r.Firms[firm]
	switch {
	case r.Nation == nil:
		return ErrNoNation
	case f.Expansion != nil:
		return ErrExpanding
	case !f.Enabled:
		return ErrFirmLocked
	case method < 0 || method >= len(f.Methods) || !f.Methods[method].Runs():
		return ErrMethod
	}
	from, low := -1, 0.0
	for i := range f.Methods {
		if i == method || f.MethodLevels(i) == 0 {
			continue
		}
		p := 0.0
		if f.Methods[i].Runs() {
			p = f.LevelProfit(i, r.Prices(), r.Wage)
		}
		if from < 0 || p < low {
			from, low = i, p
		}
	}
	if from < 0 {
		return ErrNoRefit
	}
	f.Expansion = &Expansion{External: true, Refit: true, Method: method, From: from}
	return nil
}

// BuildPort starts a port on a coast tile, paid for by its nation. The nation
// needs a tech that allows ports (Clipper ships).
func (w *World) BuildPort(region int) error {
	r := w.Regions[region]
	switch {
	case r.Nation == nil:
		return ErrNoNation
	case !r.Coastal:
		return ErrNotCoast
	case r.Port:
		return ErrHasPort
	case r.Project != nil:
		return ErrBuilding
	case !r.Nation.CanBuildPorts():
		return ErrNeedTech
	case r.Nation.Ports() >= r.Nation.PortAllowance():
		return ErrPortLimit
	}
	r.Project = &Project{Port: true, Materials: map[GoodID]float64{}}
	return nil
}

// TilesPerPort: a nation may have one port for every TilesPerPort tiles it
// holds, and at least one. Every port trades with every other, so the
// routes grow with the square of the ports: the allowance keeps them to
// about the number of land routes on the biggest map. A nation that
// conquers ports keeps them, but builds no more until it is back under
// its allowance.
const TilesPerPort = 16

// PortAllowance is how many ports the nation may have.
func (n *Nation) PortAllowance() int { return max(1, len(n.Regions)/TilesPerPort) }

// Ports is how many ports the nation has, counting those being built.
func (n *Nation) Ports() int {
	k := 0
	for _, r := range n.Regions {
		if r.Port || (r.Project != nil && r.Project.Port) {
			k++
		}
	}
	return k
}

// RemovePort pulls down a tile's port: its sea routes close, and what
// they had bought or had at sea is lost.
func (w *World) RemovePort(region int) error {
	r := w.Regions[region]
	switch {
	case r.Nation == nil:
		return ErrNoNation
	case !r.Port:
		return ErrNoPort
	}
	r.Port = false
	gone := func(t *Trader) bool { return t.FixedDistance && (t.Origin == r.Region || t.Dest == r.Region) }
	// every port had routes to and from this one
	w.World.RemoveTraders(gone, func(x *economy.Region) bool { return w.Regions[x.ID].Port || x == r.Region })
	for i, id := range w.Ports {
		if id == r.ID {
			w.Ports = append(w.Ports[:i:i], w.Ports[i+1:]...)
			break
		}
	}
	return nil
}

// Sea routes: a fixed cost for loading and unloading, plus distance at sea,
// cheaper with the owners' shipping technology.
const (
	portHandling = 0.2
	seaPerHex    = 0.12
)

// HexDistance is the number of hops between two tiles of an odd-r hex grid.
func HexDistance(r1, c1, r2, c2 int) int {
	cube := func(r, c int) (int, int, int) {
		x := c - (r-(r&1))/2
		return x, -x - r, r
	}
	x1, y1, z1 := cube(r1, c1)
	x2, y2, z2 := cube(r2, c2)
	abs := func(v int) int {
		if v < 0 {
			return -v
		}
		return v
	}
	return max(abs(x1-x2), abs(y1-y2), abs(z1-z2))
}

func (w *World) seaHop(a, b *economy.Region) float64 {
	mult := func(x *economy.Region) float64 {
		r := w.Regions[x.ID]
		if r.Nation == nil {
			return 1
		}
		return r.Nation.coastMult()
	}
	d := float64(HexDistance(a.Row, a.Col, b.Row, b.Col))
	return portHandling + seaPerHex*d*(mult(a)+mult(b))/2
}

// openNewPorts opens sea routes for ports finished this tick.
func (w *World) openNewPorts() {
	for _, r := range w.Regions {
		if r.Port && !w.hasPortRoutes(r.ID) {
			w.openPort(r)
		}
	}
}

func (w *World) hasPortRoutes(id int) bool {
	for _, p := range w.Ports {
		if p == id {
			return true
		}
	}
	return false
}

// openPort adds traders for every good both ways between r and every port
// already open.
func (w *World) openPort(r *Region) {
	if !math.IsInf(w.Spec.TransportCost, 1) {
		var added []*Trader
		for _, id := range w.Ports {
			p := w.Regions[id]
			hop := w.seaHop(r.Region, p.Region)
			for _, g := range FullTradable {
				a := NewTrader(g, r.Region, p.Region, hop, w.Spec.TransportCost)
				b := NewTrader(g, p.Region, r.Region, hop, w.Spec.TransportCost)
				a.FixedDistance, b.FixedDistance = true, true // the houses fund them from their purse
				added = append(added, a, b)
			}
		}
		w.World.AddTraders(added)
	}
	w.Ports = append(w.Ports, r.ID)
}

// borders reports whether a tile neighbours one of the nation's tiles.
func (w *World) borders(n *Nation, r *Region) bool {
	found := false
	for _, x := range w.World.Adjacent(r.ID) {
		if w.Regions[x[0]].Nation == n {
			found = true
			break
		}
	}
	return found
}

// transfer hands a tile to a nation. Its infrastructure stays; projects and
// state-funded expansions under way are abandoned.
func (w *World) transfer(r *Region, n *Nation) {
	if old := r.Nation; old != nil {
		for i, x := range old.Regions {
			if x == r {
				old.Regions = append(old.Regions[:i:i], old.Regions[i+1:]...)
				break
			}
		}
		if old.Eliminated() {
			for j := range old.AtWar {
				if old.AtWar[j] {
					old.AtWar[j], w.Nations[j].AtWar[old.ID] = false, false
				}
			}
			w.emit(-1, "eliminated", r.ID, "%s has fallen", old.Name)
		}
	}
	r.Siege = nil
	r.Project = nil
	for _, f := range r.Firms {
		for _, m := range f.Methods {
			m.TFP = m.BaseTFP
		}
		if f.Expansion != nil && f.Expansion.External {
			f.Expansion = nil
		}
	}
	// (the new owner's applyTech below sets productivity and which firms work)
	r.setNation(n)
	n.Regions = append(n.Regions, r)
	r.applyAllegiance() // its people's loyalties stay as they were: to the old owner, or to the new one if it had it before
	n.applyTech()
	w.refreshAllHops()
}

// Annex hands a tile to a nation outright (scenarios, tests).
func (w *World) Annex(region, nation int) { w.transfer(w.Regions[region], w.Nations[nation]) }

// CancelBuild abandons a tile's project; what was delivered is lost.
func (w *World) CancelBuild(region int) error {
	r := w.Regions[region]
	if r.Project == nil {
		return ErrNoProject
	}
	r.Project = nil
	return nil
}

// SetInfra sets a tile's infrastructure level directly (scenarios, tests).
func (w *World) SetInfra(region, level int) {
	w.Regions[region].setInfra(level)
	w.refreshHops(w.Regions[region])
}

// GrantTech gives a nation a technology outright (scenarios, tests).
func (w *World) GrantTech(nation int, tech string) {
	n := w.Nations[nation]
	n.Known[TechID(tech)] = true
	n.applyTech()
	w.refreshAllHops()
}

// landNeighbours calls f for each land tile next to r, until it returns false.
func (w *World) landNeighbours(r *Region, f func(nb *Region) bool) {
	for _, x := range w.World.Adjacent(r.ID) {
		if !f(w.Regions[x[0]]) {
			return
		}
	}
}

// settleTaxes pays the nation the taxes in its tiles' ledgers.
func (n *Nation) settleTaxes() {
	for _, r := range n.Regions {
		n.Treasury += r.Withheld
		n.Revenue += r.Withheld
		r.Withheld = 0
	}
}

// IsCity reports whether a tile has grown into a city (see economy.World.IsCity).
func (w *World) IsCity(r *Region) bool { return w.World.IsCity(r.Region) }

// UpdateAgglomeration sets what the tile's size and schooling do to its
// firms (see economy.Region.UpdateAgglomeration), and to its university.
func (r *Region) UpdateAgglomeration() {
	r.Region.UpdateAgglomeration()
	r.scholars()
}

// scholars sets the tile's university's productivity: scholars come from
// the schooled.
func (r *Region) scholars() {
	for _, f := range r.Firms {
		if f.Managed && isService(f, University) {
			f.EduMult = scienceUnschooled + scienceEducation*r.Educated
		}
	}
}
