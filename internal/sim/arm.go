package sim

import "math"

// Side-loader arm geometry in the truck frame. The arm is mounted on the
// left flank just behind the cab, out of the driver's sight: the fork camera
// on the dashboard is the only way to see it.
const (
	ArmAlong   = 1.2 // forward offset of the arm from the truck centre
	ArmStowed  = 0.2 // gripper extension beyond the flank when tucked away
	ArmMinExt  = 0.35
	ArmMaxExt  = 3.6
	BinRadius  = 0.32
	BinHeight  = 1.05
	GripHeight = 0.6 // height of the gripper jaws above the ground

	// Line-up tolerances along the kerb. Generous on purpose: this is meant
	// to be fun.
	AlignTolerance   = 0.9
	PerfectTolerance = 0.3
	MaxPickupSpeed   = 3.0 // m/s; the truck auto-stops for a pickup

	armPivotLat = TruckHalfW + 0.05
	armPivotUp  = 1.1
)

// MinBinLateral and MaxBinLateral bound the distance from the truck's
// centreline at which a bin can be grabbed.
const (
	MinBinLateral = TruckHalfW + ArmMinExt + BinRadius
	MaxBinLateral = TruckHalfW + ArmMaxExt + BinRadius
)

// ArmPhase is a step of the pickup cycle.
type ArmPhase int

const (
	ArmIdle    ArmPhase = iota
	ArmReach            // telescope out to the bin
	ArmGrip             // close the jaws
	ArmLift             // swing the bin up over the hopper
	ArmTip              // shake it empty
	ArmLower            // bring it back down
	ArmRelease          // open the jaws
	ArmStow             // retract
)

var phaseTime = [...]float64{
	ArmIdle: 0, ArmReach: 0.4, ArmGrip: 0.18, ArmLift: 0.6,
	ArmTip: 0.55, ArmLower: 0.5, ArmRelease: 0.18, ArmStow: 0.35,
}

// Arm is the side-loader's state.
type Arm struct {
	Phase ArmPhase
	T     float64 // time spent in the current phase
	Bin   int     // bin being handled, -1 for a swing at thin air
	ext   float64 // extension at the start of the current phase
	reach float64 // target extension
	Along float64 // bin offset along the truck when grabbed, relative to ArmAlong
}

// Busy reports whether a cycle is under way.
func (a *Arm) Busy() bool { return a.Phase != ArmIdle }

// Holding reports whether the arm has a bin off the ground.
func (a *Arm) Holding() bool {
	return a.Bin >= 0 && a.Phase >= ArmGrip && a.Phase <= ArmRelease
}

// Progress is the eased 0..1 progress through the current phase.
func (a *Arm) Progress() float64 {
	d := phaseTime[a.Phase]
	if d == 0 {
		return 0
	}
	return smooth(clamp(a.T/d, 0, 1))
}

func smooth(x float64) float64 { return x * x * (3 - 2*x) }

func (a *Arm) start(bin int, reach, along float64) {
	*a = Arm{Phase: ArmReach, Bin: bin, ext: ArmStowed, reach: clamp(reach, ArmMinExt, ArmMaxExt), Along: along}
}

// step advances the arm, returning the phase it entered (or ArmIdle if the
// phase did not change).
func (a *Arm) step(dt float64) ArmPhase {
	if a.Phase == ArmIdle {
		return ArmIdle
	}
	a.T += dt
	if a.T < phaseTime[a.Phase] {
		return ArmIdle
	}
	a.T -= phaseTime[a.Phase]
	switch {
	case a.Phase == ArmGrip && a.Bin < 0:
		a.Phase = ArmStow // nothing to lift
	case a.Phase == ArmStow:
		a.Phase, a.T = ArmIdle, 0
		return ArmIdle
	default:
		a.Phase++
	}
	a.ext = a.Extension()
	return a.Phase
}

// Extension is how far the gripper currently sits beyond the flank.
func (a *Arm) Extension() float64 {
	u := a.Progress()
	switch a.Phase {
	case ArmReach:
		return lerp(ArmStowed, a.reach, u)
	case ArmGrip, ArmRelease:
		return a.reach
	case ArmStow:
		return lerp(a.ext, ArmStowed, u)
	case ArmLift, ArmTip, ArmLower:
		return a.reach // the lift path is computed in Pose
	}
	return ArmStowed
}

func lerp(a, b, t float64) float64 { return a + (b-a)*t }

// L3 is a point in the truck frame: forward, leftward and upward offsets.
type L3 struct{ Along, Lat, Up float64 }

// ArmPose describes the arm for rendering.
type ArmPose struct {
	Pivot L3      // where the boom meets the truck
	Grip  L3      // centre of the gripper jaws
	Tilt  float64 // roll of the gripper about the along axis, towards the truck
	Claw  float64 // 0 open .. 1 closed
	Bin   L3      // base centre of a held bin
}

// Pose computes where each part of the arm is right now.
func (a *Arm) Pose() ArmPose {
	u := a.Progress()
	lift := 0.0
	switch a.Phase {
	case ArmLift:
		lift = u
	case ArmTip:
		lift = 1
	case ArmLower:
		lift = 1 - u
	}
	ext := a.Extension()
	low := L3{ArmAlong + a.Along, TruckHalfW + ext, GripHeight}
	high := L3{ArmAlong + a.Along*0.5, TruckHalfW + 0.35, 3.85}
	grip := L3{
		Along: lerp(low.Along, high.Along, lift),
		Lat:   lerp(low.Lat, high.Lat, smooth(lift)),
		Up:    lerp(low.Up, high.Up, math.Sin(lift*math.Pi/2)),
	}
	tilt := lift * 2.45
	if a.Phase == ArmTip {
		tilt += 0.22 * math.Sin(a.T*38) * (1 - a.T/phaseTime[ArmTip])
	}
	claw := 0.0
	switch a.Phase {
	case ArmGrip:
		claw = u
	case ArmLift, ArmTip, ArmLower:
		claw = 1
	case ArmRelease:
		claw = 1 - u
	}
	if a.Bin < 0 && a.Phase == ArmStow {
		claw = 1 - u
	}
	// The bin hangs from the jaws, which grip it on the truck-facing side.
	up := L3{0, -math.Sin(tilt), math.Cos(tilt)}
	out := L3{0, math.Cos(tilt), math.Sin(tilt)}
	bin := L3{
		Along: grip.Along,
		Lat:   grip.Lat - up.Lat*GripHeight + out.Lat*BinRadius,
		Up:    grip.Up - up.Up*GripHeight + out.Up*BinRadius,
	}
	return ArmPose{
		Pivot: L3{grip.Along, armPivotLat, armPivotUp},
		Grip:  grip,
		Tilt:  tilt,
		Claw:  claw,
		Bin:   bin,
	}
}
