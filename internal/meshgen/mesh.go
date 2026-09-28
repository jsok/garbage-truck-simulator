// Package meshgen builds flat-shaded, vertex-coloured triangle meshes for the
// suburb, the truck and the bins. It has no engine dependency: the renderer
// copies the arrays straight into GPU buffers.
package meshgen

import "math"

// V3 is a 3D point or direction (Y up).
type V3 struct{ X, Y, Z float32 }

// RGBA is a linear colour.
type RGBA struct{ R, G, B, A float32 }

func v3(x, y, z float64) V3 { return V3{float32(x), float32(y), float32(z)} }

func (a V3) Add(b V3) V3        { return V3{a.X + b.X, a.Y + b.Y, a.Z + b.Z} }
func (a V3) Sub(b V3) V3        { return V3{a.X - b.X, a.Y - b.Y, a.Z - b.Z} }
func (a V3) Scale(s float32) V3 { return V3{a.X * s, a.Y * s, a.Z * s} }
func (a V3) Dot(b V3) float32   { return a.X*b.X + a.Y*b.Y + a.Z*b.Z }
func (a V3) Cross(b V3) V3 {
	return V3{a.Y*b.Z - a.Z*b.Y, a.Z*b.X - a.X*b.Z, a.X*b.Y - a.Y*b.X}
}
func (a V3) Norm() V3 {
	l := float32(math.Sqrt(float64(a.Dot(a))))
	if l == 0 {
		return a
	}
	return a.Scale(1 / l)
}

// Hex makes a colour from 0xRRGGBB.
func Hex(c uint32) RGBA {
	return RGBA{float32(c>>16&0xff) / 255, float32(c>>8&0xff) / 255, float32(c&0xff) / 255, 1}
}

// Shade scales a colour's brightness.
func (c RGBA) Shade(f float32) RGBA {
	return RGBA{min(c.R*f, 1), min(c.G*f, 1), min(c.B*f, 1), c.A}
}

// Mix blends towards o by t.
func (c RGBA) Mix(o RGBA, t float32) RGBA {
	return RGBA{c.R + (o.R-c.R)*t, c.G + (o.G-c.G)*t, c.B + (o.B-c.B)*t, c.A + (o.A-c.A)*t}
}

// Mesh is a triangle soup with per-vertex normals and colours. Triangles are
// stored clockwise as seen from the front, which is Godot's convention.
//
// Each vertex also carries a surface material and a wind-sway weight, taken
// from Mat and Sway at the time the vertex is added. The renderer's shader
// uses them for procedural surface detail.
type Mesh struct {
	P []V3
	N []V3
	C []RGBA
	M []Material
	S []float32

	Mat  Material
	Sway float32
}

// Material selects how the shader details a surface.
type Material uint8

const (
	MatPlain Material = iota
	MatGrass
	MatAsphalt
	MatConcrete
	MatBrick
	MatWeatherboard
	MatRender
	MatRoofTile
	MatRoofMetal
	MatGlass
	MatFoliage
	MatBark
	MatPaint // glossy vehicle paint
	MatPlastic
	MatRoadPaint
	MatLight // emissive
	MatRubber
	MatMetal
	MatSolar
	MatDash // textured interior plastic
	MatGrassBlade
)

// With sets the material for subsequently added geometry and returns a
// function restoring the previous one: defer m.With(MatBrick)().
func (m *Mesh) With(mat Material) func() {
	prev := m.Mat
	m.Mat = mat
	return func() { m.Mat = prev }
}

func (m *Mesh) vert(p, n V3, c RGBA) {
	m.P = append(m.P, p)
	m.N = append(m.N, n)
	m.C = append(m.C, c)
	m.M = append(m.M, m.Mat)
	m.S = append(m.S, m.Sway)
}

// Empty reports whether the mesh has no triangles.
func (m *Mesh) Empty() bool { return len(m.P) == 0 }

// Tri adds a triangle whose vertices run counter-clockwise when seen from the
// side it should be visible from.
func (m *Mesh) Tri(a, b, c V3, col RGBA) {
	n := b.Sub(a).Cross(c.Sub(a)).Norm()
	m.vert(a, n, col)
	m.vert(c, n, col)
	m.vert(b, n, col)
}

// triFacing adds a triangle visible from the side want points to, with a
// shared normal n.
func (m *Mesh) triFacing(a, b, c, want, n V3, col RGBA) {
	if b.Sub(a).Cross(c.Sub(a)).Dot(want) < 0 {
		b, c = c, b
	}
	m.TriN(a, b, c, n, n, n, col)
}

// TriN is Tri with explicit per-vertex normals, for smooth shading.
func (m *Mesh) TriN(a, b, c, na, nb, nc V3, col RGBA) {
	m.vert(a, na, col)
	m.vert(c, nc, col)
	m.vert(b, nb, col)
}

