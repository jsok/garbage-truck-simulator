package sim

import (
	"math"
	"testing"
)

func run(s *Session, secs float64, in Input) {
	const dt = 1.0 / 60
	for i := 0; i < int(secs/dt); i++ {
		s.Step(dt, in)
		in.Pickup = false
	}
}

func newTestSession() *Session { return NewSession(Config{TimeLimit: 120, Seed: 3}) }

func TestAlignedPickupCollectsBin(t *testing.T) {
	s := newTestSession()
	s.PlaceAtBin(0, 0)
	tg := s.Target()
	if tg.Bin != 0 || !tg.Aligned || !tg.Perfect {
		t.Fatalf("expected bin 0 perfectly aligned, got %+v", tg)
	}
	run(s, 0.1, Input{Pickup: true})
	if !s.Truck.Locked || !s.Arm.Busy() {
		t.Fatalf("pickup did not start")
	}
	home := s.Town.Bins[0].Pos
	run(s, 4, Input{})
	b := s.Town.Bins[0]
	if !b.Collected || b.Held {
		t.Fatalf("bin not collected: %+v", b)
	}
	if s.Arm.Busy() || s.Truck.Locked {
		t.Fatalf("arm should be idle and truck free")
	}
	if d := b.Pos.Dist(home); d > 0.05 {
		t.Errorf("bin put back %.2fm from where it was", d)
	}
	want := 150
	if b.Colour == s.Bonus {
		want = 300
	}
	if s.Score != want || s.Collected() != 1 || s.Counts[b.Colour] != 1 {
		t.Errorf("score %d (want %d), collected %d", s.Score, want, s.Collected())
	}
}

func TestMisalignedPickupMisses(t *testing.T) {
	s := newTestSession()
	s.PlaceAtBin(0, 2.5)
	tg := s.Target()
	if tg.Aligned || tg.Along < 2 {
		t.Fatalf("expected bin 2.5m ahead, got %+v", tg)
	}
	run(s, 0.1, Input{Pickup: true})
	run(s, 3, Input{})
	if s.Town.Bins[0].Collected || s.Misses != 1 || s.Score != 0 {
		t.Fatalf("misaligned pickup should miss")
	}
}

func TestDrivingForwardAlignsBin(t *testing.T) {
	s := newTestSession()
	s.PlaceAtBin(0, 2.0)
	// Creep forward until the screen says we are lined up, then grab.
	for i := 0; i < 600 && !s.Target().Aligned; i++ {
		s.Step(1.0/60, Input{Controls: Controls{Throttle: 0.3}})
	}
	if !s.Target().Aligned {
		t.Fatalf("never lined up: %+v", s.Target())
	}
	run(s, 0.1, Input{Pickup: true})
	run(s, 4, Input{})
	if !s.Town.Bins[0].Collected {
		t.Fatalf("bin not collected after lining up")
	}
}

func TestTooFastToPickUp(t *testing.T) {
	s := newTestSession()
	s.PlaceAtBin(0, 0)
	s.Truck.Speed = 8
	s.Step(1.0/60, Input{Pickup: true})
	if s.Arm.Busy() {
		t.Fatalf("arm should not deploy at speed")
	}
	found := false
	for _, e := range s.Events() {
		found = found || e.Kind == EvTooFast
	}
	if !found {
		t.Fatalf("expected a too-fast event")
	}
}

func TestComboMultiplies(t *testing.T) {
	s := newTestSession()
	s.Bonus = Colour(-1) // take the bonus colour out of the maths
	s.nextBonus = math.Inf(1)
	for i := range 3 {
		s.PlaceAtBin(i, 0)
		run(s, 0.1, Input{Pickup: true})
		run(s, 4, Input{})
	}
	if s.Combo != 3 || s.Score != 150+230+300 {
		t.Fatalf("combo %d score %d", s.Combo, s.Score)
	}
}

func TestShiftEndsOnTime(t *testing.T) {
	s := NewSession(Config{TimeLimit: 2, Seed: 3})
	run(s, 2.5, Input{Controls: Controls{Throttle: 1}})
	if !s.Over || s.Remaining() != 0 {
		t.Fatalf("shift should be over")
	}
}

func TestTruckDrivesAndTurns(t *testing.T) {
	s := newTestSession()
	h0 := s.Truck.Heading
	run(s, 3, Input{Controls: Controls{Throttle: 1}})
	if s.Truck.Speed < 8 {
		t.Fatalf("truck too slow: %.1f", s.Truck.Speed)
	}
	run(s, 1, Input{Controls: Controls{Throttle: 0.3, Steer: 1}})
	if dh := WrapAngle(s.Truck.Heading - h0); dh < 0.3 {
		t.Fatalf("truck did not turn right: %.2f", dh)
	}
}

func TestTruckIsBlockedByHouses(t *testing.T) {
	s := newTestSession()
	h := s.Town.Houses[0]
	s.Truck = Truck{Pos: h.P.Add(Dir(h.Heading).Scale(15)), Heading: h.Heading + math.Pi}
	run(s, 6, Input{Controls: Controls{Throttle: 1}})
	if d := s.Truck.Pos.Dist(h.P); d < h.D/2 {
		t.Fatalf("truck drove into the house (%.1fm from centre)", d)
	}
}
