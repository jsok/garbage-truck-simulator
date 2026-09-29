package sim

import (
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"sort"
)

// Street cross-section, measured from the road centreline. Traffic keeps to
// the left (as in Australia), so the truck's left side faces the kerb.
const (
	RoadHalfWidth  = 4.0  // kerb to centreline
	LaneOffset     = 2.0  // centre of the left lane
	BinKerbOffset  = 4.75 // bins sit on the nature strip, just past the kerb
	FootpathInner  = 6.0
	FootpathOuter  = 7.4
	BulbRadius     = 10.5 // paved radius of a cul-de-sac turning circle
	sampleSpacing  = 2.0
	junctionRadius = 13.0 // no bins, trees or markings this close to a junction
)

// Colour of a wheelie bin. They follow the Australian kerbside scheme.
type Colour int

const (
	Red    Colour = iota // general waste
	Yellow               // recycling
	Green                // garden organics
)

// Colours lists every bin colour in display order.
var Colours = [...]Colour{Red, Yellow, Green}

func (c Colour) String() string {
	switch c {
	case Red:
		return "RED"
	case Yellow:
		return "YELLOW"
	default:
		return "GREEN"
	}
}

// Node is a junction, bend or cul-de-sac end in the street network.
type Node struct {
	P     V2
	Roads []int
	Bulb  bool // a cul-de-sac turning circle
}

// Road is one street between two nodes, stored as a finely sampled centreline.
type Road struct {
	Name string
	A, B int
	Pts  []V2
	Cum  []float64 // arc length at each point
}

// Len is the arc length of the road in metres.
func (r *Road) Len() float64 { return r.Cum[len(r.Cum)-1] }

// At returns the point and unit tangent at arc length s.
func (r *Road) At(s float64) (V2, V2) {
	s = clamp(s, 0, r.Len())
	i := sort.SearchFloat64s(r.Cum, s)
	if i <= 0 {
		i = 1
	}
	if i >= len(r.Pts) {
		i = len(r.Pts) - 1
	}
	a, b := r.Pts[i-1], r.Pts[i]
	seg := r.Cum[i] - r.Cum[i-1]
	t := 0.0
	if seg > 0 {
		t = (s - r.Cum[i-1]) / seg
	}
	return a.Lerp(b, t), b.Sub(a).Norm()
}

// House is a suburban home facing the street.
type House struct {
	P       V2
	Heading float64 // direction the front door faces
	W, D    float64 // frontage width and depth
	Storeys int
	Wall    int // palette indices, interpreted by the renderer
	Roof    int
	Garage  bool
	Chimney bool
	Drive   [2]V2 // driveway centreline, kerb to garage
	Seed    uint64
}

// Spot is where a resident left their bin.
type Spot int

const (
	SpotKerb   Spot = iota // on the nature strip, as it should be
	SpotGutter             // right on the edge of the road
	SpotBack               // at the back of the footpath, a long reach away
	SpotRoad               // wheeled or blown out into the street
)

// Bin is a wheelie bin put out on the kerb.
type Bin struct {
	Home        V2 // where the resident left it
	Pos         V2 // where it is now (trucks can nudge bins)
	Vel         V2 // sliding after being knocked
	Heading     float64
	Colour      Colour
	Spot        Spot
	Overflowing bool // rubbish piled up under a propped-open lid
	Collected   bool
	Held        bool // currently in the arm's grip
	Fallen      bool // lying on its back; a full bin that falls over is lost
}

// FallDir is the direction the top of a fallen bin points. A fallen bin
// lies on its back, so it faces straight up with its front towards the sky.
func (b *Bin) FallDir() V2 { return Dir(b.Heading + math.Pi) }

// body is the circle a bin occupies; lying down, it reaches further.
func (b *Bin) body() (V2, float64) {
	if b.Fallen {
		return b.Pos.Add(b.FallDir().Scale(0.5)), 0.45
	}
	return b.Pos, BinRadius
}

// Tree is decorative greenery; its trunk is an obstacle.
type Tree struct {
	P      V2
	Height float64
	Radius float64
	Kind   int
	Street bool
}

// Pole is a timber power pole on the nature strip. Consecutive poles along
// the same road (Road, in order of S) carry overhead wires between them.
type Pole struct {
	P     V2
	Road  int
	S     float64
	Side  float64 // +1 left of the road's direction, -1 right
	Light bool    // carries a street light over the road
}

