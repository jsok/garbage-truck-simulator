// Command sheet tiles captured frames into one contact sheet PNG.
// Usage: sheet -o out.png -cols 2 -w 576 frame1.png frame2.png ...
package main

import (
	"flag"
	"image"
	"image/png"
	"os"
)

func main() {
	out := flag.String("o", "sheet.png", "output file")
	cols := flag.Int("cols", 2, "columns")
	w := flag.Int("w", 576, "tile width")
	flag.Parse()
	var tiles []image.Image
	for _, p := range flag.Args() {
		f, err := os.Open(p)
		if err != nil {
			panic(err)
		}
		img, err := png.Decode(f)
		f.Close()
		if err != nil {
			panic(err)
		}
		tiles = append(tiles, img)
	}
	if len(tiles) == 0 {
		return
	}
	b := tiles[0].Bounds()
	h := *w * b.Dy() / b.Dx()
	rows := (len(tiles) + *cols - 1) / *cols
	sheet := image.NewRGBA(image.Rect(0, 0, *w**cols, h*rows))
	for i, t := range tiles {
		x0, y0 := (i%*cols)**w, (i / *cols)*h
		tb := t.Bounds()
		for y := 0; y < h; y++ {
			for x := 0; x < *w; x++ {
				sheet.Set(x0+x, y0+y, t.At(tb.Min.X + x*tb.Dx() / *w, tb.Min.Y+y*tb.Dy()/h))
			}
		}
	}
	f, err := os.Create(*out)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	png.Encode(f, sheet)
}
