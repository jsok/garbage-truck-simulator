package osm

import (
	"cmp"
	"errors"
	"math"
	"slices"

	"github.com/jsok/garbage-truck-simulator/internal/sim"
)

// Clean-up applied to real street data, which is messier than the game's own
// suburbs: staggered junctions a few metres apart, roundabouts, stubs where a
// street leaves the map, and sharp corners the kerb geometry cannot follow.
const (
	edgeMargin    = 14.0 // streets stop this far inside the area, leaving room for the bush
	mergeDist     = 12.0 // junctions joined by a shorter street become one
	minStub       = 30.0 // shorter dead ends are dropped
	maxRoundabout = 60.0 // roundabouts narrower than this become a plain junction
	cornerRadius  = 14.0 // bends within a street are rounded to this radius
	minGap        = 16.0 // closer streets side by side would overlap
)

const earthRadius = 6371008.8 // metres

// Projection maps degrees to game metres around an origin, with +X east and
// +Y south (the sim's ground plane seen from above has +Y pointing down).
// It is equirectangular, which is plenty accurate over a suburb.
type Projection struct{ Lat0, Lon0 float64 }

// Centre projects around the middle of b.
func Centre(b BBox) Projection {
	return Projection{(b.South + b.North) / 2, (b.West + b.East) / 2}
}

func (p Projection) k() (float64, float64) {
	m := math.Pi / 180 * earthRadius
	return m * math.Cos(p.Lat0*math.Pi/180), m
}

// ToGame converts degrees to game metres.
func (p Projection) ToGame(lat, lon float64) sim.V2 {
	kx, ky := p.k()
	return sim.V2{X: (lon - p.Lon0) * kx, Y: -(lat - p.Lat0) * ky}
}

// ToLatLon converts game metres back to degrees.
func (p Projection) ToLatLon(v sim.V2) (lat, lon float64) {
	kx, ky := p.k()
	return p.Lat0 - v.Y/ky, p.Lon0 + v.X/kx
}

// Options choose what goes into a layout.
type Options struct {
	MainRoads bool // include MainClasses as well as Classes
}

// Stats describe how much of the street network made it into a layout.
type Stats struct {
	Kept, Cut float64 // metres of street kept, and dropped for not joining up
}

// Build turns the streets in d into a layout covering b, projected with
// Centre(b). Streets are clipped to b, tidied up, and only the largest
// connected network is kept.
func Build(d *Data, b BBox, opt Options) (*sim.Layout, Stats, error) {
	classes := Classes
	if opt.MainRoads {
		classes = append(slices.Clone(Classes), MainClasses...)
	}
	proj := Centre(b)
	g := &graph{pos: map[int64]sim.V2{}, next: -1}
	g.lo, g.hi = proj.ToGame(b.North, b.West), proj.ToGame(b.South, b.East)
	var ways []Element
	for _, e := range d.Elements {
		switch e.Type {
		case "node":
			g.pos[e.ID] = proj.ToGame(e.Lat, e.Lon)
		case "way":
			if slices.Contains(classes, e.Tags["highway"]) && e.Tags["area"] != "yes" {
				ways = append(ways, e)
			}
		}
	}
	ways = g.collapseRoundabouts(ways)
	g.trace(ways)
	g.splitLoops()
	g.mergeJunctions()
	g.splitLoops()
	g.dropParallel()
	g.mergeJunctions() // joins the side streets dropParallel reconnected
	g.splitLoops()
	for g.pruneStubs() {
	}
	var st Stats
	st.Kept, st.Cut = g.keepLargest()
	if len(g.roads) == 0 {
		return nil, st, errors.New("osm: no streets in this area")
	}

	l := &sim.Layout{Attribution: Attribution, Min: g.lo, Max: g.hi}
	index := map[int64]int{}
	node := func(id int64) int {
		i, ok := index[id]
		if !ok {
			i = len(l.Nodes)
			index[id] = i
			l.Nodes = append(l.Nodes, g.pos[id])
		}
		return i
	}
	for _, r := range g.roads {
		l.Roads = append(l.Roads, sim.LayoutRoad{
			Name: r.name,
			A:    node(r.a),
			B:    node(r.b),
			Pts:  roundCorners(r.pts, cornerRadius),
		})
	}
	return l, st, nil
}