// Obstacle blocks the truck: either a circle (Radius > 0) or an oriented box.
type Obstacle struct {
	P            V2
	Heading      float64
	HalfW, HalfD float64
	Radius       float64
}

type roadSeg struct {
	road int
	i    int // segment Pts[i]..Pts[i+1]
}

// Town is a generated suburb.
type Town struct {
	Seed     int64
	Min, Max V2 // drivable bounds
	Nodes    []Node
	Roads    []Road
	Houses   []House
	Bins     []Bin
	Trees    []Tree
	Poles    []Pole
	Obs      []Obstacle

	StartPos     V2
	StartHeading float64

	segs    []roadSeg
	segGrid *grid
	obsGrid *grid
}

var streetNames = []string{
	"Wattle St", "Banksia Cres", "Kookaburra Ave", "Jacaranda Dr", "Grevillea Ct",
	"Waratah Rd", "Bottlebrush Way", "Currawong Cl", "Lorikeet Pde", "Magpie Ln",
	"Eucalypt Gr", "Boronia St", "Acacia Rd", "Possum Pl", "Wombat Way",
	"Echidna Ct", "Galah Cres", "Kurrajong Ave", "Melaleuca Dr", "Ironbark Rd",
	"Blackwood St", "Casuarina Cl", "Frangipani Ct", "Bilby Pl", "Rosella St",
	"Paperbark Pde", "Emu Ave", "Tea Tree Ln", "Wallaby Way", "Coolabah Rd",
	"Lilly Pilly Gr", "Quandong St", "Kangaroo Cres", "Platypus Pl", "Bellbird Dr",
	"Brolga Ave", "Cockatoo Ct", "Mulga Rd", "Sassafras St", "Dingo Ln",
}

// Generate builds a suburb deterministically from seed.
func Generate(seed int64) *Town {
	for attempt := uint64(0); attempt < 500; attempt++ {
		if t := tryGenerate(seed, attempt); t != nil {
			return t
		}
	}
	panic("sim: could not generate a town")
}

