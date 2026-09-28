package meshgen

import (
	"math"

	"github.com/jsok/garbage-truck-simulator/internal/sim"
)

// Palette.
var (
	Asphalt  = Hex(0x46484d)
	Footpath = Hex(0xbdb9ae)
	Kerb     = Hex(0xcfcbc0)
	Concrete = Hex(0xb0aca1)
	Paint    = Hex(0xf2f2ea)
	Glass    = Hex(0x3a4c60)
	Trim     = Hex(0xf2f0ea)
	Timber   = Hex(0x6e5a44)

	walls = []RGBA{Hex(0xede3c8), Hex(0xf2dfa0), Hex(0xb9d3e0), Hex(0xb8c9a3),
		Hex(0xe8b49a), Hex(0xf4f2ec), Hex(0xa65a42), Hex(0xc89b6d)}
	roofs  = []RGBA{Hex(0xb5553a), Hex(0x4a4e54), Hex(0x5b6d82), Hex(0x7a4a36), Hex(0x5e7a5a)}
	doors  = []RGBA{Hex(0x8a2b2b), Hex(0x2b4a8a), Hex(0x2f6b3f), Hex(0xe0b030), Hex(0x3a3a3a)}
	fences = []RGBA{Hex(0x55604f), Hex(0x6c6a5f), Hex(0xb9b2a0), Hex(0x3e4a52)}
	leaf   = []RGBA{Hex(0x4e8a3a), Hex(0x3f7a34), Hex(0x6c9a45), Hex(0x5d8f3c)}
	gum    = []RGBA{Hex(0x7e9a6a), Hex(0x8aa377), Hex(0x6d8c5e)}
	pine   = []RGBA{Hex(0x2f5e34), Hex(0x3a6b3c)}
	bloom  = []RGBA{Hex(0xe84a6a), Hex(0xf4d23c), Hex(0xf2f2f2), Hex(0x9a6ad8), Hex(0xf08a2c)}
)

// Chunks buckets geometry by map area so the renderer can cull it.
type Chunks map[[2]int]*Mesh

const chunkSize = 110.0

func (c Chunks) at(p sim.V2) *Mesh {
	k := [2]int{int(math.Floor(p.X / chunkSize)), int(math.Floor(p.Y / chunkSize))}
	m := c[k]
	if m == nil {
		m = &Mesh{}
		c[k] = m
	}
	return m
}

// At maps a ground-plane point to 3D at height y.
func At(p sim.V2, y float64) V3 { return v3(p.X, y, p.Y) }

func ground(v V3) sim.V2 { return sim.V2{X: float64(v.X), Y: float64(v.Z)} }

// TownMeshes holds the static scenery.
type TownMeshes struct {
	Ground  Chunks // receives shadows only
	Scenery Chunks // houses, trees, roads
	Far     *Mesh  // hills on the horizon
}

// Heights of the flat layers, far enough apart to avoid z-fighting.
const (
	yFootpath = 0.018
	yDrive    = 0.03
	yRoad     = 0.04
	yGutter   = 0.046
	yLine     = 0.052
	kerbH     = 0.15
)

// rng is a tiny deterministic generator for decorative variation.
type rng uint64

func (r *rng) next() float64 {
	*r = *r*6364136223846793005 + 1442695040888963407
	return float64(*r>>11) / float64(1<<53)
}

func (r *rng) pick(p []RGBA) RGBA { return p[int(r.next()*float64(len(p)))%len(p)] }

// BuildTown generates all static geometry for a suburb.
func BuildTown(t *sim.Town) TownMeshes {
	tm := TownMeshes{Ground: Chunks{}, Scenery: Chunks{}, Far: &Mesh{}}
	buildGround(t, tm.Ground)
	buildHills(t, tm.Far)
	for ri := range t.Roads {
		buildRoad(t, ri, tm.Scenery)
	}
	for _, n := range t.Nodes {
		if n.Bulb {
			buildBulb(t, n, tm.Scenery)
		} else if len(n.Roads) > 1 {
			m := tm.Scenery.at(n.P)
			m.Mat = MatAsphalt
			m.Disc(At(n.P, yRoad), sim.RoadHalfWidth*1.35, 24, Asphalt)
			m.Mat = MatPlain
		}
	}
	for i := range t.Houses {
		buildHouse(t, &t.Houses[i], tm.Scenery.at(t.Houses[i].P))
	}
	for i, tr := range t.Trees {
		buildTree(tr, uint64(i)*2654435761+uint64(t.Seed), tm.Scenery.at(tr.P))
	}
	buildPoles(t, tm.Scenery)
	return tm
}

