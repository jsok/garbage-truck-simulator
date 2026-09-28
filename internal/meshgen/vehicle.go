package meshgen

import (
	"math"

	"github.com/jsok/garbage-truck-simulator/internal/sim"
)

// Truck-local space follows Godot's conventions: +X right, +Y up, -Z forward,
// origin on the ground under the middle of the truck.

// TruckPoint converts a sim truck-frame point to truck-local space.
func TruckPoint(p sim.L3) V3 { return v3(-p.Lat, p.Up, -p.Along) }

// Driver's eye and dashboard layout (right-hand drive).
var (
	EyePos       = V3{0.55, 2.55, -3.3}
	WheelPos     = V3{0.55, 1.98, -3.86}
	WheelTilt    = 0.5 // radians back from vertical
	ScreenPos    = V3{0.02, 2.2, -4.12}
	ScreenSize   = [2]float64{0.58, 0.394}
	SpeedoPos    = V3{0.62, 2.08, -4.2}
	cabFront     = -4.6
	cabBack      = -2.35
	bodyFront    = -2.2
	bodyBack     = 4.35
	truckHalfW   = sim.TruckHalfW
	CabWhite     = Hex(0xf1f1ec)
	BodyGreen    = Hex(0x2f8f4e)
	ArmYellow    = Hex(0xf2b420)
	Tyre         = Hex(0x1c1c1e)
	Hub          = Hex(0xb8bcc2)
	Chrome       = Hex(0xd8dce0)
	DarkMetal    = Hex(0x3a3d42)
	InteriorGrey = Hex(0x6a6e74)
	DashGrey     = Hex(0x2e3035)
	Windscreen   = [4]V3{{-1.14, 1.99, -4.47}, {1.14, 1.99, -4.47}, {1.12, 2.97, -4.55}, {-1.12, 2.97, -4.55}}
)

// BinColours are the colours of the three bin types.
var BinColours = map[sim.Colour]RGBA{
	sim.Red:    Hex(0xd8342c),
	sim.Yellow: Hex(0xf6c21c),
	sim.Green:  Hex(0x78c043),
}

// Tyre adds a smooth tyre of radius r and width w with its axle along local
// X at (x, y, z), and a hub with wheel nuts on the outer side s (±1).
func (m *Mesh) Tyre(f Frame, x, y, z, r, w, s float64, n int) {
	defer m.With(MatRubber)()
	ax := f.Dir(1, 0, 0)
	c := f.At(x, y, z)
	// Tread and rounded shoulders.
	prof := [][2]float64{{-w / 2, r * 0.9}, {-w/2 + 0.04, r}, {w/2 - 0.04, r}, {w / 2, r * 0.9}}
	for k := 0; k+1 < len(prof); k++ {
		for i := range n {
			a0 := 2 * math.Pi * float64(i) / float64(n)
			a1 := 2 * math.Pi * float64(i+1) / float64(n)
			dir := func(a float64) V3 { return f.Dir(0, math.Sin(a), math.Cos(a)) }
			p := func(a float64, q [2]float64) V3 {
				return c.Add(ax.Scale(float32(q[0]))).Add(dir(a).Scale(float32(q[1])))
			}
			nr := func(a float64, q [2]float64) V3 {
				return dir(a).Add(ax.Scale(float32(q[0] / w * 1.2))).Norm()
			}
			a, b := prof[k], prof[k+1]
			m.smoothQuad(p(a0, a), p(a1, a), p(a1, b), p(a0, b), nr(a0, a), nr(a1, a), nr(a1, b), nr(a0, b), dir(a0), Tyre)
		}
	}
	for _, sd := range []float64{-1, 1} {
		ring := make([]V3, n)
		for i := range n {
			a := 2 * math.Pi * float64(i) / float64(n)
			ring[i] = c.Add(ax.Scale(float32(sd * w / 2))).Add(f.Dir(0, math.Sin(a), math.Cos(a)).Scale(float32(r * 0.9)))
		}
		m.Poly(ring, ax.Scale(float32(sd)), Tyre.Shade(1.2))
	}
	// Hub.
	m.Mat = MatMetal
	hub := c.Add(ax.Scale(float32(s * (w/2 - 0.02))))
	m.Tube(hub, hub.Add(ax.Scale(float32(s*0.04))), r*0.58, r*0.5, n, Hub, true)
	m.Tube(hub, hub.Add(ax.Scale(float32(s*0.07))), r*0.18, r*0.14, 10, Chrome, true)
	for i := range 6 {
		a := 2 * math.Pi * float64(i) / 6
		p := hub.Add(ax.Scale(float32(s * 0.04))).Add(f.Dir(0, math.Sin(a), math.Cos(a)).Scale(float32(r * 0.3)))
		m.Tube(p, p.Add(ax.Scale(float32(s*0.03))), 0.018, 0.018, 6, Chrome, true)
	}
}

