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
	Tyre         = Hex(0x1c1c1e)
	Hub          = Hex(0xb8bcc2)
	DarkMetal    = Hex(0x3a3d42)
	InteriorGrey = Hex(0x5d6168)
	DashGrey     = Hex(0x2c2e33)
)

// BinColours are the lid colours of the three bin types.
var BinColours = map[sim.Colour]RGBA{
	sim.Red:    Hex(0xd8342c),
	sim.Yellow: Hex(0xf6c21c),
	sim.Green:  Hex(0x78c043),
}

// TruckBody builds the exterior of the truck, which only the fork camera
// (and the title screen) can see.
func TruckBody() *Mesh {
	m := &Mesh{}
	f := Identity
	w := truckHalfW
	// Chassis and wheels.
	m.Box(f, -0.9, 0.45, -4.3, 0.9, 0.9, 4.4, DarkMetal)
	for _, z := range []float64{-3.3, 1.9, 3.1} {
		for _, x := range []float64{-1.05, 1.05} {
			m.Wheel(f, x, 0.52, z, 0.52, 0.42, 14, Tyre, Hub)
		}
		m.Box(f, -w-0.02, 1.04, z-0.7, w+0.02, 1.12, z+0.7, DarkMetal)
	}
	// Cab.
	m.Box(f, -w, 0.75, cabFront, w, 3.15, cabBack, CabWhite)
	m.Box(f, -w+0.08, 1.95, cabFront-0.02, w-0.08, 2.98, cabFront+0.1, Glass)
	for _, s := range []float64{-1, 1} {
		m.Box(f, s*w-0.02, 1.9, -4.4, s*w+0.02, 2.95, -2.9, Glass)
		m.Box(f, s*(w+0.3)-0.04, 2.25, -4.72, s*(w+0.3)+0.04, 2.7, -4.6, DarkMetal) // mirror
	}
	m.Box(f, -w+0.15, 0.85, cabFront-0.04, w-0.15, 1.55, cabFront, DarkMetal) // grille
	for _, x := range []float64{-0.95, 0.95} {
		m.Box(f, x-0.2, 1.1, cabFront-0.06, x+0.2, 1.35, cabFront, Hex(0xfff6d0))
	}
	m.Box(f, -w-0.05, 0.5, cabFront-0.18, w+0.05, 0.8, cabFront+0.1, DarkMetal) // bumper
	m.Box(f, -0.35, 3.15, -3.7, 0.35, 3.35, -3.3, Hex(0xff9a1f))                // beacon
	// Compactor body with a white stripe.
	m.Box(f, -w, 0.9, bodyFront, w, 3.45, bodyBack, BodyGreen)
	m.Box(f, -w-0.01, 2.0, bodyFront+0.1, w+0.01, 2.25, bodyBack-0.1, CabWhite, FaceTop, FaceBottom)
	// Hopper opening above the arm on the left.
	m.Box(f, -w+0.08, 3.44, bodyFront+0.1, 0.3, 3.47, -0.2, Hex(0x151515))
	// Tailgate and lights.
	m.Box(f, -w+0.05, 1.0, bodyBack, w-0.05, 3.3, bodyBack+0.25, BodyGreen.Shade(0.85))
	for _, x := range []float64{-1.0, 1.0} {
		m.Box(f, x-0.15, 1.1, bodyBack+0.25, x+0.15, 1.35, bodyBack+0.28, Hex(0xd02020))
	}
	// Arm carriage rail on the left flank.
	m.Box(f, -w-0.12, 0.95, -sim.ArmAlong-0.8, -w, 1.25, -sim.ArmAlong+0.8, DarkMetal)
	return m
}

// ArmBoom is a unit-length boom running from the origin along +Y.
func ArmBoom() *Mesh {
	m := &Mesh{}
	m.Box(Identity, -0.11, 0, -0.11, 0.11, 1, 0.11, Hex(0xf2b420))
	return m
}

// ArmHead is the gripper carriage, centred on the origin, with its jaws
// reaching along -X (towards the kerb when mounted on the left).
func ArmHead() *Mesh {
	m := &Mesh{}
	m.Box(Identity, -0.12, -0.28, -0.5, 0.12, 0.28, 0.5, DarkMetal)
	m.Box(Identity, -0.14, -0.08, -0.52, -0.1, 0.08, 0.52, Hex(0xf2b420))
	return m
}