func tryGenerate(seed int64, attempt uint64) *Town {
	rng := rand.New(rand.NewPCG(uint64(seed), attempt*0x9E3779B97F4A7C15+1))
	const cols, rows, spacing = 6, 5, 105.0
	t := &Town{Seed: seed}
	t.Min = V2{-0.6 * spacing, -0.6 * spacing}
	t.Max = V2{(cols - 1 + 0.6) * spacing, (rows - 1 + 0.6) * spacing}

	// Jittered grid of junctions, with a couple of interior holes so that some
	// blocks are much larger than others.
	idx := make([][]int, cols)
	holes := map[[2]int]bool{}
	for n := rng.IntN(3); n > 0; n-- {
		holes[[2]int{1 + rng.IntN(cols-2), 1 + rng.IntN(rows-2)}] = true
	}
	for i := range cols {
		idx[i] = make([]int, rows)
		for j := range rows {
			idx[i][j] = -1
			if holes[[2]int{i, j}] {
				continue
			}
			jit := 0.26 * spacing
			p := V2{float64(i)*spacing + (rng.Float64()*2-1)*jit, float64(j)*spacing + (rng.Float64()*2-1)*jit}
			idx[i][j] = len(t.Nodes)
			t.Nodes = append(t.Nodes, Node{P: p})
		}
	}

	type edge struct{ a, b int }
	var cand []edge
	for i := range cols {
		for j := range rows {
			a := idx[i][j]
			if a < 0 {
				continue
			}
			if i+1 < cols && idx[i+1][j] >= 0 {
				cand = append(cand, edge{a, idx[i+1][j]})
			}
			if j+1 < rows && idx[i][j+1] >= 0 {
				cand = append(cand, edge{a, idx[i][j+1]})
			}
			if i+1 < cols && j+1 < rows && rng.Float64() < 0.14 {
				if rng.Float64() < 0.5 {
					if b := idx[i+1][j+1]; b >= 0 {
						cand = append(cand, edge{a, b})
					}
				} else if c, d := idx[i+1][j], idx[i][j+1]; c >= 0 && d >= 0 {
					cand = append(cand, edge{c, d})
				}
			}
		}
	}
	rng.Shuffle(len(cand), func(i, j int) { cand[i], cand[j] = cand[j], cand[i] })

	// Random spanning tree keeps everything connected; extra edges add loops.
	parent := make([]int, len(t.Nodes))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(x int) int {
		for parent[x] != x {
			parent[x] = parent[parent[x]]
			x = parent[x]
		}
		return x
	}
	var chosen []edge
	crosses := func(e edge) bool {
		for _, c := range chosen {
			if c.a == e.a || c.a == e.b || c.b == e.a || c.b == e.b {
				continue
			}
			if segsIntersect(t.Nodes[e.a].P, t.Nodes[e.b].P, t.Nodes[c.a].P, t.Nodes[c.b].P) {
				return true
			}
		}
		return false
	}
	var extra []edge
	for _, e := range cand {
		if ra, rb := find(e.a), find(e.b); ra != rb && !crosses(e) {
			parent[ra] = rb
			chosen = append(chosen, e)
		} else {
			extra = append(extra, e)
		}
	}
	if len(chosen) != len(t.Nodes)-1 {
		return nil
	}
	for _, e := range extra {
		if rng.Float64() < 0.5 && !crosses(e) {
			chosen = append(chosen, e)
		}
	}
	for _, e := range chosen {
		t.addRoad(e.a, e.b)
	}

	// Cul-de-sacs sprout into the widest gap around a junction.
	for tries, culs := 0, 0; tries < 30 && culs < 5; tries++ {
		n := rng.IntN(len(t.Nodes))
		if t.Nodes[n].Bulb || len(t.Nodes[n].Roads) > 3 {
			continue
		}
		dir, gap := t.widestGap(n)
		if gap < 1.9 {
			continue
		}
		dir += (rng.Float64()*2 - 1) * 0.2
		end := t.Nodes[n].P.Add(Dir(dir).Scale(42 + rng.Float64()*22))
		if end.X < t.Min.X+25 || end.Y < t.Min.Y+25 || end.X > t.Max.X-25 || end.Y > t.Max.Y-25 {
			continue
		}
		ok := true
		for ri := range t.Roads {
			r := &t.Roads[ri]
			a, b := t.Nodes[r.A].P, t.Nodes[r.B].P
			if d, _ := segDist(end, a, b); d < 38 {
				ok = false
			}
			if r.A != n && r.B != n && segsIntersect(t.Nodes[n].P, end, a, b) {
				ok = false
			}
		}
		if !ok {
			continue
		}
		t.Nodes = append(t.Nodes, Node{P: end, Bulb: true})
		t.addRoad(n, len(t.Nodes)-1)
		culs++
	}

	// Every dead end gets a turning circle, with the bulb at the road's B end.
	for ni := range t.Nodes {
		n := &t.Nodes[ni]
		if len(n.Roads) != 1 {
			continue
		}
		n.Bulb = true
		if r := &t.Roads[n.Roads[0]]; r.A == ni {
			r.A, r.B = r.B, r.A
		}
	}

	// Bend every street into a gentle curve; retry straighter if that makes
	// streets collide.
	for ri := range t.Roads {
		curvy := 0.8
		if rng.Float64() < 0.2 {
			curvy = 0.1
		}
		t.shapeRoad(ri, rng, curvy)
	}
	// Re-roll clashing streets, getting straighter each pass.
	for _, curvy := range []float64{0.8, 0.8, 0.6, 0.4, 0.2, 0} {
		bad := t.badRoads()
		if len(bad) == 0 {
			break
		}
		for _, ri := range bad {
			t.shapeRoad(ri, rng, curvy)
		}
	}
	if len(t.badRoads()) > 0 {
		return nil
	}

	t.nameRoads(rng)
	if t.furnish(rng, 40) != nil {
		return nil
	}
	return t
}

// nameRoads gives every unnamed road a made-up street name.
func (t *Town) nameRoads(rng *rand.Rand) {
	names := rng.Perm(len(streetNames))
	for ri := range t.Roads {
		if t.Roads[ri].Name == "" {
			t.Roads[ri].Name = streetNames[names[ri%len(names)]]
		}
	}
}

// furnish fills in everything that is not street layout: houses, bins, poles,
// trees and the truck's starting point. It fails if the result has fewer than
// minBins bins or nowhere to start.
func (t *Town) furnish(rng *rand.Rand, minBins int) error {
	t.indexRoads()
	t.placeHouses(rng)
	t.placePoles(rng)
	t.placeTrees(rng)
	if len(t.Bins) < minBins {
		return fmt.Errorf("sim: only room for %d bins, need %d", len(t.Bins), minBins)
	}
	t.indexObstacles()
	if !t.chooseStart() {
		return errors.New("sim: no street is long enough to start on")
	}
	t.scatterBins(rng)
	return nil
}

