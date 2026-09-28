package meshgen

import (
	"math"

	"github.com/jsok/garbage-truck-simulator/internal/sim"
)

// wall is a frame on the outside face of a wall: X runs along it, Y up and
// Z out of the house.
func wallFrame(o, along, out V3) Frame { return Frame{O: o, X: along, Y: V3{0, 1, 0}, Z: out} }

// window adds a framed window centred at x along the wall, with its sill at y.
func window(m *Mesh, wf Frame, x, y, ww, wh float64) {
	defer m.With(MatGlass)()
	m.Box(wf, x-ww/2, y, 0, x+ww/2, y+wh, 0.015, Glass)
	m.Mat = MatPaint
	const t, p = 0.07, 0.06 // frame thickness and projection
	m.Box(wf, x-ww/2-t, y-t, 0, x-ww/2, y+wh+t, p, Trim, FaceBack)
	m.Box(wf, x+ww/2, y-t, 0, x+ww/2+t, y+wh+t, p, Trim, FaceBack)
	m.Box(wf, x-ww/2, y+wh, 0, x+ww/2, y+wh+t, p, Trim, FaceBack)
	m.Box(wf, x-0.02, y, 0, x+0.02, y+wh, 0.04, Trim, FaceBack)                         // mullion
	m.Box(wf, x-ww/2, y+wh*0.62, 0, x+ww/2, y+wh*0.62+0.035, 0.04, Trim, FaceBack)      // transom
	m.Box(wf, x-ww/2-0.12, y-0.07, 0, x+ww/2+0.12, y, 0.14, Trim.Shade(0.95), FaceBack) // sill
}

