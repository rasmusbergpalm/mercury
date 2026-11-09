package engine

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strings"
)

// Map generation: a seeded world of continents and islands in an ocean,
// with terrain from elevation, moisture and latitude, named tiles, and
// nations spread across the land, each starting from a few tiles round its
// capital. The same options and seed always give the same map.

// MapOptions say what kind of world to generate.
type MapOptions struct {
	Rows, Cols int
	Nations    int     // how many nations start on the map
	Land       float64 // share of the map that is land (0: defaultLand)
	Seed       int64
}

// Map generation rules.
const (
	defaultLand    = 0.55
	minIsland      = 3   // smaller islands sink back into the sea
	startTiles     = 4   // each nation starts with about this many tiles
	mountainShare  = 0.1 // the highest land is mountains...
	hillShare      = 0.18
	MaxNations     = 12
	MaxMapCells    = 2500
	genSeaRoutes   = 8  // a port trades with this many nearest ports
	minNationTiles = 12 // land tiles per nation the map needs at least
	minCapitalDist = 4  // hexes between capitals: their starting lands don't touch
)

// MapSize is a preset world size.
type MapSize struct {
	Name       string
	Rows, Cols int
	Nations    int // the default number of nations
}

// MapSizes are the world sizes on offer, from a small map like the classic
// one (about 36 land tiles) to about a thousand land tiles.
var MapSizes = []MapSize{
	{"small", 8, 9, 3},
	{"medium", 14, 18, 5},
	{"large", 22, 28, 7},
	{"huge", 36, 50, 10},
}

// nationColors are told apart on the map, in order of use.
var nationColors = []string{"#b8322e", "#2c5aa0", "#c08a1e", "#4e7d3a", "#6b3f8c", "#1f7a7a",
	"#c25e1b", "#a83c78", "#3d4a5c", "#7a5c2e", "#5a8fbf", "#8c8c2a"}

// GenerateGame makes a game scenario on a freshly generated map.
func GenerateGame(o MapOptions) (GameSpec, error) {
	if o.Rows < 4 || o.Cols < 4 || o.Rows*o.Cols > MaxMapCells {
		return GameSpec{}, fmt.Errorf("a map must be at least 4×4 and at most %d cells", MaxMapCells)
	}
	if o.Nations < 1 || o.Nations > MaxNations {
		return GameSpec{}, fmt.Errorf("a map holds 1 to %d nations", MaxNations)
	}
	land := o.Land
	if land == 0 {
		land = defaultLand
	}
	if land < 0.2 || land > 0.9 {
		return GameSpec{}, fmt.Errorf("land must be between 20%% and 90%% of the map")
	}
	rng := rand.New(rand.NewSource(o.Seed))
	g := newGenGrid(o.Rows, o.Cols)
	g.shape(rng, land)
	if g.landCount() < o.Nations*minNationTiles {
		return GameSpec{}, fmt.Errorf("the map has too little land for %d nations", o.Nations)
	}
	g.terrain(rng)
	owners, err := g.placeNations(rng, o.Nations)
	if err != nil {
		return GameSpec{}, err
	}
	spec := GameSpec{TransportCost: 1, TaxRate: 0.1, Seed: o.Seed, SeaRoutes: genSeaRoutes}
	for r := 0; r < o.Rows; r++ {
		var row, nrow strings.Builder
		for c := 0; c < o.Cols; c++ {
			i := r*o.Cols + c
			row.WriteByte(g.sym[i])
			if owners[i] >= 0 {
				nrow.WriteByte(byte('A' + owners[i]))
			} else {
				nrow.WriteByte('.')
			}
		}
		spec.Map, spec.NationMap = append(spec.Map, row.String()), append(spec.NationMap, nrow.String())
	}
	spec.Names = NameTiles(spec.Map, o.Seed)
	used := map[string]bool{}
	for k := 0; k < o.Nations; k++ {
		name := nationName(rng, used)
		spec.Nations = append(spec.Nations, NationSpec{byte('A' + k), name, nationColors[k%len(nationColors)]})
	}
	return spec, nil
}

// genGrid is a map being generated: per cell, its elevation and moisture,
// whether it is land, and its terrain symbol.
type genGrid struct {
	rows, cols  int
	elev, moist []float64
	land        []bool
	sym         []byte
}

func newGenGrid(rows, cols int) *genGrid {
	n := rows * cols
	return &genGrid{rows: rows, cols: cols, elev: make([]float64, n), moist: make([]float64, n),
		land: make([]bool, n), sym: make([]byte, n)}
}

