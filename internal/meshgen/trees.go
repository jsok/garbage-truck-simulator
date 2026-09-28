package meshgen

import (
	"math"

	"github.com/jsok/garbage-truck-simulator/internal/sim"
)

// canopy scatters n foliage blobs around centre c within radius r.
func canopy(m *Mesh, r *rng, c V3, radius, squash float64, n int, cols []RGBA, seed uint64) {
	m.Mat = MatFoliage
	base := r.pick(cols)
	for i := range n {
		a := r.next() * 2 * math.Pi
		d := radius * 0.55 * math.Sqrt(r.next())
		off := v3(math.Cos(a)*d, (r.next()-0.4)*radius*0.5*squash, math.Sin(a)*d)
		s := radius * (0.5 + 0.3*r.next())
		m.Sway = float32(0.45 + 0.4*r.next())
		m.Blob(c.Add(off), s, s*squash, s, seed+uint64(i)*977, base.Shade(float32(0.9+0.2*r.next())))
	}
	m.Sway = 0
}

func buildTree(tr sim.Tree, seed uint64, m *Mesh) {
	defer m.With(MatPlain)()
	r := rng(seed)
	c := At(tr.P, 0)
	H, R := tr.Height, tr.Radius
	up := func(y float64) V3 { return c.Add(v3(0, y, 0)) }
	switch tr.Kind {
	case sim.TreeGum:
		// A leaning pale trunk that forks into a few limbs.
		m.Mat = MatBark
		bark := Hex(0xd6cebf).Mix(Hex(0xb0a898), float32(r.next()))
		lean := v3((r.next()-0.5)*0.8, 0, (r.next()-0.5)*0.8)
		mid := up(H * 0.45).Add(lean)
		m.Tube(c, mid, 0.26, 0.19, 8, bark, false)
		var tips []V3
		for i := range 3 {
			a := r.next()*2*math.Pi + float64(i)*2.1
			tip := mid.Add(v3(math.Cos(a)*R*0.6, H*(0.25+0.15*r.next()), math.Sin(a)*R*0.6))
			m.Tube(mid, tip, 0.15, 0.06, 6, bark, false)
			tips = append(tips, tip)
		}
		for i, tip := range tips {
			canopy(m, &r, tip, R*0.55, 0.7, 3, gum, seed+uint64(i)*31)
			m.Mat = MatBark
		}
	case sim.TreeConifer:
		m.Mat = MatBark
		m.Tube(c, up(H*0.35), 0.2, 0.14, 7, Hex(0x5a4030), false)
		m.Mat = MatFoliage
		col := r.pick(pine)
		const tiers = 4
		for i := range tiers {
			f := float64(i) / tiers
			rad := R * (1 - 0.72*f)
			y0 := H * (0.18 + 0.2*f)
			y1 := y0 + H*(0.4-0.05*f)
			m.Sway = float32(0.15 + 0.3*f)
			m.Frustum(Frame{O: c, X: V3{1, 0, 0}, Y: V3{0, 1, 0}, Z: V3{0, 0, 1}}, 0, 0, rad, 0, y0, y1, 11, col.Shade(float32(0.9+0.12*f)), true)
		}
		m.Sway = 0
	case sim.TreePalm:
		m.Mat = MatBark
		bend := v3((r.next()-0.5)*1.6, 0, (r.next()-0.5)*1.6)
		prev := c
		const segs = 6
		for i := 1; i <= segs; i++ {
			f := float64(i) / segs
			p := up(H * f).Add(bend.Scale(float32(f * f)))
			m.Tube(prev, p, 0.24-0.06*f+0.02, 0.24-0.06*f, 8, Hex(0x8a7458).Shade(float32(0.9+0.15*float64(i%2))), false)
			prev = p
		}
		// Fronds: drooping double-sided ribbons.
		m.Mat = MatFoliage
		m.Sway = 1
		col := Hex(0x4f7a2e)
		n := 10
		for i := range n {
			a := 2*math.Pi*float64(i)/float64(n) + r.next()*0.3
			dir := v3(math.Cos(a), 0, math.Sin(a))
			side := v3(-math.Sin(a), 0, math.Cos(a))
			l := R * (1.1 + 0.4*r.next())
			lift := 0.6 + 0.5*r.next()
			var spine []V3
			for k := 0; k <= 6; k++ {
				f := float64(k) / 6
				spine = append(spine, prev.Add(dir.Scale(float32(l*f))).Add(v3(0, lift*math.Sin(f*math.Pi*0.8)-f*f*l*0.45, 0)))
			}
			for k := 0; k < 6; k++ {
				w0 := 0.5 * math.Sin(math.Pi*float64(k)/6+0.2)
				w1 := 0.5 * math.Sin(math.Pi*float64(k+1)/6+0.2)
				a0, b0 := spine[k].Add(side.Scale(float32(w0))), spine[k].Sub(side.Scale(float32(w0)))
				a1, b1 := spine[k+1].Add(side.Scale(float32(w1))), spine[k+1].Sub(side.Scale(float32(w1)))
				// Droop the leaflets.
				a1, b1 = a1.Add(V3{0, -0.15, 0}), b1.Add(V3{0, -0.15, 0})
				m.Quad(a0, b0, b1, a1, V3{0, 1, 0}, col)
				m.Quad(a0, b0, b1, a1, V3{0, -1, 0}, col.Shade(0.8))
			}
		}
		m.Sway = 0
	default:
		// Round leafy tree: a trunk with branches into a clustered canopy.
		m.Mat = MatBark
		bark := Hex(0x6b5038)
		fork := up(H * 0.45)
		m.Tube(c, fork, 0.24, 0.17, 8, bark, false)
		for i := range 3 {
			a := r.next()*2*math.Pi + float64(i)*2.1
			tip := fork.Add(v3(math.Cos(a)*R*0.45, H*0.25, math.Sin(a)*R*0.45))
			m.Tube(fork, tip, 0.12, 0.05, 6, bark, false)
		}
		cols := leaf
		if r.next() < 0.1 {
			cols = []RGBA{Hex(0x8c6cc8), Hex(0x9a78d0)} // jacaranda
		}
		canopy(m, &r, up(H*0.72), R, 0.8, 6, cols, seed)
	}
}