func buildHouse(t *sim.Town, h *sim.House, m *Mesh) {
	defer m.With(MatPlain)()
	f := sim.Dir(h.Heading)
	fr := YawFrame(At(h.P, 0), f.X, f.Y)
	r := rng(h.Seed)
	wall := walls[h.Wall%len(walls)]
	roof := roofs[h.Roof%len(roofs)]
	door := doors[int(h.Seed>>8)%len(doors)]
	w, d := h.W, h.D
	storey := 2.7
	top := storey * float64(h.Storeys)

	wallMat := MatRender
	switch {
	case h.Wall >= 6:
		wallMat = MatBrick
	case r.next() < 0.55:
		wallMat = MatWeatherboard
	}
	roofMat := MatRoofMetal
	if (h.Roof == 0 || h.Roof == 3) && r.next() < 0.85 {
		roofMat = MatRoofTile
	}

	// Which side of the frontage the driveway is on (local X).
	dv := h.Drive[1].Sub(h.P)
	gx := math.Copysign(1, dv.Dot(f.Right()))

	// Concrete driveway from the kerb to the house, and a path to the door.
	m.Mat = MatConcrete
	d0, d1 := h.Drive[0], h.Drive[1]
	u := d1.Sub(d0).Norm()
	hwD := 1.6
	m.Quad(At(d0.Add(u.Left().Scale(hwD)), yDrive), At(d0.Add(u.Right().Scale(hwD)), yDrive),
		At(d1.Add(u.Right().Scale(hwD)), yDrive), At(d1.Add(u.Left().Scale(hwD)), yDrive), V3{0, 1, 0}, Concrete)
	doorX := -gx * w * 0.2
	pw := 0.55
	pathStart := -d0.Sub(h.P).Dot(f) + (sim.FootpathOuter - sim.RoadHalfWidth)
	m.Quad(fr.At(doorX-pw, yFootpath+0.005, pathStart), fr.At(doorX+pw, yFootpath+0.005, pathStart),
		fr.At(doorX+pw, yFootpath+0.005, -d/2), fr.At(doorX-pw, yFootpath+0.005, -d/2), V3{0, 1, 0}, Footpath.Shade(0.95))

	// Walls, with a brick base course under timber and rendered houses.
	m.Mat = wallMat
	m.Box(fr, -w/2, 0, -d/2, w/2, top, d/2, wall, FaceBottom)
	if wallMat != MatBrick {
		m.Mat = MatBrick
		m.Box(fr, -w/2-0.03, 0, -d/2-0.03, w/2+0.03, 0.45, d/2+0.03, Hex(0x8e4d3a), FaceBottom)
	}

	front := wallFrame(fr.At(0, 0, -d/2), fr.X, fr.Z.Scale(-1))
	back := wallFrame(fr.At(0, 0, d/2), fr.X, fr.Z)
	leftW := wallFrame(fr.At(-w/2, 0, 0), fr.Z, fr.X.Scale(-1))
	rightW := wallFrame(fr.At(w/2, 0, 0), fr.Z, fr.X)

	var gx0, gx1 float64
	if h.Garage {
		gw := 3.4
		gx0, gx1 = gx*w/2-gx*gw, gx*w/2
		if gx0 > gx1 {
			gx0, gx1 = gx1, gx0
		}
		m.Mat = MatPaint
		gd := Hex(0xe6e3dc)
		if r.next() < 0.4 {
			gd = roof.Shade(1.2)
		}
		m.Box(front, gx0+0.2, 0, 0, gx1-0.2, 2.2, 0.03, gd, FaceBottom)
		for y := 0.3; y < 2.2; y += 0.28 {
			m.Box(front, gx0+0.2, y, 0.03, gx1-0.2, y+0.03, 0.045, gd.Shade(0.85), FaceBottom)
		}
		m.Box(front, gx0+0.1, 2.2, 0, gx1-0.1, 2.3, 0.06, Trim, FaceBottom)
	}
	// Front door, frame, step and knob.
	m.Mat = MatPaint
	m.Box(front, doorX-0.48, 0.18, 0, doorX+0.48, 2.12, 0.03, door, FaceBottom)
	m.Box(front, doorX-0.58, 0, 0, doorX-0.48, 2.22, 0.06, Trim, FaceBottom)
	m.Box(front, doorX+0.48, 0, 0, doorX+0.58, 2.22, 0.06, Trim, FaceBottom)
	m.Box(front, doorX-0.58, 2.12, 0, doorX+0.58, 2.22, 0.06, Trim, FaceBottom)
	m.Mat = MatMetal
	m.Box(front, doorX+0.3, 1.0, 0.03, doorX+0.38, 1.06, 0.08, Hex(0xd4af37))
	m.Mat = MatConcrete
	m.Box(front, doorX-0.9, 0, 0, doorX+0.9, 0.18, 0.9, Concrete.Shade(1.05), FaceBottom)

	// Windows.
	for s := 0; s < h.Storeys; s++ {
		y := 0.95 + float64(s)*storey
		for _, x := range []float64{-w * 0.34, w * 0.34, gx * w * 0.02} {
			if h.Garage && s == 0 && x > gx0-0.9 && x < gx1+0.9 {
				continue
			}
			if s == 0 && math.Abs(x-doorX) < 1.4 {
				continue
			}
			window(m, front, x, y, 1.4, 1.2)
		}
		window(m, back, -w*0.25, y, 1.4, 1.2)
		window(m, back, w*0.25, y, 1.4, 1.2)
		window(m, leftW, 0, y, 1.2, 1.1)
		window(m, rightW, 0, y, 1.2, 1.1)
	}

	// Verandah over the front door on some houses.
	if !h.Garage || r.next() < 0.35 {
		vx0, vx1 := doorX-2.0, doorX+2.0
		if h.Garage {
			if gx > 0 {
				vx1 = math.Min(vx1, gx0-0.2)
			} else {
				vx0 = math.Max(vx0, gx1+0.2)
			}
		}
		vx0, vx1 = math.Max(vx0, -w/2), math.Min(vx1, w/2)
		depth := 2.2
		m.Mat = MatPaint
		for _, x := range []float64{vx0 + 0.1, vx1 - 0.1} {
			m.Box(front, x-0.06, 0, depth-0.12, x+0.06, 2.45, depth, Trim, FaceBottom)
		}
		m.Box(front, vx0, 2.45, depth-0.14, vx1, 2.58, depth, Trim) // beam
		m.Mat = MatRoofMetal
		a, b := front.At(vx0-0.1, 2.85, 0), front.At(vx1+0.1, 2.85, 0)
		c, e := front.At(vx1+0.1, 2.58, depth+0.25), front.At(vx0-0.1, 2.58, depth+0.25)
		m.Quad(a, b, c, e, front.Dir(0, 1, 0.3), roof.Shade(1.1))
		m.Quad(a, b, c, e, front.Dir(0, -1, 0), Trim.Shade(0.8))
		m.Mat = MatConcrete
		m.Box(front, vx0, 0, 0, vx1, 0.12, depth, Concrete.Shade(1.08), FaceBottom)
	}

	// Roof: gable (ridge along the frontage) or hip, with fascia and gutters.
	oh := 0.5
	rh := 1.6 + r.next()*1.2
	x0, x1, z0, z1 := -w/2-oh, w/2+oh, -d/2-oh, d/2+oh
	y0, y1 := top, top+rh
	gable := r.next() < 0.5
	m.Mat = roofMat
	fl, fr1 := fr.At(x0, y0, z0), fr.At(x1, y0, z0)
	bl, br := fr.At(x0, y0, z1), fr.At(x1, y0, z1)
	var rl, rr V3
	if gable {
		rl, rr = fr.At(x0, y1, 0), fr.At(x1, y1, 0)
		m.Quad(fl, fr1, rr, rl, fr.Dir(0, 1, -1), roof)
		m.Quad(bl, br, rr, rl, fr.Dir(0, 1, 1), roof)
		m.Mat = wallMat
		gy := y1 - rh*oh/(d/2+oh)
		m.Poly([]V3{fr.At(-w/2, y0, -d/2), fr.At(-w/2, y0, d/2), fr.At(-w/2, gy, 0)}, fr.Dir(-1, 0, 0), wall)
		m.Poly([]V3{fr.At(w/2, y0, -d/2), fr.At(w/2, y0, d/2), fr.At(w/2, gy, 0)}, fr.Dir(1, 0, 0), wall)
		m.Mat = MatPaint // barge boards
		m.Beam(fl, rl, 0.08, Trim)
		m.Beam(bl, rl, 0.08, Trim)
		m.Beam(fr1, rr, 0.08, Trim)
		m.Beam(br, rr, 0.08, Trim)
	} else {
		inset := math.Min((z1-z0)/2, (x1-x0)/2) * 0.9
		rl, rr = fr.At(x0+inset, y1, 0), fr.At(x1-inset, y1, 0)
		m.Quad(fl, fr1, rr, rl, fr.Dir(0, 1, -1), roof)
		m.Quad(bl, br, rr, rl, fr.Dir(0, 1, 1), roof)
		m.Poly([]V3{fl, bl, rl}, fr.Dir(-1, 1, 0), roof)
		m.Poly([]V3{fr1, br, rr}, fr.Dir(1, 1, 0), roof)
		m.Mat = MatPaint
		for _, pr := range [][2]V3{{fl, rl}, {bl, rl}, {fr1, rr}, {br, rr}} {
			m.Beam(pr[0], pr[1], 0.1, roof.Shade(0.9)) // hip caps
		}
	}
	m.Mat = MatPaint
	m.Beam(rl.Sub(fr.Dir(0.05, 0, 0)), rr.Add(fr.Dir(0.05, 0, 0)), 0.14, roof.Shade(0.85)) // ridge cap
	// Soffit, fascia and quad gutters.
	m.Mat = MatPlain
	m.Quad(fl, fr1, br, bl, fr.Dir(0, -1, 0), Trim.Shade(0.8))
	m.Mat = MatPaint
	gc := Trim
	if r.next() < 0.5 {
		gc = roof.Shade(1.1)
	}
	for _, z := range []float64{z0, z1} {
		s := math.Copysign(1, z)
		m.Box(fr, x0, y0-0.22, z-s*0.03, x1, y0+0.02, z, Trim, FaceTop)
		zz := [2]float64{z, z + s*0.14}
		if zz[0] > zz[1] {
			zz[0], zz[1] = zz[1], zz[0]
		}
		m.Box(fr, x0-0.02, y0-0.16, zz[0], x1+0.02, y0+0.02, zz[1], gc)
	}
	// Downpipes at the front corners.
	for _, x := range []float64{-w/2 + 0.1, w/2 - 0.1} {
		m.Box(fr, x-0.04, 0, -d/2-0.12, x+0.04, y0-0.1, -d/2-0.04, gc)
	}

	if h.Chimney {
		m.Mat = MatBrick
		cx := (r.next()*2 - 1) * w * 0.3
		m.Box(fr, cx-0.35, top, d*0.1, cx+0.35, y1+0.6, d*0.1+0.7, Hex(0x9a5a44), FaceBottom)
		m.Mat = MatConcrete
		m.Box(fr, cx-0.42, y1+0.6, d*0.1-0.07, cx+0.42, y1+0.7, d*0.1+0.77, Concrete)
	}
	// Solar panels on the street-facing roof.
	if r.next() < 0.35 {
		m.Mat = MatSolar
		pos := func(x, t float64) V3 { return fr.At(x, y0+t*rh, z0*(1-t)) }
		nrm := pos(0, 1).Sub(pos(0, 0)).Cross(fr.Dir(1, 0, 0)).Norm()
		if nrm.Dot(fr.Dir(0, 1, 0)) < 0 {
			nrm = nrm.Scale(-1)
		}
		lift := nrm.Scale(0.07)
		cols := 3 + int(r.next()*3)
		span := math.Min(w*0.7, float64(cols)*1.05)
		if !gable {
			span = math.Min(span, (x1-x0)-2*math.Min((z1-z0)/2, (x1-x0)/2)*0.9)
		}
		for c := range cols {
			xa := -span/2 + span*float64(c)/float64(cols) + 0.03
			xb := -span/2 + span*float64(c+1)/float64(cols) - 0.03
			for _, tr := range [][2]float64{{0.22, 0.5}, {0.53, 0.8}} {
				m.Quad(pos(xa, tr[0]).Add(lift), pos(xb, tr[0]).Add(lift), pos(xb, tr[1]).Add(lift), pos(xa, tr[1]).Add(lift), nrm, Hex(0x1a2340))
			}
		}
	}
	// TV antenna on the ridge.
	if r.next() < 0.35 {
		m.Mat = MatMetal
		base := rl.Lerp(rr, 0.3)
		mast := base.Add(V3{0, 1.6, 0})
		m.Beam(base, mast, 0.04, Hex(0x9aa0a6))
		for i := range 4 {
			y := mast.Add(V3{0, float32(-0.15 * float64(i)), 0})
			l := 0.9 - 0.15*float64(i)
			m.Beam(y.Sub(fr.Z.Scale(float32(l/2))), y.Add(fr.Z.Scale(float32(l/2))), 0.025, Hex(0x9aa0a6))
		}
	}
	// Air conditioner on the side wall away from the driveway.
	if r.next() < 0.45 {
		side := leftW
		if gx < 0 {
			side = rightW
		}
		m.Mat = MatMetal
		m.Box(side, -0.4, 0.3, 0, 0.4, 0.95, 0.32, Hex(0xd8d8d2))
		m.Mat = MatRubber
		m.Box(side, -0.3, 0.38, 0.32, 0.3, 0.87, 0.33, Hex(0x3a3a3a))
	}

	// Garden bed along the front wall, with shrubs and flowers.
	m.Mat = MatPlain
	bedX0, bedX1 := -w/2, w/2
	if h.Garage {
		if gx > 0 {
			bedX1 = gx0
		} else {
			bedX0 = gx1
		}
	}
	for _, seg := range [][2]float64{{bedX0, doorX - 0.7}, {doorX + 0.7, bedX1}} {
		if seg[1]-seg[0] < 0.8 {
			continue
		}
		m.Quad(front.At(seg[0], 0.012, 0), front.At(seg[1], 0.012, 0), front.At(seg[1], 0.012, 1.1), front.At(seg[0], 0.012, 1.1), V3{0, 1, 0}, Hex(0x4a3526))
		m.Mat = MatFoliage
		m.Sway = 0.12
		for x := seg[0] + 0.5; x < seg[1]-0.3; x += 0.9 + r.next()*0.6 {
			s := 0.35 + r.next()*0.3
			m.Blob(front.At(x, s*0.8, 0.55), s, s*0.8, s, h.Seed+uint64(x*100), r.pick(leaf).Shade(0.85))
			if r.next() < 0.6 {
				fc := r.pick(bloom)
				for k := range 3 {
					fx := x + (r.next()-0.5)*0.6
					m.Blob(front.At(fx, 0.25+float64(k)*0.05, 0.85+r.next()*0.2), 0.09, 0.07, 0.09, h.Seed+uint64(k), fc)
				}
			}
		}
		m.Sway = 0
		m.Mat = MatPlain
	}

	// Letterbox by the driveway.
	side := h.P.Sub(d1)
	side = side.Sub(u.Scale(side.Dot(u))).Norm()
	lb := d0.Add(u.Scale(sim.FootpathOuter - sim.RoadHalfWidth + 0.4)).Add(side.Scale(2.2))
	lf := YawFrame(At(lb, 0), -u.X, -u.Y)
	if wallMat == MatBrick {
		m.Mat = MatBrick
		m.Box(lf, -0.22, 0, -0.22, 0.22, 1.1, 0.22, wall, FaceBottom)
		m.Mat = MatPaint
		m.Box(lf, -0.15, 0.7, -0.23, 0.15, 0.85, -0.2, Hex(0x2a2a2a))
	} else {
		m.Mat = MatBark
		m.Box(lf, -0.05, 0, -0.05, 0.05, 0.9, 0.05, Hex(0x5a4632), FaceBottom)
		m.Mat = MatPaint
		m.Box(lf, -0.2, 0.9, -0.3, 0.2, 1.2, 0.2, door.Shade(1.1))
	}

	buildFence(t, h, fr, r.pick(fences), m)
}

