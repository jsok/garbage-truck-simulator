package osm

import (
	"encoding/json"
	"math"
	"os"
	"testing"

	"github.com/jsok/garbage-truck-simulator/internal/sim"
)

// malvernEast is the area of testdata/malvern-east.json, a slice of
// Melbourne's south-east. © OpenStreetMap contributors, ODbL.
var malvernEast = BBox{South: -37.883, West: 145.055, North: -37.876, East: 145.066}

func loadFixture(t *testing.T) *Data {
	t.Helper()
	b, err := os.ReadFile("testdata/malvern-east.json")
	if err != nil {
		t.Fatal(err)
	}
	var d Data
	if err := json.Unmarshal(b, &d); err != nil {
		t.Fatal(err)
	}
	return &d
}

func TestProjectionRoundTrips(t *testing.T) {
	p := Centre(malvernEast)
	v := p.ToGame(-37.88, 145.06)
	lat, lon := p.ToLatLon(v)
	if math.Abs(lat+37.88) > 1e-9 || math.Abs(lon-145.06) > 1e-9 {
		t.Fatalf("round trip gave %v, %v", lat, lon)
	}
	// North is up the screen (-Y) and east is to the right (+X).
	if n := p.ToGame(malvernEast.North, p.Lon0); n.Y >= 0 {
		t.Errorf("north projected to %v", n)
	}
	if e := p.ToGame(p.Lat0, malvernEast.East); e.X <= 0 {
		t.Errorf("east projected to %v", e)
	}
	// About 11 km per 0.1° of latitude.
	if d := p.ToGame(-37.9, 145.06).Dist(p.ToGame(-37.8, 145.06)); math.Abs(d-11119) > 5 {
		t.Errorf("0.1° of latitude is %.0fm", d)
	}
}

// grid builds Data from ways given as node coordinates in metres from the
// centre of a 400m square.
func grid(ways ...[]sim.V2) (*Data, BBox) {
	b := BBox{South: -0.0018, West: -0.0023, North: 0.0018, East: 0.0023}
	p := Centre(b)
	var d Data
	ids := map[sim.V2]int64{}
	for wi, w := range ways {
		way := Element{Type: "way", ID: int64(1000 + wi), Tags: map[string]string{"highway": "residential", "name": "Way"}}
		for _, v := range w {
			id, ok := ids[v]
			if !ok {
				id = int64(len(ids) + 1)
				ids[v] = id
				lat, lon := p.ToLatLon(v)
				d.Elements = append(d.Elements, Element{Type: "node", ID: id, Lat: lat, Lon: lon})
			}
			way.Nodes = append(way.Nodes, id)
		}
		d.Elements = append(d.Elements, way)
	}
	return &d, b
}

func TestBuildJoinsWaysAtJunctionsOnly(t *testing.T) {
	// A street split into two ways end to end, crossed by another.
	d, b := grid(
		[]sim.V2{{X: -150}, {X: 0}},
		[]sim.V2{{X: 0}, {X: 60}, {X: 150}},
		[]sim.V2{{X: 60, Y: -150}, {X: 60}, {X: 60, Y: 150}},
	)
	d.Elements[len(d.Elements)-1].Tags["name"] = "Cross St"
	l, _, err := Build(d, b, Options{})
	if err != nil {
		t.Fatal(err)
	}
	// Both streets are cut at the crossing, giving four roads.
	if len(l.Roads) != 4 || len(l.Nodes) != 5 {
		t.Fatalf("got %d roads and %d nodes, want 4 and 5", len(l.Roads), len(l.Nodes))
	}
}

