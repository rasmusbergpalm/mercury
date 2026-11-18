package game

import (
	"container/heap"
	"fmt"
	"math"
	"sort"
	"strings"
)

// DivideLand shares out all the land of a scenario among its nations, so
// that a game starts with no free land. Each tile goes to the nation that
// can march to it cheapest from its centre, with a weight per nation tuned
// until the shares are as even as it gets; the centres start at the
// nations' capitals (their first tiles) and move to the middle of their
// land, k-means fashion, so each nation holds a compact piece of land. Then
// a few tiles pass between neighbours to even out the last differences
// (see balance). Where a nation starts only seeds where its land will be:
// the tiles it started with may go to its neighbours.
func DivideLand(spec GameSpec) (GameSpec, error) {
	rows := len(spec.Map)
	if rows == 0 || len(spec.NationMap) != rows {
		return spec, fmt.Errorf("the nation map doesn't fit the map")
	}
	cols := len(spec.Map[0])
	owner := make([]int, rows*cols) // nation index, -1 free, -2 sea
	symbol := map[byte]int{}
	for k, n := range spec.Nations {
		symbol[n.Symbol] = k
	}
	start := make([][]int, len(spec.Nations)) // each nation's tiles at the start
	free := 0
	for r := 0; r < rows; r++ {
		if len(spec.Map[r]) != cols || len(spec.NationMap[r]) != cols {
			return spec, fmt.Errorf("map row %d has the wrong length", r)
		}
		for c := 0; c < cols; c++ {
			i := r*cols + c
			switch {
			case spec.Map[r][c] == Sea:
				owner[i] = -2
			case spec.NationMap[r][c] == '.':
				owner[i] = -1
				free++
			default:
				k, ok := symbol[spec.NationMap[r][c]]
				if !ok {
					return spec, fmt.Errorf("unknown nation symbol %q", spec.NationMap[r][c])
				}
				owner[i] = k
				start[k] = append(start[k], i)
			}
		}
	}
	for k, s := range start {
		if len(s) == 0 {
			return spec, fmt.Errorf("%s holds no land to grow from", spec.Nations[k].Name)
		}
	}
	neighbours := func(i int) []int {
		var out []int
		for _, nb := range HexNeighbors(i/cols, i%cols, rows, cols) {
			out = append(out, nb[0]*cols+nb[1])
		}
		return out
	}
	k := len(spec.Nations)
	land := free
	for _, s := range start {
		land += len(s)
	}
	mean := float64(land) / float64(k)
	// Each nation has a centre, a tile that starts as its capital (its first
	// tile) and moves, as in k-means, to the middle of the land it is given:
	// share the land out from the centres, move each centre to the tile of
	// its land nearest that land's centre of mass, and again, until they
	// settle. A centre stuck in a corner would leave its nation reaching
	// round its neighbours for land; in the middle, each holds a compact
	// piece round it.
	center := make([]int, k)
	for n := range start {
		center[n] = start[n][0]
	}
	isCenter := make([]bool, len(owner))
	size := make([]int, k)
	weight := make([]float64, k)
	// home is how far, as the crow flies, a tile is from a nation's centre
	home := func(n, i int) int {
		c := center[n]
		return HexDistance(i/cols, i%cols, c/cols, c%cols)
	}
	for round := 0; round < 50; round++ {
		for i := range isCenter {
			isCenter[i] = false
		}
		for _, c := range center {
			isCenter[c] = true
		}
		// how far each nation would march from its centre to every tile over
		// land (mountains are harder going), not through other centres: a
		// march through one would leave what lies beyond cut off
		march := make([][]float64, k)
		for n := range spec.Nations {
			march[n] = landDistances(spec.Map, []int{center[n]}, func(i int) []int {
				var out []int
				for _, nb := range neighbours(i) {
					if !isCenter[nb] {
						out = append(out, nb)
					}
				}
				return out
			})
		}
		shareOut(owner, center, isCenter, march, weight, size, mean, home)
		// move each centre to the middle of its land
		moved := false
		for n := range spec.Nations {
			sx, sy, cnt := 0.0, 0.0, 0.0
			for i, o := range owner {
				if o == n {
					x, y := hexXY(i, cols)
					sx, sy, cnt = sx+x, sy+y, cnt+1
				}
			}
			best, bestD := center[n], math.Inf(1)
			for i, o := range owner {
				if o != n {
					continue
				}
				x, y := hexXY(i, cols)
				if d := math.Hypot(x-sx/cnt, y-sy/cnt); d < bestD {
					best, bestD = i, d
				}
			}
			if best != center[n] {
				center[n], moved = best, true
			}
		}
		if !moved {
			break
		}
	}
	// the centres are the nations' capitals while the last differences are
	// evened out
	capitals := make([][]int, k)
	for n, c := range center {
		capitals[n] = []int{c}
	}
	balance(owner, capitals, size, neighbours, home)
	out := spec
	out.NationMap = make([]string, rows)
	for r := 0; r < rows; r++ {
		var b strings.Builder
		for c := 0; c < cols; c++ {
			if o := owner[r*cols+c]; o >= 0 {
				b.WriteByte(spec.Nations[o].Symbol)
			} else {
				b.WriteByte('.')
			}
		}
		out.NationMap[r] = b.String()
	}
	return out, nil
}