// buildFence runs a Colorbond fence around the back yard, from the side
// walls back past the rear of the house. Sections that would cross a road or
// another house are left out.
func buildFence(t *sim.Town, h *sim.House, fr Frame, col RGBA, m *Mesh) {
	w, d := h.W, h.D
	xs := w/2 + 2.4
	zf, zb := -d/2+1.6, d/2+7
	pts := [][2]float64{{-w / 2, zf}, {-xs, zf}, {-xs, zb}, {xs, zb}, {xs, zf}, {w / 2, zf}}
	m.Mat = MatRoofMetal
	defer func() { m.Mat = MatPlain }()
	for i := 0; i+1 < len(pts); i++ {
		a, b := pts[i], pts[i+1]
		l := math.Hypot(b[0]-a[0], b[1]-a[1])
		n := max(1, int(math.Ceil(l/2.4)))
		for s := range n {
			f0, f1 := float64(s)/float64(n), float64(s+1)/float64(n)
			p0 := fr.At(a[0]+(b[0]-a[0])*f0, 0, a[1]+(b[1]-a[1])*f0)
			p1 := fr.At(a[0]+(b[0]-a[0])*f1, 0, a[1]+(b[1]-a[1])*f1)
			mid := ground(p0.Lerp(p1, 0.5))
			if t.RoadDist(mid) < sim.RoadHalfWidth+4 || insideOtherHouse(t, h, mid) {
				continue
			}
			along := p1.Sub(p0).Norm()
			out := along.Cross(V3{0, 1, 0})
			wf := wallFrame(p0, along, out)
			sl := float64(p1.Sub(p0).Dot(along))
			m.Box(wf, 0, 0, -0.02, sl, 1.8, 0.02, col)
			m.Box(wf, -0.04, 0, -0.04, 0.04, 1.85, 0.04, col.Shade(0.8))
		}
	}
}

func insideOtherHouse(t *sim.Town, self *sim.House, p sim.V2) bool {
	for i := range t.Houses {
		o := &t.Houses[i]
		if o == self || o.P.Dist(p) > 20 {
			continue
		}
		f := sim.Dir(o.Heading)
		d := p.Sub(o.P)
		if math.Abs(d.Dot(f)) < o.D/2+0.5 && math.Abs(d.Dot(f.Left())) < o.W/2+0.5 {
			return true
		}
	}
	return false
}
