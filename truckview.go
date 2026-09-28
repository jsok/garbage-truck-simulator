package main

import (
	"fmt"
	"math"

	"graphics.gd/classdb/BaseMaterial3D"
	"graphics.gd/classdb/Camera3D"
	"graphics.gd/classdb/GUI"
	"graphics.gd/classdb/Label3D"
	"graphics.gd/classdb/MeshInstance3D"
	"graphics.gd/classdb/Node"
	"graphics.gd/classdb/Node3D"
	"graphics.gd/classdb/QuadMesh"
	"graphics.gd/classdb/StandardMaterial3D"
	"graphics.gd/classdb/SubViewport"
	"graphics.gd/variant/Basis"
	"graphics.gd/variant/Color"
	"graphics.gd/variant/Transform3D"
	"graphics.gd/variant/Vector2"
	"graphics.gd/variant/Vector2i"
	"graphics.gd/variant/Vector3"

	"github.com/jsok/garbage-truck-simulator/internal/meshgen"
	"github.com/jsok/garbage-truck-simulator/internal/sim"
)

// Fork camera resolution; matches the aspect of the dashboard screen.
const forkW, forkH = 736, 500

// TruckView draws the truck, its arm and the driver's cab.
type TruckView struct {
	root   Node3D.Instance
	boom   Node3D.Instance
	head   Node3D.Instance
	claws  [2]Node3D.Instance
	wheel  Node3D.Instance
	speedo Label3D.Instance

	CabCam  Camera3D.Instance
	ForkCam Camera3D.Instance
	ForkVP  SubViewport.Instance

	lean  float64 // 0 looking ahead .. 1 leaning in to read the screen
	shake float64
	clock float64
}