// TruckBody builds the exterior of the truck, which only the fork camera
// (and the outside debug views) can see.
func TruckBody() *Mesh {
	m := &Mesh{}
	f := Identity
	w := truckHalfW

	// Chassis, fuel tank, exhaust and wheels.
	m.Mat = MatMetal
	m.Box(f, -0.9, 0.45, -4.3, 0.9, 0.9, 4.4, DarkMetal)
	m.Tube(v3(0.95, 0.72, -1.9), v3(0.95, 0.72, -0.6), 0.28, 0.28, 14, Chrome, true)
	m.Tube(v3(0.95, 3.0, -2.28), v3(0.95, 3.9, -2.28), 0.08, 0.08, 10, Chrome, true)
	for _, z := range []float64{-3.3, 1.9, 3.1} {
		for _, x := range []float64{-1.02, 1.02} {
			m.Tyre(f, x, 0.52, z, 0.52, 0.42, math.Copysign(1, x), 20)
		}
		m.Mat = MatPaint
		m.Box(f, -w-0.04, 1.06, z-0.72, w+0.04, 1.12, z+0.72, DarkMetal) // mudguard
		m.Mat = MatRubber
		for _, x := range []float64{-w, w} {
			m.Box(f, x-0.02, 0.35, z+0.55, x+0.02, 1.06, z+0.6, Hex(0x151515)) // mud flap
		}
	}

	// Cab: painted panels, glass, doors and trim.
	m.Mat = MatPaint
	m.RoundedPrism(Frame{O: v3(0, 0, (cabFront+cabBack)/2), X: V3{1, 0, 0}, Y: V3{0, 1, 0}, Z: V3{0, 0, 1}},
		w, (cabBack-cabFront)/2, w-0.06, (cabBack-cabFront)/2-0.06, 0.12, 0.75, 3.15, CabWhite, true, false)
	m.Mat = MatGlass
	m.Quad(Windscreen[0].Add(V3{0, 0, -0.08}), Windscreen[1].Add(V3{0, 0, -0.08}), Windscreen[2].Add(V3{0, 0, -0.03}), Windscreen[3].Add(V3{0, 0, -0.03}), V3{0, 0.1, -1}, Glass)
	for _, s := range []float64{-1, 1} {
		m.Box(f, s*w-0.015, 1.9, -4.35, s*w+0.015, 2.95, -2.9, Glass)
		m.Mat = MatRubber
		m.Box(f, s*w-0.02, 0.8, -2.95, s*w+0.02, 3.0, -2.92, Hex(0x222222)) // door shut line
		m.Mat = MatMetal
		m.Box(f, s*(w+0.02)-0.01, 1.7, -3.2, s*(w+0.02)+0.01, 1.76, -3.0, Chrome)    // handle
		m.Box(f, s*(w+0.3)-0.05, 2.2, -4.74, s*(w+0.3)+0.05, 2.75, -4.58, DarkMetal) // mirror
		m.Box(f, s*w, 2.45, -4.68, s*(w+0.28), 2.5, -4.62, DarkMetal)
		m.Box(f, s*(w-0.2)-0.2, 0.55, -4.1, s*(w-0.2)+0.2, 0.6, -3.1, Chrome) // step
		m.Mat = MatPaint
	}
	// Grille, lights, bumper and number plate.
	m.Mat = MatMetal
	m.Box(f, -w+0.2, 0.85, cabFront-0.05, w-0.2, 1.62, cabFront, DarkMetal)
	for y := 0.92; y < 1.6; y += 0.12 {
		m.Box(f, -w+0.25, y, cabFront-0.08, w-0.25, y+0.04, cabFront-0.05, Chrome)
	}
	m.Mat = MatLight
	for _, x := range []float64{-0.95, 0.95} {
		m.Box(f, x-0.2, 1.12, cabFront-0.07, x+0.2, 1.36, cabFront, Hex(0xfff6d0).Shade(0.5))
		m.Box(f, x-0.2, 0.98, cabFront-0.07, x+0.2, 1.08, cabFront, Hex(0xff9a1f).Shade(0.5))
	}
	m.Mat = MatPaint
	m.Box(f, -w-0.05, 0.48, cabFront-0.2, w+0.05, 0.8, cabFront+0.1, DarkMetal)
	m.Mat = MatPlain
	m.Box(f, -0.26, 0.55, cabFront-0.22, 0.26, 0.72, cabFront-0.2, Hex(0xf4f4f0))
	m.Box(f, -1.0, 3.12, cabFront-0.12, 1.0, 3.18, cabFront+0.3, DarkMetal) // sun visor
	m.Mat = MatLight
	m.Tube(v3(0, 3.15, -3.5), v3(0, 3.38, -3.5), 0.16, 0.13, 12, Hex(0xff8a10).Shade(0.6), true) // beacon

	// Compactor body with ribs, stripe and side lights.
	m.Mat = MatPaint
	m.Box(f, -w, 0.9, bodyFront, w, 3.45, bodyBack, BodyGreen)
	m.Box(f, -w-0.012, 1.95, bodyFront+0.1, w+0.012, 2.25, bodyBack-0.1, CabWhite, FaceTop, FaceBottom)
	m.Box(f, -w-0.03, 3.38, bodyFront, w+0.03, 3.5, bodyBack, BodyGreen.Shade(0.9)) // top rail
	for z := bodyFront + 1.6; z < bodyBack-0.3; z += 0.85 {
		for _, s := range []float64{-1, 1} {
			m.Box(f, s*w-0.05, 0.95, z-0.05, s*w+0.05, 3.38, z+0.05, BodyGreen.Shade(0.92))
		}
	}
	m.Mat = MatLight
	for z := bodyFront + 0.8; z < bodyBack; z += 1.6 {
		for _, s := range []float64{-1, 1} {
			m.Box(f, s*w-0.02, 1.0, z-0.07, s*w+0.02, 1.08, z+0.07, Hex(0xff9a1f).Shade(0.5))
		}
	}
	// Hopper opening above the arm.
	m.Mat = MatPlain
	m.Box(f, -w+0.08, 3.47, bodyFront+0.1, 0.3, 3.5, -0.2, Hex(0x151515))

	// Tailgate: hinged, with lights, chevrons and a ladder.
	m.Mat = MatPaint
	m.RoundedPrism(Frame{O: v3(0, 0, bodyBack+0.2), X: V3{1, 0, 0}, Y: V3{0, 1, 0}, Z: V3{0, 0, 1}}, w-0.02, 0.22, w-0.06, 0.18, 0.1, 1.0, 3.35, BodyGreen.Shade(0.85), true, true)
	for i := range 8 {
		col := Hex(0xd82020)
		if i%2 == 1 {
			col = Hex(0xf2f2ea)
		}
		m.Box(f, -w+0.05+float64(i)*0.3, 1.02, bodyBack+0.42, -w+0.33+float64(i)*0.3, 1.2, bodyBack+0.44, col)
	}
	m.Mat = MatLight
	for _, x := range []float64{-1.0, 1.0} {
		m.Box(f, x-0.14, 1.3, bodyBack+0.4, x+0.14, 1.52, bodyBack+0.44, Hex(0xd02020).Shade(0.6))
	}
	m.Mat = MatMetal
	for _, x := range []float64{0.7, 1.0} {
		m.Box(f, x-0.02, 0.6, bodyBack+0.45, x+0.02, 2.6, bodyBack+0.49, Chrome)
	}
	for y := 0.8; y < 2.6; y += 0.3 {
		m.Box(f, 0.7, y, bodyBack+0.45, 1.0, y+0.03, bodyBack+0.49, Chrome)
	}

	// Arm carriage rail on the left flank.
	m.Mat = MatMetal
	m.Box(f, -w-0.14, 0.95, -sim.ArmAlong-0.85, -w, 1.28, -sim.ArmAlong+0.85, DarkMetal)
	m.Tube(v3(-w-0.1, 1.12, -sim.ArmAlong-0.8), v3(-w-0.1, 1.12, -sim.ArmAlong+0.8), 0.05, 0.05, 8, Chrome, true)
	return m
}

