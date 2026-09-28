package meshgen

import (
	"math"

	"github.com/jsok/garbage-truck-simulator/internal/sim"
)

// GrassTuft is a clump of crossed, bent blades rooted at the origin.
func GrassTuft() *Mesh {
	m := &Mesh{Mat: MatGrassBlade, Sway: 0.25}
	base, tip := Hex(0x578a3a), Hex(0x9cbd5c)
	up := V3{0, 1, 0}
	for i := range 3 {
		a := float64(i) * math.Pi / 3
		dir := v3(math.Cos(a), 0, math.Sin(a))
		lean := v3(-math.Sin(a), 0, math.Cos(a)).Scale(0.08)
		// Two segments per card, tapering and bending over.
		pts := [][2]V3{
			{dir.Scale(-0.14), dir.Scale(0.14)},
			{dir.Scale(-0.09).Add(V3{0, 0.16, 0}).Add(lean.Scale(0.4)), dir.Scale(0.09).Add(V3{0, 0.16, 0}).Add(lean.Scale(0.4))},
			{dir.Scale(-0.02).Add(V3{0, 0.3, 0}).Add(lean), dir.Scale(0.02).Add(V3{0, 0.3, 0}).Add(lean)},
		}
		cols := []RGBA{base, base.Mix(tip, 0.55), tip}
		face := dir.Cross(up)
		for k := 0; k+1 < len(pts); k++ {
			a0, b0, a1, b1 := pts[k][0], pts[k][1], pts[k+1][0], pts[k+1][1]
			for _, s := range []float32{1, -1} {
				// Mostly-up normals make the blades light like the lawn.
				n := up.Scale(0.8).Add(face.Scale(0.2 * s)).Norm()
				col := cols[k].Mix(cols[k+1], 0.5)
				m.triFacing(a0, b0, b1, face.Scale(s), n, col)
				m.triFacing(a0, b1, a1, face.Scale(s), n, col)
			}
		}
	}
	return m
}

// Tuft places one grass clump.
type Tuft struct {
	X, Z       float32
	Yaw, Scale float32
}

// GrassChunk is the size of the square cells grass is bucketed into.
const GrassChunk = 40.0

// GrassTufts scatters grass clumps over lawns and verges, bucketed by
// GrassChunk-sized cells.
func GrassTufts(t *sim.Town) map[[2]int][]Tuft {
	// Houses and driveways, indexed coarsely for fast rejection.
	const cell = 20.0
	near := map[[2]int][]*sim.House{}
	for i := range t.Houses {
		h := &t.Houses[i]
		k := [2]int{int(math.Floor(h.P.X / cell)), int(math.Floor(h.P.Y / cell))}
		for dx := -1; dx <= 1; dx++ {
			for dz := -1; dz <= 1; dz++ {
				kk := [2]int{k[0] + dx, k[1] + dz}
				near[kk] = append(near[kk], h)
			}
		}
	}
	blocked := func(p sim.V2) bool {
		for _, h := range near[[2]int{int(math.Floor(p.X / cell)), int(math.Floor(p.Y / cell))}] {
			f := sim.Dir(h.Heading)
			d := p.Sub(h.P)
			if math.Abs(d.Dot(f)) < h.D/2+1.3 && math.Abs(d.Dot(f.Left())) < h.W/2+0.3 {
				return true
			}
			a, b := h.Drive[0], h.Drive[1]
			ab := b.Sub(a)
			s := math.Max(0, math.Min(1, p.Sub(a).Dot(ab)/ab.Dot(ab)))
			if p.Dist(a.Add(ab.Scale(s))) < 1.9 {
				return true
			}
		}
		return false
	}
	out := map[[2]int][]Tuft{}
	const step = 0.85
	r := rng(uint64(t.Seed) * 7919)
	for x := t.Min.X; x < t.Max.X; x += step {
		for z := t.Min.Y; z < t.Max.Y; z += step {
			p := sim.V2{X: x + (r.next()-0.5)*step, Y: z + (r.next()-0.5)*step}
			// Patchy: denser in unmown corners.
			if r.next() > 0.3+0.6*noise(p.X/13, p.Y/13) {
				continue
			}
			d := t.RoadDistWithin(p, 9)
			if d < sim.RoadHalfWidth+0.45 || (d > sim.FootpathInner-0.15 && d < sim.FootpathOuter+0.15) || blocked(p) {
				continue
			}
			k := [2]int{int(math.Floor(p.X / GrassChunk)), int(math.Floor(p.Y / GrassChunk))}
			out[k] = append(out[k], Tuft{X: float32(p.X), Z: float32(p.Y), Yaw: float32(r.next() * 2 * math.Pi), Scale: float32(0.7 + 0.7*r.next())})
		}
	}
	return out
}