// Poly adds a convex planar polygon, oriented so that it faces towards out.
func (m *Mesh) Poly(pts []V3, out V3, col RGBA) {
	if len(pts) < 3 {
		return
	}
	n := pts[1].Sub(pts[0]).Cross(pts[2].Sub(pts[0]))
	flip := n.Dot(out) < 0
	for i := 1; i+1 < len(pts); i++ {
		if flip {
			m.Tri(pts[0], pts[i+1], pts[i], col)
		} else {
			m.Tri(pts[0], pts[i], pts[i+1], col)
		}
	}
}

// Quad adds a planar quad a-b-c-d facing towards out.
func (m *Mesh) Quad(a, b, c, d V3, out V3, col RGBA) {
	m.Poly([]V3{a, b, c, d}, out, col)
}

// Append copies another mesh into this one.
func (m *Mesh) Append(o *Mesh) {
	m.P = append(m.P, o.P...)
	m.N = append(m.N, o.N...)
	m.C = append(m.C, o.C...)
	m.M = append(m.M, o.M...)
	m.S = append(m.S, o.S...)
}

// Frame is a local coordinate system: points are O + X*x + Y*y + Z*z.
type Frame struct{ O, X, Y, Z V3 }

// Identity is the world frame.
var Identity = Frame{X: V3{1, 0, 0}, Y: V3{0, 1, 0}, Z: V3{0, 0, 1}}

// YawFrame is a frame at o rotated about Y so that local -Z faces the
// horizontal direction (fx, fz).
func YawFrame(o V3, fx, fz float64) Frame {
	f := v3(fx, 0, fz).Norm()
	return Frame{O: o, X: V3{-f.Z, 0, f.X}, Y: V3{0, 1, 0}, Z: f.Scale(-1)}
}

// At maps a local point to the parent space.
func (f Frame) At(x, y, z float64) V3 {
	return f.O.Add(f.X.Scale(float32(x))).Add(f.Y.Scale(float32(y))).Add(f.Z.Scale(float32(z)))
}

// Dir maps a local direction to the parent space.
func (f Frame) Dir(x, y, z float64) V3 {
	return f.X.Scale(float32(x)).Add(f.Y.Scale(float32(y))).Add(f.Z.Scale(float32(z)))
}

// Sub nests a frame translated to local (x, y, z) and yawed by angle.
func (f Frame) Sub(x, y, z, yaw float64) Frame {
	s, c := math.Sincos(yaw)
	return Frame{O: f.At(x, y, z), X: f.Dir(c, 0, -s), Y: f.Y, Z: f.Dir(s, 0, c)}
}

// Box adds an axis-aligned (in f) box spanning local x0..x1, y0..y1, z0..z1.
// Faces in skip (a bitmask of FaceXXX) are left out.
func (m *Mesh) Box(f Frame, x0, y0, z0, x1, y1, z1 float64, col RGBA, skip ...Face) {
	var mask Face
	for _, s := range skip {
		mask |= s
	}
	p := func(x, y, z float64) V3 { return f.At(x, y, z) }
	type side struct {
		face       Face
		a, b, c, d V3
		out        V3
		shade      float32
	}
	sides := []side{
		{FaceTop, p(x0, y1, z0), p(x1, y1, z0), p(x1, y1, z1), p(x0, y1, z1), f.Dir(0, 1, 0), 1.0},
		{FaceBottom, p(x0, y0, z0), p(x1, y0, z0), p(x1, y0, z1), p(x0, y0, z1), f.Dir(0, -1, 0), 1.0},
		{FaceRight, p(x1, y0, z0), p(x1, y1, z0), p(x1, y1, z1), p(x1, y0, z1), f.Dir(1, 0, 0), 1.0},
		{FaceLeft, p(x0, y0, z0), p(x0, y1, z0), p(x0, y1, z1), p(x0, y0, z1), f.Dir(-1, 0, 0), 1.0},
		{FaceBack, p(x0, y0, z1), p(x1, y0, z1), p(x1, y1, z1), p(x0, y1, z1), f.Dir(0, 0, 1), 1.0},
		{FaceFront, p(x0, y0, z0), p(x1, y0, z0), p(x1, y1, z0), p(x0, y1, z0), f.Dir(0, 0, -1), 1.0},
	}
	for _, s := range sides {
		if mask&s.face == 0 {
			m.Quad(s.a, s.b, s.c, s.d, s.out, col.Shade(s.shade))
		}
	}
}

// Face selects sides of a box.
type Face uint8

const (
	FaceTop Face = 1 << iota
	FaceBottom
	FaceLeft
	FaceRight
	FaceFront // local -Z
	FaceBack  // local +Z
)