func newTruckView(parent Node.Instance, overlay Node.Instance) *TruckView {
	tv := &TruckView{root: Node3D.New()}
	addChild(parent, tv.root.AsNode())
	r := tv.root.AsNode()

	addChild(r, meshNode(meshgen.TruckBody(), layerTruck, false).AsNode())
	tv.boom = meshNode(meshgen.ArmBoom(), layerTruck, true).AsNode3D()
	addChild(r, tv.boom.AsNode())
	tv.head = meshNode(meshgen.ArmHead(), layerTruck, true).AsNode3D()
	addChild(r, tv.head.AsNode())
	for i, side := range []float32{1, -1} {
		tv.claws[i] = meshNode(meshgen.ArmClaw(side), layerTruck, true).AsNode3D()
		addChild(tv.head.AsNode(), tv.claws[i].AsNode())
	}

	// Cab interior, and glass with a faint reflection of the sky.
	addChild(r, meshNode(meshgen.Cab(), layerCab, false).AsNode())
	glass := &meshgen.Mesh{}
	ws := meshgen.Windscreen
	glass.Quad(ws[0], ws[1], ws[2], ws[3], meshgen.V3{Y: -0.1, Z: 1}, meshgen.RGBA{R: 1, G: 1, B: 1, A: 1})
	for _, x := range []float32{-sim.TruckHalfW + 0.03, sim.TruckHalfW - 0.03} {
		glass.Quad(meshgen.V3{X: x, Y: 1.93, Z: -4.35}, meshgen.V3{X: x, Y: 1.93, Z: -2.6}, meshgen.V3{X: x, Y: 2.95, Z: -2.6}, meshgen.V3{X: x, Y: 2.95, Z: -4.35}, meshgen.V3{X: -x}, meshgen.RGBA{R: 1, G: 1, B: 1, A: 1})
	}
	gm := StandardMaterial3D.New()
	gb := gm.AsBaseMaterial3D()
	gb.SetTransparency(BaseMaterial3D.TransparencyAlpha)
	gb.SetAlbedoColor(Color.RGBA{R: 0.8, G: 0.88, B: 0.95, A: 0.06})
	gb.SetRoughness(0.03)
	gb.SetMetallic(0.2)
	gb.SetMetallicSpecular(0.9)
	gb.SetCullMode(BaseMaterial3D.CullDisabled)
	gn := meshNode(glass, layerCab, false)
	gn.AsGeometryInstance3D().SetMaterialOverride(gm.AsMaterial())
	addChild(r, gn.AsNode())
	tv.wheel = meshNode(meshgen.SteeringWheel(), layerCab, false).AsNode3D()
	addChild(r, tv.wheel.AsNode())

	// The fork camera renders into a texture shown on the dashboard.
	tv.ForkVP = SubViewport.New()
	tv.ForkVP.SetSize(Vector2i.New(forkW, forkH))
	tv.ForkVP.SetRenderTargetUpdateMode(SubViewport.UpdateAlways)
	addChild(r, tv.ForkVP.AsNode())
	tv.ForkCam = Camera3D.New()
	tv.ForkCam.SetCullMask(layerWorld | layerTruck)
	tv.ForkCam.SetFov(66)
	tv.ForkCam.SetNear(0.1)
	addChild(tv.ForkVP.AsNode(), tv.ForkCam.AsNode())
	tv.ForkCam.SetCurrent(true)
	addChild(tv.ForkVP.AsNode(), overlay)

	screen := Node3D.New()
	addChild(r, screen.AsNode())
	target := Vector3.Sub(Vector3.MulX(Vector3.XYZ(meshgen.ScreenPos), 2), Vector3.XYZ(meshgen.EyePos))
	screenXf := Transform3D.LookingAt(Transform3D.BasisOrigin{Basis: Basis.New(), Origin: Vector3.XYZ(meshgen.ScreenPos)}, target, Vector3.Up)
	screen.SetTransform(screenXf)
	addChild(screen.AsNode(), meshNode(meshgen.ScreenBezel(), layerCab, false).AsNode())
	quad := QuadMesh.New()
	quad.AsPlaneMesh().SetSize(Vector2.New(meshgen.ScreenSize[0], meshgen.ScreenSize[1]))
	mat := StandardMaterial3D.New()
	bm := mat.AsBaseMaterial3D()
	bm.SetShadingMode(BaseMaterial3D.ShadingModeUnshaded)
	bm.SetAlbedoTexture(tv.ForkVP.AsViewport().GetTexture().AsTexture2D())
	bm.SetDisableFog(true)
	sq := MeshInstance3D.New()
	sq.SetMesh(quad.AsMesh())
	sq.AsGeometryInstance3D().SetMaterialOverride(mat.AsMaterial())
	sq.AsVisualInstance3D().SetLayers(layerCab)
	addChild(screen.AsNode(), sq.AsNode())

	tv.speedo = Label3D.New()
	tv.speedo.SetFontSize(48)
	tv.speedo.SetPixelSize(0.0006)
	tv.speedo.SetModulate(Color.RGBA{R: 0.55, G: 1, B: 0.7, A: 1})
	tv.speedo.SetOutlineSize(0)
	tv.speedo.SetHorizontalAlignment(GUI.HorizontalAlignmentCenter)
	tv.speedo.AsVisualInstance3D().SetLayers(layerCab)
	tv.speedo.AsGeometryInstance3D().SetCastShadow(0)
	speedoXf := Transform3D.LookingAt(Transform3D.BasisOrigin{Basis: Basis.New(), Origin: Vector3.XYZ(meshgen.SpeedoPos)},
		Vector3.Sub(Vector3.MulX(Vector3.XYZ(meshgen.SpeedoPos), 2), Vector3.XYZ(meshgen.EyePos)), Vector3.Up)
	tv.speedo.AsNode3D().SetTransform(speedoXf)
	addChild(r, tv.speedo.AsNode())

	tv.CabCam = Camera3D.New()
	tv.CabCam.SetCullMask(layerWorld | layerCab | layerMarkers)
	tv.CabCam.SetFov(70)
	tv.CabCam.SetNear(0.05)
	tv.CabCam.SetFar(900)
	addChild(r, tv.CabCam.AsNode())
	return tv
}

