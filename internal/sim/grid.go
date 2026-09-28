package sim

import "math"

// grid is a uniform spatial hash of axis-aligned bounds, used for
// neighbourhood queries against road segments and obstacles.
type grid struct {
	cell  float64
	cells map[[2]int][]int32
	stamp []uint32 // per-id visit stamp, used to de-duplicate query results
	epoch uint32
}

func newGrid(cell float64) *grid {
	return &grid{cell: cell, cells: make(map[[2]int][]int32)}
}

func (g *grid) key(x, y float64) (int, int) {
	return int(math.Floor(x / g.cell)), int(math.Floor(y / g.cell))
}

func (g *grid) insert(id int, lo, hi V2) {
	x0, y0 := g.key(lo.X, lo.Y)
	x1, y1 := g.key(hi.X, hi.Y)
	for x := x0; x <= x1; x++ {
		for y := y0; y <= y1; y++ {
			k := [2]int{x, y}
			g.cells[k] = append(g.cells[k], int32(id))
		}
	}
	for len(g.stamp) <= id {
		g.stamp = append(g.stamp, 0)
	}
}

// each calls fn once for every id whose bounds may overlap the box lo..hi.
func (g *grid) each(lo, hi V2, fn func(id int)) {
	g.epoch++
	x0, y0 := g.key(lo.X, lo.Y)
	x1, y1 := g.key(hi.X, hi.Y)
	for x := x0; x <= x1; x++ {
		for y := y0; y <= y1; y++ {
			for _, id := range g.cells[[2]int{x, y}] {
				if g.stamp[id] == g.epoch {
					continue
				}
				g.stamp[id] = g.epoch
				fn(int(id))
			}
		}
	}
}

func (g *grid) around(p V2, r float64, fn func(id int)) {
	g.each(V2{p.X - r, p.Y - r}, V2{p.X + r, p.Y + r}, fn)
}