// Cylinder adds an upright n-sided prism of radius r from local y0 to y1.
func (m *Mesh) Cylinder(f Frame, x, z, r, y0, y1 float64, n int, col RGBA, caps bool) {
	m.Frustum(f, x, z, r, r, y0, y1, n, col, caps)
}

// Frustum adds an upright n-sided truncated cone (r1 == 0 makes a cone).
func (m *Mesh) Frustum(f Frame, x, z, r0, r1, y0, y1 float64, n int, col RGBA, caps bool) {
	ring := func(r, y float64) []V3 {
		pts := make([]V3, n)
		for i := range n {
			a := 2 * math.Pi * float64(i) / float64(n)
			pts[i] = f.At(x+r*math.Cos(a), y, z+r*math.Sin(a))
		}
		return pts
	}
	lo, hi := ring(r0, y0), ring(r1, y1)
	c := f.At(x, (y0+y1)/2, z)
	for i := range n {
		j := (i + 1) % n
		mid := lo[i].Add(lo[j]).Add(hi[i]).Add(hi[j]).Scale(0.25)
		out := mid.Sub(c)
		if r1 == 0 {
			m.Poly([]V3{lo[i], lo[j], hi[i]}, out, col)
		} else {
			m.Quad(lo[i], lo[j], hi[j], hi[i], out, col)
		}
	}
	if caps {
		m.Poly(lo, f.Dir(0, -1, 0), col)
		if r1 > 0 {
			m.Poly(hi, f.Dir(0, 1, 0), col)
		}
	}
}

// Wheel adds an n-sided disc of radius r and width w lying on its side,
// with its axle along local X at (x, y, z).
func (m *Mesh) Wheel(f Frame, x, y, z, r, w float64, n int, tyre, hub RGBA) {
	ring := func(dx float64) []V3 {
		pts := make([]V3, n)
		for i := range n {
			a := 2 * math.Pi * float64(i) / float64(n)
			pts[i] = f.At(x+dx, y+r*math.Sin(a), z+r*math.Cos(a))
		}
		return pts
	}
	l, rr := ring(-w/2), ring(w/2)
	c := f.At(x, y, z)
	for i := range n {
		j := (i + 1) % n
		mid := l[i].Add(l[j]).Add(rr[i]).Add(rr[j]).Scale(0.25)
		m.Quad(l[i], l[j], rr[j], rr[i], mid.Sub(c), tyre)
	}
	m.Poly(l, f.Dir(-1, 0, 0), tyre)
	m.Poly(rr, f.Dir(1, 0, 0), tyre)
	// Hubcaps just proud of each side.
	for _, s := range []float64{-1, 1} {
		pts := make([]V3, n)
		for i := range n {
			a := 2 * math.Pi * float64(i) / float64(n)
			pts[i] = f.At(x+s*(w/2+0.01), y+r*0.55*math.Sin(a), z+r*0.55*math.Cos(a))
		}
		m.Poly(pts, f.Dir(s, 0, 0), hub)
	}
}

// smoothQuad adds quad a-b-c-d with per-corner normals, facing towards out.
func (m *Mesh) smoothQuad(a, b, c, d, na, nb, nc, nd, out V3, col RGBA) {
	if b.Sub(a).Cross(d.Sub(a)).Dot(out) < 0 {
		b, d = d, b
		nb, nd = nd, nb
	}
	m.TriN(a, b, c, na, nb, nc, col)
	m.TriN(a, c, d, na, nc, nd, col)
}

// Blob adds a lumpy, smooth-shaded ball, used for foliage. Jitter is derived
// from seed so the same tree always looks the same.
func (m *Mesh) Blob(c V3, rx, ry, rz float64, seed uint64, col RGBA) {
	const rings, segs = 6, 10
	pts := make([][]V3, rings+1)
	nrm := make([][]V3, rings+1)
	for i := 0; i <= rings; i++ {
		phi := math.Pi * float64(i) / rings
		pts[i] = make([]V3, segs)
		nrm[i] = make([]V3, segs)
		for j := range segs {
			th := 2*math.Pi*float64(j)/segs + float64(i%2)*math.Pi/segs
			k := 1.0
			if i > 0 && i < rings {
				seed = seed*6364136223846793005 + 1442695040888963407
				k = 0.86 + 0.26*float64(seed>>40)/float64(1<<24)
			}
			d := v3(math.Sin(phi)*math.Cos(th), math.Cos(phi), math.Sin(phi)*math.Sin(th))
			pts[i][j] = c.Add(v3(rx*k*float64(d.X), ry*k*float64(d.Y), rz*k*float64(d.Z)))
			nrm[i][j] = v3(float64(d.X)/rx, float64(d.Y)/ry, float64(d.Z)/rz).Norm()
		}
	}
	for i := range rings {
		for j := range segs {
			k := (j + 1) % segs
			a, b, cc, d := pts[i][j], pts[i][k], pts[i+1][k], pts[i+1][j]
			out := a.Add(b).Add(cc).Add(d).Scale(0.25).Sub(c)
			shade := float32(1.04 - 0.1*float64(i)/rings) // darker underneath
			m.smoothQuad(a, b, cc, d, nrm[i][j], nrm[i][k], nrm[i+1][k], nrm[i+1][j], out, col.Shade(shade))
		}
	}
}

