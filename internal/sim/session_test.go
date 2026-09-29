package sim

import (
	"math"
	"slices"
	"testing"
)

func run(s *Session, secs float64, in Input) {
	const dt = 1.0 / 60
	for i := 0; i < int(secs/dt); i++ {
		s.Step(dt, in)
		in.Pickup = false
	}
}

// newTestSession starts a shift with no random mishaps.
func newTestSession() *Session {
	s := NewSession(Config{TimeLimit: 120, Seed: 3})
	s.Chances = Chances{}
	return s
}

// plainBins lists the bins left tidily on the kerb, not overflowing.
func plainBins(s *Session) []int {
	var out []int
	for i, b := range s.Town.Bins {
		if b.Spot == SpotKerb && !b.Overflowing {
			out = append(out, i)
		}
	}
	return out
}

// tipped returns the EvTipped events raised since the last call.
func tipped(s *Session) []Event {
	var out []Event
	for _, e := range s.Events() {
		if e.Kind == EvTipped {
			out = append(out, e)
		}
	}
	return out
}

func TestAlignedPickupCollectsBin(t *testing.T) {
	s := newTestSession()
	i := plainBins(s)[0]
	s.PlaceAtBin(i, 0)
	tg := s.Target()
	if tg.Bin != i || !tg.Aligned || !tg.Perfect {
		t.Fatalf("expected bin %d perfectly aligned, got %+v", i, tg)
	}
	run(s, 0.1, Input{Pickup: true})
	if !s.Truck.Locked || !s.Arm.Busy() {
		t.Fatalf("pickup did not start")
	}
	home := s.Town.Bins[i].Pos
	run(s, 4, Input{})
	b := s.Town.Bins[i]
	if !b.Collected || b.Held || b.Fallen {
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
	i := plainBins(s)[0]
	s.PlaceAtBin(i, 2.5)
	tg := s.Target()
	if tg.Aligned || tg.Along < 2 {
		t.Fatalf("expected bin 2.5m ahead, got %+v", tg)
	}
	run(s, 0.1, Input{Pickup: true})
	run(s, 3, Input{})
	if b := s.Town.Bins[i]; b.Collected || b.Fallen || s.Misses != 1 || s.Score != 0 {
		t.Fatalf("misaligned pickup should miss without touching the bin")
	}
}

func TestDrivingForwardAlignsBin(t *testing.T) {
	s := newTestSession()
	i := plainBins(s)[0]
	s.PlaceAtBin(i, 2.0)
	// Creep forward until the screen says we are lined up, then grab.
	for i := 0; i < 600 && !s.Target().Aligned; i++ {
		s.Step(1.0/60, Input{Controls: Controls{Throttle: 0.3}})
	}
	if !s.Target().Aligned {
		t.Fatalf("never lined up: %+v", s.Target())
	}
	run(s, 0.1, Input{Pickup: true})
	run(s, 4, Input{})
	if !s.Town.Bins[i].Collected {
		t.Fatalf("bin not collected after lining up")
	}
}

func TestTooFastToPickUp(t *testing.T) {
	s := newTestSession()
	s.PlaceAtBin(plainBins(s)[0], 0)
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
	for _, i := range plainBins(s)[:3] {
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

func TestOverflowingBinScoresMore(t *testing.T) {
	s := newTestSession()
	s.Bonus = Colour(-1)
	i := slices.IndexFunc(s.Town.Bins, func(b Bin) bool { return b.Spot == SpotKerb && b.Overflowing })
	if i < 0 {
		t.Fatal("no overflowing bin on the kerb")
	}
	s.PlaceAtBin(i, 0)
	run(s, 0.1, Input{Pickup: true})
	run(s, 4, Input{})
	if b := s.Town.Bins[i]; !b.Collected || b.Overflowing || s.Score != 200 {
		t.Fatalf("score %d, bin %+v", s.Score, b)
	}
}

func TestRunningIntoBinTipsItOver(t *testing.T) {
	s := newTestSession()
	i := plainBins(s)[0]
	b := &s.Town.Bins[i]
	// Drive along the nature strip straight at it.
	s.PlaceAtBin(i, 10)
	s.Truck.Pos = s.Truck.Pos.Add(s.Truck.Forward().Left().Scale(BinKerbOffset - LaneOffset))
	s.Truck.Speed = 5
	run(s, 3, Input{Controls: Controls{Throttle: 0.3}})
	if !b.Fallen || b.Collected || s.Spilled != 1 {
		t.Fatalf("bin should have spilled: %+v", *b)
	}
	if b.Pos.Dist(b.Home) < 1 {
		t.Errorf("bin only moved %.2fm", b.Pos.Dist(b.Home))
	}
	// Spilled bins are gone for good.
	s.PlaceAtBin(i, 0)
	if tg := s.Target(); tg.Bin == i {
		t.Fatalf("a fallen bin should not be a target")
	}
}

func TestNudgingBinLeavesItStanding(t *testing.T) {
	s := newTestSession()
	i := plainBins(s)[0]
	b := &s.Town.Bins[i]
	// Front of the truck just touching the bin, barely moving.
	s.PlaceAtBin(i, bodyCircles[0]+bodyRadius+BinRadius-0.05-ArmAlong)
	s.Truck.Pos = s.Truck.Pos.Add(s.Truck.Forward().Left().Scale(BinKerbOffset - LaneOffset))
	s.Truck.Speed = 0.4
	run(s, 1, Input{})
	if b.Fallen || b.Pos == b.Home {
		t.Fatalf("bin should be nudged, not tipped: %+v", *b)
	}
}

func TestBadlyLinedUpForkKnocksBinOver(t *testing.T) {
	s := newTestSession()
	i := plainBins(s)[0]
	s.PlaceAtBin(i, 1.3)
	if tg := s.Target(); tg.Aligned || !tg.InReach {
		t.Fatalf("expected an unaligned bin in reach, got %+v", tg)
	}
	run(s, 0.1, Input{Pickup: true})
	run(s, 3, Input{})
	ev := tipped(s)
	if b := s.Town.Bins[i]; !b.Fallen || b.Collected || s.Misses != 1 || len(ev) != 1 || ev[0].Cause != TipFork {
		t.Fatalf("fork should have knocked the bin over: %+v, events %+v", b, ev)
	}
}

func TestDroppedBinIsLost(t *testing.T) {
	s := newTestSession()
	s.Chances.Drop = 1
	i := plainBins(s)[0]
	s.PlaceAtBin(i, 0)
	run(s, 0.1, Input{Pickup: true})
	run(s, 4, Input{})
	b := s.Town.Bins[i]
	if !b.Fallen || b.Collected || b.Held || s.Score != 0 || s.Spilled != 1 {
		t.Fatalf("bin should have been dropped: %+v score %d", b, s.Score)
	}
	if s.Arm.Busy() || s.Truck.Locked {
		t.Fatalf("arm should be idle and truck free")
	}
	if d := b.Pos.Sub(s.Truck.Pos).Dot(s.Truck.Forward().Left()); d < TruckHalfW+BinRadius {
		t.Errorf("bin landed under the truck (%.2fm out)", d)
	}
}

func TestBinCanFallWhenPutDown(t *testing.T) {
	s := newTestSession()
	s.Bonus = Colour(-1)
	s.Chances.PutDown = 1
	i := plainBins(s)[0]
	s.PlaceAtBin(i, 0)
	run(s, 0.1, Input{Pickup: true})
	run(s, 4, Input{})
	b := s.Town.Bins[i]
	ev := tipped(s)
	if !b.Collected || !b.Fallen || s.Knocked != 1 || s.Spilled != 0 || len(ev) != 1 || ev[0].Cause != TipPutDown {
		t.Fatalf("bin should be emptied then fall over: %+v, events %+v", b, ev)
	}
	if s.Score != 150-KnockPenalty {
		t.Errorf("score %d", s.Score)
	}
}
