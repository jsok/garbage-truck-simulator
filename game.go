package main

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"graphics.gd/classdb/Camera3D"
	"graphics.gd/classdb/CanvasLayer"
	"graphics.gd/classdb/Control"
	"graphics.gd/classdb/Input"
	"graphics.gd/classdb/InputEvent"
	"graphics.gd/classdb/InputEventKey"
	"graphics.gd/classdb/Node3D"
	"graphics.gd/classdb/OS"
	"graphics.gd/classdb/SceneTree"
	"graphics.gd/classdb/Viewport"
	"graphics.gd/variant/Basis"
	"graphics.gd/variant/Float"
	"graphics.gd/variant/Object"
	"graphics.gd/variant/Transform3D"
	"graphics.gd/variant/Vector2"
	"graphics.gd/variant/Vector3"

	"github.com/jsok/garbage-truck-simulator/internal/sim"
)

type gameState int

const (
	stateTitle gameState = iota
	stateCountdown
	statePlaying
	statePaused
	stateResults
)

// Shift lengths offered on the title screen, in seconds.
var shiftLengths = []int{60, 120, 180, 300, 600}

// Game is the root node: it owns the session and all of the views.
type Game struct {
	Node3D.Extension[Game]

	state     gameState
	seed      int64
	timeIdx   int
	sess      *sim.Session
	world     *WorldView
	truck     *TruckView
	audio     *Audio
	titleCam  Camera3D.Instance
	debugCam  Camera3D.Instance
	hud       *HUD
	minimap   *Minimap
	overlay   *ForkOverlay
	popups    []popup
	clock     float64
	countdown float64
	pickup    bool // pickup pressed since the last step
	wasLined  bool
	lastTick  int
	best      map[string]int
	newRecord bool

	// Command line options, mostly for development.
	autoplay bool
	scenario string
	view     string

	synthKeys bool // replay a canned key sequence through the input system
	synthStep int
}

// synthScript is a key sequence for checking the real input path: start a
// shift from the title screen, drive forward, then swing the arm.
var synthScript = []struct {
	at      float64
	key     Input.Key
	pressed bool
}{
	{0.5, Input.KeyEnter, true}, {0.6, Input.KeyEnter, false},
	{4.0, Input.KeyW, true}, {6.0, Input.KeyW, false},
	{6.2, Input.KeyD, true}, {6.8, Input.KeyD, false},
	{7.5, Input.KeySpace, true}, {7.6, Input.KeySpace, false},
	{9.0, Input.KeyEscape, true}, {9.1, Input.KeyEscape, false},
	{9.5, Input.KeyQ, true}, {9.6, Input.KeyQ, false},
	{10.5, Input.KeyN, true}, {10.6, Input.KeyN, false},
	{11.5, Input.KeyEnter, true}, {11.6, Input.KeyEnter, false},
}

func (g *Game) replayKeys() {
	for g.synthStep < len(synthScript) && g.clock >= synthScript[g.synthStep].at {
		k := synthScript[g.synthStep]
		ev := InputEventKey.New()
		ev.SetKeycode(k.key)
		ev.SetPressed(k.pressed)
		Input.ParseInputEvent(ev.AsInputEvent())
		g.synthStep++
	}
}

func (g *Game) timeLimit() float64 { return float64(shiftLengths[g.timeIdx]) }
func (g *Game) bestKey() string    { return strconv.Itoa(shiftLengths[g.timeIdx]) }

func (g *Game) parseArgs() {
	g.seed = rand.Int64N(10000)
	g.timeIdx = 2
	for _, arg := range OS.GetCmdlineUserArgs() {
		k, v, _ := strings.Cut(strings.TrimLeft(arg, "-"), "=")
		switch k {
		case "time":
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				if !slices.Contains(shiftLengths, n) {
					shiftLengths = append(shiftLengths, n)
					slices.Sort(shiftLengths)
				}
				g.timeIdx = slices.Index(shiftLengths, n)
			}
		case "seed":
			if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				g.seed = n
			}
		case "play":
			g.autoplay = true
		case "scenario":
			g.scenario, g.autoplay = v, true
		case "view":
			g.view = v
		case "synth-keys":
			g.synthKeys = true
		}
	}
}

func scoresPath() string { return filepath.Join(OS.GetUserDataDir(), "scores.json") }

