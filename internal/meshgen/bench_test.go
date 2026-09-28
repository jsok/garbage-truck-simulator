package meshgen

import (
	"testing"

	"github.com/jsok/garbage-truck-simulator/internal/sim"
)

func BenchmarkBuildTown(b *testing.B) {
	town := sim.Generate(42)
	for b.Loop() {
		BuildTown(town)
		GrassTufts(town)
	}
}