// neighbours lists the cells next to cell i.
func (g *genGrid) neighbours(i int) []int {
	var out []int
	for _, nb := range HexNeighbors(i/g.cols, i%g.cols, g.rows, g.cols) {
		out = append(out, nb[0]*g.cols+nb[1])
	}
	return out
}

// xy is a cell's centre in hex-width units (odd rows shifted half a hex).
func (g *genGrid) xy(i int) (float64, float64) {
	r, c := i/g.cols, i%g.cols
	return float64(c) + 0.5*float64(r&1), float64(r) * math.Sqrt(3) / 2
}

// noise is smooth value noise over the map: a few octaves of random values
// on coarse lattices, interpolated.
type noise struct {
	octaves []noiseOctave
}

type noiseOctave struct {
	scale, weight float64
	w, h          int
	v             []float64
}

func newNoise(rng *rand.Rand, w, h, base float64) noise {
	var n noise
	scale, weight := base, 1.0
	for k := 0; k < 4; k++ {
		gw, gh := int(w/scale)+3, int(h/scale)+3
		o := noiseOctave{scale: scale, weight: weight, w: gw, h: gh, v: make([]float64, gw*gh)}
		for i := range o.v {
			o.v[i] = rng.Float64()
		}
		n.octaves = append(n.octaves, o)
		scale, weight = scale/2, weight/2
	}
	return n
}

func (n noise) at(x, y float64) float64 {
	s, total := 0.0, 0.0
	for _, o := range n.octaves {
		fx, fy := x/o.scale, y/o.scale
		x0, y0 := int(fx), int(fy)
		tx, ty := smooth(fx-float64(x0)), smooth(fy-float64(y0))
		v := func(a, b int) float64 { return o.v[b*o.w+a] }
		top := v(x0, y0)*(1-tx) + v(x0+1, y0)*tx
		bottom := v(x0, y0+1)*(1-tx) + v(x0+1, y0+1)*tx
		s += o.weight * (top*(1-ty) + bottom*ty)
		total += o.weight
	}
	return s / total
}

func smooth(t float64) float64 { return t * t * (3 - 2*t) }

// shape raises land from the sea: noise falling off towards the map's
// edges, cut at the level that leaves the asked share of land; tiny islands
// sink.
func (g *genGrid) shape(rng *rand.Rand, land float64) {
	w, h := g.xy(g.rows*g.cols - 1)
	w, h = w+1, h+1
	base := math.Max(4, math.Min(w, h)/2.5) // continents a few across
	elev := newNoise(rng, w, h, base)
	moist := newNoise(rng, w, h, base/1.5)
	for i := range g.elev {
		x, y := g.xy(i)
		// distance to the nearest edge, as a share of the half-size
		dx := math.Min(x+0.5, w-x-0.5) / (w / 2)
		dy := math.Min(y+0.5, h-y-0.5) / (h / 2)
		edge := math.Min(1, math.Min(dx, dy)*2.5)
		g.elev[i] = elev.at(x, y) * (0.35 + 0.65*edge)
		g.moist[i] = moist.at(x, y)
	}
	level := quantile(g.elev, 1-land)
	for i, e := range g.elev {
		g.land[i] = e > level
	}
	for _, island := range g.islands() {
		if len(island) < minIsland {
			for _, i := range island {
				g.land[i] = false
			}
		}
	}
}

// islands lists the connected bodies of land, biggest first (then by their
// first cell).
func (g *genGrid) islands() [][]int {
	seen := make([]bool, len(g.land))
	var out [][]int
	for i := range g.land {
		if !g.land[i] || seen[i] {
			continue
		}
		var body []int
		stack := []int{i}
		seen[i] = true
		for len(stack) > 0 {
			c := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			body = append(body, c)
			for _, nb := range g.neighbours(c) {
				if g.land[nb] && !seen[nb] {
					seen[nb] = true
					stack = append(stack, nb)
				}
			}
		}
		sort.Ints(body)
		out = append(out, body)
	}
	sort.SliceStable(out, func(a, b int) bool { return len(out[a]) > len(out[b]) })
	return out
}

func (g *genGrid) landCount() int {
	n := 0
	for _, l := range g.land {
		if l {
			n++
		}
	}
	return n
}