// ArmBoom is a unit-length telescopic boom running from the origin along +Y.
func ArmBoom() *Mesh {
	m := &Mesh{}
	m.Mat = MatPaint
	m.Box(Identity, -0.12, 0, -0.12, 0.12, 0.6, 0.12, ArmYellow)
	m.Mat = MatMetal
	m.Box(Identity, -0.09, 0.6, -0.09, 0.09, 1, 0.09, Chrome)
	m.Mat = MatRubber
	m.Box(Identity, -0.13, 0.58, -0.13, 0.13, 0.62, 0.13, Hex(0x1a1a1a))
	return m
}

// ArmHead is the gripper carriage, centred on the origin, with its jaws
// reaching along -X (towards the kerb when mounted on the left).
func ArmHead() *Mesh {
	m := &Mesh{}
	m.Mat = MatPaint
	m.Box(Identity, -0.12, -0.28, -0.52, 0.12, 0.28, 0.52, ArmYellow)
	m.Mat = MatMetal
	m.Box(Identity, -0.14, -0.1, -0.54, -0.1, 0.1, 0.54, DarkMetal)
	m.Tube(v3(0.05, 0.18, -0.45), v3(0.05, 0.18, 0.45), 0.045, 0.045, 8, Chrome, true)
	return m
}

// ArmClaw is one curved jaw finger, extending from the origin along -X and
// curving towards +Z (side 1) or -Z (side -1).
func ArmClaw(side float32) *Mesh {
	m := &Mesh{}
	m.Mat = MatPaint
	pts := []V3{{0, 0, 0}, {-0.3, 0, 0.03 * side}, {-0.55, 0, 0.1 * side}, {-0.68, 0, 0.2 * side}}
	for i := 0; i+1 < len(pts); i++ {
		m.Beam(pts[i], pts[i+1], 0.1, ArmYellow)
	}
	m.Mat = MatRubber
	z0, z1 := 0.02*float64(side), 0.06*float64(side)
	m.Box(Identity, -0.62, -0.12, math.Min(z0, z1), -0.4, 0.12, math.Max(z0, z1), Hex(0x1a1a1a)) // grip pad
	return m
}