// Transform is the truck's current world transform.
func (tv *TruckView) Transform() Transform3D.BasisOrigin { return tv.root.Transform() }

func (tv *TruckView) bump(strength float64) { tv.shake = math.Min(1, tv.shake+strength*0.25) }

func (tv *TruckView) sync(s *sim.Session, dt float64, lean bool) {
	tv.clock += dt
	tr := &s.Truck
	tv.root.SetTransform(groundXform(tr.Pos, tr.Heading, 0))

	// Arm.
	pose := s.Arm.Pose()
	pivot := Vector3.XYZ(meshgen.TruckPoint(pose.Pivot))
	grip := Vector3.XYZ(meshgen.TruckPoint(pose.Grip))
	d := Vector3.Sub(grip, pivot)
	l := float64(Vector3.Length(d))
	y := Vector3.Normalized(d)
	x := Vector3.Normalized(Vector3.Cross(y, vec(0, 0, 1)))
	z := Vector3.Cross(x, y)
	tv.boom.SetTransform(xform(Basis.XYZ{X: x, Y: Vector3.MulX(y, l), Z: z}, pivot))
	tv.head.SetTransform(xform(rollBasis(-pose.Tilt), grip))
	spread := 0.62 - 0.27*pose.Claw
	splay := (1 - pose.Claw) * 0.45
	tv.claws[0].SetTransform(xform(yBasis(-splay), vec(-0.1, 0, -spread)))
	tv.claws[1].SetTransform(xform(yBasis(splay), vec(-0.1, 0, spread)))

	// Cab details.
	wheel := Basis.Mul(xBasis(-meshgen.WheelTilt), rollBasis(-tr.Steer*3.2))
	tv.wheel.SetTransform(xform(wheel, Vector3.XYZ(meshgen.WheelPos)))
	tv.speedo.SetText(fmt.Sprintf("%d km/h", int(math.Round(math.Abs(tr.Speed)*3.6))))

	// Driver's head: a little bounce with speed and bumps, and a lean
	// towards the screen on request.
	target := 0.0
	if lean {
		target = 1
	}
	tv.lean += (target - tv.lean) * math.Min(1, dt*7)
	tv.shake = math.Max(0, tv.shake-dt*1.8)
	bob := 0.012*math.Sin(tv.clock*11)*math.Min(1, math.Abs(tr.Speed)/10) + tv.shake*0.05*math.Sin(tv.clock*47)
	eye := Vector3.Add(Vector3.XYZ(meshgen.EyePos), vec(0, bob, 0))
	look := Vector3.Add(eye, vec(-0.12-tr.Steer*0.25, -0.2, -1))
	screenEye := Vector3.Lerp(Vector3.XYZ(meshgen.ScreenPos), eye, 0.55)
	eye = Vector3.Lerp(eye, screenEye, tv.lean)
	look = Vector3.Lerp(look, Vector3.XYZ(meshgen.ScreenPos), tv.lean)
	tv.CabCam.AsNode3D().SetTransform(Transform3D.LookingAt(Transform3D.BasisOrigin{Basis: Basis.New(), Origin: eye}, look, Vector3.Up))

	// Fork camera: high on the flank above the arm, looking out and down
	// at the kerb, so "ahead" is to the right of the screen.
	camPos := vec(-sim.TruckHalfW-0.2, 3.0, -sim.ArmAlong-1.0)
	camLook := vec(-3.1, 0.35, -sim.ArmAlong+0.15)
	local := Transform3D.LookingAt(Transform3D.BasisOrigin{Basis: Basis.New(), Origin: camPos}, camLook, Vector3.Up)
	tv.ForkCam.AsNode3D().SetGlobalTransform(Transform3D.Mul(tv.Transform(), local))
}