// graph is the street network while it is being cleaned up. Node ids are
// OpenStreetMap ids, or negative for nodes we made up.
type graph struct {
	pos    map[int64]sim.V2
	next   int64
	lo, hi sim.V2 // bounds of the area
	roads  []*road
}

type road struct {
	a, b int64
	pts  []sim.V2 // from a to b
	name string
}

func (r *road) len() float64 { return polyLen(r.pts) }

func (r *road) reverse() {
	r.a, r.b = r.b, r.a
	slices.Reverse(r.pts)
}

func (g *graph) newNode(p sim.V2) int64 {
	id := g.next
	g.next--
	g.pos[id] = p
	return id
}

func wayName(e Element) string {
	if n := e.Tags["name"]; n != "" {
		return n
	}
	return e.Tags["ref"]
}

// collapseRoundabouts replaces each small roundabout with a single node at
// its centre, so the streets that enter it meet at a plain junction. It
// returns the ways that are left.
func (g *graph) collapseRoundabouts(ways []Element) []Element {
	uf := unionFind{}
	for _, w := range ways {
		if isRoundabout(w) {
			for i := 1; i < len(w.Nodes); i++ {
				uf.union(w.Nodes[i-1], w.Nodes[i])
			}
		}
	}
	rings := map[int64][]int64{}
	var roots []int64
	for _, w := range ways {
		if !isRoundabout(w) {
			continue
		}
		for _, id := range w.Nodes {
			r := uf.find(id)
			if _, ok := rings[r]; !ok {
				roots = append(roots, r)
			}
			if !slices.Contains(rings[r], id) {
				rings[r] = append(rings[r], id)
			}
		}
	}
	alias := map[int64]int64{}
	for _, r := range roots {
		var c sim.V2
		n := 0
		for _, id := range rings[r] {
			if p, ok := g.pos[id]; ok {
				c, n = c.Add(p), n+1
			}
		}
		if n == 0 {
			continue
		}
		c = c.Scale(1 / float64(n))
		wide := false
		for _, id := range rings[r] {
			if p, ok := g.pos[id]; ok && 2*p.Dist(c) > maxRoundabout {
				wide = true
			}
		}
		if wide {
			continue
		}
		centre := g.newNode(c)
		for _, id := range rings[r] {
			alias[id] = centre
		}
	}
	var out []Element
	for _, w := range ways {
		if isRoundabout(w) && alias[w.Nodes[0]] != 0 {
			continue
		}
		ids := make([]int64, 0, len(w.Nodes))
		for _, id := range w.Nodes {
			if a := alias[id]; a != 0 {
				id = a
			}
			if len(ids) == 0 || ids[len(ids)-1] != id {
				ids = append(ids, id)
			}
		}
		w.Nodes = ids
		out = append(out, w)
	}
	return out
}

func isRoundabout(w Element) bool {
	j := w.Tags["junction"]
	return (j == "roundabout" || j == "circular") && len(w.Nodes) > 1
}

type vert struct {
	id int64
	p  sim.V2
}

// clip cuts a way to the area inside the edge margin. Where it crosses the
// edge it gets a new dead-end node.
func (g *graph) clip(ids []int64) [][]vert {
	lo := g.lo.Add(sim.V2{X: edgeMargin, Y: edgeMargin})
	hi := g.hi.Sub(sim.V2{X: edgeMargin, Y: edgeMargin})
	var pieces [][]vert
	var cur []vert
	flush := func() {
		if len(cur) > 1 {
			pieces = append(pieces, cur)
		}
		cur = nil
	}
	var prev *vert
	for _, id := range ids {
		p, ok := g.pos[id]
		if !ok {
			continue // node missing from the extract
		}
		v := vert{id, p}
		if prev == nil {
			prev = &v
			continue
		}
		a, b := *prev, v
		prev = &v
		t0, t1, ok := clipSeg(a.p, b.p, lo, hi)
		if !ok {
			flush()
			continue
		}
		start, end := a, b
		if t0 > 0 {
			flush()
			q := a.p.Lerp(b.p, t0)
			start = vert{g.newNode(q), q}
		}
		if len(cur) == 0 {
			cur = []vert{start}
		}
		if t1 < 1 {
			q := a.p.Lerp(b.p, t1)
			end = vert{g.newNode(q), q}
		}
		cur = append(cur, end)
		if t1 < 1 {
			flush()
		}
	}
	flush()
	return pieces
}