func (g *Game) loadScores() {
	g.best = map[string]int{}
	if b, err := os.ReadFile(scoresPath()); err == nil {
		_ = json.Unmarshal(b, &g.best)
	}
}

func (g *Game) saveScores() {
	if b, err := json.Marshal(g.best); err == nil {
		_ = os.MkdirAll(filepath.Dir(scoresPath()), 0o755)
		_ = os.WriteFile(scoresPath(), b, 0o644)
	}
}

func (g *Game) Ready() {
	g.parseArgs()
	g.loadScores()
	root := g.AsNode()
	addEnvironment(root)
	g.audio = newAudio(root)

	g.overlay = new(ForkOverlay)
	g.overlay.game = g
	g.overlay.AsControl().SetSize(Vector2.New(forkW, forkH))
	g.truck = newTruckView(root, g.overlay.AsNode())

	g.titleCam = Camera3D.New()
	g.titleCam.SetCullMask(layerWorld | layerTruck | layerMarkers)
	g.titleCam.SetFar(1500)
	addChild(root, g.titleCam.AsNode())
	g.debugCam = Camera3D.New()
	g.debugCam.SetCullMask(layerWorld | layerTruck | layerMarkers)
	g.debugCam.SetFar(1500)
	addChild(root, g.debugCam.AsNode())

	layer := CanvasLayer.New()
	addChild(root, layer.AsNode())
	g.hud = new(HUD)
	g.hud.game = g
	g.hud.AsControl().SetAnchorsPreset(Control.PresetFullRect)
	g.hud.AsControl().SetMouseFilter(Control.MouseFilterIgnore)
	addChild(layer.AsNode(), g.hud.AsNode())
	g.minimap = new(Minimap)
	g.minimap.game = g
	g.minimap.AsControl().SetClipContents(true)
	g.minimap.AsControl().SetMouseFilter(Control.MouseFilterIgnore)
	addChild(layer.AsNode(), g.minimap.AsNode())

	g.newSession()
	g.toTitle()
	if g.autoplay {
		g.start()
	}
}

// newSession prepares a fresh shift, reusing the scenery when the suburb
// has not changed.
func (g *Game) newSession() {
	g.sess = sim.NewSession(sim.Config{TimeLimit: g.timeLimit(), Seed: g.seed})
	if g.world != nil && g.world.seed == g.seed {
		g.world.bindBins(g.sess.Town)
	} else {
		if g.world != nil {
			g.world.free()
		}
		g.world = newWorldView(g.AsNode(), g.sess.Town)
	}
	g.popups = nil
	g.lastTick = -1
	g.wasLined = false
	g.newRecord = false
}

func (g *Game) toTitle() {
	g.state = stateTitle
	g.titleCam.MakeCurrent()
}

func (g *Game) start() {
	g.newSession()
	if g.scenario == "pickup" {
		g.sess.PlaceAtBin(0, 3)
	}
	g.state = stateCountdown
	g.countdown = 3
	if g.autoplay {
		g.countdown = 0.01
	}
	g.truck.CabCam.MakeCurrent()
	switch g.view {
	case "chase", "top":
		g.debugCam.MakeCurrent()
	}
}

func (g *Game) say(msg string, col [4]float64, size int) {
	g.popups = append(g.popups, popup{msg: msg, col: col, size: size})
	if len(g.popups) > 4 {
		g.popups = g.popups[1:]
	}
}

var (
	gold  = [4]float64{1, 0.9, 0.3, 1}
	green = [4]float64{0.5, 1, 0.6, 1}
	pink  = [4]float64{1, 0.5, 0.65, 1}
	white = [4]float64{1, 1, 1, 1}
)

