// Package engine is the Mercury economic simulation: pops, firms, regional
// markets and traders that pay for transport. It is deterministic: the same
// scenario stepped the same number of times gives bit-identical state.
package engine

import "fmt"

// GoodID indexes Goods.
type GoodID int

// Good is a tradable good (or the transport service) and its physical traits.
type Good struct {
	Name string
	// Bulk is transport units needed per unit shipped per unit of distance.
	Bulk float64
	// Spoilage is the fraction of unsold stock lost per tick, and per unit of
	// distance in transit.
	Spoilage float64
	// Game marks goods that only exist in game worlds (the classic world, the
	// one the Python reference specifies, doesn't trade them).
	Game bool
}

// Goods lists every good. The order is part of the simulation's definition.
var Goods = []Good{
	// basic consumer goods
	{"bread", 1.0, 0.30, false},
	{"potatoes", 2.0, 0.10, false},
	{"meat", 1.0, 0.40, false},
	{"fish", 1.0, 0.50, false},
	{"clothes", 0.5, 0.02, false},
	{"wood", 2.0, 0.02, false},
	{"coal", 2.0, 0.00, false},
	// intermediate consumer goods
	{"furniture", 1.5, 0.02, false},
	{"ironware", 1.0, 0.00, false},
	{"beer", 1.5, 0.10, false},
	{"tea", 0.3, 0.02, false},
	{"sugar", 0.5, 0.02, false},
	// luxury consumer goods
	{"wine", 0.5, 0.01, false},
	{"fine clothes", 0.2, 0.01, false},
	{"porcelain", 0.5, 0.00, false},
	// industrial inputs (not consumed by pops)
	{"grain", 1.5, 0.05, false},
	{"iron ore", 2.0, 0.00, false},
	{"iron", 1.0, 0.00, false},
	{"wool", 1.0, 0.02, false},
	{"cotton", 1.0, 0.02, false},
	{"cloth", 0.5, 0.02, false},
	{"sugar cane", 2.0, 0.20, false},
	{"grapes", 1.0, 0.30, false},
	{"silk", 0.2, 0.01, false},
	{"clay", 2.0, 0.00, false},
	// a service: whatever isn't used in the tick it's produced is lost
	{"transport", 0.0, 1.00, false},
	// building work, bought by nations for infrastructure; local and perishable
	{"construction", 0.0, 1.00, false},
	// research work, bought by nations to develop technology; local and perishable
	{"research", 0.0, 1.00, false},
	// war materiel and fuel, game worlds only
	{"arms", 0.5, 0.00, true},
	{"artillery", 2.0, 0.00, true},
	{"munitions", 1.0, 0.00, true},
	{"vehicles", 3.0, 0.00, true},
	{"aircraft", 2.0, 0.00, true},
	{"oil", 1.5, 0.00, true},
}

// NumGoods is len(Goods).
var NumGoods = len(Goods)

var goodIndex = func() map[string]GoodID {
	m := make(map[string]GoodID, len(Goods))
	for i, g := range Goods {
		m[g.Name] = GoodID(i)
	}
	return m
}()

// G looks a good up by name and panics if it doesn't exist.
func G(name string) GoodID {
	id, ok := goodIndex[name]
	if !ok {
		panic(fmt.Sprintf("unknown good %q", name))
	}
	return id
}

// Transport is the transport service good.
var Transport = G("transport")

// Construction is building work; nations buy it to build infrastructure.
var Construction = G("construction")

// Research is research work; nations buy it to develop technology.
var Research = G("research")

// IsService reports whether a good is a local service that can't be shipped.
func IsService(g GoodID) bool { return g == Transport || g == Construction || g == Research }

// Tradable lists the goods traders ship in the classic world.
var Tradable = func() []GoodID {
	var out []GoodID
	for i, g := range Goods {
		if !IsService(GoodID(i)) && !g.Game {
			out = append(out, GoodID(i))
		}
	}
	return out
}()

// GameTradable lists the goods traders ship in game worlds: Tradable plus war
// materiel and oil.
var GameTradable = func() []GoodID {
	var out []GoodID
	for i := range Goods {
		if !IsService(GoodID(i)) {
			out = append(out, GoodID(i))
		}
	}
	return out
}()

// Composite is a set of substitutable production inputs, e.g. fuel = wood or coal.
type Composite struct {
	Name  string
	Goods []GoodID
	Sigma float64 // elasticity of substitution (≠ 1)
}

// Composites are the substitutable inputs firms can use.
var Composites = []Composite{
	{"fuel", []GoodID{G("wood"), G("coal")}, 3.0},
	{"fiber", []GoodID{G("wool"), G("cotton")}, 3.0},
}

// CompositeID looks a composite up by name and panics if it doesn't exist.
func CompositeID(name string) int {
	for i, c := range Composites {
		if c.Name == name {
			return i
		}
	}
	panic(fmt.Sprintf("unknown composite %q", name))
}