func (t *Town) addRoad(a, b int) {
	ri := len(t.Roads)
	t.Roads = append(t.Roads, Road{A: a, B: b})
	t.Nodes[a].Roads = append(t.Nodes[a].Roads, ri)
	t.Nodes[b].Roads = append(t.Nodes[b].Roads, ri)
}

// widestGap finds the bisector and size of the largest angular gap between
// the (straight-line) roads leaving node n.
func (t *Town) widestGap(n int) (float64, float64) {
	var angles []float64
	for _, ri := range t.Nodes[n].Roads {
		r := &t.Roads[ri]
		other := r.B
		if other == n {
			other = r.A
		}
		angles = append(angles, t.Nodes[other].P.Sub(t.Nodes[n].P).Angle())
	}
	if len(angles) == 0 {
		return 0, 2 * math.Pi
	}
	sort.Float64s(angles)
	best, bestDir := 0.0, 0.0
	for i, a := range angles {
		next := angles[(i+1)%len(angles)]
		if i == len(angles)-1 {
			next += 2 * math.Pi
		}
		if gap := next - a; gap > best {
			best, bestDir = gap, a+gap/2
		}
	}
	return bestDir, best
}

// shapeRoad lays a cubic Bézier between the road's nodes. curvy scales how
// far the end tangents may swing away from the straight line.
func (t *Town) shapeRoad(ri int, rng *rand.Rand, curvy float64) {
	r := &t.Roads[ri]
	a, b := t.Nodes[r.A].P, t.Nodes[r.B].P
	u := b.Sub(a).Norm()
	l := a.Dist(b)
	a1 := (0.4 + 0.6*rng.Float64()) * curvy
	if rng.Float64() < 0.5 {
		a1 = -a1
	}
	a2 := -a1 // symmetric arc
	if rng.Float64() < 0.35 {
		a2 = a1 * (rng.Float64()*2 - 1) // lopsided or S-bend
	}
	c1 := a.Add(u.Rot(a1).Scale(l / 3))
	c2 := b.Sub(u.Rot(a2).Scale(l / 3))
	var raw []V2
	for i := 0; i <= 96; i++ {
		s := float64(i) / 96
		m := 1 - s
		p := a.Scale(m * m * m).Add(c1.Scale(3 * m * m * s)).Add(c2.Scale(3 * m * s * s)).Add(b.Scale(s * s * s))
		raw = append(raw, p)
	}
	r.Pts, r.Cum = resample(raw, sampleSpacing)
}

func resample(raw []V2, step float64) ([]V2, []float64) {
	cum := make([]float64, len(raw))
	for i := 1; i < len(raw); i++ {
		cum[i] = cum[i-1] + raw[i].Dist(raw[i-1])
	}
	total := cum[len(cum)-1]
	n := max(2, int(math.Ceil(total/step))+1)
	pts := make([]V2, n)
	out := make([]float64, n)
	j := 1
	for i := range n {
		s := total * float64(i) / float64(n-1)
		for j < len(raw)-1 && cum[j] < s {
			j++
		}
		seg := cum[j] - cum[j-1]
		f := 0.0
		if seg > 0 {
			f = (s - cum[j-1]) / seg
		}
		pts[i] = raw[j-1].Lerp(raw[j], f)
		out[i] = s
	}
	return pts, out
}

