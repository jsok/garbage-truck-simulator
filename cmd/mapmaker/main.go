// Command mapmaker builds game maps on real streets from OpenStreetMap.
// Houses, bins and trees are still placed at random.
//
// With no arguments it serves a page for picking an area on a map:
//
//	go run ./cmd/mapmaker
//
// Or give it the area (south,west,north,east in degrees) directly:
//
//	go run ./cmd/mapmaker -bbox=-37.883,145.055,-37.876,145.066 -name="Malvern East"
//
// Either way the map is saved to the -out directory; play it with the game's
// --map option.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/jsok/garbage-truck-simulator/internal/osm"
	"github.com/jsok/garbage-truck-simulator/internal/sim"
)

// Areas outside these sizes (in metres) are refused: too small to hold a
// round, or too big for Overpass and the game to handle comfortably.
const (
	minSide = 150.0
	maxSide = 2000.0
)

func main() {
	addr := flag.String("addr", "localhost:8765", "address to serve the map picker on")
	out := flag.String("out", "maps", "directory to save maps in")
	bbox := flag.String("bbox", "", "area to build, as south,west,north,east; skips the map picker")
	name := flag.String("name", "", "map name, with -bbox")
	osmFile := flag.String("osm", "", "Overpass JSON to use instead of downloading, with -bbox")
	mainRoads := flag.Bool("main-roads", false, "include main roads, with -bbox")
	flag.Parse()

	if *bbox == "" {
		serve(*addr, *out)
		return
	}
	b, err := parseBBox(*bbox)
	if err != nil {
		log.Fatal(err)
	}
	var d *osm.Data
	if *osmFile != "" {
		d = new(osm.Data)
		raw, err := os.ReadFile(*osmFile)
		if err == nil {
			err = json.Unmarshal(raw, d)
		}
		if err != nil {
			log.Fatal(err)
		}
	} else if d, err = osm.Fetch(context.Background(), b); err != nil {
		log.Fatal(err)
	}
	l, st, t, err := build(d, b, osm.Options{MainRoads: *mainRoads}, 1)
	if err != nil {
		log.Fatal(err)
	}
	if hint := cutHint(st, *mainRoads); hint != "" {
		log.Print(hint)
	}
	path, err := save(*out, *name, l)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%d streets, %d houses, %d bins\nsaved %s\nplay it with: %s\n", len(l.Roads), len(t.Houses), len(t.Bins), path, playCommand(path))
}

func parseBBox(s string) (osm.BBox, error) {
	var v [4]float64
	parts := strings.Split(s, ",")
	if len(parts) != 4 {
		return osm.BBox{}, errors.New("bbox must be south,west,north,east")
	}
	for i, p := range parts {
		f, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
		if err != nil {
			return osm.BBox{}, fmt.Errorf("bbox: %w", err)
		}
		v[i] = f
	}
	return osm.BBox{South: v[0], West: v[1], North: v[2], East: v[3]}, nil
}

// size is the width and height of b in metres.
func size(b osm.BBox) (float64, float64) {
	p := osm.Centre(b)
	d := p.ToGame(b.South, b.East).Sub(p.ToGame(b.North, b.West))
	return d.X, d.Y
}

func checkSize(b osm.BBox) error {
	if b.North <= b.South || b.East <= b.West {
		return errors.New("the area is empty")
	}
	w, h := size(b)
	if w < minSide || h < minSide {
		return fmt.Errorf("the area is %.0f × %.0f m; make it at least %.0f m across", w, h, minSide)
	}
	if w > maxSide || h > maxSide {
		return fmt.Errorf("the area is %.0f × %.0f m; keep it under %.0f m across", w, h, maxSide)
	}
	return nil
}

// build turns street data into a layout, and checks that it makes a
// playable suburb.
func build(d *osm.Data, b osm.BBox, opt osm.Options, seed int64) (*sim.Layout, osm.Stats, *sim.Town, error) {
	if err := checkSize(b); err != nil {
		return nil, osm.Stats{}, nil, err
	}
	l, st, err := osm.Build(d, b, opt)
	if err != nil {
		return nil, st, nil, err
	}
	t, err := sim.FromLayout(l, seed)
	return l, st, t, err
}

// cutHint explains when a lot of street was dropped for not joining up.
func cutHint(st osm.Stats, mainRoads bool) string {
	if st.Cut < 0.25*(st.Kept+st.Cut) {
		return ""
	}
	msg := fmt.Sprintf("%.1f km of street was left out because it doesn't join up with the rest.", st.Cut/1000)
	if !mainRoads {
		msg += " Including main roads may connect it."
	}
	return msg
}

// save writes l to dir under a file name made from name, returning its
// absolute path. Coordinates are rounded to the centimetre.
func save(dir, name string, l *sim.Layout) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "My suburb"
	}
	l.Name = name
	r := func(v sim.V2) sim.V2 { return sim.V2{X: math.Round(v.X*100) / 100, Y: math.Round(v.Y*100) / 100} }
	l.Min, l.Max = r(l.Min), r(l.Max)
	for i := range l.Nodes {
		l.Nodes[i] = r(l.Nodes[i])
	}
	for i := range l.Roads {
		for j := range l.Roads[i].Pts {
			l.Roads[i].Pts[j] = r(l.Roads[i].Pts[j])
		}
	}
	b, err := json.Marshal(l)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path, err := filepath.Abs(filepath.Join(dir, slug(name)+".json"))
	if err != nil {
		return "", err
	}
	return path, os.WriteFile(path, b, 0o644)
}

var nonWord = regexp.MustCompile(`[^a-z0-9]+`)

func slug(name string) string {
	s := strings.Trim(nonWord.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if s == "" {
		s = "map"
	}
	return s
}

func playCommand(path string) string {
	return fmt.Sprintf("~/gd/bin/Godot.app/Contents/MacOS/Godot --path graphics -- --map=%q", path)
}
