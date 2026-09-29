package sim

import "testing"

func TestFromLayoutRejectsBrokenLayouts(t *testing.T) {
	bounds := func(l Layout) Layout { l.Min, l.Max = V2{-100, -100}, V2{100, 100}; return l }
	for name, l := range map[string]Layout{
		"no roads":     bounds(Layout{Nodes: []V2{{}, {X: 50}}}),
		"no bounds":    {Nodes: []V2{{}, {X: 50}}, Roads: []LayoutRoad{{A: 0, B: 1, Pts: []V2{{}, {X: 50}}}}},
		"missing node": bounds(Layout{Nodes: []V2{{}}, Roads: []LayoutRoad{{A: 0, B: 1, Pts: []V2{{}, {X: 50}}}}}),
		"one point":    bounds(Layout{Nodes: []V2{{}, {X: 50}}, Roads: []LayoutRoad{{A: 0, B: 1, Pts: []V2{{}}}}}),
		// Room for a handful of houses, but not a shift's worth of bins.
		"too small": bounds(Layout{Nodes: []V2{{X: -60}, {X: 60}}, Roads: []LayoutRoad{{A: 0, B: 1, Pts: []V2{{X: -60}, {X: 60}}}}}),
	} {
		if _, err := FromLayout(&l, 1); err == nil {
			t.Errorf("%s: built a town", name)
		}
	}
}

func TestFromLayoutBuildsOnTheGivenStreets(t *testing.T) {
	// A cross of four 300m streets, one of them named.
	l := Layout{Min: V2{-340, -340}, Max: V2{340, 340}, Nodes: []V2{{}, {X: 300}, {X: -300}, {Y: 300}, {Y: -300}}}
	for i := 1; i <= 4; i++ {
		l.Roads = append(l.Roads, LayoutRoad{A: 0, B: i, Pts: []V2{{}, l.Nodes[i]}})
	}
	l.Roads[0].Name = "High St"
	town, err := FromLayout(&l, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(town.Roads) != 4 || town.Roads[0].Name != "High St" || town.Roads[1].Name == "" {
		t.Errorf("roads not kept as given: %+v", town.Roads)
	}
	for i, r := range town.Roads {
		// Dead ends get turning circles, always at the road's B end.
		if !town.Nodes[r.B].Bulb || town.Nodes[r.A].Bulb {
			t.Errorf("road %d: bulb not at B", i)
		}
		if d := r.Pts[0].Dist(V2{}); d > 1e-9 {
			t.Errorf("road %d starts %.2fm from the junction", i, d)
		}
	}
	if d := town.RoadDist(town.StartPos); d < LaneOffset-0.3 || d > LaneOffset+0.3 {
		t.Errorf("start is %.2fm from centreline", d)
	}
}