// badRoads returns roads that come too close to another road away from the
// junction they share, leave the map, or meet another road at too sharp an
// angle.
func (t *Town) badRoads() []int {
	bad := map[int]bool{}
	for i := range t.Roads {
		ri := &t.Roads[i]
		for _, p := range ri.Pts {
			if p.X < t.Min.X+12 || p.Y < t.Min.Y+12 || p.X > t.Max.X-12 || p.Y > t.Max.Y-12 {
				bad[i] = true
				break
			}
		}
		for j := i + 1; j < len(t.Roads); j++ {
			rj := &t.Roads[j]
			shared := -1
			for _, n := range []int{ri.A, ri.B} {
				if n == rj.A || n == rj.B {
					shared = n
				}
			}
			if roadsClash(t, ri, rj, shared) {
				bad[i], bad[j] = true, true
			}
		}
	}
	for n := range t.Nodes {
		var dirs []V2
		var ids []int
		for _, ri := range t.Nodes[n].Roads {
			r := &t.Roads[ri]
			var d V2
			if r.A == n {
				d = r.Pts[min(4, len(r.Pts)-1)].Sub(r.Pts[0])
			} else {
				d = r.Pts[max(0, len(r.Pts)-5)].Sub(r.Pts[len(r.Pts)-1])
			}
			dirs = append(dirs, d.Norm())
			ids = append(ids, ri)
		}
		for a := range dirs {
			for b := a + 1; b < len(dirs); b++ {
				if dirs[a].Dot(dirs[b]) > math.Cos(40*math.Pi/180) {
					bad[ids[a]], bad[ids[b]] = true, true
				}
			}
		}
	}
	out := make([]int, 0, len(bad))
	for ri := range bad {
		out = append(out, ri)
	}
	sort.Ints(out)
	return out
}

func roadsClash(t *Town, a, b *Road, shared int) bool {
	const minGap = 26.0 // leaves room for a row of houses between streets
	for _, p := range a.Pts {
		for _, q := range b.Pts {
			d := p.Dist(q)
			if d >= minGap {
				continue
			}
			if shared >= 0 {
				np := t.Nodes[shared].P
				// Near their shared junction the roads are allowed to meet;
				// further out they must diverge.
				if p.Dist(np) < 48 && q.Dist(np) < 48 {
					continue
				}
			}
			return true
		}
	}
	return false
}

func (t *Town) indexRoads() {
	t.segs = t.segs[:0]
	t.segGrid = newGrid(24)
	for ri := range t.Roads {
		r := &t.Roads[ri]
		for i := 0; i+1 < len(r.Pts); i++ {
			a, b := r.Pts[i], r.Pts[i+1]
			id := len(t.segs)
			t.segs = append(t.segs, roadSeg{ri, i})
			t.segGrid.insert(id, V2{math.Min(a.X, b.X), math.Min(a.Y, b.Y)}, V2{math.Max(a.X, b.X), math.Max(a.Y, b.Y)})
		}
	}
}

// RoadDist is the distance from p to the nearest paved centreline, treating
// cul-de-sac bulbs as widened road ends (so the kerb is always RoadHalfWidth).
// Results beyond 40m are reported as 40.
func (t *Town) RoadDist(p V2) float64 {
	d, _, _ := t.NearestRoad(p)
	return d
}

// NearestRoad returns the distance to the nearest road, its index and the arc
// length along it. Road index is -1 if nothing is within 40m.
func (t *Town) NearestRoad(p V2) (dist float64, road int, s float64) {
	return t.nearestRoadWithin(p, 40)
}

// RoadDistWithin is RoadDist with a shorter search radius, for bulk queries.
// Distances beyond reach are reported as reach.
func (t *Town) RoadDistWithin(p V2, reach float64) float64 {
	d, _, _ := t.nearestRoadWithin(p, reach)
	return d
}

func (t *Town) nearestRoadWithin(p V2, reach float64) (dist float64, road int, s float64) {
	dist, road = reach, -1
	t.segGrid.around(p, reach, func(id int) {
		sg := t.segs[id]
		r := &t.Roads[sg.road]
		d, f := segDist(p, r.Pts[sg.i], r.Pts[sg.i+1])
		if d < dist {
			dist, road = d, sg.road
			s = r.Cum[sg.i] + f*(r.Cum[sg.i+1]-r.Cum[sg.i])
		}
	})
	for ni := range t.Nodes {
		n := &t.Nodes[ni]
		if !n.Bulb {
			continue
		}
		if d := p.Dist(n.P) - (BulbRadius - RoadHalfWidth); d < dist {
			dist = math.Max(d, 0)
			road = n.Roads[0]
			s = t.Roads[road].Len()
		}
	}
	return dist, road, s
}

func (t *Town) nearJunction(p V2, r float64) bool {
	for _, n := range t.Nodes {
		if !n.Bulb && len(n.Roads) > 1 && p.Dist(n.P) < r {
			return true
		}
	}
	return false
}

func (t *Town) inBounds(p V2, margin float64) bool {
	return p.X > t.Min.X+margin && p.Y > t.Min.Y+margin && p.X < t.Max.X-margin && p.Y < t.Max.Y-margin
}

