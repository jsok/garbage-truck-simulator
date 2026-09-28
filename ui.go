package main

import (
	"graphics.gd/classdb/CanvasItem"
	"graphics.gd/classdb/Font"
	"graphics.gd/classdb/GUI"
	"graphics.gd/classdb/SystemFont"
	"graphics.gd/classdb/TextServer"
	"graphics.gd/variant/Angle"
	"graphics.gd/variant/Color"
	"graphics.gd/variant/Object"
	"graphics.gd/variant/Rect2"
	"graphics.gd/variant/Vector2"
	"math"

	"github.com/jsok/garbage-truck-simulator/internal/meshgen"
	"github.com/jsok/garbage-truck-simulator/internal/sim"
)

var uiFont Font.Instance

func font() Font.Instance {
	if uiFont == Font.Nil {
		sf := SystemFont.New()
		sf.SetFontNames([]string{"Arial Rounded MT Bold", "Avenir Next", "Helvetica Neue", "Arial"})
		sf.SetFontWeight(700)
		uiFont = Object.Leak(sf.AsFont())
	}
	return uiFont
}

func rgba(r, g, b, a float64) Color.RGBA {
	return Color.RGBA{R: f32(r), G: f32(g), B: f32(b), A: f32(a)}
}

func withAlpha(c Color.RGBA, a float64) Color.RGBA { c.A = f32(a); return c }

func binColour(c sim.Colour) Color.RGBA { return Color.RGBA(meshgen.BinColours[c]) }

type align = GUI.HorizontalAlignment

const (
	left   = GUI.HorizontalAlignmentLeft
	centre = GUI.HorizontalAlignmentCenter
	right  = GUI.HorizontalAlignmentRight
)

// text draws an outlined string with its baseline at y. For centred or
// right-aligned text, x is the left edge of a box of the given width.
func text(ci CanvasItem.Instance, x, y float64, s string, size int, col Color.RGBA, al align, width float64) {
	ex := CanvasItem.Expanded(ci)
	pos := Vector2.New(x, y)
	outline := max(size/7, 3)
	shadow := rgba(0, 0, 0, 0.55*float64(col.A))
	ex.DrawStringOutline(font(), pos, s, al, f32(width), size, outline, shadow, TextServer.JustificationKashida|TextServer.JustificationWordBound, 0, 0, 0)
	ex.DrawString(font(), pos, s, al, f32(width), size, col, TextServer.JustificationKashida|TextServer.JustificationWordBound, 0, 0, 0)
}

func textWidth(s string, size int) float64 {
	return float64(Font.Expanded(font()).GetStringSize(s, left, -1, size, TextServer.JustificationKashida|TextServer.JustificationWordBound, 0, 0).X)
}

func rect(ci CanvasItem.Instance, x, y, w, h float64, col Color.RGBA) {
	ci.DrawRect(Rect2.New(x, y, w, h), col)
}

// panel draws a rounded translucent box.
func panel(ci CanvasItem.Instance, x, y, w, h float64, col Color.RGBA) {
	r := min(12.0, h/2, w/2)
	var pts []Vector2.XY
	corners := [][3]float64{{x + w - r, y + r, -math.Pi / 2}, {x + w - r, y + h - r, 0}, {x + r, y + h - r, math.Pi / 2}, {x + r, y + r, math.Pi}}
	for _, c := range corners {
		for i := 0; i <= 6; i++ {
			a := c[2] + float64(i)/6*math.Pi/2
			pts = append(pts, Vector2.New(c[0]+r*math.Cos(a), c[1]+r*math.Sin(a)))
		}
	}
	ci.DrawColoredPolygon(pts, col)
}

func outlineRect(ci CanvasItem.Instance, x, y, w, h float64, col Color.RGBA, width float64) {
	CanvasItem.Expanded(ci).DrawRect(Rect2.New(x, y, w, h), col, false, f32(width), true)
}

func arc(ci CanvasItem.Instance, x, y, r, from, to float64, col Color.RGBA, width float64) {
	CanvasItem.Expanded(ci).DrawArc(Vector2.New(x, y), f32(r), Angle.Radians(from), Angle.Radians(to), 32, col, f32(width), true)
}

func line(ci CanvasItem.Instance, x0, y0, x1, y1 float64, col Color.RGBA, width float64) {
	CanvasItem.Expanded(ci).DrawLine(Vector2.New(x0, y0), Vector2.New(x1, y1), col, f32(width), true)
}

// triangle draws a filled arrowhead pointing in direction dir (+1 right, -1 left).
func arrow(ci CanvasItem.Instance, x, y, size, dir float64, col Color.RGBA) {
	pts := []Vector2.XY{Vector2.New(x+dir*size, y), Vector2.New(x-dir*size*0.6, y-size), Vector2.New(x-dir*size*0.6, y+size)}
	ci.DrawColoredPolygon(pts, col)
}