// balance evens out the nations' shares of the land after they have grown:
// while the biggest holds at least two tiles more than the smallest, a tile
// passes along the shortest chain of bordering nations from the one to the
// other (each nation in the chain gives a border tile to the next), so the
// big one shrinks by a tile, the small one grows by one and those between
// keep their size. A nation never gives up its capital (the first tile it
// started with), nor a tile whose loss would cut its land in two; of the tiles it may give, it gives
// the one nearest the taker's home and furthest from its own, which keeps
// borders smooth. It stops when the shares are even (within a tile) or no
// chain can carry a tile any more.
func balance(owner []int, start [][]int, size []int, neighbours func(int) []int, home func(k, i int) int) {
	k := len(start)
	capital := make([]bool, len(owner)) // each nation's first tile, never given
	for _, s := range start {
		capital[s[0]] = true
	}
	// whole reports whether nation x's land stays in one piece without tile t
	seen := make([]int, len(owner))
	mark := 0
	whole := func(x, t int) bool {
		mark++
		from := -1
		for _, s := range start[x] {
			if s != t && owner[s] == x {
				from = s
				break
			}
		}
		if from < 0 {
			return false
		}
		stack, n := []int{from}, 1
		seen[from] = mark
		for len(stack) > 0 {
			c := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			for _, nb := range neighbours(c) {
				if nb != t && owner[nb] == x && seen[nb] != mark {
					seen[nb] = mark
					n++
					stack = append(stack, nb)
				}
			}
		}
		return n == size[x]-1
	}
	// give is the tile x would give y, or -1
	give := func(x, y int) int {
		type cand struct{ tile, score int }
		var cands []cand
		for i, o := range owner {
			if o != x || capital[i] {
				continue
			}
			for _, nb := range neighbours(i) {
				if owner[nb] == y {
					cands = append(cands, cand{i, home(y, i) - home(x, i)})
					break
				}
			}
		}
		sort.SliceStable(cands, func(a, b int) bool { return cands[a].score < cands[b].score })
		for _, c := range cands {
			if whole(x, c.tile) {
				return c.tile
			}
		}
		return -1
	}
	// chain is the shortest chain of nations from a to b along which tiles
	// can pass, or nil
	chain := func(a, b int) []int {
		prev := make([]int, k)
		for i := range prev {
			prev[i] = -1
		}
		prev[a] = a
		queue := []int{a}
		for len(queue) > 0 {
			x := queue[0]
			queue = queue[1:]
			if x == b {
				var path []int
				for y := b; y != a; y = prev[y] {
					path = append([]int{y}, path...)
				}
				return append([]int{a}, path...)
			}
			for y := 0; y < k; y++ {
				if prev[y] < 0 && give(x, y) >= 0 {
					prev[y] = x
					queue = append(queue, y)
				}
			}
		}
		return nil
	}
	for round := 0; round < len(owner); round++ {
		bySize := make([]int, k)
		for i := range bySize {
			bySize[i] = i
		}
		sort.SliceStable(bySize, func(a, b int) bool { return size[bySize[a]] > size[bySize[b]] })
		moved := false
		// the most uneven pair first: the biggest nation and the smallest it
		// can reach
	pairs:
		for _, big := range bySize {
			for j := k - 1; j >= 0; j-- {
				small := bySize[j]
				if size[big]-size[small] < 2 {
					break
				}
				path := chain(big, small)
				if path == nil {
					continue
				}
				for n := 0; n+1 < len(path); n++ {
					t := give(path[n], path[n+1])
					if t < 0 {
						break // the chain broke as tiles moved: try again next round
					}
					owner[t] = path[n+1]
					size[path[n]]--
					size[path[n+1]]++
				}
				moved = true
				break pairs
			}
		}
		if !moved {
			return
		}
	}
}

