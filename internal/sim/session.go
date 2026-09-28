package sim

import (
	"math"
	"math/rand/v2"
)

// Scoring.
const (
	BasePoints    = 100
	PerfectBonus  = 50
	ComboWindow   = 20.0 // seconds between pickups to keep a combo going
	MaxMultiplier = 3.0
	BonusInterval = 40.0 // seconds between bonus colour changes
)

// Config sets up a shift.
type Config struct {
	TimeLimit float64 // seconds
	Seed      int64
}

// Input is everything the player can do in one step.
type Input struct {
	Controls
	Pickup bool // edge-triggered: the pickup button was just pressed
}

// EventKind classifies things the presentation layer should react to.
type EventKind int

const (
	EvPickup    EventKind = iota // arm heads out for an aligned bin
	EvCollected                  // bin emptied into the hopper
	EvMiss                       // swung at nothing
	EvTooFast                    // aligned, but moving too fast to grab
	EvBump                       // hit something
	EvShove                      // nudged a bin with the truck
	EvArmPhase                   // arm moved to a new phase
	EvBonus                      // a new bonus colour
	EvTimeUp
)

// Event is something that happened during a step.
type Event struct {
	Kind     EventKind
	Bin      int
	Points   int
	Perfect  bool
	Combo    int
	Colour   Colour
	Strength float64
	Phase    ArmPhase
}

// Target is the bin the arm would go for right now.
type Target struct {
	Bin     int     // -1 if nothing is close
	Along   float64 // metres to drive forward (negative: back up) to line up
	Lateral float64 // bin distance from the truck centreline, leftwards
	InReach bool    // lateral distance is within the arm's reach
	Aligned bool
	Perfect bool
}

// Session is one shift on the truck.
type Session struct {
	Cfg   Config
	Town  *Town
	Truck Truck
	Arm   Arm

	Clock     float64
	Score     int
	Counts    [len(Colours)]int
	Combo     int
	BestCombo int
	Perfects  int
	Misses    int
	Bonus     Colour
	Over      bool

	lastCollect float64
	nextBonus   float64
	scored      bool // current arm cycle has been scored
	perfect     bool // current arm cycle was lined up perfectly
	events      []Event
	rng         *rand.Rand
}

// NewSession starts a shift in a freshly generated suburb.
func NewSession(cfg Config) *Session {
	if cfg.TimeLimit <= 0 {
		cfg.TimeLimit = 180
	}
	town := Generate(cfg.Seed)
	s := &Session{
		Cfg:         cfg,
		Town:        town,
		Truck:       Truck{Pos: town.StartPos, Heading: town.StartHeading},
		Arm:         Arm{Bin: -1},
		lastCollect: math.Inf(-1),
		rng:         rand.New(rand.NewPCG(uint64(cfg.Seed), 99)),
	}
	s.Bonus = Colours[s.rng.IntN(len(Colours))]
	s.nextBonus = BonusInterval
	return s
}

// Remaining is the time left on the clock.
func (s *Session) Remaining() float64 { return math.Max(0, s.Cfg.TimeLimit-s.Clock) }

// Collected is the total number of bins emptied.
func (s *Session) Collected() int {
	n := 0
	for _, c := range s.Counts {
		n += c
	}
	return n
}

// Multiplier is the combo multiplier the next pickup would get if it is made
// in time.
func (s *Session) Multiplier(combo int) float64 {
	return math.Min(MaxMultiplier, 1+0.5*float64(max(combo, 1)-1))
}

// ComboLeft is the time left to keep the current combo going.
func (s *Session) ComboLeft() float64 {
	return math.Max(0, ComboWindow-(s.Clock-s.lastCollect))
}

// Events returns and clears the events raised since the last call.
func (s *Session) Events() []Event {
	ev := s.events
	s.events = nil
	return ev
}

func (s *Session) emit(e Event) { s.events = append(s.events, e) }

// Target finds the best bin for the arm to go for.
func (s *Session) Target() Target {
	best := Target{Bin: -1}
	bestScore := math.Inf(1)
	f := s.Truck.Forward()
	mount := s.Truck.Local(ArmAlong, 0)
	for i := range s.Town.Bins {
		b := &s.Town.Bins[i]
		if b.Collected || b.Held {
			continue
		}
		d := b.Pos.Sub(mount)
		along, lat := d.Dot(f), d.Dot(f.Left())
		if lat < 0.8 || lat > 9 || math.Abs(along) > 15 {
			continue
		}
		off := math.Max(0, lat-MaxBinLateral) + math.Max(0, MinBinLateral-lat)
		if score := math.Abs(along) + 2*off; score < bestScore {
			bestScore = score
			best = Target{Bin: i, Along: along, Lateral: lat}
		}
	}
	if best.Bin >= 0 {
		best.InReach = best.Lateral >= MinBinLateral && best.Lateral <= MaxBinLateral
		best.Aligned = best.InReach && math.Abs(best.Along) <= AlignTolerance
		best.Perfect = best.Aligned && math.Abs(best.Along) <= PerfectTolerance
	}
	return best
}

