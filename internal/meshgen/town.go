package meshgen

import (
	"math"

	"github.com/jsok/garbage-truck-simulator/internal/sim"
)

// Palette.
var (
	Asphalt  = Hex(0x3d3f44)
	Footpath = Hex(0xb9b5ab)
	Kerb     = Hex(0xd2cec4)
	Concrete = Hex(0xaaa69c)
	Paint    = Hex(0xeeeee4)
	Glass    = Hex(0x2c3c50)
	Trim     = Hex(0xf2f0ea)

	walls = []RGBA{Hex(0xede3c8), Hex(0xf2dfa0), Hex(0xb9d3e0), Hex(0xb8c9a3),
		Hex(0xe8b49a), Hex(0xf4f2ec), Hex(0xa65a42), Hex(0xc89b6d)}
	roofs = []RGBA{Hex(0xb5553a), Hex(0x4a4e54), Hex(0x5b6d82), Hex(0x6b4a3a), Hex(0x5e7a5a)}
	doors = []RGBA{Hex(0x8a2b2b), Hex(0x2b4a8a), Hex(0x2f6b3f), Hex(0xe0b030), Hex(0x3a3a3a)}
	leaf  = []RGBA{Hex(0x4e8a3a), Hex(0x3f7a34), Hex(0x6c9a45), Hex(0x5d8f3c)}
	gum   = []RGBA{Hex(0x7e9a6a), Hex(0x8aa377), Hex(0x6d8c5e)}
	pine  = []RGBA{Hex(0x2f5e34), Hex(0x3a6b3c)}
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

// TownMeshes holds the static scenery.
type TownMeshes struct {
	Ground  Chunks // receives shadows only
	Scenery Chunks // houses, trees, roads
}

// Heights of the flat layers, far enough apart to avoid z-fighting.
const (
	yFootpath = 0.018
	yDrive    = 0.03
	yRoad     = 0.04
	yLine     = 0.052
	kerbH     = 0.14
)

// BuildTown generates all static geometry for a suburb.
func BuildTown(t *sim.Town) TownMeshes {
	tm := TownMeshes{Ground: Chunks{}, Scenery: Chunks{}}
	buildGround(t, tm.Ground)
	for ri := range t.Roads {
		buildRoad(t, ri, tm.Scenery)
	}
	for _, n := range t.Nodes {
		if n.Bulb {
			buildBulb(t, n, tm.Scenery)
		} else if len(n.Roads) > 1 {
			tm.Scenery.at(n.P).Disc(At(n.P, yRoad), sim.RoadHalfWidth*1.35, 20, Asphalt)
		}
	}
	for i := range t.Houses {
		buildHouse(&t.Houses[i], tm.Scenery.at(t.Houses[i].P))
	}
	for i, tr := range t.Trees {
		buildTree(tr, uint64(i)*2654435761+uint64(t.Seed), tm.Scenery.at(tr.P))
	}
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
	return Hex(0x5f9a44).Mix(Hex(0x86ad4f), float32(n)).Shade(float32(0.92 + 0.12*noise(x/3.1, z/3.1)))
}

func buildGround(t *sim.Town, out Chunks) {
	const cell = 6.0
	lo, hi := t.Min.Sub(sim.V2{X: 260, Y: 260}), t.Max.Add(sim.V2{X: 260, Y: 260})
	up := V3{0, 1, 0}
	for x := lo.X; x < hi.X; x += cell {
		for z := lo.Y; z < hi.Y; z += cell {
			m := out.at(sim.V2{X: x + cell/2, Y: z + cell/2})
			corners := [4]V3{v3(x, 0, z), v3(x+cell, 0, z), v3(x+cell, 0, z+cell), v3(x, 0, z+cell)}
			cols := [4]RGBA{}
			for i, c := range corners {
				cols[i] = grassAt(float64(c.X), float64(c.Z))
			}
			// Two triangles, clockwise from above.
			for _, tri := range [2][3]int{{0, 1, 2}, {0, 2, 3}} {
				for _, i := range tri {
					m.P = append(m.P, corners[i])
					m.N = append(m.N, up)
					m.C = append(m.C, cols[i])
				}
			}
		}
	}
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
	var left, right []V3
	for i := range r.Pts {
		left = append(left, edgeAt(r, i, hw, yRoad))
		right = append(right, edgeAt(r, i, -hw, yRoad))
	}
	m.Strip(left, right, Asphalt)

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
	for _, side := range []float64{1, -1} {
		for i := 0; i+1 < len(r.Pts); i++ {
			p0 := edgeAt(r, i, side*hw, 0)
			p1 := edgeAt(r, i+1, side*hw, 0)
			if !clear(sim.V2{X: float64(p0.X), Y: float64(p0.Z)}, 0.4) || !clear(sim.V2{X: float64(p1.X), Y: float64(p1.Z)}, 0.4) {
				continue
			}
			// Kerb: a low raised lip at the road edge.
			k0, k1 := edgeAt(r, i, side*(hw+0.25), 0), edgeAt(r, i+1, side*(hw+0.25), 0)
			h := V3{0, kerbH, 0}
			m.Quad(p0.Add(h), k0.Add(h), k1.Add(h), p1.Add(h), V3{0, 1, 0}, Kerb)
			inward := p0.Sub(k0)
			m.Quad(p0.Add(V3{0, yRoad, 0}), p1.Add(V3{0, yRoad, 0}), p1.Add(h), p0.Add(h), inward, Kerb.Shade(0.85))
		}
		for i := 0; i+1 < len(r.Pts); i++ {
			a0, a1 := edgeAt(r, i, side*sim.FootpathInner, yFootpath), edgeAt(r, i+1, side*sim.FootpathInner, yFootpath)
			b0, b1 := edgeAt(r, i, side*sim.FootpathOuter, yFootpath), edgeAt(r, i+1, side*sim.FootpathOuter, yFootpath)
			if !clear(sim.V2{X: float64(b0.X), Y: float64(b0.Z)}, 3.8) || !clear(sim.V2{X: float64(b1.X), Y: float64(b1.Z)}, 3.8) {
				continue
			}
			m.Quad(a0, b0, b1, a1, V3{0, 1, 0}, Footpath)
		}
	}
	// Dashed centre line: 3m dashes, 6m gaps.
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
}

func buildBulb(t *sim.Town, n sim.Node, out Chunks) {
	m := out.at(n.P)
	ri := n.Roads[0]
	m.Disc(At(n.P, yRoad), sim.BulbRadius, 28, Asphalt)
	// Kerb and footpath ring, broken where the street comes in.
	const segs = 36
	for i := range segs {
		a0 := 2 * math.Pi * float64(i) / segs
		a1 := 2 * math.Pi * float64(i+1) / segs
		ring := func(a, r float64) sim.V2 { return n.P.Add(sim.Dir(a).Scale(r)) }
		if roadCorridor(t, ri, ring(a0, sim.BulbRadius+0.3)) || roadCorridor(t, ri, ring(a1, sim.BulbRadius+0.3)) {
			continue
		}
		h := kerbH
		p0, p1 := At(ring(a0, sim.BulbRadius), h), At(ring(a1, sim.BulbRadius), h)
		k0, k1 := At(ring(a0, sim.BulbRadius+0.25), h), At(ring(a1, sim.BulbRadius+0.25), h)
		m.Quad(p0, k0, k1, p1, V3{0, 1, 0}, Kerb)
		m.Quad(At(ring(a0, sim.BulbRadius), yRoad), At(ring(a1, sim.BulbRadius), yRoad), p1, p0, At(n.P, 0).Sub(p0), Kerb.Shade(0.85))
		f0, f1 := sim.BulbRadius+(sim.FootpathInner-sim.RoadHalfWidth), sim.BulbRadius+(sim.FootpathOuter-sim.RoadHalfWidth)
		if roadCorridor(t, ri, ring(a0, f1+3.5)) || roadCorridor(t, ri, ring(a1, f1+3.5)) {
			continue
		}
		m.Quad(At(ring(a0, f0), yFootpath), At(ring(a0, f1), yFootpath), At(ring(a1, f1), yFootpath), At(ring(a1, f0), yFootpath), V3{0, 1, 0}, Footpath)
	}
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

func buildHouse(h *sim.House, m *Mesh) {
	f := sim.Dir(h.Heading)
	fr := YawFrame(At(h.P, 0), f.X, f.Y)
	rng := h.Seed
	next := func() float64 {
		rng = rng*6364136223846793005 + 1442695040888963407
		return float64(rng>>11) / float64(1<<53)
	}
	wall := walls[h.Wall%len(walls)]
	roof := roofs[h.Roof%len(roofs)]
	door := doors[int(h.Seed>>8)%len(doors)]
	w, d := h.W, h.D
	storey := 2.7
	top := storey * float64(h.Storeys)

	// Which side of the frontage the driveway is on (local X).
	dv := h.Drive[1].Sub(h.P)
	driveX := dv.Dot(f.Right())
	gx := math.Copysign(1, driveX)

	// Concrete driveway from the kerb to the house.
	d0, d1 := h.Drive[0], h.Drive[1]
	u := d1.Sub(d0).Norm()
	hwD := 1.6
	m.Quad(At(d0.Add(u.Left().Scale(hwD)), yDrive), At(d0.Add(u.Right().Scale(hwD)), yDrive),
		At(d1.Add(u.Right().Scale(hwD)), yDrive), At(d1.Add(u.Left().Scale(hwD)), yDrive), V3{0, 1, 0}, Concrete)
	// A path from the footpath to the front door.
	doorX := -gx * w * 0.2
	pw := 0.55
	pathStart := -d0.Sub(h.P).Dot(f) + (sim.FootpathOuter - sim.RoadHalfWidth)
	m.Quad(fr.At(doorX-pw, yFootpath+0.005, pathStart), fr.At(doorX+pw, yFootpath+0.005, pathStart),
		fr.At(doorX+pw, yFootpath+0.005, -d/2), fr.At(doorX-pw, yFootpath+0.005, -d/2), V3{0, 1, 0}, Footpath.Shade(0.95))

	// Main body, and garage on the driveway side.
	m.Box(fr, -w/2, 0, -d/2, w/2, top, d/2, wall, FaceBottom)
	if h.Garage {
		gw := 3.4
		x0, x1 := gx*w/2-gx*gw, gx*w/2
		if x0 > x1 {
			x0, x1 = x1, x0
		}
		// Recessed garage door.
		m.Box(fr, x0+0.2, 0, -d/2-0.02, x1-0.2, 2.2, -d/2+0.1, Hex(0xe8e6e0).Shade(0.9), FaceBottom)
		for y := 0.35; y < 2.2; y += 0.45 {
			m.Box(fr, x0+0.2, y, -d/2-0.04, x1-0.2, y+0.04, -d/2, Hex(0xc8c6c0), FaceBottom)
		}
	}
	// Front door with a little porch step.
	m.Box(fr, doorX-0.5, 0, -d/2-0.03, doorX+0.5, 2.15, -d/2, door, FaceBottom)
	m.Box(fr, doorX-0.1-0.5, 2.15, -d/2-0.04, doorX+0.6, 2.25, -d/2, Trim, FaceBottom)
	m.Box(fr, doorX-0.9, 0, -d/2-0.9, doorX+0.9, 0.18, -d/2, Concrete.Shade(1.05), FaceBottom)
	m.Cylinder(fr, doorX+0.42, -d/2-0.05, 0.05, 1.0, 1.08, 4, Hex(0xd4af37), false)

	// Windows on every wall.
	window := func(x, y, z float64, face int) {
		ww, wh := 1.3, 1.15
		switch face {
		case 0: // front (-Z)
			m.Box(fr, x-ww/2-0.08, y-0.08, z-0.05, x+ww/2+0.08, y+wh+0.08, z, Trim, FaceBottom)
			m.Box(fr, x-ww/2, y, z-0.07, x+ww/2, y+wh, z-0.05, Glass, FaceBottom)
		case 1: // back (+Z)
			m.Box(fr, x-ww/2-0.08, y-0.08, z, x+ww/2+0.08, y+wh+0.08, z+0.05, Trim, FaceBottom)
			m.Box(fr, x-ww/2, y, z+0.05, x+ww/2, y+wh, z+0.07, Glass, FaceBottom)
		case 2: // sides (±X), z is the along-depth position, x the wall
			s := math.Copysign(1, x)
			m.Box(fr, x, y-0.08, z-ww/2-0.08, x+s*0.05, y+wh+0.08, z+ww/2+0.08, Trim, FaceBottom)
			m.Box(fr, x+s*0.05, y, z-ww/2, x+s*0.07, y+wh, z+ww/2, Glass, FaceBottom)
		}
	}
	for s := 0; s < h.Storeys; s++ {
		y := 0.95 + float64(s)*storey
		for _, x := range []float64{-w * 0.36, w * 0.36, -gx * w * 0.02} {
			if h.Garage && s == 0 && x*gx > 0 {
				continue
			}
			if s == 0 && math.Abs(x-doorX) < 1.3 {
				continue
			}
			window(x, y, -d/2, 0)
		}
		window(-w*0.25, y, d/2, 1)
		window(w*0.25, y, d/2, 1)
		window(-w/2, y, 0, 2)
		window(w/2, y, 0, 2)
	}

	// Roof: gable (ridge along the frontage) or hip.
	oh := 0.45
	rh := 1.6 + next()*1.2
	x0, x1, z0, z1 := -w/2-oh, w/2+oh, -d/2-oh, d/2+oh
	y0, y1 := top, top+rh
	if next() < 0.5 {
		fl, fr1 := fr.At(x0, y0, z0), fr.At(x1, y0, z0)
		bl, br := fr.At(x0, y0, z1), fr.At(x1, y0, z1)
		rl, rr := fr.At(x0, y1, 0), fr.At(x1, y1, 0)
		m.Quad(fl, fr1, rr, rl, fr.Dir(0, 1, -1), roof)
		m.Quad(bl, br, rr, rl, fr.Dir(0, 1, 1), roof.Shade(0.9))
		m.Poly([]V3{fr.At(-w/2, y0, -d/2), fr.At(-w/2, y0, d/2), fr.At(-w/2, y1-rh*oh/(d/2+oh), 0)}, fr.Dir(-1, 0, 0), wall)
		m.Poly([]V3{fr.At(w/2, y0, -d/2), fr.At(w/2, y0, d/2), fr.At(w/2, y1-rh*oh/(d/2+oh), 0)}, fr.Dir(1, 0, 0), wall)
		m.Poly([]V3{fl, bl, rl}, fr.Dir(0, -1, 0), roof.Shade(0.6))
		m.Poly([]V3{fr1, br, rr}, fr.Dir(0, -1, 0), roof.Shade(0.6))
		// Soffits.
		m.Quad(fl, fr1, br, bl, fr.Dir(0, -1, 0), roof.Shade(0.55))
	} else {
		inset := math.Min((z1-z0)/2, (x1-x0)/2) * 0.9
		rl, rr := fr.At(x0+inset, y1, 0), fr.At(x1-inset, y1, 0)
		fl, fr1 := fr.At(x0, y0, z0), fr.At(x1, y0, z0)
		bl, br := fr.At(x0, y0, z1), fr.At(x1, y0, z1)
		m.Quad(fl, fr1, rr, rl, fr.Dir(0, 1, -1), roof)
		m.Quad(bl, br, rr, rl, fr.Dir(0, 1, 1), roof.Shade(0.9))
		m.Poly([]V3{fl, bl, rl}, fr.Dir(-1, 1, 0), roof.Shade(0.95))
		m.Poly([]V3{fr1, br, rr}, fr.Dir(1, 1, 0), roof.Shade(0.85))
		m.Quad(fl, fr1, br, bl, fr.Dir(0, -1, 0), roof.Shade(0.55))
	}
	if h.Chimney {
		cx := (next()*2 - 1) * w * 0.3
		m.Box(fr, cx-0.35, top, d*0.1, cx+0.35, y1+0.6, d*0.1+0.7, Hex(0x9a5a44), FaceBottom)
	}

	// Shrubs either side of the door, and a letterbox by the driveway.
	for _, s := range []float64{-1.3, 1.3} {
		m.Blob(fr.At(doorX+s, 0.45, -d/2-0.7), 0.6, 0.5, 0.5, h.Seed+uint64(s*10+20), leaf[int(h.Seed>>20)%len(leaf)])
	}
	side := h.P.Sub(d1)
	side = side.Sub(u.Scale(side.Dot(u))).Norm()
	lb := d0.Add(u.Scale(sim.FootpathOuter - sim.RoadHalfWidth + 0.4)).Add(side.Scale(2.2))
	lf := YawFrame(At(lb, 0), -u.X, -u.Y)
	m.Box(lf, -0.05, 0, -0.05, 0.05, 0.9, 0.05, Hex(0x5a4632), FaceBottom)
	m.Box(lf, -0.2, 0.9, -0.3, 0.2, 1.2, 0.2, door.Shade(1.1))
}

func buildTree(tr sim.Tree, seed uint64, m *Mesh) {
	c := At(tr.P, 0)
	fr := Identity
	fr.O = c
	pick := func(p []RGBA) RGBA { return p[int(seed>>16)%len(p)] }
	switch tr.Kind {
	case 1: // gum tree: pale trunk, clumps of sparse grey-green foliage
		m.Frustum(fr, 0, 0, 0.22, 0.12, 0, tr.Height*0.75, 5, Hex(0xcdc4b4), false)
		for i := 0; i < 4; i++ {
			a := float64(i)*1.9 + float64(seed%7)
			r := tr.Radius * 0.55
			off := v3(math.Cos(a)*r, tr.Height*(0.62+0.1*float64(i%2)), math.Sin(a)*r)
			m.Blob(c.Add(off), tr.Radius*0.6, tr.Radius*0.45, tr.Radius*0.6, seed+uint64(i), pick(gum))
		}
	case 2: // conifer
		m.Cylinder(fr, 0, 0, 0.18, 0, tr.Height*0.3, 5, Hex(0x5a4030), false)
		col := pick(pine)
		m.Frustum(fr, 0, 0, tr.Radius, 0, tr.Height*0.2, tr.Height*0.75, 7, col, true)
		m.Frustum(fr, 0, 0, tr.Radius*0.7, 0, tr.Height*0.5, tr.Height*1.05, 7, col.Shade(1.1), true)
	default: // round leafy tree
		m.Frustum(fr, 0, 0, 0.22, 0.15, 0, tr.Height*0.6, 5, Hex(0x6b4e33), false)
		col := pick(leaf)
		m.Blob(c.Add(v3(0, tr.Height*0.68, 0)), tr.Radius, tr.Radius*0.8, tr.Radius, seed, col)
		m.Blob(c.Add(v3(tr.Radius*0.4, tr.Height*0.85, tr.Radius*0.2)), tr.Radius*0.65, tr.Radius*0.55, tr.Radius*0.65, seed+1, col.Shade(1.08))
	}
}
