package main

import (
	"fmt"
	"math"

	"graphics.gd/classdb/Control"
	"graphics.gd/variant/Color"
	"graphics.gd/variant/Vector2"

	"github.com/jsok/garbage-truck-simulator/internal/sim"
)

// ForkOverlay draws the guidance graphics over the fork camera picture.
type ForkOverlay struct {
	Control.Extension[ForkOverlay]
	game *Game
}

var (
	okGreen = rgba(0.35, 1, 0.45, 1)
	amber   = rgba(1, 0.75, 0.2, 1)
	hotPink = rgba(1, 0.4, 0.6, 1)
)

// project maps a truck-frame ground point to fork camera pixels.
func (o *ForkOverlay) project(s *sim.Session, along, lat, up float64) (Vector2.XY, bool) {
	p := s.Truck.Local(along, lat)
	w := vec(p.X, up, p.Y)
	cam := o.game.truck.ForkCam
	if cam.IsPositionBehind(w) {
		return Vector2.XY{}, false
	}
	return cam.UnprojectPosition(w), true
}

func (o *ForkOverlay) Draw() {
	g := o.game
	if g == nil || g.sess == nil {
		return
	}
	s := g.sess
	ci := o.AsCanvasItem()
	const W, H = float64(forkW), float64(forkH)
	t := s.Target()
	busy := s.Arm.Busy()

	status, col := g.forkStatus(t)

	// Pickup zone painted on the kerb.
	zone := func(tol float64, fill Color.RGBA) {
		var pts []Vector2.XY
		for _, c := range [][2]float64{{-tol, sim.MinBinLateral}, {tol, sim.MinBinLateral}, {tol, sim.MaxBinLateral}, {-tol, sim.MaxBinLateral}} {
			p, ok := o.project(s, sim.ArmAlong+c[0], c[1], 0.03)
			if !ok {
				return
			}
			pts = append(pts, p)
		}
		ci.DrawColoredPolygon(pts, fill)
	}
	if !busy {
		zc := amber
		if t.Aligned {
			zc = okGreen
		}
		zone(sim.AlignTolerance, withAlpha(zc, 0.16))
		zone(sim.PerfectTolerance, withAlpha(zc, 0.2))
		// Centre line of the arm.
		a, ok1 := o.project(s, sim.ArmAlong, sim.MinBinLateral-0.3, 0.03)
		b, ok2 := o.project(s, sim.ArmAlong, sim.MaxBinLateral+0.6, 0.03)
		if ok1 && ok2 {
			line(ci, float64(a.X), float64(a.Y), float64(b.X), float64(b.Y), withAlpha(zc, 0.85), 3)
		}
	}

	// Ring the target bin.
	if t.Bin >= 0 && !busy {
		b := s.Town.Bins[t.Bin]
		p, ok := o.project(s, 0, 0, 0)
		if ok {
			w := vec(b.Pos.X, 0.55, b.Pos.Y)
			if !o.game.truck.ForkCam.IsPositionBehind(w) {
				p = o.game.truck.ForkCam.UnprojectPosition(w)
				r := 58.0 + 6*math.Sin(g.clock*8)
				arc(ci, float64(p.X), float64(p.Y), r, 0, 2*math.Pi, withAlpha(col, 0.9), 4)
				arc(ci, float64(p.X), float64(p.Y), r+8, 0, 2*math.Pi, withAlpha(binColour(b.Colour), 0.9), 3)
			}
		}
	}

	// Direction arrows at the screen edges.
	if !busy && t.Bin >= 0 && t.InReach && !t.Aligned {
		dir := 1.0
		x := W - 50
		if t.Along < 0 {
			dir, x = -1, 50
		}
		n := min(3, 1+int(math.Abs(t.Along)/2))
		for i := range n {
			pulse := 0.5 + 0.5*math.Sin(g.clock*9-float64(i))
			arrow(ci, x-dir*float64(i)*34, H/2, 24, dir, withAlpha(amber, 0.5+0.5*pulse))
		}
	}

	// Scanlines and a frame, so it reads as a little CCTV monitor.
	for y := 0.0; y < H; y += 4 {
		rect(ci, 0, y, W, 1, rgba(0, 0, 0, 0.08))
	}
	rect(ci, 0, 0, W, 44, rgba(0, 0, 0, 0.55))
	rect(ci, 0, H-70, W, 70, rgba(0, 0, 0, 0.6))
	if int(g.clock*2)%2 == 0 {
		ci.DrawCircle(Vector2.New(22, 22), 8, rgba(1, 0.2, 0.2, 1))
	}
	text(ci, 38, 32, "FORK CAM", 26, rgba(1, 1, 1, 0.9), left, 0)
	text(ci, 0, 32, fmt.Sprintf("%d km/h  ", int(math.Round(math.Abs(s.Truck.Speed)*3.6))), 26, rgba(1, 1, 1, 0.9), right, W)
	size := 38
	if textWidth(status, size) > W-20 {
		size = 30
	}
	text(ci, 0, H-22, status, size, col, centre, W)
	outlineRect(ci, 2, 2, W-4, H-4, withAlpha(col, 0.7), 4)
}

// forkStatus is the instruction line on the fork screen.
func (g *Game) forkStatus(t sim.Target) (string, Color.RGBA) {
	s := g.sess
	switch {
	case s.Arm.Busy() && s.Arm.Bin < 0:
		return "MISSED! LINE UP A BIN", hotPink
	case s.Arm.Phase == sim.ArmTip:
		return "TIPPING!", okGreen
	case s.Arm.Busy():
		return "ARM WORKING...", okGreen
	case t.Bin < 0:
		return "NO BINS HERE - FIND ONE ON YOUR LEFT", rgba(0.8, 0.85, 0.9, 1)
	case t.Lateral > sim.MaxBinLateral:
		return "MOVE CLOSER TO THE KERB", amber
	case t.Lateral < sim.MinBinLateral:
		return "TOO CLOSE - EASE AWAY", amber
	case t.Aligned && math.Abs(s.Truck.Speed) > sim.MaxPickupSpeed:
		return "SLOW DOWN!", amber
	case t.Perfect:
		return "PERFECT!  PRESS SPACE", okGreen
	case t.Aligned:
		return "LINED UP!  PRESS SPACE", okGreen
	case t.Along > 0:
		return fmt.Sprintf("FORWARD %.1f m  >>", t.Along), amber
	default:
		return fmt.Sprintf("<<  BACK %.1f m", -t.Along), amber
	}
}
