// Command townmap renders a generated suburb to a PNG, for checking the
// generator without launching the game.
package main

import (
	"flag"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"

	"github.com/jsok/garbage-truck-simulator/internal/sim"
)

func main() {
	seed := flag.Int64("seed", 1, "town seed")
	out := flag.String("o", "town.png", "output file")
	scale := flag.Float64("scale", 2, "pixels per metre")
	flag.Parse()

	t := sim.Generate(*seed)
	pad := 30.0
	w := int((t.Max.X - t.Min.X + 2*pad) * *scale)
	h := int((t.Max.Y - t.Min.Y + 2*pad) * *scale)
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	px := func(p sim.V2) (float64, float64) {
		return (p.X - t.Min.X + pad) * *scale, (p.Y - t.Min.Y + pad) * *scale
	}
	disc := func(p sim.V2, r float64, c color.Color) {
		cx, cy := px(p)
		rr := r * *scale
		for y := int(cy - rr); y <= int(cy+rr); y++ {
			for x := int(cx - rr); x <= int(cx+rr); x++ {
				if math.Hypot(float64(x)-cx, float64(y)-cy) <= rr {
					img.Set(x, y, c)
				}
			}
		}
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{96, 150, 70, 255})
		}
	}
	for _, r := range t.Roads {
		for _, p := range r.Pts {
			disc(p, sim.RoadHalfWidth, color.RGBA{60, 60, 64, 255})
		}
	}
	for _, n := range t.Nodes {
		if n.Bulb {
			disc(n.P, sim.BulbRadius, color.RGBA{60, 60, 64, 255})
		}
	}
	for _, hs := range t.Houses {
		f := sim.Dir(hs.Heading)
		for a := -hs.D / 2; a <= hs.D/2; a += 0.4 {
			for b := -hs.W / 2; b <= hs.W/2; b += 0.4 {
				disc(hs.P.Add(f.Scale(a)).Add(f.Left().Scale(b)), 0.3, color.RGBA{200, 120, 90, 255})
			}
		}
		for s := 0.0; s <= 1; s += 0.05 {
			disc(hs.Drive[0].Lerp(hs.Drive[1], s), 1.2, color.RGBA{180, 180, 170, 255})
		}
	}
	for _, tr := range t.Trees {
		disc(tr.P, tr.Radius*0.6, color.RGBA{40, 100, 40, 255})
	}
	cols := map[sim.Colour]color.RGBA{sim.Red: {230, 40, 40, 255}, sim.Yellow: {250, 220, 30, 255}, sim.Green: {60, 230, 60, 255}}
	for _, b := range t.Bins {
		disc(b.Home, 1.1, cols[b.Colour])
	}
	disc(t.StartPos, 3, color.RGBA{255, 255, 255, 255})
	f, err := os.Create(*out)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		panic(err)
	}
}