func (t *Town) placeHouses(rng *rand.Rand) {
	for ri := range t.Roads {
		r := &t.Roads[ri]
		end := r.Len() - 12
		if t.Nodes[r.B].Bulb {
			end = r.Len() - BulbRadius - 5
		}
		for _, side := range []float64{1, -1} {
			for s := 12 + rng.Float64()*8; s < end; s += 19 + rng.Float64()*8 {
				p, tg := r.At(s)
				n := tg.Left().Scale(side)
				t.tryHouse(rng, p, n, tg)
			}
		}
		if bulb := t.Nodes[r.B]; bulb.Bulb {
			_, tg := r.At(r.Len())
			for _, a := range []float64{-1.25, -0.42, 0.42, 1.25} {
				n := tg.Rot(a)
				p := bulb.P.Add(n.Scale(BulbRadius - RoadHalfWidth))
				t.tryHouse(rng, p, n, n.Right())
			}
		}
	}
}

// tryHouse attempts to build a house on the lot whose frontage is at road
// point p, with n pointing away from the road and tg along it.
func (t *Town) tryHouse(rng *rand.Rand, p, n, tg V2) {
	h := House{
		W:       9 + rng.Float64()*3.5,
		D:       8 + rng.Float64()*2.5,
		Storeys: 1,
		Wall:    rng.IntN(8),
		Roof:    rng.IntN(5),
		Garage:  rng.Float64() < 0.55,
		Chimney: rng.Float64() < 0.3,
		Seed:    rng.Uint64(),
	}
	if rng.Float64() < 0.25 {
		h.Storeys = 2
	}
	setback := RoadHalfWidth + 7 + rng.Float64()*3 + h.D/2
	h.P = p.Add(n.Scale(setback))
	h.Heading = n.Scale(-1).Angle()
	if !t.inBounds(h.P, 10) {
		return
	}
	right := tg // house local right when facing the road is -tg... sign doesn't matter for footprint
	for _, cx := range []float64{-1, 1} {
		for _, cy := range []float64{-1, 1} {
			c := h.P.Add(right.Scale(cx * h.W / 2)).Add(n.Scale(cy * h.D / 2))
			if t.RoadDist(c) < RoadHalfWidth+5.5 {
				return
			}
		}
	}
	if t.RoadDist(h.P) < setback-0.6 {
		return
	}
	r := 0.5*math.Max(h.W, h.D) + 1.2
	for _, o := range t.Houses {
		if o.P.Dist(h.P) < r+0.5*math.Max(o.W, o.D)+1.2 {
			return
		}
	}

	flip := 1.0
	if rng.Float64() < 0.5 {
		flip = -1
	}
	driveAlong := flip * (h.W/2 - 2.2)
	front := h.P.Sub(n.Scale(h.D / 2))
	h.Drive = [2]V2{p.Add(n.Scale(RoadHalfWidth)).Add(tg.Scale(driveAlong)), front.Add(tg.Scale(driveAlong))}
	t.Houses = append(t.Houses, h)
	t.Obs = append(t.Obs, Obstacle{P: h.P, Heading: h.Heading, HalfW: h.W / 2, HalfD: h.D / 2})

	if rng.Float64() > 0.62 {
		return
	}
	bin := p.Add(n.Scale(BinKerbOffset)).Sub(tg.Scale(flip * (1.2 + rng.Float64()*1.6)))
	if t.RoadDist(bin) < BinKerbOffset-0.3 || t.nearJunction(bin, junctionRadius) {
		return
	}
	t.Bins = append(t.Bins, Bin{
		Home:    bin,
		Pos:     bin,
		Heading: n.Scale(-1).Angle() + (rng.Float64()*2-1)*0.25,
		Colour:  Colours[rng.IntN(len(Colours))],
	})
}

