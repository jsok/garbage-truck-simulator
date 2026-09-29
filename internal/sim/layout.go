package sim

import (
	"errors"
	"fmt"
	"math/rand/v2"
)

// Layout is a street network to build a suburb around, such as one imported
// from a real map. Houses, bins, poles and trees are still placed at random.
type Layout struct {
	Name        string       `json:"name,omitempty"`
	Attribution string       `json:"attribution,omitempty"` // credit for the map data, shown in game
	Min         V2           `json:"min"`                   // drivable bounds
	Max         V2           `json:"max"`
	Nodes       []V2         `json:"nodes"` // junctions and dead ends
	Roads       []LayoutRoad `json:"roads"`
}

// LayoutRoad is one street between two nodes. Pts runs from node A to node B,
// inclusive, and may be as coarse as the source data.
type LayoutRoad struct {
	Name string `json:"name,omitempty"`
	A    int    `json:"a"`
	B    int    `json:"b"`
	Pts  []V2   `json:"pts"`
}

// MinLayoutBins is the fewest bins a layout must hold to be worth a shift.
const MinLayoutBins = 15

// FromLayout builds a suburb on a fixed street layout. The seed only affects
// what is placed along the streets.
func FromLayout(l *Layout, seed int64) (*Town, error) {
	if err := l.validate(); err != nil {
		return nil, err
	}
	var err error
	for attempt := uint64(0); attempt < 20; attempt++ {
		t := l.town(seed)
		rng := rand.New(rand.NewPCG(uint64(seed), attempt*0x9E3779B97F4A7C15+7))
		t.nameRoads(rng)
		if err = t.furnish(rng, MinLayoutBins); err == nil {
			return t, nil
		}
	}
	return nil, err
}

func (l *Layout) validate() error {
	if len(l.Roads) == 0 {
		return errors.New("sim: layout has no roads")
	}
	if l.Max.X <= l.Min.X || l.Max.Y <= l.Min.Y {
		return errors.New("sim: layout has empty bounds")
	}
	for i, r := range l.Roads {
		if r.A < 0 || r.A >= len(l.Nodes) || r.B < 0 || r.B >= len(l.Nodes) {
			return fmt.Errorf("sim: road %d joins missing nodes", i)
		}
		if len(r.Pts) < 2 {
			return fmt.Errorf("sim: road %d has fewer than two points", i)
		}
	}
	return nil
}

// town lays out the streets, with a turning circle at every dead end.
func (l *Layout) town(seed int64) *Town {
	t := &Town{Seed: seed, Min: l.Min, Max: l.Max}
	for _, p := range l.Nodes {
		t.Nodes = append(t.Nodes, Node{P: p})
	}
	for _, lr := range l.Roads {
		t.addRoad(lr.A, lr.B)
		r := &t.Roads[len(t.Roads)-1]
		r.Name = lr.Name
		r.Pts, r.Cum = resample(lr.Pts, sampleSpacing)
	}
	for ni := range t.Nodes {
		n := &t.Nodes[ni]
		if len(n.Roads) != 1 {
			continue
		}
		n.Bulb = true
		if r := &t.Roads[n.Roads[0]]; r.A == ni {
			r.A, r.B = r.B, r.A
			reverse(r)
		}
	}
	return t
}

// reverse flips a road's direction, keeping Cum as arc length from A.
func reverse(r *Road) {
	for i, j := 0, len(r.Pts)-1; i < j; i, j = i+1, j-1 {
		r.Pts[i], r.Pts[j] = r.Pts[j], r.Pts[i]
	}
	total := r.Len()
	for i, j := 0, len(r.Cum)-1; i <= j; i, j = i+1, j-1 {
		r.Cum[i], r.Cum[j] = total-r.Cum[j], total-r.Cum[i]
	}
}