// terrain gives every land cell its terrain: the highest land mountains,
// then hills; below them tropical in the warm south when it is wet enough,
// forest where it is wet, plains elsewhere.
func (g *genGrid) terrain(rng *rand.Rand) {
	var landElev, landMoist []float64
	for i, l := range g.land {
		if l {
			landElev = append(landElev, g.elev[i])
			landMoist = append(landMoist, g.moist[i])
		}
	}
	mountain := quantile(landElev, 1-mountainShare)
	hill := quantile(landElev, 1-mountainShare-hillShare)
	wet := quantile(landMoist, 0.6)
	for i, l := range g.land {
		if !l {
			g.sym[i] = Sea
			continue
		}
		r := i / g.cols
		south := float64(r) / float64(g.rows-1) // 0 north, 1 south
		warm := south + 0.25*(g.moist[i]-0.5) + 0.1*(rng.Float64()-0.5)
		switch {
		case g.elev[i] > mountain:
			g.sym[i] = 'M'
		case g.elev[i] > hill:
			g.sym[i] = 'H'
		case warm > 0.7:
			g.sym[i] = 'T'
		case g.moist[i] > wet:
			g.sym[i] = 'F'
		default:
			g.sym[i] = 'P'
		}
	}
}

// placeNations picks capitals far apart on the land (on the big islands,
// away from the mountains) and grows each nation to startTiles tiles
// around its capital. It returns each cell's nation, or -1.
func (g *genGrid) placeNations(rng *rand.Rand, k int) ([]int, error) {
	owner := make([]int, len(g.land))
	for i := range owner {
		owner[i] = -1
	}
	// candidate capitals: land on islands big enough for a nation to grow,
	// not mountains
	var cands []int
	for _, island := range g.islands() {
		if len(island) < 2*startTiles {
			continue
		}
		for _, i := range island {
			if g.sym[i] != 'M' {
				cands = append(cands, i)
			}
		}
	}
	if len(cands) < k {
		return nil, fmt.Errorf("the map has no room for %d nations", k)
	}
	dist := func(a, b int) int {
		return HexDistance(a/g.cols, a%g.cols, b/g.cols, b%g.cols)
	}
	// farthest-point sampling from a random first capital
	capitals := []int{cands[rng.Intn(len(cands))]}
	near := make([]int, len(cands))
	for i, c := range cands {
		near[i] = dist(c, capitals[0])
	}
	for len(capitals) < k {
		best := -1
		for i, c := range cands {
			if near[i] > 0 && (best < 0 || near[i] > near[best]) && !g.crowded(c, capitals) {
				best = i
			}
		}
		if best < 0 {
			return nil, fmt.Errorf("the map has no room for %d nations", k)
		}
		capitals = append(capitals, cands[best])
		for i, c := range cands {
			near[i] = min(near[i], dist(c, cands[best]))
		}
	}
	// each nation grows in turn, a tile at a time, to the nearest free land
	// (lowland first) round its capital
	for n, c := range capitals {
		owner[c] = n
	}
	for size := 1; size < startTiles; size++ {
		for n, c := range capitals {
			best, bestScore := -1, math.Inf(1)
			for i, o := range owner {
				if o != n {
					continue
				}
				for _, nb := range g.neighbours(i) {
					if !g.land[nb] || owner[nb] >= 0 || g.nextToOther(nb, n, owner) {
						continue
					}
					score := float64(dist(nb, c))
					if g.sym[nb] == 'M' {
						score += 1.5
					}
					if score < bestScore || (score == bestScore && nb < best) {
						best, bestScore = nb, score
					}
				}
			}
			if best >= 0 {
				owner[best] = n
			}
		}
	}
	return owner, nil
}

// crowded reports whether a capital at c would sit too near another.
func (g *genGrid) crowded(c int, capitals []int) bool {
	for _, x := range capitals {
		if HexDistance(c/g.cols, c%g.cols, x/g.cols, x%g.cols) < minCapitalDist {
			return true
		}
	}
	return false
}

// nextToOther reports whether cell i borders a nation other than n (nations
// start apart, with free land between them).
func (g *genGrid) nextToOther(i, n int, owner []int) bool {
	for _, nb := range g.neighbours(i) {
		if owner[nb] >= 0 && owner[nb] != n {
			return true
		}
	}
	return false
}

// quantile is the q-quantile of xs (which it doesn't change).
func quantile(xs []float64, q float64) float64 {
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	if len(s) == 0 {
		return 0
	}
	i := int(math.Round(q * float64(len(s)-1)))
	return s[max(0, min(len(s)-1, i))]
}