// BinBody builds a wheelie bin without its lid; the origin is the centre of
// its base, with the front (away from the hinge) facing -Z.
func BinBody(c sim.Colour) *Mesh {
	m := &Mesh{}
	col := BinColours[c].Shade(0.82)
	f := Identity
	m.Mat = MatPlastic
	m.RoundedPrism(f, 0.24, 0.27, 0.285, 0.325, 0.06, 0.06, 0.9, col, false, true)
	m.RoundedPrism(f, 0.3, 0.34, 0.305, 0.345, 0.06, 0.88, 0.95, col.Shade(0.92), true, false)   // rim
	m.RoundedPrism(f, 0.25, 0.28, 0.255, 0.285, 0.06, 0.06, 0.16, col.Shade(0.85), false, false) // foot
	m.Mat = MatPlain
	m.Quad(v3(-0.26, 0.955, -0.3), v3(0.26, 0.955, -0.3), v3(0.26, 0.955, 0.3), v3(-0.26, 0.955, 0.3), V3{0, 1, 0}, Hex(0x1e1e1e)) // rubbish
	m.Box(f, -0.12, 0.62, -0.31, 0.12, 0.72, -0.302, Hex(0xf4f4f0))                                                                // council sticker
	// Handle, axle and wheels at the back.
	m.Mat = MatPlastic
	m.Tube(v3(-0.24, 0.9, 0.37), v3(0.24, 0.9, 0.37), 0.022, 0.022, 8, col.Shade(0.7), true)
	for _, x := range []float64{-0.22, 0.22} {
		m.Beam(v3(x, 0.84, 0.31), v3(x, 0.9, 0.37), 0.04, col.Shade(0.7))
	}
	m.Mat = MatMetal
	m.Tube(v3(-0.3, 0.11, 0.26), v3(0.3, 0.11, 0.26), 0.015, 0.015, 6, DarkMetal, true)
	m.Tyre(f, -0.28, 0.11, 0.26, 0.11, 0.06, -1, 12)
	m.Tyre(f, 0.28, 0.11, 0.26, 0.11, 0.06, 1, 12)
	return m
}