func hash2(x, y int) float64 {
	h := uint32(x)*374761393 + uint32(y)*668265263
	h = (h ^ (h >> 13)) * 1274126177
	return float64(h^(h>>16)) / float64(math.MaxUint32)
}

// noise is smooth value noise in 0..1.
func noise(x, y float64) float64 {
	xi, yi := math.Floor(x), math.Floor(y)
	fx, fy := x-xi, y-yi
	fx, fy = fx*fx*(3-2*fx), fy*fy*(3-2*fy)
	a := hash2(int(xi), int(yi))
	b := hash2(int(xi)+1, int(yi))
	c := hash2(int(xi), int(yi)+1)
	d := hash2(int(xi)+1, int(yi)+1)
	return a + (b-a)*fx + (c-a)*fy + (a-b-c+d)*fx*fy
}

func grassAt(x, z float64) RGBA {
	n := 0.6*noise(x/37, z/37) + 0.4*noise(x/9, z/9)
	return Hex(0x4f8639).Mix(Hex(0x7d9f48), float32(n)).Shade(float32(0.94 + 0.1*noise(x/3.1, z/3.1)))
}

func buildGround(t *sim.Town, out Chunks) {
	const cell = 6.0
	lo, hi := t.Min.Sub(sim.V2{X: 260, Y: 260}), t.Max.Add(sim.V2{X: 260, Y: 260})
	up := V3{0, 1, 0}
	for x := lo.X; x < hi.X; x += cell {
		for z := lo.Y; z < hi.Y; z += cell {
			m := out.at(sim.V2{X: x + cell/2, Y: z + cell/2})
			m.Mat = MatGrass
			corners := [4]V3{v3(x, 0, z), v3(x+cell, 0, z), v3(x+cell, 0, z+cell), v3(x, 0, z+cell)}
			cols := [4]RGBA{}
			for i, c := range corners {
				cols[i] = grassAt(float64(c.X), float64(c.Z))
			}
			// Two triangles, clockwise from above.
			for _, tri := range [2][3]int{{0, 1, 2}, {0, 2, 3}} {
				for _, i := range tri {
					m.vert(corners[i], up, cols[i])
				}
			}
		}
	}
}

// buildHills rings the suburb with rolling, wooded hills.
func buildHills(t *sim.Town, m *Mesh) {
	c := t.Min.Lerp(t.Max, 0.5)
	r0 := t.Min.Dist(t.Max)/2 + 200
	const angles, rings = 96, 8
	pt := func(i, j int) (V3, RGBA) {
		a := 2 * math.Pi * float64(i%angles) / angles
		f := float64(j) / (rings - 1)
		r := r0 + f*520
		dir := sim.Dir(a)
		p := c.Add(dir.Scale(r))
		n := noise(dir.X*3+7, dir.Y*3+2)*0.6 + noise(dir.X*9, dir.Y*9)*0.4
		hgt := math.Pow(f, 0.8) * (35 + 110*n)
		if j == 0 {
			hgt = -1 // tuck the inner edge under the ground plane
		}
		col := Hex(0x4f7d3a).Mix(Hex(0x2f5230), float32(f*0.8+0.2*n))
		return At(p, hgt), col
	}
	m.Mat = MatGrass
	for i := range angles {
		for j := range rings - 1 {
			a, ca := pt(i, j)
			b, _ := pt(i+1, j)
			cc, _ := pt(i+1, j+1)
			d, _ := pt(i, j+1)
			m.Quad(a, b, cc, d, V3{0, 1, 0}, ca)
		}
	}
	m.Mat = MatPlain
}