// clipSeg is Liang–Barsky: the parameter range of segment ab inside the box.
func clipSeg(a, b, lo, hi sim.V2) (t0, t1 float64, ok bool) {
	t0, t1 = 0, 1
	d := b.Sub(a)
	for _, pq := range [4][2]float64{{-d.X, a.X - lo.X}, {d.X, hi.X - a.X}, {-d.Y, a.Y - lo.Y}, {d.Y, hi.Y - a.Y}} {
		p, q := pq[0], pq[1]
		if p == 0 {
			if q < 0 {
				return 0, 0, false
			}
			continue
		}
		r := q / p
		if p < 0 {
			t0 = math.Max(t0, r)
		} else {
			t1 = math.Min(t1, r)
		}
	}
	return t0, t1, t1-t0 > 1e-9
}

// trace splits the ways into roads between junctions and dead ends. Ways
// that merely continue one another under the same name become one road.
func (g *graph) trace(ways []Element) {
	type edge struct {
		u, v int64
		name string
	}
	var edges []edge
	seen := map[[2]int64]bool{}
	adj := map[int64][]int{}
	for _, w := range ways {
		for _, piece := range g.clip(w.Nodes) {
			for i := 1; i < len(piece); i++ {
				u, v := piece[i-1].id, piece[i].id
				k := [2]int64{min(u, v), max(u, v)}
				if u == v || seen[k] {
					continue // overlapping ways
				}
				seen[k] = true
				adj[u] = append(adj[u], len(edges))
				adj[v] = append(adj[v], len(edges))
				edges = append(edges, edge{u, v, wayName(w)})
			}
		}
	}
	isVertex := map[int64]bool{}
	var vertices []int64
	for id, es := range adj {
		if len(es) != 2 || edges[es[0]].name != edges[es[1]].name {
			isVertex[id] = true
			vertices = append(vertices, id)
		}
	}
	slices.Sort(vertices)
	done := make([]bool, len(edges))
	walk := func(from int64, e int) {
		r := &road{a: from, pts: []sim.V2{g.pos[from]}, name: edges[e].name}
		cur := from
		for {
			done[e] = true
			next := edges[e].u
			if next == cur {
				next = edges[e].v
			}
			r.pts = append(r.pts, g.pos[next])
			cur = next
			if isVertex[cur] {
				break
			}
			if es := adj[cur]; es[0] == e {
				e = es[1]
			} else {
				e = es[0]
			}
		}
		r.b = cur
		g.roads = append(g.roads, r)
	}
	for _, id := range vertices {
		for _, e := range adj[id] {
			if !done[e] {
				walk(id, e)
			}
		}
	}
	// Whatever is left are closed rings with no junctions on them.
	for e := range edges {
		if !done[e] {
			isVertex[edges[e].u] = true
			walk(edges[e].u, e)
		}
	}
}

// splitLoops breaks roads that start and end at the same node in two, since
// the game expects a road to join two different nodes. Tiny loops are dropped.
func (g *graph) splitLoops() {
	var out []*road
	for _, r := range g.roads {
		if r.a != r.b {
			out = append(out, r)
			continue
		}
		if len(r.pts) < 4 || r.len() < 3*mergeDist {
			continue
		}
		mid := len(r.pts) / 2
		m := g.newNode(r.pts[mid])
		out = append(out,
			&road{a: r.a, b: m, pts: slices.Clone(r.pts[:mid+1]), name: r.name},
			&road{a: m, b: r.b, pts: slices.Clone(r.pts[mid:]), name: r.name})
	}
	g.roads = out
}