// BinLid builds a lid hinged at the origin, extending towards -Z.
func BinLid(c sim.Colour) *Mesh {
	m := &Mesh{}
	m.Mat = MatPlastic
	col := BinColours[c]
	m.RoundedPrism(Frame{O: v3(0, 0, -0.355), X: V3{1, 0, 0}, Y: V3{0, 1, 0}, Z: V3{0, 0, 1}}, 0.315, 0.37, 0.305, 0.36, 0.05, 0, 0.05, col, true, true)
	m.Box(Identity, -0.3, -0.06, -0.73, 0.3, 0.0, -0.7, col.Shade(0.85)) // front lip
	m.Box(Identity, -0.08, 0.05, -0.7, 0.08, 0.07, -0.6, col.Shade(0.8)) // lifting tab
	for _, x := range []float64{-0.2, 0.2} {
		m.Tube(v3(x-0.06, 0, 0), v3(x+0.06, 0, 0), 0.03, 0.03, 8, col.Shade(0.75), true)
	}
	return m
}

// BinLidHinge is where the lid attaches to the body, in bin space.
var BinLidHinge = V3{0, 0.95, 0.35}

// Marker is a floating diamond shown above uncollected bins.
func Marker(col RGBA) *Mesh {
	m := &Mesh{}
	top, bot := V3{0, 0.35, 0}, V3{0, -0.35, 0}
	ring := []V3{{0.22, 0, 0}, {0, 0, 0.22}, {-0.22, 0, 0}, {0, 0, -0.22}}
	for i := range ring {
		a, b := ring[i], ring[(i+1)%4]
		out := a.Add(b)
		m.Poly([]V3{a, b, top}, out.Add(V3{0, 0.3, 0}), col)
		m.Poly([]V3{a, b, bot}, out.Add(V3{0, -0.3, 0}), col.Shade(0.7))
	}
	return m
}