// edgeAt offsets road point i sideways by d (positive to the left).
func edgeAt(r *sim.Road, i int, d, y float64) V3 {
	a, b := r.Pts[max(i-1, 0)], r.Pts[min(i+1, len(r.Pts)-1)]
	n := b.Sub(a).Norm().Left()
	return At(r.Pts[i].Add(n.Scale(d)), y)
}

func buildRoad(t *sim.Town, ri int, out Chunks) {
	r := &t.Roads[ri]
	m := out.at(r.Pts[len(r.Pts)/2])
	hw := sim.RoadHalfWidth
	m.Mat = MatAsphalt
	// Lanes are strips across the road: dusty edges, darker wheel paths.
	bands := []struct {
		off   float64
		shade float32
	}{{-hw, 1.12}, {-3.1, 0.93}, {-2.7, 0.88}, {-1.4, 0.93}, {-0.9, 1.0}, {0.9, 1.0}, {1.4, 0.93}, {2.7, 0.88}, {3.1, 0.93}, {hw, 1.12}}
	for b := 0; b+1 < len(bands); b++ {
		var left, right []V3
		for i := range r.Pts {
			left = append(left, edgeAt(r, i, bands[b+1].off, yRoad))
			right = append(right, edgeAt(r, i, bands[b].off, yRoad))
		}
		m.Strip(left, right, Asphalt.Shade((bands[b].shade+bands[b+1].shade)/2))
	}

	var bulb *sim.Node
	if b := &t.Nodes[r.B]; b.Bulb {
		bulb = b
	}
	// clear reports whether a point on this road's verge is clear of other
	// pavement (and of this road's own cul-de-sac bulb).
	clear := func(p sim.V2, margin float64) bool {
		if bulb != nil && p.Dist(bulb.P) < sim.BulbRadius+margin {
			return false
		}
		return t.OtherRoadDist(p, ri) > hw+margin
	}
	m.Mat = MatConcrete
	for _, side := range []float64{1, -1} {
		for i := 0; i+1 < len(r.Pts); i++ {
			p0 := edgeAt(r, i, side*hw, 0)
			p1 := edgeAt(r, i+1, side*hw, 0)
			if !clear(ground(p0), 0.4) || !clear(ground(p1), 0.4) {
				continue
			}
			kerbAndGutter(m, p0, p1, edgeAt(r, i, side*(hw-0.4), 0), edgeAt(r, i+1, side*(hw-0.4), 0),
				edgeAt(r, i, side*(hw+0.3), 0), edgeAt(r, i+1, side*(hw+0.3), 0))
		}
		for i := 0; i+1 < len(r.Pts); i++ {
			a0, a1 := edgeAt(r, i, side*sim.FootpathInner, yFootpath), edgeAt(r, i+1, side*sim.FootpathInner, yFootpath)
			b0, b1 := edgeAt(r, i, side*sim.FootpathOuter, yFootpath), edgeAt(r, i+1, side*sim.FootpathOuter, yFootpath)
			if !clear(ground(b0), 3.8) || !clear(ground(b1), 3.8) {
				continue
			}
			m.Quad(a0, b0, b1, a1, V3{0, 1, 0}, Footpath)
			// Expansion joint every other segment.
			if i%2 == 0 {
				j0, j1 := a0.Add(V3{0, 0.002, 0}), b0.Add(V3{0, 0.002, 0})
				d := a1.Sub(a0).Norm().Scale(0.03)
				m.Quad(j0, j1, j1.Add(d), j0.Add(d), V3{0, 1, 0}, Footpath.Shade(0.75))
			}
		}
		// Stormwater drain grates in the gutter.
		for s := 20.0; s < r.Len()-10; s += 47 {
			p, tg := r.At(s + side*6)
			n := tg.Left().Scale(side)
			g := p.Add(n.Scale(hw - 0.2))
			if !clear(g, 1) {
				continue
			}
			fr := YawFrame(At(g, yGutter+0.003), tg.X, tg.Y)
			m.Mat = MatMetal
			m.Box(fr, -0.14, 0, -0.32, 0.14, 0.004, 0.32, Hex(0x4a4a4c), FaceBottom)
			for k := -2; k <= 2; k++ {
				m.Box(fr, -0.1, 0.004, float64(k)*0.12-0.025, 0.1, 0.006, float64(k)*0.12+0.025, Hex(0x151515))
			}
			m.Mat = MatConcrete
		}
	}
	// Dashed centre line: 3m dashes, 6m gaps.
	m.Mat = MatRoadPaint
	for s := 4.5; s+3 < r.Len(); s += 9 {
		p0, tg0 := r.At(s)
		p1, tg1 := r.At(s + 3)
		if !clear(p0, 2) || !clear(p1, 2) {
			continue
		}
		w := 0.08
		m.Quad(At(p0.Add(tg0.Left().Scale(w)), yLine), At(p0.Add(tg0.Right().Scale(w)), yLine),
			At(p1.Add(tg1.Right().Scale(w)), yLine), At(p1.Add(tg1.Left().Scale(w)), yLine), V3{0, 1, 0}, Paint)
	}
	m.Mat = MatPlain
}

