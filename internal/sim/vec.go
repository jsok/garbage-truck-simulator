// Package sim holds the engine-independent game logic: the procedurally
// generated suburb, the truck, the side-loader arm and the scoring session.
//
// The world lives on a 2D ground plane. Godot maps sim (X, Y) to 3D (X, 0, Y),
// which makes the plane a "screen-like" frame: looking from above, +X points
// right and +Y points down. A heading h faces Dir(h) = (cos h, sin h).
package sim

import "math"

// V2 is a point or direction on the ground plane, in metres.
type V2 struct{ X, Y float64 }

func (a V2) Add(b V2) V2             { return V2{a.X + b.X, a.Y + b.Y} }
func (a V2) Sub(b V2) V2             { return V2{a.X - b.X, a.Y - b.Y} }
func (a V2) Scale(s float64) V2      { return V2{a.X * s, a.Y * s} }
func (a V2) Dot(b V2) float64        { return a.X*b.X + a.Y*b.Y }
func (a V2) Cross(b V2) float64      { return a.X*b.Y - a.Y*b.X }
func (a V2) Len() float64            { return math.Hypot(a.X, a.Y) }
func (a V2) Dist(b V2) float64       { return a.Sub(b).Len() }
func (a V2) Angle() float64          { return math.Atan2(a.Y, a.X) }
func (a V2) Lerp(b V2, t float64) V2 { return a.Add(b.Sub(a).Scale(t)) }

// Norm returns a unit vector in the direction of a (or zero).
func (a V2) Norm() V2 {
	l := a.Len()
	if l == 0 {
		return V2{}
	}
	return a.Scale(1 / l)
}

// Left is the direction 90° to the left of a when travelling along a.
func (a V2) Left() V2 { return V2{a.Y, -a.X} }

// Right is the direction 90° to the right of a when travelling along a.
func (a V2) Right() V2 { return V2{-a.Y, a.X} }

// Rot rotates a by angle radians (towards +Y from +X).
func (a V2) Rot(angle float64) V2 {
	s, c := math.Sincos(angle)
	return V2{a.X*c - a.Y*s, a.X*s + a.Y*c}
}

// Dir is the unit vector for heading h.
func Dir(h float64) V2 { return V2{math.Cos(h), math.Sin(h)} }

// WrapAngle maps a into (-π, π].
func WrapAngle(a float64) float64 {
	for a > math.Pi {
		a -= 2 * math.Pi
	}
	for a <= -math.Pi {
		a += 2 * math.Pi
	}
	return a
}

func clamp(x, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, x)) }

// approach moves x towards target by at most step.
func approach(x, target, step float64) float64 {
	if x < target {
		return math.Min(x+step, target)
	}
	return math.Max(x-step, target)
}

// segDist returns the distance from p to segment ab and the parameter t of
// the closest point along it.
func segDist(p, a, b V2) (float64, float64) {
	ab := b.Sub(a)
	l2 := ab.Dot(ab)
	t := 0.0
	if l2 > 0 {
		t = clamp(p.Sub(a).Dot(ab)/l2, 0, 1)
	}
	return p.Dist(a.Add(ab.Scale(t))), t
}

// segsIntersect reports whether segments ab and cd properly cross.
func segsIntersect(a, b, c, d V2) bool {
	d1 := b.Sub(a).Cross(c.Sub(a))
	d2 := b.Sub(a).Cross(d.Sub(a))
	d3 := d.Sub(c).Cross(a.Sub(c))
	d4 := d.Sub(c).Cross(b.Sub(c))
	return d1*d2 < 0 && d3*d4 < 0
}
