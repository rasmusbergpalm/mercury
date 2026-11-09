package engine

import (
	"fmt"
	"sort"
	"strings"
)

// DivideLand shares out every free tile of a scenario among its nations, so
// that a game starts with no free land: one tile at a time, the smallest
// nation that can still grow claims the free tile next to its land nearest
// to where it started (lowland before mountains), so that each grows round
// its heartland and they end up about the same size where they have the
// room. Land none of them can reach (another island) goes to the
// nearest nation, which then spreads over it. The nations keep the tiles
// they had.
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
	// home is how far a cell is from the nearest of a nation's starting tiles
	home := func(k, i int) int {
		d := -1
		for _, s := range start[k] {
			if x := HexDistance(i/cols, i%cols, s/cols, s%cols); d < 0 || x < d {
				d = x
			}
		}
		return d
	}
	neighbours := func(i int) []int {
		var out []int
		for _, nb := range HexNeighbors(i/cols, i%cols, rows, cols) {
			out = append(out, nb[0]*cols+nb[1])
		}
		return out
	}
	// a claim's cost: the distance home, and a little more for mountains
	cost := func(k, i int) float64 {
		d := float64(home(k, i))
		if spec.Map[i/cols][i%cols] == 'M' {
			d += 1.5
		}
		return d
	}
	size := make([]int, len(spec.Nations))
	for k := range start {
		size[k] = len(start[k])
	}
	// claim is the free tile next to nation k's land it would take next, or -1
	claim := func(k int) int {
		best, bestCost := -1, 0.0
		for i, o := range owner {
			if o != k {
				continue
			}
			for _, nb := range neighbours(i) {
				if owner[nb] != -1 {
					continue
				}
				if c := cost(k, nb); best < 0 || c < bestCost || (c == bestCost && nb < best) {
					best, bestCost = nb, c
				}
			}
		}
		return best
	}
	for free > 0 {
		// the smallest nation that can still grow claims a tile (the first
		// among equals), so they stay as even as the land lets them
		grew := false
		order := make([]int, len(spec.Nations))
		for k := range order {
			order[k] = k
		}
		sort.SliceStable(order, func(a, b int) bool { return size[order[a]] < size[order[b]] })
		for _, k := range order {
			if i := claim(k); i >= 0 {
				owner[i] = k
				size[k]++
				free--
				grew = true
				break
			}
		}
		if grew || free == 0 {
			continue
		}
		// no nation borders the free land left: hand its first tile to the
		// nearest nation (the first among equals), which grows from there
		for i, o := range owner {
			if o != -1 {
				continue
			}
			best := 0
			for k := range spec.Nations {
				if home(k, i) < home(best, i) {
					best = k
				}
			}
			owner[i] = best
			size[best]++
			free--
			break
		}
	}
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