// kerbAndGutter builds an Australian kerb profile between road-edge points
// p0-p1: a concrete gutter strip on the road side (g0-g1) and a raised,
// rounded kerb out to k0-k1.
func kerbAndGutter(m *Mesh, p0, p1, g0, g1, k0, k1 V3) {
	up := V3{0, 1, 0}
	y := func(v V3, h float64) V3 { return V3{v.X, float32(h), v.Z} }
	m.Quad(y(g0, yGutter), y(p0, yGutter), y(p1, yGutter), y(g1, yGutter), up, Kerb.Shade(0.88))
	inward := g0.Sub(p0)
	// Face, rounded nose, top.
	n0 := p0.Lerp(k0, 0.25)
	n1 := p1.Lerp(k1, 0.25)
	m.Quad(y(p0, yGutter), y(p1, yGutter), y(p1, kerbH-0.04), y(p0, kerbH-0.04), inward, Kerb.Shade(0.92))
	m.Quad(y(p0, kerbH-0.04), y(p1, kerbH-0.04), y(n1, kerbH), y(n0, kerbH), inward.Add(V3{0, 1, 0}), Kerb)
	m.Quad(y(n0, kerbH), y(n1, kerbH), y(k1, kerbH), y(k0, kerbH), up, Kerb)
	m.Quad(y(k0, kerbH), y(k1, kerbH), y(k1, 0), y(k0, 0), k0.Sub(p0), Kerb.Shade(0.8))
}

// Lerp interpolates between two points.
func (a V3) Lerp(b V3, t float64) V3 { return a.Add(b.Sub(a).Scale(float32(t))) }

func buildBulb(t *sim.Town, n sim.Node, out Chunks) {
	m := out.at(n.P)
	ri := n.Roads[0]
	m.Mat = MatAsphalt
	m.Disc(At(n.P, yRoad), sim.BulbRadius, 36, Asphalt)
	m.Mat = MatConcrete
	// Kerb and footpath ring, broken where the street comes in.
	const segs = 44
	R := sim.BulbRadius
	for i := range segs {
		a0 := 2 * math.Pi * float64(i) / segs
		a1 := 2 * math.Pi * float64(i+1) / segs
		ring := func(a, r float64) sim.V2 { return n.P.Add(sim.Dir(a).Scale(r)) }
		if roadCorridor(t, ri, ring(a0, R+0.3)) || roadCorridor(t, ri, ring(a1, R+0.3)) {
			continue
		}
		kerbAndGutter(m, At(ring(a0, R), 0), At(ring(a1, R), 0), At(ring(a0, R-0.4), 0), At(ring(a1, R-0.4), 0),
			At(ring(a0, R+0.3), 0), At(ring(a1, R+0.3), 0))
		f0, f1 := R+(sim.FootpathInner-sim.RoadHalfWidth), R+(sim.FootpathOuter-sim.RoadHalfWidth)
		if roadCorridor(t, ri, ring(a0, f1+3.5)) || roadCorridor(t, ri, ring(a1, f1+3.5)) {
			continue
		}
		m.Quad(At(ring(a0, f0), yFootpath), At(ring(a0, f1), yFootpath), At(ring(a1, f1), yFootpath), At(ring(a1, f0), yFootpath), V3{0, 1, 0}, Footpath)
	}
	m.Mat = MatPlain
}