// mergeJunctions collapses streets shorter than mergeDist, joining the nodes
// at either end into one junction at their centroid.
func (g *graph) mergeJunctions() {
	uf := unionFind{}
	var out []*road
	for _, r := range g.roads {
		if r.len() < mergeDist {
			uf.union(r.a, r.b)
		} else {
			out = append(out, r)
		}
	}
	sum := map[int64]sim.V2{}
	count := map[int64]float64{}
	for id := range uf {
		root := uf.find(id)
		sum[root] = sum[root].Add(g.pos[id])
		count[root]++
	}
	for root, s := range sum {
		g.pos[root] = s.Scale(1 / count[root])
	}
	for _, r := range out {
		r.a, r.b = uf.find(r.a), uf.find(r.b)
		pa, pb := g.pos[r.a], g.pos[r.b]
		// Drop points that now double back towards the moved junction.
		pts := []sim.V2{pa}
		for _, p := range r.pts[1 : len(r.pts)-1] {
			if p.Dist(pa) > 5 && p.Dist(pb) > 5 {
				pts = append(pts, p)
			}
		}
		r.pts = append(pts, pb)
	}
	g.roads = out
}

// dropParallel removes streets that run alongside others, closer than the
// game's roads can be, for most of their length: one carriageway of a
// divided road, the two sides of a one-way loop, a service road beside its
// street. Shorter streets go first, so the longest of a group survives.
// Streets that fold back alongside themselves go too. Side streets that
// only met a dropped street are joined onto the one that stays.
func (g *graph) dropParallel() {
	type box struct{ lo, hi sim.V2 }
	boxes := make([]box, len(g.roads))
	for i, r := range g.roads {
		b := box{r.pts[0], r.pts[0]}
		for _, p := range r.pts {
			b.lo = sim.V2{X: math.Min(b.lo.X, p.X), Y: math.Min(b.lo.Y, p.Y)}
			b.hi = sim.V2{X: math.Max(b.hi.X, p.X), Y: math.Max(b.hi.Y, p.Y)}
		}
		boxes[i] = b
	}
	overlap := func(i, j int) bool {
		return boxes[i].lo.X <= boxes[j].hi.X+minGap && boxes[j].lo.X <= boxes[i].hi.X+minGap &&
			boxes[i].lo.Y <= boxes[j].hi.Y+minGap && boxes[j].lo.Y <= boxes[i].hi.Y+minGap
	}
	order := make([]int, len(g.roads))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(i, j int) int { return cmp.Compare(g.roads[i].len(), g.roads[j].len()) })

	dropped := make([]bool, len(g.roads))
	partner := make([]int, len(g.roads)) // the kept street a dropped one ran beside, or -1
	for i, r := range g.roads {
		partner[i] = -1
		// Loops go first, so that nothing is judged by what runs beside them.
		dropped[i] = folded(r)
	}
	for _, i := range order {
		a := g.roads[i]
		if dropped[i] {
			continue
		}
		var others []int
		for j := range g.roads {
			if j != i && !dropped[j] && overlap(i, j) {
				others = append(others, j)
			}
		}
		// Two streets joining the same junctions, never far apart: a lens
		// where a road splits around a traffic island.
		if j := slices.IndexFunc(others, func(j int) bool { return twin(a, g.roads[j]) }); j >= 0 {
			dropped[i], partner[i] = true, others[j]
			continue
		}
		ends := []sim.V2{a.pts[0], a.pts[len(a.pts)-1]}
		skip := math.Min(minGap, a.len()/4) // short roads can't keep clear of their ends
		pts := resample(a.pts, 2)
		near, total := 0, 0
		beside := map[int]int{}
		for k, p := range pts {
			if p.Dist(ends[0]) < skip || p.Dist(ends[1]) < skip {
				continue
			}
			total++
			tg := pts[min(k+1, len(pts)-1)].Sub(pts[max(k-1, 0)]).Norm()
			for _, j := range others {
				b := g.roads[j]
				d, seg, q := nearestOnLine(p, b.pts)
				if d >= minGap || math.Abs(tg.Dot(b.pts[seg+1].Sub(b.pts[seg]).Norm())) < 0.85 {
					continue
				}
				// Where the streets meet, one may carry on from the other:
				// then the nearest point is the shared node itself.
				if (b.a == a.a || b.a == a.b) && q.Dist(g.pos[b.a]) < 1 ||
					(b.b == a.a || b.b == a.b) && q.Dist(g.pos[b.b]) < 1 {
					continue
				}
				near++
				beside[j]++
				break
			}
		}
		if total == 0 || 2*near <= total {
			continue
		}
		dropped[i] = true
		for j, n := range beside {
			if partner[i] < 0 || n > beside[partner[i]] || (n == beside[partner[i]] && j < partner[i]) {
				partner[i] = j
			}
		}
	}

	all := g.roads
	g.roads = nil
	uf := unionFind{}
	used := map[int64]bool{}
	for i, r := range all {
		if !dropped[i] {
			g.roads = append(g.roads, r)
			uf.union(r.a, r.b)
			used[r.a], used[r.b] = true, true
		}
	}
	for _, i := range order {
		k := partner[i]
		for k >= 0 && dropped[k] {
			k = partner[k]
		}
		if k < 0 {
			continue
		}
		net := uf.find(all[k].a) // the kept street may since have been split
		for _, u := range []int64{all[i].a, all[i].b} {
			if !used[u] || uf.find(u) == net {
				continue
			}
			var best *road
			bestD, seg, q := math.Inf(1), 0, sim.V2{}
			for _, r := range g.roads {
				if uf.find(r.a) != uf.find(net) {
					continue
				}
				if d, s, p := nearestOnLine(g.pos[u], r.pts); d < bestD {
					best, bestD, seg, q = r, d, s, p
				}
			}
			if best == nil || bestD > 2*minGap {
				continue
			}
			w := g.split(best, seg, q)
			g.roads = append(g.roads, &road{a: u, b: w, pts: []sim.V2{g.pos[u], q}, name: all[i].name})
			uf.union(u, w)
			used[w] = true
		}
	}
}