// Cab builds the inside of the cab as seen from the driver's seat.
func Cab() *Mesh {
	m := &Mesh{}
	f := Identity
	w := truckHalfW
	m.Mat = MatDash
	// Dashboard: padded top, sloped face, vents and an instrument hood.
	m.Box(f, -w, 1.2, -4.5, w, 1.96, -4.12, DashGrey)
	m.RoundedPrism(Frame{O: v3(0, 0, -4.32), X: V3{1, 0, 0}, Y: V3{0, 1, 0}, Z: V3{0, 0, 1}}, w-0.02, 0.22, w-0.04, 0.2, 0.06, 1.9, 1.99, DashGrey.Shade(1.25), true, false)
	m.Quad(v3(-w, 1.96, -4.12), v3(w, 1.96, -4.12), v3(w, 1.55, -3.92), v3(-w, 1.55, -3.92), V3{0, 0.5, 1}, DashGrey.Shade(1.1))
	vent := func(x float64) {
		m.Mat = MatPlain
		m.Box(f, x-0.1, 1.72, -4.06, x+0.1, 1.84, -4.0, Hex(0x151517))
		for y := 1.74; y < 1.83; y += 0.025 {
			m.Box(f, x-0.095, y, -4.0, x+0.095, y+0.008, -3.99, Hex(0x3a3c40))
		}
		m.Mat = MatDash
	}
	for _, x := range []float64{-1.0, -0.5, 0.18, 1.05} {
		vent(x)
	}
	m.RoundedPrism(Frame{O: v3(0.62, 0, -4.2), X: V3{1, 0, 0}, Y: V3{0, 1, 0}, Z: V3{0, 0, 1}}, 0.33, 0.1, 0.31, 0.08, 0.05, 1.96, 2.07, DashGrey.Shade(0.8), true, false) // instrument hood
	m.Tube(v3(0.55, 1.62, -4.02), v3(0.55, 1.85, -3.76), 0.06, 0.05, 10, DashGrey.Shade(0.7), true)                                                                        // column
	// Two dials behind the wheel.
	for _, x := range []float64{0.42, 0.82} {
		c := v3(x, 2.0, -4.1)
		m.Mat = MatMetal
		m.Tube(c, c.Add(V3{0, 0.01, 0.02}), 0.075, 0.075, 16, Chrome, true)
		m.Mat = MatPlain
		m.Tube(c.Add(V3{0, 0.005, 0.01}), c.Add(V3{0, 0.012, 0.025}), 0.065, 0.065, 16, Hex(0x111214), true)
		m.Mat = MatLight
		m.Beam(c.Add(V3{0, 0.01, 0.027}), c.Add(v3(0.035, 0.05, 0.027)), 0.006, Hex(0xff5020).Shade(0.6))
	}
	// Buttons and LEDs by the screen.
	m.Mat = MatLight
	for i, col := range []RGBA{Hex(0x30ff60), Hex(0xffb020), Hex(0x30ff60), Hex(0xff3030)} {
		x := -0.36 + float64(i)*0.05
		m.Box(f, x-0.012, 1.9, -4.02, x+0.012, 1.925, -4.0, col.Shade(0.45))
	}
	m.Mat = MatDash
	m.Box(f, -1.1, 1.3, -4.02, -0.4, 1.6, -3.96, DashGrey.Shade(1.15)) // glovebox
	m.Box(f, -0.8, 1.54, -3.96, -0.7, 1.56, -3.94, Chrome)

	// Windscreen frame, doors and pillars.
	m.Mat = MatDash
	for _, s := range []float64{-1, 1} {
		m.Tube(v3(s*(w-0.07), 1.98, -4.47), v3(s*(w-0.07), 3.08, -4.55), 0.07, 0.07, 8, InteriorGrey, true) // A-pillar
		m.Box(f, s*w-0.08, 1.0, -4.5, s*w, 1.92, -2.4, InteriorGrey.Shade(0.9))                             // door panel
		m.Box(f, s*w-0.14, 1.45, -3.9, s*w-0.08, 1.55, -3.0, InteriorGrey.Shade(0.75))                      // armrest
		m.Box(f, s*w-0.1, 1.85, -4.4, s*w, 1.95, -2.4, InteriorGrey.Shade(1.1))                             // sill
		m.Box(f, s*w-0.1, 1.0, -2.55, s*w, 3.1, -2.4, InteriorGrey)                                         // B-pillar
		m.Mat = MatMetal
		m.Box(f, s*w-0.12, 1.62, -3.4, s*w-0.09, 1.68, -3.25, Chrome) // door handle
		m.Box(f, s*(w+0.3)-0.05, 2.2, -4.74, s*(w+0.3)+0.05, 2.75, -4.58, DarkMetal)
		m.Box(f, s*w, 2.45, -4.68, s*(w+0.28), 2.5, -4.62, DarkMetal) // mirror arm
		m.Mat = MatGlass
		m.Box(f, s*(w+0.3)-0.04, 2.23, -4.585, s*(w+0.3)+0.04, 2.72, -4.575, Hex(0x9aabbc))
		m.Mat = MatDash
	}
	m.Box(f, -w, 2.96, -4.55, w, 3.1, -4.3, InteriorGrey)            // header
	m.Box(f, -w, 3.08, -4.55, w, 3.16, -2.4, Hex(0xbdbcb4))          // roof lining
	m.Box(f, 0.12, 2.87, -4.3, 1.05, 2.94, -3.92, Hex(0x8c8a80))     // driver's sun visor
	m.Box(f, -1.05, 2.9, -4.3, -0.12, 2.96, -3.95, Hex(0x8c8a80))    // passenger's, folded up
	m.Box(f, -w, 1.0, -2.45, w, 3.1, -2.35, InteriorGrey.Shade(0.8)) // back wall
	m.Box(f, -w, 0.98, -4.5, w, 1.02, -2.4, Hex(0x2a2a2a))           // floor
	m.Mat = MatLight
	m.Box(f, -0.15, 3.05, -3.4, 0.15, 3.08, -3.25, Hex(0xf6f2e0).Shade(0.35)) // cab light
	m.Mat = MatPlastic
	m.Box(f, -w+0.1, 2.75, -3.3, -w+0.13, 2.8, -2.9, DarkMetal) // grab handle

	// Passenger seat, console, and the things drivers leave lying about.
	m.Mat = MatDash
	seat := Frame{O: v3(-0.58, 0, -2.9), X: V3{1, 0, 0}, Y: V3{0, 1, 0}, Z: V3{0, 0, 1}}
	m.RoundedPrism(seat, 0.36, 0.3, 0.34, 0.28, 0.08, 1.02, 1.5, Hex(0x3b4a5c), true, false)
	m.RoundedPrism(Frame{O: v3(-0.58, 0, -2.6), X: V3{1, 0, 0}, Y: V3{0, 1, 0}, Z: V3{0, 0, 1}}, 0.34, 0.08, 0.3, 0.07, 0.06, 1.5, 2.45, Hex(0x3b4a5c), true, false)
	m.Box(f, -0.1, 1.0, -3.9, 0.15, 1.45, -2.9, DashGrey)
	m.Mat = MatPlain
	m.Box(f, -0.8, 1.5, -3.1, -0.35, 1.53, -2.72, Hex(0xff7a10)) // hi-vis vest
	m.Box(f, -0.8, 1.531, -2.95, -0.35, 1.54, -2.9, Hex(0xd8d8c8))
	m.Box(f, -1.05, 1.99, -4.35, -0.8, 2.0, -4.18, Hex(0x8a6a44)) // clipboard
	m.Box(f, -1.03, 2.0, -4.33, -0.82, 2.005, -4.2, Hex(0xf4f4ee))
	m.Mat = MatPlastic
	m.Tube(v3(-0.62, 1.99, -4.28), v3(-0.62, 2.12, -4.28), 0.045, 0.055, 12, Hex(0xf3efe6), true) // coffee
	m.Mat = MatPlain
	m.Tube(v3(-0.62, 2.12, -4.28), v3(-0.62, 2.135, -4.28), 0.057, 0.057, 12, Hex(0x3a2a20), true)
	// Wipers resting at the base of the windscreen.
	m.Mat = MatRubber
	for _, x := range []float64{-0.75, 0.35} {
		m.Beam(v3(x, 1.97, -4.49), v3(x+0.7, 2.0, -4.5), 0.02, Hex(0x151515))
	}
	return m
}

