package main

import (
	"fmt"
	"math"

	"graphics.gd/classdb/Control"
	"graphics.gd/variant/Rect2"
	"graphics.gd/variant/Vector2"

	"github.com/jsok/garbage-truck-simulator/internal/sim"
)

// HUD draws everything on top of the driver's view, plus the menus.
type HUD struct {
	Control.Extension[HUD]
	game *Game
}

// Minimap is a heading-up map of the nearby streets and bins.
type Minimap struct {
	Control.Extension[Minimap]
	game *Game
}

type popup struct {
	msg  string
	col  [4]float64
	age  float64
	size int
}

func clockText(secs float64) string {
	s := int(math.Ceil(secs))
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

func rank(bins int) string {
	switch {
	case bins == 0:
		return "Did you even leave the depot?"
	case bins < 5:
		return "Trainee Binfluencer"
	case bins < 10:
		return "Kerbside Cadet"
	case bins < 18:
		return "Wheelie Good Driver"
	case bins < 28:
		return "Bin Whisperer"
	default:
		return "Legend of the Kerb"
	}
}

func (h *HUD) Draw() {
	g := h.game
	if g == nil || g.sess == nil {
		return
	}
	ci := h.AsCanvasItem()
	sz := h.AsControl().Size()
	W, H := float64(sz.X), float64(sz.Y)
	s := g.sess
	ui := math.Max(0.7, H/1080) // scale for the window size
	fs := func(px float64) int { return int(px * ui) }

	switch g.state {
	case stateTitle:
		h.drawTitle(W, H, ui)
		return
	}
	if g.view == "fork" {
		ci.DrawTextureRect(g.truck.ForkVP.AsViewport().GetTexture().AsTexture2D(), Rect2.New(0, 0, W, H), false)
	}

	// Timer.
	rem := s.Remaining()
	tcol := rgba(1, 1, 1, 1)
	scale := 1.0
	if rem < 20 && g.state == statePlaying {
		tcol = rgba(1, 0.35, 0.3, 1)
		scale = 1 + 0.08*math.Max(0, math.Sin(g.clock*math.Pi*2))
	}
	panel(ci, W/2-110*ui, 14*ui, 220*ui, 84*ui, rgba(0, 0, 0, 0.45))
	text(ci, W/2-110*ui, 76*ui, clockText(rem), int(float64(fs(60))*scale), tcol, centre, 220*ui)
	if _, road, _ := s.Town.NearestRoad(s.Truck.Pos); road >= 0 {
		text(ci, W/2-200*ui, 128*ui, s.Town.Roads[road].Name, fs(24), rgba(1, 1, 1, 0.85), centre, 400*ui)
	}

	// Score and bins.
	panel(ci, 18*ui, 14*ui, 300*ui, 128*ui, rgba(0, 0, 0, 0.45))
	text(ci, 36*ui, 50*ui, "SCORE", fs(22), rgba(1, 1, 1, 0.7), left, 0)
	text(ci, 36*ui, 96*ui, fmt.Sprintf("%d", s.Score), fs(46), rgba(1, 0.92, 0.4, 1), left, 0)
	for i, c := range sim.Colours {
		x := (36 + float64(i)*90) * ui
		panel(ci, x, 108*ui, 26*ui, 26*ui, binColour(c))
		text(ci, x+34*ui, 131*ui, fmt.Sprintf("%d", s.Counts[c]), fs(26), rgba(1, 1, 1, 1), left, 0)
	}

	// Bonus colour and combo.
	bx := W - 318*ui
	panel(ci, bx, 14*ui, 300*ui, 128*ui, rgba(0, 0, 0, 0.45))
	text(ci, bx+18*ui, 50*ui, "BONUS x2", fs(22), rgba(1, 1, 1, 0.7), left, 0)
	pulse := 0.85 + 0.15*math.Sin(g.clock*5)
	panel(ci, bx+18*ui, 62*ui, 150*ui, 36*ui, withAlpha(binColour(s.Bonus), pulse))
	text(ci, bx+18*ui, 90*ui, s.Bonus.String(), fs(26), rgba(1, 1, 1, 1), centre, 150*ui)
	if remain := s.ComboLeft(); s.Combo > 1 && remain > 0 {
		text(ci, bx+180*ui, 90*ui, fmt.Sprintf("x%.1f", s.Multiplier(s.Combo)), fs(34), rgba(0.5, 1, 0.6, 1), left, 0)
		rect(ci, bx+18*ui, 112*ui, 264*ui, 12*ui, rgba(1, 1, 1, 0.15))
		rect(ci, bx+18*ui, 112*ui, 264*ui*remain/sim.ComboWindow, 12*ui, rgba(0.5, 1, 0.6, 0.9))
		text(ci, bx+18*ui, 138*ui, fmt.Sprintf("COMBO %d", s.Combo), fs(18), rgba(0.5, 1, 0.6, 1), left, 0)
	} else {
		text(ci, bx+180*ui, 90*ui, "combo", fs(18), rgba(1, 1, 1, 0.4), left, 0)
	}

	// The key guide, early in the shift.
	if s.Clock < 25 || g.state == stateCountdown {
		lines := []string{"W/S or Up/Down  drive / brake / reverse", "A/D or Left/Right  steer", "SPACE  grab the bin", "F (hold)  lean in to the fork screen", "H  horn     Esc  pause"}
		for i, l := range lines {
			text(ci, W-560*ui, (180+float64(i)*32)*ui, l, fs(22), rgba(1, 1, 1, 0.8), right, 540*ui)
		}
	}

	// Popups rise and fade from the middle of the screen.
	for i, p := range g.popups {
		a := math.Min(1, 3*(1.8-p.age))
		y := H*0.36 - p.age*60*ui - float64(i)*8*ui
		grow := 1 + 0.25*math.Max(0, 0.15-p.age)/0.15
		text(ci, 0, y, p.msg, int(float64(fs(float64(p.size)))*grow), rgba(p.col[0], p.col[1], p.col[2], a), centre, W)
	}

	switch g.state {
	case stateCountdown:
		n := int(math.Ceil(g.countdown))
		msg := fmt.Sprintf("%d", n)
		text(ci, 0, H*0.45, msg, fs(160), rgba(1, 0.92, 0.4, 1), centre, W)
		text(ci, 0, H*0.45+70*ui, "Get ready - bins are out on your LEFT!", fs(34), rgba(1, 1, 1, 1), centre, W)
	case statePaused:
		rect(ci, 0, 0, W, H, rgba(0, 0, 0, 0.55))
		text(ci, 0, H*0.42, "PAUSED", fs(90), rgba(1, 1, 1, 1), centre, W)
		text(ci, 0, H*0.42+70*ui, "Esc  resume      Q  end shift", fs(34), rgba(1, 1, 1, 0.85), centre, W)
	case stateResults:
		h.drawResults(W, H, ui)
	}
}

func (h *HUD) drawTitle(W, H, ui float64) {
	g := h.game
	ci := h.AsCanvasItem()
	fs := func(px float64) int { return int(px * ui) }
	rect(ci, 0, 0, W, H, rgba(0, 0.05, 0.1, 0.25))
	bounce := 6 * math.Sin(g.clock*2.2) * ui
	text(ci, 0, H*0.2+bounce, "BIN DAY", fs(150), rgba(1, 0.85, 0.2, 1), centre, W)
	text(ci, 0, H*0.2+70*ui, "a garbage truck simulator", fs(38), rgba(1, 1, 1, 0.95), centre, W)

	pw, ph := 720*ui, 380*ui
	px, py := (W-pw)/2, H*0.34
	panel(ci, px, py, pw, ph, rgba(0, 0, 0, 0.55))
	y := py + 60*ui
	text(ci, px, y, fmt.Sprintf("<  Shift length: %s  >", clockText(g.timeLimit())), fs(40), rgba(1, 1, 1, 1), centre, pw)
	y += 56 * ui
	text(ci, px, y, fmt.Sprintf("Suburb #%d (N: new)     Graphics: %s (G)", g.seed, g.quality), fs(28), rgba(1, 1, 1, 0.8), centre, pw)
	y += 44 * ui
	if best := g.best[g.bestKey()]; best > 0 {
		text(ci, px, y, fmt.Sprintf("Best for this shift length: %d", best), fs(26), rgba(1, 0.92, 0.4, 0.95), centre, pw)
	}
	y += 60 * ui
	text(ci, px, y, "Line up bins on the kerb using the FORK CAM", fs(26), rgba(1, 1, 1, 0.85), centre, pw)
	y += 36 * ui
	text(ci, px, y, "screen by your steering wheel, then press SPACE.", fs(26), rgba(1, 1, 1, 0.85), centre, pw)
	y += 50 * ui
	for i, c := range sim.Colours {
		x := px + pw/2 - 250*ui + float64(i)*180*ui
		panel(ci, x, y-24*ui, 28*ui, 28*ui, binColour(c))
		text(ci, x+36*ui, y, []string{"Rubbish", "Recycling", "Garden"}[i], fs(24), rgba(1, 1, 1, 0.9), left, 0)
	}
	if int(g.clock*1.6)%2 == 0 {
		text(ci, 0, py+ph+80*ui, "Press ENTER to start your shift", fs(44), rgba(0.5, 1, 0.6, 1), centre, W)
	}
}

func (h *HUD) drawResults(W, H, ui float64) {
	g := h.game
	s := g.sess
	ci := h.AsCanvasItem()
	fs := func(px float64) int { return int(px * ui) }
	rect(ci, 0, 0, W, H, rgba(0, 0, 0, 0.6))
	text(ci, 0, H*0.18, "SHIFT OVER!", fs(110), rgba(1, 0.85, 0.2, 1), centre, W)
	text(ci, 0, H*0.18+60*ui, rank(s.Collected()), fs(40), rgba(1, 1, 1, 1), centre, W)
	y := H*0.18 + 170*ui
	text(ci, 0, y, fmt.Sprintf("%d points", s.Score), fs(80), rgba(1, 1, 1, 1), centre, W)
	if g.newRecord && int(g.clock*3)%2 == 0 {
		text(ci, 0, y+60*ui, "NEW BEST!", fs(40), rgba(0.5, 1, 0.6, 1), centre, W)
	}
	y += 130 * ui
	for i, c := range sim.Colours {
		x := W/2 - 270*ui + float64(i)*190*ui
		panel(ci, x, y-34*ui, 40*ui, 40*ui, binColour(c))
		text(ci, x+50*ui, y, fmt.Sprintf("x %d", s.Counts[c]), fs(38), rgba(1, 1, 1, 1), left, 0)
	}
	y += 70 * ui
	stats := fmt.Sprintf("%d bins   %d perfect   best combo %d   %d misses   %.1f km driven",
		s.Collected(), s.Perfects, s.BestCombo, s.Misses, s.Truck.Odo/1000)
	text(ci, 0, y, stats, fs(30), rgba(1, 1, 1, 0.85), centre, W)
	text(ci, 0, y+100*ui, "ENTER  another shift      Esc  menu", fs(34), rgba(0.5, 1, 0.6, 1), centre, W)
}

func (m *Minimap) Draw() {
	g := m.game
	if g == nil || g.sess == nil || (g.state != statePlaying && g.state != stateCountdown) {
		return
	}
	s := g.sess
	ci := m.AsCanvasItem()
	sz := m.AsControl().Size()
	W, H := float64(sz.X), float64(sz.Y)
	rect(ci, 0, 0, W, H, rgba(0.12, 0.25, 0.12, 0.8))
	scale := W / 190 // pixels per metre
	cx, cy := W/2, H*0.62
	h := s.Truck.Heading
	rot := -math.Pi/2 - h
	sn, cs := math.Sincos(rot)
	toMap := func(p sim.V2) Vector2.XY {
		d := p.Sub(s.Truck.Pos)
		return Vector2.New(cx+(d.X*cs-d.Y*sn)*scale, cy+(d.X*sn+d.Y*cs)*scale)
	}
	near := func(p sim.V2, r float64) bool { return p.Dist(s.Truck.Pos) < r }
	road := rgba(0.55, 0.55, 0.58, 1)
	for _, r := range s.Town.Roads {
		var pts []Vector2.XY
		flush := func() {
			if len(pts) > 1 {
				ci.MoreArgs().DrawPolyline(pts, road, f32(2*sim.RoadHalfWidth*scale), true)
			}
			pts = pts[:0]
		}
		for i, p := range r.Pts {
			if !near(p, 220) {
				flush()
				continue
			}
			if i%2 == 0 || i == len(r.Pts)-1 {
				pts = append(pts, toMap(p))
			}
		}
		flush()
	}
	for _, n := range s.Town.Nodes {
		if near(n.P, 220) {
			r := sim.RoadHalfWidth * 1.3
			if n.Bulb {
				r = sim.BulbRadius
			}
			ci.DrawCircle(toMap(n.P), f32(r*scale), road)
		}
	}
	for i, b := range s.Town.Bins {
		if b.Collected || b.Held || !near(b.Pos, 200) {
			continue
		}
		r := 4.5
		if b.Colour == s.Bonus {
			r = 5.5 + 1.5*math.Sin(g.clock*6+float64(i))
		}
		p := toMap(b.Pos)
		ci.DrawCircle(p, f32(r+1.5), rgba(0, 0, 0, 0.6))
		ci.DrawCircle(p, f32(r), binColour(b.Colour))
	}
	// The truck.
	pts := []Vector2.XY{Vector2.New(cx, cy-11), Vector2.New(cx-7, cy+8), Vector2.New(cx+7, cy+8)}
	ci.DrawColoredPolygon(pts, rgba(1, 1, 1, 1))
	outlineRect(ci, 1, 1, W-2, H-2, rgba(1, 1, 1, 0.5), 2)
}
