package economy

import "fmt"

// GoodID indexes Goods.
type GoodID int

// Good is a tradable good (or a service) and its physical traits.
type Good struct {
	Name string
	// Bulk is transport units needed per unit shipped per unit of distance.
	Bulk float64
	// Spoilage is the fraction of unsold stock lost per tick, and per unit of
	// distance in transit.
	Spoilage float64
	// Service marks a good made and used where it is (transport, building
	// work): it is never shipped.
	Service bool
}

// Goods lists every good (data/goods.json). The order is part of the
// simulation's definition.
var Goods = func() []Good {
	out := make([]Good, len(goodsData.Goods))
	for i, g := range goodsData.Goods {
		out[i] = Good{g.Name, g.Bulk, g.Spoilage, g.Service}
	}
	return out
}()

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

// Construction is building work: owners buy it to build and maintain
// their works, and outside buyers for whatever they build.
var Construction = G("construction")

// IsService reports whether a good is a local service that can't be shipped.
func IsService(g GoodID) bool {
	return Goods[g].Service
}

// Tradable lists the goods merchants ship: every good but the services.
var Tradable = func() []GoodID {
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

// Composites are the substitutable inputs firms can use (data/goods.json).
var Composites = func() []Composite {
	var out []Composite
	for _, c := range goodsData.Composites {
		var gs []GoodID
		for _, n := range c.Goods {
			gs = append(gs, G(n))
		}
		out = append(out, Composite{c.Name, gs, c.Sigma})
	}
	return out
}()

func compositeIndex(name string) int {
	for i, c := range Composites {
		if c.Name == name {
			return i
		}
	}
	return -1
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
