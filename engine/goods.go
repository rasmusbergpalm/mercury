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
}

// Goods lists every good. The order is part of the simulation's definition.
var Goods = []Good{
	// basic consumer goods
	{"bread", 1.0, 0.30},
	{"potatoes", 2.0, 0.10},
	{"meat", 1.0, 0.40},
	{"fish", 1.0, 0.50},
	{"clothes", 0.5, 0.02},
	{"wood", 2.0, 0.02},
	{"coal", 2.0, 0.00},
	// intermediate consumer goods
	{"furniture", 1.5, 0.02},
	{"ironware", 1.0, 0.00},
	{"beer", 1.5, 0.10},
	{"tea", 0.3, 0.02},
	{"sugar", 0.5, 0.02},
	// luxury consumer goods
	{"wine", 0.5, 0.01},
	{"fine clothes", 0.2, 0.01},
	{"porcelain", 0.5, 0.00},
	// industrial inputs (not consumed by pops)
	{"grain", 1.5, 0.05},
	{"iron ore", 2.0, 0.00},
	{"iron", 1.0, 0.00},
	{"wool", 1.0, 0.02},
	{"cotton", 1.0, 0.02},
	{"cloth", 0.5, 0.02},
	{"sugar cane", 2.0, 0.20},
	{"grapes", 1.0, 0.30},
	{"silk", 0.2, 0.01},
	{"clay", 2.0, 0.00},
	// a service: whatever isn't used in the tick it's produced is lost
	{"transport", 0.0, 1.00},
	// building work, bought by nations for infrastructure; local and perishable
	{"construction", 0.0, 1.00},
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

// Tradable lists every good that traders can ship (all but the services).
var Tradable = func() []GoodID {
	var out []GoodID
	for i := range Goods {
		if GoodID(i) != Transport && GoodID(i) != Construction {
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