func (g *Game) UnhandledInput(event InputEvent.Instance) {
	key, ok := Object.As[InputEventKey.Instance](event)
	if g.scenario != "" || !ok || !event.IsPressed() || event.IsEcho() {
		return
	}
	k := key.Keycode()
	switch g.state {
	case stateTitle:
		switch k {
		case Input.KeyLeft, Input.KeyA:
			g.timeIdx = max(0, g.timeIdx-1)
			g.audio.Play("select", 1, -6)
		case Input.KeyRight, Input.KeyD:
			g.timeIdx = min(len(shiftLengths)-1, g.timeIdx+1)
			g.audio.Play("select", 1, -6)
		case Input.KeyN:
			g.seed = rand.Int64N(10000)
			g.newSession()
			g.audio.Play("select", 1.5, -6)
		case Input.KeyEnter, Input.KeyKpEnter, Input.KeySpace:
			g.start()
		case Input.KeyEscape:
			SceneTree.Get(g.AsNode()).Quit()
		}
	case stateCountdown, statePlaying:
		switch k {
		case Input.KeySpace, Input.KeyE:
			g.pickup = true
		case Input.KeyEscape, Input.KeyP:
			if g.state == statePlaying {
				g.state = statePaused
			}
		}
	case statePaused:
		switch k {
		case Input.KeyEscape, Input.KeyP:
			g.state = statePlaying
		case Input.KeyQ:
			g.newSession()
			g.toTitle()
		}
	case stateResults:
		switch k {
		case Input.KeyEnter, Input.KeyKpEnter:
			g.start()
		case Input.KeyEscape:
			g.newSession()
			g.toTitle()
		}
	}
}

// pressed reports whether any of keys is held (never during scripted runs).
func (g *Game) pressed(keys ...Input.Key) bool {
	if g.scenario != "" {
		return false
	}
	for _, k := range keys {
		if Input.IsKeyPressed(k) {
			return true
		}
	}
	return false
}

func (g *Game) controls() sim.Input {
	var in sim.Input
	if g.scenario != "" {
		g.autopilot(&in) // scripted runs ignore the keyboard
		return in
	}
	if g.pressed(Input.KeyW, Input.KeyUp) {
		in.Throttle += 1
	}
	if g.pressed(Input.KeyS, Input.KeyDown) {
		in.Throttle -= 1
	}
	if g.pressed(Input.KeyA, Input.KeyLeft) {
		in.Steer -= 1
	}
	if g.pressed(Input.KeyD, Input.KeyRight) {
		in.Steer += 1
	}
	in.Pickup = g.pickup
	g.pickup = false
	return in
}

// autopilot drives the scripted scenarios used for screenshots.
func (g *Game) autopilot(in *sim.Input) {
	t := g.sess.Target()
	switch g.scenario {
	case "pickup":
		if !g.sess.Arm.Busy() && t.Bin >= 0 {
			if t.Aligned && math.Abs(t.Along) < 0.25 && math.Abs(g.sess.Truck.Speed) < 1 {
				in.Pickup = true
			} else {
				in.Throttle = math.Max(-1, math.Min(1, t.Along*0.8-g.sess.Truck.Speed*1.5))
			}
		}
	case "drive":
		in.Throttle = 0.7
	}
}