// ArmClaw is one jaw finger, extending from the origin along -X.
func ArmClaw() *Mesh {
	m := &Mesh{}
	m.Box(Identity, -0.62, -0.06, -0.05, 0, 0.06, 0.05, Hex(0xf2b420))
	m.Box(Identity, -0.66, -0.12, -0.08, -0.56, 0.12, 0.08, DarkMetal)
	return m
}

// BinBody builds a wheelie bin without its lid; the origin is the centre of
// its base, with the front (away from the hinge) facing -Z.
func BinBody(c sim.Colour) *Mesh {
	m := &Mesh{}
	col := BinColours[c].Shade(0.78)
	bw, bd := 0.24, 0.27
	tw, td := 0.29, 0.33
	h := 0.95
	b := [4]V3{v3(-bw, 0.08, -bd), v3(bw, 0.08, -bd), v3(bw, 0.08, bd), v3(-bw, 0.08, bd)}
	t := [4]V3{v3(-tw, h, -td), v3(tw, h, -td), v3(tw, h, td), v3(-tw, h, td)}
	c0 := V3{0, float32(h / 2), 0}
	for i := range 4 {
		j := (i + 1) % 4
		mid := b[i].Add(b[j]).Add(t[i]).Add(t[j]).Scale(0.25)
		m.Quad(b[i], b[j], t[j], t[i], mid.Sub(c0), col)
	}
	m.Poly(b[:], V3{0, -1, 0}, col.Shade(0.6))
	m.Poly(t[:], V3{0, 1, 0}, Hex(0x222222)) // rubbish inside, seen when the lid opens
	// Rim, handle and wheels at the back.
	m.Box(Identity, -tw-0.02, h-0.06, -td-0.02, tw+0.02, h, td+0.02, col.Shade(0.9), FaceTop)
	m.Box(Identity, -0.22, h-0.12, td+0.02, 0.22, h-0.04, td+0.1, col.Shade(0.7))
	m.Wheel(Identity, -0.27, 0.1, bd-0.02, 0.1, 0.06, 8, Tyre, Hub)
	m.Wheel(Identity, 0.27, 0.1, bd-0.02, 0.1, 0.06, 8, Tyre, Hub)
	return m
}

// BinLid builds a lid hinged at the origin, extending towards -Z.
func BinLid(c sim.Colour) *Mesh {
	m := &Mesh{}
	m.Box(Identity, -0.31, 0, -0.71, 0.31, 0.05, 0.02, BinColours[c])
	m.Box(Identity, -0.31, -0.05, -0.73, 0.31, 0.05, -0.69, BinColours[c].Shade(0.85))
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
	// Dashboard: a deep shelf under the windscreen with a sloped face.
	m.Box(f, -w, 1.2, -4.5, w, 1.98, -4.12, DashGrey)
	m.Quad(v3(-w, 1.98, -4.12), v3(w, 1.98, -4.12), v3(w, 1.55, -3.9), v3(-w, 1.55, -3.9), V3{0, 0.5, 1}, DashGrey.Shade(1.2))
	m.Box(f, 0.3, 1.98, -4.3, 0.95, 2.06, -4.12, DashGrey.Shade(0.8)) // instrument hood
	m.Box(f, 0.47, 1.6, -4.0, 0.63, 1.76, -3.7, DashGrey.Shade(0.7))  // column
	// Windscreen frame.
	for _, s := range []float64{-1, 1} {
		m.Box(f, s*w-0.14, 1.98, -4.5, s*w, 3.1, -4.34, InteriorGrey)
		m.Box(f, s*w-0.06, 1.0, -4.5, s*w, 1.92, -2.4, InteriorGrey.Shade(0.9))  // door panel
		m.Box(f, s*w-0.08, 1.85, -4.4, s*w, 1.95, -2.4, InteriorGrey.Shade(1.1)) // window sill
		m.Box(f, s*w-0.1, 1.0, -2.55, s*w, 3.1, -2.4, InteriorGrey)              // B-pillar
		m.Box(f, s*(w+0.3)-0.04, 2.25, -4.72, s*(w+0.3)+0.04, 2.7, -4.6, DarkMetal)
		m.Box(f, s*(w+0.3)-0.03, 2.28, -4.605, s*(w+0.3)+0.03, 2.67, -4.595, Hex(0x8a9aa8))
		m.Box(f, s*w, 2.45, -4.66, s*(w+0.28), 2.49, -4.62, DarkMetal) // mirror arm
	}
	m.Box(f, -w, 2.95, -4.5, w, 3.1, -4.25, InteriorGrey)            // header
	m.Box(f, -w, 3.08, -4.5, w, 3.16, -2.4, Hex(0xb9b8b0))           // roof lining
	m.Box(f, 0.1, 2.88, -4.28, 1.05, 2.95, -3.9, Hex(0x8c8a80))      // sun visor
	m.Box(f, -w, 1.0, -2.45, w, 3.1, -2.35, InteriorGrey.Shade(0.8)) // back wall
	m.Box(f, -w, 0.98, -4.5, w, 1.02, -2.4, Hex(0x2a2a2a))           // floor
	// Passenger seat and a console.
	m.Box(f, -0.95, 1.0, -3.2, -0.2, 1.5, -2.6, Hex(0x3b4a5c))
	m.Box(f, -0.95, 1.5, -2.72, -0.2, 2.45, -2.55, Hex(0x3b4a5c))
	m.Box(f, -0.1, 1.0, -3.9, 0.15, 1.45, -2.9, DashGrey)
	// A coffee cup, obviously.
	m.Frustum(f, -0.75, -4.3, 0.045, 0.055, 1.98, 2.12, 8, Hex(0xf3efe6), true)
	m.Frustum(f, -0.75, -4.3, 0.056, 0.056, 2.12, 2.14, 8, Hex(0x3a2a20), true)
	return m
}