// landDistances is how far a march from the given tiles is to every cell of
// the map over land (+Inf for the sea and land out of reach): a step onto a
// tile costs 1, onto a mountain 2.
func landDistances(gameMap []string, from []int, neighbours func(int) []int) []float64 {
	cols := len(gameMap[0])
	dist := make([]float64, len(gameMap)*cols)
	for i := range dist {
		dist[i] = math.Inf(1)
	}
	q := &pathQueue{}
	for _, f := range from {
		dist[f] = 0
		heap.Push(q, pathItem{0, f})
	}
	for q.Len() > 0 {
		it := heap.Pop(q).(pathItem)
		if it.d != dist[it.id] {
			continue
		}
		for _, nb := range neighbours(it.id) {
			sym := gameMap[nb/cols][nb%cols]
			if sym == Sea {
				continue
			}
			step := 1.0
			if sym == 'M' {
				step = 2
			}
			if d := it.d + step; d < dist[nb] {
				dist[nb] = d
				heap.Push(q, pathItem{d, nb})
			}
		}
	}
	return dist
}

// shareOut gives every land tile to the nation that reaches it cheapest
// from its centre, its march plus the nation's weight, tuning the weights
// (up for nations above the mean, down for those below) until the shares
// are as even as they get. Taking each tile by its shortest march keeps
// every nation's land in one piece round its centre. The weights it ends on
// are where the next call starts. Land no nation can march to goes to the
// nearest centre.
func shareOut(owner, center []int, isCenter []bool, march [][]float64, weight []float64, size []int, mean float64, home func(n, i int) int) {
	assign := func() {
		for n := range size {
			size[n] = 1 // its centre
		}
		for n, c := range center {
			owner[c] = n
		}
		for i, o := range owner {
			if o < -1 || isCenter[i] {
				continue
			}
			best, bestCost := -1, math.Inf(1)
			for n := range march {
				if c := march[n][i] + weight[n]; c < bestCost {
					best, bestCost = n, c
				}
			}
			if best < 0 {
				best = 0
				for n := range march {
					if home(n, i) < home(best, i) {
						best = n
					}
				}
			}
			owner[i] = best
			size[best]++
		}
	}
	spread := func() (int, int) {
		lo, hi, sq := math.MaxInt, 0, 0
		for _, n := range size {
			lo, hi = min(lo, n), max(hi, n)
			sq += n * n
		}
		return hi - lo, sq
	}
	step := math.Sqrt(mean) // about a nation's radius, in marches
	bestWeight := append([]float64(nil), weight...)
	bestSpread, bestSq := math.MaxInt, 0
	for round := 0; round < 400; round++ {
		assign()
		sp, sq := spread()
		if sp < bestSpread || (sp == bestSpread && sq < bestSq) {
			bestSpread, bestSq = sp, sq
			copy(bestWeight, weight)
		}
		if sp <= 1 {
			break
		}
		for n, s := range size {
			weight[n] += step * (float64(s) - mean) / mean
		}
		step *= 0.985
	}
	copy(weight, bestWeight)
	assign()
}

// hexXY is a cell's centre in hex widths (odd rows shifted half a hex).
func hexXY(i, cols int) (float64, float64) {
	r, c := i/cols, i%cols
	return float64(c) + 0.5*float64(r&1), float64(r) * math.Sqrt(3) / 2
}