// roadCorridor reports whether p lies within the paved width of road ri
// itself (not counting its bulb).
func roadCorridor(t *sim.Town, ri int, p sim.V2) bool {
	r := &t.Roads[ri]
	for i := 0; i+1 < len(r.Pts); i++ {
		a, b := r.Pts[i], r.Pts[i+1]
		ab := b.Sub(a)
		f := math.Max(0, math.Min(1, p.Sub(a).Dot(ab)/ab.Dot(ab)))
		if p.Dist(a.Add(ab.Scale(f))) < sim.RoadHalfWidth+0.3 {
			return true
		}
	}
	return false
}

const poleHeight = 9.6

// poleArm is where wire k (-1, 0, 1) attaches to pole p.
func poleArm(t *sim.Town, p sim.Pole, k float64) V3 {
	_, tg := t.Roads[p.Road].At(p.S)
	return At(p.P.Add(tg.Left().Scale(k*0.95)), poleHeight-0.55)
}

func buildPoles(t *sim.Town, out Chunks) {
	for i, p := range t.Poles {
		m := out.at(p.P)
		_, tg := t.Roads[p.Road].At(p.S)
		n := tg.Left()
		base := At(p.P, 0)
		m.Mat = MatBark
		m.Tube(base, At(p.P, poleHeight), 0.16, 0.12, 8, Timber, true)
		arm0, arm1 := At(p.P.Add(n.Scale(-1.2)), poleHeight-0.7), At(p.P.Add(n.Scale(1.2)), poleHeight-0.7)
		m.Beam(arm0, arm1, 0.12, Timber.Shade(0.85))
		m.Mat = MatPlastic
		for _, k := range []float64{-1, 0, 1} {
			a := At(p.P.Add(n.Scale(k*0.95)), poleHeight-0.64)
			m.Tube(a, a.Add(V3{0, 0.12, 0}), 0.05, 0.035, 6, Hex(0x5a6a74), true)
		}
		if p.Light {
			// Street light reaching out over the road.
			in := n.Scale(-p.Side)
			m.Mat = MatMetal
			root := At(p.P, 7.6)
			tip := At(p.P.Add(in.Scale(2.4)), 8.0)
			m.Tube(root, tip, 0.05, 0.04, 6, Hex(0x9aa0a6), false)
			hf := YawFrame(tip, in.X, in.Y)
			m.Box(hf, -0.16, -0.12, -0.35, 0.16, 0.05, 0.25, Hex(0x8d949b))
			m.Mat = MatPlastic
			m.Box(hf, -0.13, -0.15, -0.3, 0.13, -0.12, 0.2, Hex(0xe8ecd8))
		}
		// Sagging wires to the next pole on this street.
		if i+1 < len(t.Poles) {
			q := t.Poles[i+1]
			if q.Road == p.Road && q.Side == p.Side && q.P.Dist(p.P) < 55 {
				m.Mat = MatRubber
				for _, k := range []float64{-1, 0, 1} {
					a, b := poleArm(t, p, k), poleArm(t, q, k)
					sag := 0.35 + 0.0004*float64(b.Sub(a).Dot(b.Sub(a)))
					const segs = 10
					prev := a
					for s := 1; s <= segs; s++ {
						f := float64(s) / segs
						pt := a.Lerp(b, f).Add(V3{0, float32(-sag * 4 * f * (1 - f)), 0})
						m.Beam(prev, pt, 0.022, Hex(0x1c1c1c))
						prev = pt
					}
				}
			}
		}
		m.Mat = MatPlain
	}
}