// ScreenBezel frames the dashboard screen; it is centred on the origin and
// faces +Z, like Godot's QuadMesh.
func ScreenBezel() *Mesh {
	m := &Mesh{}
	w, h := ScreenSize[0]/2, ScreenSize[1]/2
	m.Box(Identity, -w-0.03, -h-0.03, -0.06, w+0.03, h+0.03, -0.002, Hex(0x18191c))
	m.Box(Identity, -w-0.03, -h-0.06, -0.08, w+0.03, -h-0.03, 0.01, Hex(0x18191c))
	m.Box(Identity, -0.03, -h-0.25, -0.12, 0.03, -h-0.03, -0.06, DarkMetal) // mount
	return m
}

// SteeringWheel is centred on the origin in the XY plane, facing +Z.
func SteeringWheel() *Mesh {
	m := &Mesh{}
	const r, n = 0.22, 18
	rim := Hex(0x202124)
	for i := range n {
		a0 := 2 * math.Pi * float64(i) / n
		a1 := 2 * math.Pi * float64(i+1) / n
		sub := Identity.Sub(0, 0, 0, 0)
		p0 := v3(r*math.Cos(a0), r*math.Sin(a0), 0)
		p1 := v3(r*math.Cos(a1), r*math.Sin(a1), 0)
		dir := p1.Sub(p0).Norm()
		mid := p0.Add(p1).Scale(0.5)
		out := mid.Norm()
		sub.O = mid
		sub.X = dir
		sub.Y = out
		sub.Z = V3{0, 0, 1}
		l := float64(p1.Sub(p0).Dot(dir)) / 2
		m.Box(sub, -l-0.004, -0.018, -0.022, l+0.004, 0.018, 0.022, rim)
	}
	for _, a := range []float64{0, math.Pi, -math.Pi / 2} {
		sub := Identity
		sub.X = v3(math.Cos(a), math.Sin(a), 0)
		sub.Y = v3(-math.Sin(a), math.Cos(a), 0)
		m.Box(sub, 0, -0.018, -0.012, r, 0.018, 0.012, Hex(0x3a3b3f))
	}
	m.Box(Identity, -0.06, -0.06, -0.03, 0.06, 0.06, 0.02, Hex(0x3a3b3f))
	m.Box(Identity, -0.025, -0.025, 0.02, 0.025, 0.025, 0.025, Hex(0x2f8f4e))
	return m
}
