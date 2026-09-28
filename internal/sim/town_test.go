package sim

import (
	"math"
	"testing"
)

func TestGenerateIsDeterministic(t *testing.T) {
	a, b := Generate(7), Generate(7)
	if len(a.Roads) != len(b.Roads) || len(a.Bins) != len(b.Bins) || a.StartPos != b.StartPos {
		t.Fatalf("same seed produced different towns")
	}
}

func TestTownsArePlayable(t *testing.T) {
	for seed := int64(1); seed <= 25; seed++ {
		town := Generate(seed)
		deadEnds, curved := 0, 0
		for _, n := range town.Nodes {
			if n.Bulb {
				deadEnds++
			}
		}
		for _, r := range town.Roads {
			chord := r.Pts[0].Dist(r.Pts[len(r.Pts)-1])
			if r.Len() > chord*1.02 {
				curved++
			}
		}
		if curved < len(town.Roads)/3 {
			t.Errorf("seed %d: only %d/%d roads are curved", seed, curved, len(town.Roads))
		}
		if len(town.Bins) < 40 || len(town.Houses) < 60 {
			t.Errorf("seed %d: sparse town: %d houses, %d bins", seed, len(town.Houses), len(town.Bins))
		}
		// Every bin must be reachable from the kerb and not sit on a road.
		for i, bin := range town.Bins {
			if d := town.RoadDist(bin.Home); d < RoadHalfWidth || d > BinKerbOffset+0.5 {
				t.Errorf("seed %d: bin %d is %.2fm from the road", seed, i, d)
			}
		}
		// Houses must not sit on roads.
		for i, h := range town.Houses {
			if d := town.RoadDist(h.P); d < RoadHalfWidth+8 {
				t.Errorf("seed %d: house %d is %.2fm from the road", seed, i, d)
			}
		}
		if d := town.RoadDist(town.StartPos); math.Abs(d-LaneOffset) > 0.3 {
			t.Errorf("seed %d: start is %.2fm from centreline", seed, d)
		}
		t.Logf("seed %2d: %2d nodes (%d culs-de-sac), %2d roads (%d curved), %3d houses, %3d bins, %4d trees",
			seed, len(town.Nodes), deadEnds, len(town.Roads), curved, len(town.Houses), len(town.Bins), len(town.Trees))
	}
}