// Step advances the shift by dt seconds.
func (s *Session) Step(dt float64, in Input) {
	if s.Over {
		return
	}
	s.Clock += dt
	if s.Clock >= s.nextBonus {
		s.nextBonus += BonusInterval
		prev := s.Bonus
		for s.Bonus == prev {
			s.Bonus = Colours[s.rng.IntN(len(Colours))]
		}
		s.emit(Event{Kind: EvBonus, Colour: s.Bonus})
	}

	if in.Pickup && !s.Arm.Busy() {
		s.tryPickup()
	}

	if impact, shoved := s.Truck.Step(dt, in.Controls, s.Town); impact > 0.5 {
		s.emit(Event{Kind: EvBump, Strength: impact})
	} else if shoved {
		s.emit(Event{Kind: EvShove})
	}

	if phase := s.Arm.step(dt); phase != ArmIdle {
		s.enterPhase(phase)
	} else if !s.Arm.Busy() {
		s.Truck.Locked = false
	}

	// The shift ends on the buzzer, but a bin already in the air still counts.
	if s.Clock >= s.Cfg.TimeLimit && !(s.Arm.Holding() && !s.scored) {
		s.Over = true
		s.Truck.Locked = true
		s.emit(Event{Kind: EvTimeUp})
	}
}

func (s *Session) tryPickup() {
	t := s.Target()
	switch {
	case t.Aligned && math.Abs(s.Truck.Speed) <= MaxPickupSpeed:
		s.Truck.Locked = true
		s.Truck.Speed = 0
		s.scored, s.perfect = false, t.Perfect
		reach := t.Lateral - TruckHalfW - BinRadius
		s.Arm.start(t.Bin, reach, t.Along)
		s.emit(Event{Kind: EvPickup, Bin: t.Bin, Perfect: t.Perfect})
	case t.Aligned:
		s.emit(Event{Kind: EvTooFast, Bin: t.Bin})
	default:
		reach := ArmMaxExt
		if t.Bin >= 0 && t.InReach {
			reach = t.Lateral - TruckHalfW - BinRadius
		}
		s.Arm.start(-1, reach, 0)
		s.Misses++
		s.emit(Event{Kind: EvMiss, Bin: t.Bin})
	}
}

func (s *Session) enterPhase(p ArmPhase) {
	s.emit(Event{Kind: EvArmPhase, Phase: p, Bin: s.Arm.Bin})
	if p == ArmStow {
		s.Truck.Locked = false
	}
	if s.Arm.Bin < 0 {
		return
	}
	b := &s.Town.Bins[s.Arm.Bin]
	switch p {
	case ArmGrip:
		b.Held = true
	case ArmTip:
		s.collect(s.Arm.Bin)
	case ArmStow:
		// Put the bin back on the kerb where it was picked up.
		b.Held = false
		pose := s.Arm.Pose()
		b.Pos = s.Truck.Local(pose.Bin.Along, pose.Bin.Lat)
		b.Heading = s.Truck.Heading + math.Pi/2 // facing the truck, as it was held
	}
}

func (s *Session) collect(bin int) {
	b := &s.Town.Bins[bin]
	b.Collected = true
	s.scored = true
	if s.Clock-s.lastCollect <= ComboWindow {
		s.Combo++
	} else {
		s.Combo = 1
	}
	s.lastCollect = s.Clock
	s.BestCombo = max(s.BestCombo, s.Combo)
	pts := float64(BasePoints)
	if s.perfect {
		pts += PerfectBonus
		s.Perfects++
	}
	if b.Colour == s.Bonus {
		pts *= 2
	}
	pts *= s.Multiplier(s.Combo)
	points := int(math.Round(pts/10) * 10)
	s.Score += points
	s.Counts[b.Colour]++
	s.emit(Event{Kind: EvCollected, Bin: bin, Points: points, Perfect: s.perfect, Combo: s.Combo, Colour: b.Colour})
}

// PlaceAtBin parks the truck in the kerbside lane beside bin i, with the arm
// short of the bin by alongErr metres. It is used by tests and demo scenes.
func (s *Session) PlaceAtBin(i int, alongErr float64) {
	b := s.Town.Bins[i].Pos
	_, road, at := s.Town.NearestRoad(b)
	p, tg := s.Town.Roads[road].At(at)
	if b.Sub(p).Dot(tg.Left()) < 0 {
		tg = tg.Scale(-1)
	}
	lat := b.Sub(p).Dot(tg.Left()) - LaneOffset
	s.Truck = Truck{Heading: tg.Angle()}
	s.Truck.Pos = b.Sub(tg.Scale(ArmAlong + alongErr)).Sub(tg.Left().Scale(lat))
}
