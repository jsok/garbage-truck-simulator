package sim

import "math"

// Truck dimensions and handling, in metres and seconds. The truck frame has
// "along" pointing forwards and "lateral" pointing to the left.
const (
	TruckLength = 9.2
	TruckHalfW  = 1.25
	Wheelbase   = 5.0
	MaxSpeed    = 15.0 // ≈54 km/h
	MaxReverse  = 5.0
	accel       = 4.2
	reverseAcc  = 2.6
	brakeDecel  = 11.0
	coastDecel  = 1.4
	maxSteer    = 0.62
	steerRate   = 1.9
	bodyRadius  = 1.3
)

var bodyCircles = [...]float64{3.3, 1.1, -1.1, -3.3}

// Controls is the driver's input for one step.
type Controls struct {
	Throttle  float64 // -1 brake/reverse .. 1 accelerate
	Steer     float64 // -1 left .. 1 right
	Handbrake bool
}

// Truck is the player's side-loader garbage truck.
type Truck struct {
	Pos     V2
	Heading float64
	Speed   float64 // signed speed along the heading
	Steer   float64 // front wheel angle; positive turns right
	Locked  bool    // held stationary while the arm works
	Odo     float64 // distance driven
}

// Forward is the truck's unit heading vector.
func (tr *Truck) Forward() V2 { return Dir(tr.Heading) }

// Local converts a truck-frame offset to a world position.
func (tr *Truck) Local(along, lateral float64) V2 {
	f := tr.Forward()
	return tr.Pos.Add(f.Scale(along)).Add(f.Left().Scale(lateral))
}

// Step advances the truck and resolves collisions. It returns the speed of
// any impact (0 if none) and whether it shoved a bin.
func (tr *Truck) Step(dt float64, c Controls, town *Town) (impact float64, shoved bool) {
	if tr.Locked {
		tr.Speed = approach(tr.Speed, 0, 25*dt)
	} else {
		tr.drive(dt, c)
	}
	tr.Heading = WrapAngle(tr.Heading + tr.Speed/Wheelbase*math.Tan(tr.Steer)*dt)
	tr.Pos = tr.Pos.Add(tr.Forward().Scale(tr.Speed * dt))
	tr.Odo += math.Abs(tr.Speed * dt)
	impact = tr.collide(town)
	shoved = tr.shoveBins(town.Bins)
	return impact, shoved
}

func (tr *Truck) drive(dt float64, c Controls) {
	switch {
	case c.Handbrake:
		tr.Speed = approach(tr.Speed, 0, brakeDecel*1.3*dt)
	case c.Throttle > 0 && tr.Speed < -0.2:
		tr.Speed = approach(tr.Speed, 0, brakeDecel*c.Throttle*dt)
	case c.Throttle > 0:
		taper := 1 - (tr.Speed/MaxSpeed)*(tr.Speed/MaxSpeed)
		tr.Speed = math.Min(MaxSpeed, tr.Speed+accel*c.Throttle*math.Max(taper, 0)*dt)
	case c.Throttle < 0 && tr.Speed > 0.2:
		tr.Speed = approach(tr.Speed, 0, brakeDecel*-c.Throttle*dt)
	case c.Throttle < 0:
		tr.Speed = math.Max(-MaxReverse, tr.Speed-reverseAcc*-c.Throttle*dt)
	default:
		tr.Speed = approach(tr.Speed, 0, coastDecel*dt)
	}
	// Steering lock tightens at speed so the truck stays manageable.
	limit := maxSteer / (1 + math.Abs(tr.Speed)/8)
	target := clamp(c.Steer, -1, 1) * limit
	rate := steerRate
	if c.Steer == 0 {
		rate *= 1.6 // self-centring
	}
	tr.Steer = approach(tr.Steer, target, rate*dt)
	tr.Steer = clamp(tr.Steer, -limit, limit)
}

func (tr *Truck) collide(town *Town) float64 {
	var total V2
	for range 3 {
		moved := false
		f := tr.Forward()
		for _, off := range bodyCircles {
			c := tr.Pos.Add(f.Scale(off))
			town.obsGrid.around(c, bodyRadius+9, func(id int) {
				if push := circlePush(c, bodyRadius, &town.Obs[id]); push != (V2{}) {
					tr.Pos = tr.Pos.Add(push)
					c = c.Add(push)
					total = total.Add(push)
					moved = true
				}
			})
		}
		if !moved {
			break
		}
	}
	// Keep inside the suburb.
	lo, hi := town.Min.Add(V2{6, 6}), town.Max.Sub(V2{6, 6})
	clamped := V2{clamp(tr.Pos.X, lo.X, hi.X), clamp(tr.Pos.Y, lo.Y, hi.Y)}
	total = total.Add(clamped.Sub(tr.Pos))
	tr.Pos = clamped

	if total == (V2{}) {
		return 0
	}
	n := total.Norm()
	into := -tr.Forward().Scale(tr.Speed).Dot(n)
	if into <= 0.05 {
		return 0
	}
	tr.Speed *= 0.25
	return into
}

// circlePush returns the displacement that moves a circle at c with radius r
// out of obstacle o.
func circlePush(c V2, r float64, o *Obstacle) V2 {
	d := c.Sub(o.P)
	if o.Radius > 0 {
		dist := d.Len()
		if dist >= r+o.Radius || dist == 0 {
			return V2{}
		}
		return d.Scale((r + o.Radius - dist) / dist)
	}
	f := Dir(o.Heading)
	l := f.Left()
	lx, ly := d.Dot(f), d.Dot(l)
	if math.Abs(lx) > o.HalfD+r || math.Abs(ly) > o.HalfW+r {
		return V2{}
	}
	cx, cy := clamp(lx, -o.HalfD, o.HalfD), clamp(ly, -o.HalfW, o.HalfW)
	diff := d.Sub(f.Scale(cx).Add(l.Scale(cy)))
	if dist := diff.Len(); dist > 1e-6 {
		if dist >= r {
			return V2{}
		}
		return diff.Scale((r - dist) / dist)
	}
	// Centre inside the box: leave by the shallowest side.
	px, py := o.HalfD-math.Abs(lx), o.HalfW-math.Abs(ly)
	if px < py {
		return f.Scale(math.Copysign(px+r, lx))
	}
	return l.Scale(math.Copysign(py+r, ly))
}

func (tr *Truck) shoveBins(bins []Bin) bool {
	shoved := false
	f := tr.Forward()
	for i := range bins {
		b := &bins[i]
		if b.Held {
			continue
		}
		for _, off := range bodyCircles {
			c := tr.Pos.Add(f.Scale(off))
			d := b.Pos.Sub(c)
			if dist, lim := d.Len(), bodyRadius+BinRadius; dist < lim && dist > 0 {
				b.Pos = b.Pos.Add(d.Scale((lim - dist) / dist))
				b.Heading += 0.3 * (lim - dist)
				shoved = true
			}
		}
	}
	return shoved
}