// scatterBins makes some bins awkward: overflowing, or left somewhere other
// than the kerb.
func (t *Town) scatterBins(rng *rand.Rand) {
	for i := range t.Bins {
		b := &t.Bins[i]
		b.Overflowing = rng.Float64() < 0.2
		spot, off := SpotKerb, 0.0
		switch r := rng.Float64(); {
		case r < 0.07:
			spot, off = SpotRoad, -0.6+rng.Float64()*1.8
		case r < 0.16:
			spot, off = SpotGutter, 3.5+rng.Float64()*0.3
		case r < 0.26:
			spot, off = SpotBack, 6.9+rng.Float64()*0.7
		default:
			continue
		}
		_, road, at := t.NearestRoad(b.Home)
		p, tg := t.Roads[road].At(at)
		n := tg.Left()
		if b.Home.Sub(p).Dot(n) < 0 {
			n = n.Scale(-1)
		}
		q := p.Add(n.Scale(off))
		if !t.binSpotOK(i, q, spot, off) {
			continue
		}
		b.Home, b.Pos, b.Spot = q, q, spot
		if spot == SpotRoad {
			b.Heading = rng.Float64() * 2 * math.Pi
		}
	}
}

// binSpotOK reports whether bin i can be moved to q, off metres from its
// road's centreline.
func (t *Town) binSpotOK(i int, q V2, spot Spot, off float64) bool {
	if q.Dist(t.StartPos) < 45 || t.nearJunction(q, junctionRadius) {
		return false
	}
	if spot != SpotRoad && t.RoadDist(q) < off-0.2 {
		return false // closer to some other road
	}
	for j, o := range t.Bins {
		if j != i && o.Home.Dist(q) < 1.4 {
			return false
		}
	}
	clear := true
	t.obsGrid.around(q, BinRadius+9, func(id int) {
		if circlePush(q, BinRadius+0.3, &t.Obs[id]) != (V2{}) {
			clear = false
		}
	})
	return clear
}

func (t *Town) clearOfBinsAndDrives(p V2, r float64) bool {
	for _, b := range t.Bins {
		if b.Home.Dist(p) < r {
			return false
		}
	}
	for _, h := range t.Houses {
		if d, _ := segDist(p, h.Drive[0], h.Drive[1]); d < r*0.8 {
			return false
		}
	}
	return true
}

func (t *Town) insideHouse(p V2, margin float64) bool {
	for _, h := range t.Houses {
		f := Dir(h.Heading)
		d := p.Sub(h.P)
		if math.Abs(d.Dot(f)) < h.D/2+margin && math.Abs(d.Dot(f.Left())) < h.W/2+margin {
			return true
		}
	}
	return false
}

// Tree kinds.
const (
	TreeLeafy = iota
	TreeGum
	TreeConifer
	TreePalm
)

// placePoles runs a line of power poles down one side of every street.
func (t *Town) placePoles(rng *rand.Rand) {
	for ri := range t.Roads {
		r := &t.Roads[ri]
		side := 1.0
		if rng.Float64() < 0.5 {
			side = -1
		}
		lit := false
		for s := 8 + rng.Float64()*6; s < r.Len()-6; {
			p, tg := r.At(s)
			q := p.Add(tg.Left().Scale(side * (RoadHalfWidth + 1.4)))
			if t.OtherRoadDist(q, ri) < RoadHalfWidth+2 || t.nearJunction(q, junctionRadius-2) || !t.clearOfBinsAndDrives(q, 3.2) {
				s += 3 // shuffle along until there is room
				continue
			}
			lit = !lit
			t.Poles = append(t.Poles, Pole{P: q, Road: ri, S: s, Side: side, Light: lit})
			t.Obs = append(t.Obs, Obstacle{P: q, Radius: 0.2})
			s += 34 + rng.Float64()*8
		}
	}
}