func (g *Game) Process(delta Float.X) {
	dt := float64(delta)
	g.clock += dt
	Object.Use(g.titleCam)
	Object.Use(g.debugCam)
	if g.synthKeys {
		g.replayKeys()
	}
	s := g.sess
	for i := range g.popups {
		g.popups[i].age += dt
	}
	g.popups = slices.DeleteFunc(g.popups, func(p popup) bool { return p.age > 1.8 })

	switch g.state {
	case stateCountdown:
		prev := int(math.Ceil(g.countdown))
		g.countdown -= dt
		if n := int(math.Ceil(g.countdown)); n != prev && n > 0 {
			g.audio.Play("count", 1, -4)
		}
		if g.countdown <= 0 {
			g.state = statePlaying
			g.audio.Play("go", 1, -3)
			g.say("GO!", green, 110)
		}
	case statePlaying:
		in := g.controls()
		// Fixed sub-steps keep the handling identical at any frame rate.
		steps := max(1, int(math.Ceil(dt/(1.0/120))))
		for i := range steps {
			if i > 0 {
				in.Pickup = false
			}
			s.Step(dt/float64(steps), in)
		}
		g.handleEvents()
		if rem := int(math.Ceil(s.Remaining())); rem <= 10 && rem != g.lastTick && rem > 0 {
			g.lastTick = rem
			g.audio.Play("tick", 1, -2)
		}
		if s.Over {
			g.finish()
		}
	}

	playing := g.state == statePlaying || g.state == stateCountdown
	lean := playing && g.pressed(Input.KeyF, Input.KeyTab)
	g.truck.sync(s, dt, lean)
	truckXf := g.truck.Transform()
	g.world.sync(s, truckXf, g.clock)
	throttle := 0.0
	if playing && g.pressed(Input.KeyW, Input.KeyUp) {
		throttle = 1
	}
	g.audio.Update(g.state != stateTitle && g.state != statePaused, s.Truck.Speed, throttle, playing && g.pressed(Input.KeyH), s.Arm.Busy())

	if g.state == stateTitle {
		titleCamera(g.titleCam, s.Town, g.clock)
	}
	switch g.view {
	case "chase":
		eye := Transform3D.Transform(vec(0, 7, 16), truckXf)
		t := Transform3D.BasisOrigin{Basis: Basis.New(), Origin: eye}
		g.debugCam.AsNode3D().SetGlobalTransform(Transform3D.LookingAt(t, truckXf.Origin, Vector3.Up))
	case "top":
		eye := Vector3.Add(truckXf.Origin, vec(0.01, 60, 0))
		t := Transform3D.BasisOrigin{Basis: Basis.New(), Origin: eye}
		g.debugCam.AsNode3D().SetGlobalTransform(Transform3D.LookingAt(t, truckXf.Origin, vec(0, 0, -1)))
	}

	// Keep the minimap in the bottom-left corner.
	size := Viewport.Get(g.AsNode()).GetVisibleRect().Size
	ui := math.Max(0.7, float64(size.Y)/1080)
	mm := 300 * ui
	g.minimap.AsControl().SetPosition(Vector2.New(float64(size.X)-mm-18*ui, float64(size.Y)-mm-18*ui))
	g.minimap.AsControl().SetSize(Vector2.New(mm, mm))

	g.hud.AsCanvasItem().QueueRedraw()
	g.minimap.AsCanvasItem().QueueRedraw()
	g.overlay.AsCanvasItem().QueueRedraw()
}

func (g *Game) handleEvents() {
	s := g.sess
	for _, e := range s.Events() {
		switch e.Kind {
		case sim.EvPickup:
			g.audio.Play("beep", 1.2, -6)
		case sim.EvCollected:
			g.audio.Play(strings.ToLower(e.Colour.String()), 1, -2)
			g.audio.Play("chime", 1+0.06*float64(min(e.Combo, 8)-1), -4)
			msg := fmt.Sprintf("+%d", e.Points)
			col := white
			if e.Perfect {
				msg += "  PERFECT!"
				col = gold
			}
			if e.Colour == s.Bonus {
				msg += "  BONUS!"
			}
			g.say(msg, col, 64)
			if e.Combo > 1 {
				g.say(fmt.Sprintf("COMBO x%.1f", s.Multiplier(e.Combo)), green, 44)
				g.audio.Play("combo", 1+0.1*float64(min(e.Combo, 6)), -8)
			}
		case sim.EvMiss:
			g.audio.Play("miss", 1, -4)
			g.say("WHOOSH! Missed", pink, 48)
		case sim.EvTooFast:
			g.audio.Play("miss", 1.4, -8)
			g.say("Slow down to grab it!", pink, 44)
		case sim.EvBump:
			g.audio.Play("bump", 1, math.Min(0, -12+e.Strength*3))
			g.truck.bump(e.Strength)
		case sim.EvShove:
			g.audio.Play("shove", 1+rand.Float64()*0.3, -10)
		case sim.EvArmPhase:
			if e.Phase == sim.ArmReach || e.Phase == sim.ArmRelease {
				g.audio.Play("thud", 1.6, -14)
			}
		case sim.EvBonus:
			g.say(fmt.Sprintf("BONUS: %s BINS x2!", e.Colour), gold, 52)
			g.audio.Play("combo", 0.8, -6)
		case sim.EvTimeUp:
			g.audio.Play("buzzer", 1, -3)
		}
	}
	lined := s.Target().Aligned && !s.Arm.Busy()
	if lined && !g.wasLined {
		g.audio.Play("beep", 1, -8)
	}
	g.wasLined = lined
}

func (g *Game) finish() {
	g.state = stateResults
	if s := g.sess; s.Score > g.best[g.bestKey()] {
		g.newRecord = s.Score > 0
		g.best[g.bestKey()] = s.Score
		g.saveScores()
	}
}
