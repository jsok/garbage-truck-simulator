package meshgen

import (
	"testing"
	"time"

	"github.com/jsok/garbage-truck-simulator/internal/sim"
)

func TestGrassAvoidsRoads(t *testing.T) {
	town := sim.Generate(5)
	start := time.Now()
	tufts := GrassTufts(town)
	n := 0
	for _, ts := range tufts {
		for _, tf := range ts {
			if d := town.RoadDist(sim.V2{X: float64(tf.X), Y: float64(tf.Z)}); d < sim.RoadHalfWidth+0.4 {
				t.Fatalf("tuft on the road (%.2fm)", d)
			}
			n++
		}
	}
	t.Logf("%d tufts in %d chunks, %v", n, len(tufts), time.Since(start))
}