func (t *Town) placeTrees(rng *rand.Rand) {
	add := func(tr Tree) {
		t.Trees = append(t.Trees, tr)
		t.Obs = append(t.Obs, Obstacle{P: tr.P, Radius: 0.45})
	}
	farFromTrees := func(p V2, r float64) bool {
		for _, o := range t.Trees {
			if o.P.Dist(p) < r {
				return false
			}
		}
		for _, o := range t.Poles {
			if o.P.Dist(p) < 3.5 {
				return false
			}
		}
		return true
	}
	// Street trees on the nature strip.
	for ri := range t.Roads {
		r := &t.Roads[ri]
		for _, side := range []float64{1, -1} {
			for s := 6 + rng.Float64()*10; s < r.Len()-6; s += 13 + rng.Float64()*20 {
				p, tg := r.At(s)
				q := p.Add(tg.Left().Scale(side * (RoadHalfWidth + 1.3 + rng.Float64()*0.5)))
				if t.RoadDist(q) < RoadHalfWidth+1 || t.nearJunction(q, junctionRadius) || !t.clearOfBinsAndDrives(q, 4) || !farFromTrees(q, 6) {
					continue
				}
				add(Tree{P: q, Height: 4.5 + rng.Float64()*2.5, Radius: 1.8 + rng.Float64()*0.8, Kind: rng.IntN(2), Street: true})
			}
		}
	}
	// Garden and park trees fill whatever space is left, sampled at the same
	// density as in a procedural suburb (651m by 546m) however big the map is.
	area := (t.Max.X - t.Min.X) * (t.Max.Y - t.Min.Y)
	for range int(math.Max(3500, 3500*area/(651*546))) {
		p := V2{t.Min.X + rng.Float64()*(t.Max.X-t.Min.X), t.Min.Y + rng.Float64()*(t.Max.Y-t.Min.Y)}
		if t.RoadDist(p) < 9.5 || t.insideHouse(p, 2.5) || !farFromTrees(p, 5.5) || !t.clearOfBinsAndDrives(p, 5) {
			continue
		}
		kind := rng.IntN(3)
		if rng.Float64() < 0.07 {
			kind = TreePalm
		}
		add(Tree{P: p, Height: 5 + rng.Float64()*6, Radius: 2 + rng.Float64()*2, Kind: kind})
	}
	// A bushland border hides the edge of the world.
	per := []struct{ a, b V2 }{
		{t.Min, V2{t.Max.X, t.Min.Y}}, {V2{t.Max.X, t.Min.Y}, t.Max},
		{t.Max, V2{t.Min.X, t.Max.Y}}, {V2{t.Min.X, t.Max.Y}, t.Min},
	}
	for _, e := range per {
		l := e.a.Dist(e.b)
		u := e.b.Sub(e.a).Norm()
		for row := 0; row < 3; row++ {
			for s := 0.0; s < l; s += 6 + rng.Float64()*4 {
				p := e.a.Add(u.Scale(s)).Add(u.Left().Scale(4 + float64(row)*7 + rng.Float64()*4))
				t.Trees = append(t.Trees, Tree{P: p, Height: 7 + rng.Float64()*6, Radius: 2.5 + rng.Float64()*2, Kind: rng.IntN(3)})
			}
		}
	}
}

func (t *Town) indexObstacles() {
	t.obsGrid = newGrid(16)
	for i, o := range t.Obs {
		r := o.Radius
		if r == 0 {
			r = math.Hypot(o.HalfW, o.HalfD)
		}
		t.obsGrid.insert(i, V2{o.P.X - r, o.P.Y - r}, V2{o.P.X + r, o.P.Y + r})
	}
}

func (t *Town) chooseStart() bool {
	best := -1
	for ri := range t.Roads {
		r := &t.Roads[ri]
		if t.Nodes[r.A].Bulb || t.Nodes[r.B].Bulb || r.Len() < 70 {
			continue
		}
		// Prefer a street near the middle of the map.
		c := t.Min.Lerp(t.Max, 0.5)
		if best < 0 || r.Pts[len(r.Pts)/2].Dist(c) < t.Roads[best].Pts[len(t.Roads[best].Pts)/2].Dist(c) {
			best = ri
		}
	}
	if best < 0 {
		// Real street maps may have no long through road: settle for the
		// longest street. Bulbs are always at B, so the start stays clear.
		for ri := range t.Roads {
			if l := t.Roads[ri].Len(); l >= 40 && (best < 0 || l > t.Roads[best].Len()) {
				best = ri
			}
		}
	}
	if best < 0 {
		return false
	}
	p, tg := t.Roads[best].At(18)
	t.StartPos = p.Add(tg.Left().Scale(LaneOffset))
	t.StartHeading = tg.Angle()
	return true
}

// OtherRoadDist is the distance from p to the centreline of the nearest road
// other than except (ignoring cul-de-sac bulbs). Distances beyond 40m are
// reported as 40.
func (t *Town) OtherRoadDist(p V2, except int) float64 {
	dist := 40.0
	t.segGrid.around(p, dist, func(id int) {
		sg := t.segs[id]
		if sg.road == except {
			return
		}
		r := &t.Roads[sg.road]
		if d, _ := segDist(p, r.Pts[sg.i], r.Pts[sg.i+1]); d < dist {
			dist = d
		}
	})
	return dist
}