// Tube adds a smooth-shaded tapered cylinder from a (radius r0) to b (r1).
func (m *Mesh) Tube(a, b V3, r0, r1 float64, n int, col RGBA, caps bool) {
	axis := b.Sub(a).Norm()
	ref := V3{0, 1, 0}
	if math.Abs(float64(axis.Y)) > 0.9 {
		ref = V3{1, 0, 0}
	}
	x := axis.Cross(ref).Norm()
	z := axis.Cross(x).Norm()
	ring := func(o V3, r float64) ([]V3, []V3) {
		pts, ns := make([]V3, n), make([]V3, n)
		for i := range n {
			an := 2 * math.Pi * float64(i) / float64(n)
			dir := x.Scale(float32(math.Cos(an))).Add(z.Scale(float32(math.Sin(an))))
			pts[i] = o.Add(dir.Scale(float32(r)))
			ns[i] = dir
		}
		return pts, ns
	}
	lo, ln := ring(a, r0)
	hi, _ := ring(b, r1)
	for i := range n {
		j := (i + 1) % n
		out := ln[i].Add(ln[j])
		m.smoothQuad(lo[i], lo[j], hi[j], hi[i], ln[i], ln[j], ln[j], ln[i], out, col)
	}
	if caps {
		m.Poly(lo, axis.Scale(-1), col)
		m.Poly(hi, axis, col)
	}
}

// Beam adds a square-section bar of width w from a to b.
func (m *Mesh) Beam(a, b V3, w float64, col RGBA) {
	axis := b.Sub(a)
	l := float64(math.Sqrt(float64(axis.Dot(axis))))
	if l == 0 {
		return
	}
	y := axis.Scale(float32(1 / l))
	ref := V3{0, 1, 0}
	if math.Abs(float64(y.Y)) > 0.9 {
		ref = V3{1, 0, 0}
	}
	x := y.Cross(ref).Norm()
	z := x.Cross(y).Norm()
	m.Box(Frame{O: a, X: x, Y: y, Z: z}, -w/2, 0, -w/2, w/2, l, w/2, col)
}

// RoundedPrism extrudes a rounded rectangle (half extents hx0,hz0 at y0
// tapering to hx1,hz1 at y1, corner radius r) with smooth sides.
func (m *Mesh) RoundedPrism(f Frame, hx0, hz0, hx1, hz1, r, y0, y1 float64, col RGBA, top, bottom bool) {
	const per = 4
	section := func(hx, hz, y float64) ([]V3, []V3) {
		var pts, ns []V3
		corners := [][3]float64{{hx - r, hz - r, 0}, {-(hx - r), hz - r, math.Pi / 2}, {-(hx - r), -(hz - r), math.Pi}, {hx - r, -(hz - r), 3 * math.Pi / 2}}
		for _, c := range corners {
			for i := 0; i <= per; i++ {
				a := c[2] + float64(i)/per*math.Pi/2
				pts = append(pts, f.At(c[0]+r*math.Cos(a), y, c[1]+r*math.Sin(a)))
				ns = append(ns, f.Dir(math.Cos(a), 0, math.Sin(a)).Norm())
			}
		}
		return pts, ns
	}
	lo, ln := section(hx0, hz0, y0)
	hi, _ := section(hx1, hz1, y1)
	n := len(lo)
	for i := range n {
		j := (i + 1) % n
		m.smoothQuad(lo[i], lo[j], hi[j], hi[i], ln[i], ln[j], ln[j], ln[i], ln[i].Add(ln[j]), col)
	}
	if top {
		m.Poly(hi, f.Dir(0, 1, 0), col)
	}
	if bottom {
		m.Poly(lo, f.Dir(0, -1, 0), col)
	}
}

// Strip adds a ribbon between matching left/right edges, facing up.
func (m *Mesh) Strip(left, right []V3, col RGBA) {
	for i := 0; i+1 < len(left) && i+1 < len(right); i++ {
		m.Quad(left[i], right[i], right[i+1], left[i+1], V3{0, 1, 0}, col)
	}
}

// Disc adds a flat, upward-facing n-gon.
func (m *Mesh) Disc(c V3, r float64, n int, col RGBA) {
	pts := make([]V3, n)
	for i := range n {
		a := 2 * math.Pi * float64(i) / float64(n)
		pts[i] = c.Add(v3(r*math.Cos(a), 0, r*math.Sin(a)))
	}
	m.Poly(pts, V3{0, 1, 0}, col)
}