// split cuts r at point q on its segment seg, returning the node there. A
// point close to either end uses the node already there.
func (g *graph) split(r *road, seg int, q sim.V2) int64 {
	if q.Dist(r.pts[0]) < 2 {
		return r.a
	}
	if q.Dist(r.pts[len(r.pts)-1]) < 2 {
		return r.b
	}
	w := g.newNode(q)
	rest := append([]sim.V2{q}, r.pts[seg+1:]...)
	g.roads = append(g.roads, &road{a: w, b: r.b, pts: rest, name: r.name})
	r.pts = append(slices.Clone(r.pts[:seg+1]), q)
	r.b = w
	return w
}

// twin reports whether a and b join the same two nodes and a never strays
// minGap from b.
func twin(a, b *road) bool {
	if !(a.a == b.a && a.b == b.b || a.a == b.b && a.b == b.a) {
		return false
	}
	for _, p := range resample(a.pts, 2) {
		if d, _, _ := nearestOnLine(p, b.pts); d >= minGap {
			return false
		}
	}
	return true
}

// folded reports whether most of r runs alongside another part of itself,
// like a U-shaped loop.
func folded(r *road) bool {
	pts := resample(r.pts, 4)
	const apart = 3 * minGap / 4 // samples at least 3·minGap apart along the road
	near := 0
	for i, p := range pts {
		for j, q := range pts {
			if (j-i >= apart || i-j >= apart) && p.Dist(q) < minGap {
				near++
				break
			}
		}
	}
	return 2*near > len(pts)
}

// resample returns points every step metres along a polyline.
func resample(pts []sim.V2, step float64) []sim.V2 {
	out := []sim.V2{pts[0]}
	carry := 0.0
	for i := 1; i < len(pts); i++ {
		a, b := pts[i-1], pts[i]
		l := a.Dist(b)
		for s := step - carry; s < l; s += step {
			out = append(out, a.Lerp(b, s/l))
		}
		carry = math.Mod(carry+l, step)
	}
	return append(out, pts[len(pts)-1])
}

// nearestOnLine finds the point q on a polyline nearest p, its distance and
// the index of the segment it is on.
func nearestOnLine(p sim.V2, pts []sim.V2) (dist float64, seg int, q sim.V2) {
	dist = math.Inf(1)
	for i := 1; i < len(pts); i++ {
		a, b := pts[i-1], pts[i]
		ab := b.Sub(a)
		t := 0.0
		if l2 := ab.Dot(ab); l2 > 0 {
			t = math.Max(0, math.Min(1, p.Sub(a).Dot(ab)/l2))
		}
		if c := a.Add(ab.Scale(t)); p.Dist(c) < dist {
			dist, seg, q = p.Dist(c), i-1, c
		}
	}
	return dist, seg, q
}

func (g *graph) degrees() map[int64]int {
	deg := map[int64]int{}
	for _, r := range g.roads {
		deg[r.a]++
		deg[r.b]++
	}
	return deg
}