func TestBuildClipsToArea(t *testing.T) {
	// The area spans about ±256m east-west and ±200m north-south.
	d, b := grid([]sim.V2{{X: -400}, {}, {X: 400}}, []sim.V2{{Y: -400}, {}, {Y: 400}})
	l, _, err := Build(d, b, Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range l.Roads {
		for _, p := range r.Pts {
			if p.X < l.Min.X+edgeMargin-1e-6 || p.X > l.Max.X-edgeMargin+1e-6 ||
				p.Y < l.Min.Y+edgeMargin-1e-6 || p.Y > l.Max.Y-edgeMargin+1e-6 {
				t.Fatalf("point %v is outside %v..%v", p, l.Min, l.Max)
			}
		}
	}
	if len(l.Roads) != 4 {
		t.Errorf("got %d roads, want 4 arms of a crossroads", len(l.Roads))
	}
}

func TestBuildMergesCloseJunctionsAndDropsStubs(t *testing.T) {
	d, b := grid(
		[]sim.V2{{X: -150}, {X: 0}, {X: 8}, {X: 100}, {X: 150}},
		[]sim.V2{{X: 0, Y: -150}, {X: 0}},   // meets at x=0...
		[]sim.V2{{X: 8}, {X: 8, Y: 150}},    // ...and staggered 8m along
		[]sim.V2{{X: 100}, {X: 100, Y: 12}}, // a 12m stub
	)
	l, _, err := Build(d, b, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Roads) != 4 {
		t.Fatalf("got %d roads, want the stagger merged into one crossroads", len(l.Roads))
	}
}

func TestBuildCollapsesRoundabouts(t *testing.T) {
	var ring []sim.V2
	for i := 0; i <= 8; i++ {
		a := float64(i%8) * math.Pi / 4
		ring = append(ring, sim.V2{X: 12 * math.Cos(a), Y: 12 * math.Sin(a)})
	}
	d, b := grid(ring,
		[]sim.V2{{X: -150}, ring[4]}, []sim.V2{ring[0], {X: 150}},
		[]sim.V2{{Y: -150}, ring[6]}, []sim.V2{ring[2], {Y: 150}},
	)
	for _, e := range d.Elements {
		if e.ID == 1000 {
			e.Tags["junction"] = "roundabout"
		}
	}
	l, _, err := Build(d, b, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Roads) != 4 || len(l.Nodes) != 5 {
		t.Fatalf("got %d roads and %d nodes, want a four-way junction", len(l.Roads), len(l.Nodes))
	}
}

func TestBuildKeepsOneCarriagewayOfADividedRoad(t *testing.T) {
	// A road that splits into two carriageways 8m apart for a while, each
	// with a side street off it.
	d, b := grid(
		[]sim.V2{{X: -160}, {X: 60}, {X: 160}},
		[]sim.V2{{X: -160}, {X: -120, Y: 8}, {Y: 8}, {X: 120, Y: 8}, {X: 160}},
		[]sim.V2{{X: -240}, {X: -160}},
		[]sim.V2{{X: 160}, {X: 240}},
		[]sim.V2{{Y: 8}, {Y: 150}},
		[]sim.V2{{X: 60}, {X: 60, Y: -150}},
	)
	for _, e := range d.Elements {
		if e.ID >= 1000 && e.ID <= 1003 {
			e.Tags["name"] = "Main Rd"
		}
	}
	l, st, err := Build(d, b, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if st.Cut > 0 {
		t.Errorf("%.0fm of street was cut off", st.Cut)
	}
	main := 0.0
	for _, r := range l.Roads {
		if r.Name == "Main Rd" {
			main += polyLen(r.Pts)
		}
	}
	if main > 500 {
		t.Errorf("%.0fm of Main Rd: both carriageways kept", main)
	}
}

func TestBuildDropsTrafficIslands(t *testing.T) {
	d, b := grid(
		[]sim.V2{{X: -150}, {X: -15}, {X: 15}, {X: 150}},
		[]sim.V2{{X: -15}, {Y: 5}, {X: 15}},
	)
	l, _, err := Build(d, b, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Roads) != 1 {
		t.Errorf("got %d roads, want the street without its island", len(l.Roads))
	}
}

func TestRealSuburbIsPlayable(t *testing.T) {
	l, _, err := Build(loadFixture(t), malvernEast, Options{})
	if err != nil {
		t.Fatal(err)
	}
	named := 0
	for _, r := range l.Roads {
		if r.Name != "" {
			named++
		}
	}
	if named < len(l.Roads)*9/10 {
		t.Errorf("only %d/%d roads have names", named, len(l.Roads))
	}
	// Nirvana Avenue ends in a one-way loop, which goes, and joins the rest
	// through a short link beside it, which must stay.
	nirvana := 0.0
	for _, r := range l.Roads {
		if r.Name == "Nirvana Avenue" {
			nirvana += polyLen(r.Pts)
		}
	}
	if nirvana < 500 {
		t.Errorf("only %.0fm of Nirvana Avenue kept", nirvana)
	}
	for seed := int64(1); seed <= 5; seed++ {
		town, err := sim.FromLayout(l, seed)
		if err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
		for i, h := range town.Houses {
			if d := town.RoadDist(h.P); d < sim.RoadHalfWidth+8 {
				t.Errorf("seed %d: house %d is %.2fm from the road", seed, i, d)
			}
		}
		if d := town.RoadDist(town.StartPos); math.Abs(d-sim.LaneOffset) > 0.3 {
			t.Errorf("seed %d: start is %.2fm from centreline", seed, d)
		}
		culs := 0
		for _, n := range town.Nodes {
			if n.Bulb {
				culs++
			}
		}
		t.Logf("seed %d: %d nodes (%d dead ends), %d roads, %d houses, %d bins, %d trees",
			seed, len(town.Nodes), culs, len(town.Roads), len(town.Houses), len(town.Bins), len(town.Trees))
		if len(town.Bins) < 60 {
			t.Errorf("seed %d: only %d bins", seed, len(town.Bins))
		}
	}
}