// ScreenBezel frames the dashboard screen; it is centred on the origin and
// faces +Z, like Godot's QuadMesh.
func ScreenBezel() *Mesh {
	m := &Mesh{}
	m.Mat = MatPlastic
	w, h := ScreenSize[0]/2, ScreenSize[1]/2
	m.RoundedPrism(Frame{O: v3(0, 0, -0.035), X: V3{1, 0, 0}, Y: V3{0, 0, 1}, Z: V3{0, 1, 0}}, w+0.035, h+0.035, w+0.035, h+0.035, 0.025, -0.03, 0.03, Hex(0x18191c), true, true)
	m.Box(Identity, -w+0.02, -h-0.03, 0.0, w-0.02, -h-0.02, 0.012, Hex(0x2a2b2e)) // lower lip
	m.Mat = MatMetal
	m.Box(Identity, -0.03, -h-0.25, -0.12, 0.03, -h-0.03, -0.06, DarkMetal) // mount
	m.Mat = MatLight
	m.Box(Identity, w-0.03, -h-0.028, 0.003, w-0.015, -h-0.022, 0.013, Hex(0x30ff60).Shade(0.5)) // power LED
	return m
}

// SteeringWheel is centred on the origin in the XY plane, facing +Z.
func SteeringWheel() *Mesh {
	m := &Mesh{}
	m.Mat = MatRubber
	const r, n, tube = 0.22, 32, 0.022
	rim := Hex(0x202124)
	// A smooth torus rim.
	const k = 8
	for i := range n {
		for j := range k {
			pt := func(a, b float64) (V3, V3) {
				c := v3(r*math.Cos(a), r*math.Sin(a), 0)
				d := v3(math.Cos(a)*math.Cos(b), math.Sin(a)*math.Cos(b), math.Sin(b))
				return c.Add(d.Scale(tube)), d
			}
			a0, a1 := 2*math.Pi*float64(i)/n, 2*math.Pi*float64(i+1)/n
			b0, b1 := 2*math.Pi*float64(j)/k, 2*math.Pi*float64(j+1)/k
			p00, n00 := pt(a0, b0)
			p10, n10 := pt(a1, b0)
			p11, n11 := pt(a1, b1)
			p01, n01 := pt(a0, b1)
			m.smoothQuad(p00, p10, p11, p01, n00, n10, n11, n01, n00.Add(n11), rim)
		}
	}
	m.Mat = MatDash
	for _, a := range []float64{0.15, math.Pi - 0.15, -math.Pi / 2} {
		m.Beam(V3{}, v3(r*math.Cos(a), r*math.Sin(a), -0.01), 0.03, Hex(0x3a3b3f))
	}
	m.Tube(v3(0, 0, -0.03), v3(0, 0, 0.03), 0.07, 0.065, 16, Hex(0x3a3b3f), true)
	m.Mat = MatPaint
	m.Tube(v3(0, 0, 0.03), v3(0, 0, 0.036), 0.028, 0.028, 12, BodyGreen, true)
	return m
}
