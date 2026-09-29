package sim

import "math"

// Mishaps, and what they cost.
const (
	OverflowBonus = 50  // extra points for emptying an overflowing bin
	SpillPenalty  = 50  // a full bin tipped over: rubbish all over the street
	KnockPenalty  = 20  // an emptied bin left lying on its back
	KnockSpeed    = 1.0 // m/s; running into a bin faster than this tips it over
	ForkClipAlong = 1.6 // a swing this close along the kerb catches a bin with the jaws
	binFriction   = 7.0 // m/s² deceleration of a sliding bin
)

// Chances is how likely each random mishap is.
type Chances struct {
	Drop            float64 // the bin slips out of the jaws while lifting
	DropOverflowing float64 // the same, for an overflowing bin
	PutDown         float64 // the bin topples when put back; doubled unless lined up perfectly
}

// DefaultChances are the odds in a normal shift.
var DefaultChances = Chances{Drop: 0.04, DropOverflowing: 0.15, PutDown: 0.07}

// TipCause says why a bin fell over.
type TipCause int

const (
	TipHit     TipCause = iota // the truck ran into it
	TipFork                    // a badly lined-up swing knocked it
	TipDrop                    // it slipped out of the jaws
	TipPutDown                 // it toppled when put back
)

// tip knocks bin i over so that its top falls towards dir. A bin that has
// not been emptied yet spills and is lost.
func (s *Session) tip(i int, cause TipCause, dir V2) {
	b := &s.Town.Bins[i]
	if b.Fallen || b.Held {
		return
	}
	if dir == (V2{}) {
		dir = b.FallDir()
	}
	b.Fallen = true
	b.Heading = dir.Angle() + math.Pi
	lost := !b.Collected
	pen := KnockPenalty
	if lost {
		pen = SpillPenalty
		s.Spilled++
		s.Combo, s.lastCollect = 0, math.Inf(-1)
	} else {
		s.Knocked++
	}
	pen = min(pen, s.Score)
	s.Score -= pen
	s.emit(Event{Kind: EvTipped, Bin: i, Cause: cause, Lost: lost, Points: -pen, Colour: b.Colour})
}

// shoveBins pushes bins out of the truck's way. Running into a standing bin
// at more than a crawl knocks it over.
func (s *Session) shoveBins() (shoved bool) {
	tr := &s.Truck
	f := tr.Forward()
	vel := f.Scale(tr.Speed)
	for i := range s.Town.Bins {
		b := &s.Town.Bins[i]
		if b.Held {
			continue
		}
		for _, off := range bodyCircles {
			c := tr.Pos.Add(f.Scale(off))
			bc, br := b.body()
			d := bc.Sub(c)
			dist, lim := d.Len(), bodyRadius+br
			if dist >= lim || dist == 0 {
				continue
			}
			n := d.Scale(1 / dist)
			b.Pos = b.Pos.Add(n.Scale(lim - dist))
			shoved = true
			closing := vel.Dot(n)
			if !b.Fallen && closing <= KnockSpeed {
				b.Heading += 0.3 * (lim - dist)
				continue
			}
			// Bounced off the truck, a little faster than it was going.
			if gain := closing*1.3 - b.Vel.Dot(n); gain > 0 {
				b.Vel = b.Vel.Add(n.Scale(gain))
			}
			if !b.Fallen {
				s.tip(i, TipHit, b.Vel.Norm())
			}
		}
	}
	return shoved
}

// slideBins moves knocked bins until friction or an obstacle stops them.
func (s *Session) slideBins(dt float64) {
	for i := range s.Town.Bins {
		b := &s.Town.Bins[i]
		if b.Vel == (V2{}) {
			continue
		}
		sp := b.Vel.Len()
		if sp <= binFriction*dt {
			b.Vel = V2{}
			continue
		}
		b.Vel = b.Vel.Scale(1 - binFriction*dt/sp)
		b.Pos = b.Pos.Add(b.Vel.Scale(dt))
		c, r := b.body()
		s.Town.obsGrid.around(c, r+9, func(id int) {
			if push := circlePush(c, r, &s.Town.Obs[id]); push != (V2{}) {
				b.Pos, c = b.Pos.Add(push), c.Add(push)
				b.Vel = V2{}
			}
		})
	}
}

// clipped finds the bin the jaws would hit first on a swing out to reach,
// and the extension at which they hit it.
func (s *Session) clipped(reach float64) (bin int, ext float64, dir V2) {
	f := s.Truck.Forward()
	mount := s.Truck.Local(ArmAlong, 0)
	bin, ext = -1, math.Inf(1)
	for i := range s.Town.Bins {
		b := &s.Town.Bins[i]
		if b.Held || b.Fallen {
			continue
		}
		d := b.Pos.Sub(mount)
		along, lat := d.Dot(f), d.Dot(f.Left())
		e := lat - TruckHalfW - BinRadius
		if math.Abs(along) > ForkClipAlong || lat < TruckHalfW || e > reach+0.05 || e >= ext {
			continue
		}
		bin, ext = i, e
		// Swatted outwards, and on along the kerb the way the jaw hit it.
		dir = f.Left().Add(f.Scale(math.Copysign(0.6, along))).Norm()
	}
	return bin, math.Max(ArmStowed, ext), dir
}

// checkClip tips the bin a badly lined-up swing is hitting, once the jaws
// get there.
func (s *Session) checkClip() {
	if s.clipBin < 0 || (s.Arm.Phase == ArmReach && s.Arm.Extension() < s.clipExt) {
		return
	}
	s.Town.Bins[s.clipBin].Vel = s.clipDir.Scale(1.2)
	s.tip(s.clipBin, TipFork, s.clipDir)
	s.clipBin = -1
}

// checkDrop lets the bin slip out of the jaws partway up, if this lift is
// unlucky.
func (s *Session) checkDrop() {
	if s.dropAt == 0 || s.Arm.Phase != ArmLift || s.Arm.Progress() < s.dropAt {
		return
	}
	s.dropAt = 0
	i := s.Arm.Bin
	b := &s.Town.Bins[i]
	pose := s.Arm.Pose()
	b.Held = false
	b.Pos = s.Truck.Local(pose.Bin.Along, math.Max(pose.Bin.Lat, TruckHalfW+BinRadius+0.2))
	s.Arm.drop()
	out := s.Truck.Forward().Left()
	b.Vel = out.Scale(0.8)
	s.tip(i, TipDrop, out.Rot((s.rng.Float64()*2-1)*0.6))
}

// liftLuck decides whether the bin now being lifted will slip.
func (s *Session) liftLuck(b *Bin) {
	chance := s.Chances.Drop
	if b.Overflowing {
		chance = s.Chances.DropOverflowing
	}
	s.dropAt = 0
	if s.rng.Float64() < chance {
		s.dropAt = 0.25 + 0.5*s.rng.Float64()
	}
}

// putDownLuck topples a bin just put back on the ground, sometimes.
func (s *Session) putDownLuck(i int) {
	chance := s.Chances.PutDown
	if !s.perfect {
		chance *= 2
	}
	if s.rng.Float64() < chance {
		out := s.Truck.Forward().Left()
		s.tip(i, TipPutDown, out.Rot((s.rng.Float64()*2-1)*0.8))
	}
}