// pruneStubs drops short dead ends, then joins roads that meet end to end
// with nothing else at the join. It reports whether anything was dropped.
func (g *graph) pruneStubs() bool {
	deg := g.degrees()
	pruned := false
	var out []*road
	for _, r := range g.roads {
		if (deg[r.a] == 1 || deg[r.b] == 1) && r.len() < minStub {
			pruned = true
			continue
		}
		out = append(out, r)
	}
	g.roads = out
	g.dissolve()
	return pruned
}

// dissolve joins pairs of roads that are the only two at a node.
func (g *graph) dissolve() {
	for {
		deg := g.degrees()
		joined := false
		for _, r1 := range g.roads {
			for _, v := range []int64{r1.a, r1.b} {
				if deg[v] != 2 {
					continue
				}
				j := slices.IndexFunc(g.roads, func(r *road) bool { return r != r1 && (r.a == v || r.b == v) })
				if j < 0 {
					continue
				}
				r2 := g.roads[j]
				if r1.a == v {
					r1.reverse()
				}
				if r2.b == v {
					r2.reverse()
				}
				if r1.a == r2.b {
					continue // joining would make a loop
				}
				if r2.len() > r1.len() {
					r1.name = r2.name
				}
				r1.pts = append(r1.pts, r2.pts[1:]...)
				r1.b = r2.b
				g.roads = slices.Delete(g.roads, j, j+1)
				joined = true
				break
			}
			if joined {
				break
			}
		}
		if !joined {
			return
		}
	}
}

// keepLargest drops every road not in the network with the most street
// length, returning the length kept and dropped.
func (g *graph) keepLargest() (kept, cut float64) {
	uf := unionFind{}
	for _, r := range g.roads {
		uf.union(r.a, r.b)
	}
	total := map[int64]float64{}
	for _, r := range g.roads {
		total[uf.find(r.a)] += r.len()
	}
	var best int64
	for root, l := range total {
		if l > total[best] || (l == total[best] && root < best) {
			best = root
		}
		cut += l
	}
	g.roads = slices.DeleteFunc(g.roads, func(r *road) bool { return uf.find(r.a) != best })
	return total[best], cut - total[best]
}

// roundCorners replaces each bend in a polyline with a curve of the given
// radius, or as tight as the neighbouring segments allow.
func roundCorners(pts []sim.V2, radius float64) []sim.V2 {
	pts = slices.CompactFunc(slices.Clone(pts), func(a, b sim.V2) bool { return a.Dist(b) < 0.5 })
	if len(pts) < 3 {
		return pts
	}
	out := []sim.V2{pts[0]}
	for i := 1; i < len(pts)-1; i++ {
		a, p, b := pts[i-1], pts[i], pts[i+1]
		u1, u2 := p.Sub(a).Norm(), b.Sub(p).Norm()
		turn := math.Acos(math.Max(-1, math.Min(1, u1.Dot(u2))))
		if turn < 0.05 {
			out = append(out, p)
			continue
		}
		d := math.Min(radius*math.Tan(turn/2), 0.5*math.Min(p.Dist(a), p.Dist(b)))
		q1, q2 := p.Sub(u1.Scale(d)), p.Add(u2.Scale(d))
		n := int(math.Ceil(turn/0.15)) + 1
		for k := 0; k <= n; k++ {
			t := float64(k) / float64(n)
			out = append(out, q1.Scale((1-t)*(1-t)).Add(p.Scale(2*t*(1-t))).Add(q2.Scale(t*t)))
		}
	}
	return append(out, pts[len(pts)-1])
}

func polyLen(pts []sim.V2) float64 {
	l := 0.0
	for i := 1; i < len(pts); i++ {
		l += pts[i].Dist(pts[i-1])
	}
	return l
}

type unionFind map[int64]int64

func (u unionFind) find(x int64) int64 {
	if _, ok := u[x]; !ok {
		u[x] = x
	}
	for u[x] != x {
		u[x] = u[u[x]]
		x = u[x]
	}
	return x
}

func (u unionFind) union(a, b int64) {
	ra, rb := u.find(a), u.find(b)
	if ra != rb {
		u[max(ra, rb)] = min(ra, rb)
	}
}
