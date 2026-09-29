package meshgen

import (
	"testing"

	"github.com/jsok/garbage-truck-simulator/internal/sim"
)

// checkWinding asserts every triangle is stored clockwise as seen from the
// side its normal points to (Godot's front-face convention).
func checkWinding(t *testing.T, name string, m *Mesh) {
	t.Helper()
	for i := 0; i+2 < len(m.P); i += 3 {
		a, b, c := m.P[i], m.P[i+1], m.P[i+2]
		ccw := b.Sub(a).Cross(c.Sub(a))
		if ccw.Dot(ccw) < 1e-12 {
			continue // degenerate sliver
		}
		if ccw.Dot(m.N[i]) > 0 {
			t.Fatalf("%s: triangle %d is counter-clockwise from the front", name, i/3)
		}
	}
}

func TestBoxFacesOutward(t *testing.T) {
	m := &Mesh{}
	m.Box(Identity, -1, -1, -1, 1, 1, 1, Hex(0xffffff))
	if len(m.P) != 36 {
		t.Fatalf("box has %d vertices", len(m.P))
	}
	for i := 0; i < len(m.P); i += 3 {
		centre := m.P[i].Add(m.P[i+1]).Add(m.P[i+2]).Scale(1.0 / 3)
		if centre.Dot(m.N[i]) <= 0 {
			t.Fatalf("face %d normal %v points inwards", i/3, m.N[i])
		}
	}
	checkWinding(t, "box", m)
}

func TestGeneratedMeshesAreWellFormed(t *testing.T) {
	tm := BuildTown(sim.Generate(5))
	tris := 0
	for _, chunks := range []Chunks{tm.Ground, tm.Scenery} {
		for k, m := range chunks {
			if len(m.P) != len(m.N) || len(m.P) != len(m.C) || len(m.P)%3 != 0 {
				t.Fatalf("chunk %v has mismatched arrays", k)
			}
			checkWinding(t, "town", m)
			tris += len(m.P) / 3
		}
	}
	for name, m := range map[string]*Mesh{
		"truck": TruckBody(), "cab": Cab(), "wheel": SteeringWheel(), "bezel": ScreenBezel(),
		"bin": BinBody(sim.Red), "lid": BinLid(sim.Red), "marker": Marker(Hex(0xff0000)),
		"overflow": BinOverflow(), "spill red": Spill(sim.Red), "spill yellow": Spill(sim.Yellow), "spill green": Spill(sim.Green),
	} {
		checkWinding(t, name, m)
	}
	t.Logf("town: %d triangles", tris)
}
