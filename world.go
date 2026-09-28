package main

import (
	"math"

	"graphics.gd/variant/Object"

	"graphics.gd/classdb/Camera3D"
	"graphics.gd/classdb/DirectionalLight3D"
	"graphics.gd/classdb/Environment"
	"graphics.gd/classdb/Light3D"
	"graphics.gd/classdb/Mesh"
	"graphics.gd/classdb/Node"
	"graphics.gd/classdb/Node3D"
	"graphics.gd/classdb/ProceduralSkyMaterial"
	"graphics.gd/classdb/Sky"
	"graphics.gd/classdb/WorldEnvironment"
	"graphics.gd/variant/Basis"
	"graphics.gd/variant/Color"
	"graphics.gd/variant/Euler"
	"graphics.gd/variant/Transform3D"
	"graphics.gd/variant/Vector3"

	"github.com/jsok/garbage-truck-simulator/internal/meshgen"
	"github.com/jsok/garbage-truck-simulator/internal/sim"
)

// addEnvironment sets up a sunny suburban morning.
func addEnvironment(parent Node.Instance) {
	sky := ProceduralSkyMaterial.New()
	sky.SetSkyTopColor(Color.RGBA{R: 0.22, G: 0.45, B: 0.85, A: 1})
	sky.SetSkyHorizonColor(Color.RGBA{R: 0.72, G: 0.82, B: 0.92, A: 1})
	sky.SetGroundHorizonColor(Color.RGBA{R: 0.55, G: 0.62, B: 0.5, A: 1})
	sky.SetGroundBottomColor(Color.RGBA{R: 0.3, G: 0.38, B: 0.25, A: 1})
	s := Sky.New()
	s.SetSkyMaterial(sky.AsMaterial())

	env := Environment.New()
	env.SetBackgroundMode(Environment.BgSky)
	env.SetSky(s)
	env.SetAmbientLightSource(Environment.AmbientSourceSky)
	env.SetAmbientLightEnergy(0.9)
	env.SetTonemapMode(Environment.ToneMapperFilmic)
	env.SetFogEnabled(true)
	env.SetFogMode(Environment.FogModeDepth)
	env.SetFogLightColor(Color.RGBA{R: 0.72, G: 0.8, B: 0.9, A: 1})
	env.SetFogDepthBegin(160)
	env.SetFogDepthEnd(520)
	env.SetFogDensity(0.6)
	env.SetSsaoEnabled(true)
	we := WorldEnvironment.New()
	we.SetEnvironment(env)
	addChild(parent, we.AsNode())

	sun := DirectionalLight3D.New()
	sun.AsNode3D().SetRotationDegrees(Euler.Degrees{X: -52, Y: -35})
	sun.AsLight3D().SetShadowEnabled(true)
	sun.AsLight3D().SetLightEnergy(1.25)
	sun.AsLight3D().SetLightColor(Color.RGBA{R: 1, G: 0.96, B: 0.88, A: 1})
	sun.AsLight3D().SetShadowBlur(1.5)
	Light3D.Advanced(sun.AsLight3D()).SetParam(Light3D.ParamShadowMaxDistance, 140)
	addChild(parent, sun.AsNode())
}

// WorldView renders a town: static scenery plus the live bins.
type WorldView struct {
	root Node3D.Instance
	seed int64
	bins []binView

	bodies  map[sim.Colour]Mesh.Instance
	lids    map[sim.Colour]Mesh.Instance
	markers map[sim.Colour]Mesh.Instance
}

type binView struct {
	root, lid, marker Node3D.Instance
	lidAngle          float64
	grabFrom          Transform3D.BasisOrigin
	wasHeld           bool
}

func newWorldView(parent Node.Instance, town *sim.Town) *WorldView {
	w := &WorldView{
		root:    Node3D.New(),
		seed:    town.Seed,
		bodies:  map[sim.Colour]Mesh.Instance{},
		lids:    map[sim.Colour]Mesh.Instance{},
		markers: map[sim.Colour]Mesh.Instance{},
	}
	addChild(parent, w.root.AsNode())
	tm := meshgen.BuildTown(town)
	for _, m := range tm.Ground {
		addChild(w.root.AsNode(), meshNode(m, layerWorld, false).AsNode())
	}
	for _, m := range tm.Scenery {
		addChild(w.root.AsNode(), meshNode(m, layerWorld, true).AsNode())
	}
	for _, c := range sim.Colours {
		w.bodies[c] = Object.Leak(toArrayMesh(meshgen.BinBody(c)).AsMesh())
		w.lids[c] = Object.Leak(toArrayMesh(meshgen.BinLid(c)).AsMesh())
		w.markers[c] = Object.Leak(toArrayMesh(meshgen.Marker(meshgen.BinColours[c].Shade(1.15))).AsMesh())
	}
	w.bindBins(town)
	return w
}

// bindBins (re)creates the bin nodes for a town's bins.
func (w *WorldView) bindBins(town *sim.Town) {
	for _, b := range w.bins {
		b.root.AsNode().QueueFree()
		b.marker.AsNode().QueueFree()
	}
	w.bins = w.bins[:0]
	for _, b := range town.Bins {
		bv := binView{root: Node3D.New(), lid: Node3D.New()}
		addChild(w.root.AsNode(), bv.root.AsNode())
		addChild(bv.root.AsNode(), meshInstance(w.bodies[b.Colour], layerWorld, true).AsNode())
		addChild(bv.root.AsNode(), bv.lid.AsNode())
		bv.lid.SetPosition(Vector3.XYZ(meshgen.BinLidHinge))
		addChild(bv.lid.AsNode(), meshInstance(w.lids[b.Colour], layerWorld, true).AsNode())
		mk := meshInstance(w.markers[b.Colour], layerMarkers, false)
		mk.AsGeometryInstance3D().SetMaterialOverride(unshadedVertexMaterial)
		bv.marker = mk.AsNode3D()
		addChild(w.root.AsNode(), bv.marker.AsNode())
		w.bins = append(w.bins, bv)
	}
}

func (w *WorldView) free() { w.root.AsNode().QueueFree() }

// heldBinBasis orients a gripped bin: front towards the truck, rolled by tilt.
var heldBinBasis = Basis.XYZ{X: vec(0, 0, 1), Y: vec(0, 1, 0), Z: vec(-1, 0, 0)}

func (w *WorldView) sync(s *sim.Session, truck Transform3D.BasisOrigin, clock float64) {
	Object.Use(w.root)
	pose := s.Arm.Pose()
	for i := range w.bins {
		b := &s.Town.Bins[i]
		bv := &w.bins[i]
		kerb := groundXform(b.Pos, b.Heading, 0)
		target := 0.0
		switch {
		case b.Held:
			local := xform(Basis.Mul(rollBasis(-pose.Tilt), heldBinBasis), Vector3.XYZ(meshgen.TruckPoint(pose.Bin)))
			held := Transform3D.Mul(truck, local)
			if !bv.wasHeld {
				bv.grabFrom = kerb
			}
			if s.Arm.Phase == sim.ArmGrip {
				held = Transform3D.Lerp(bv.grabFrom, held, s.Arm.Progress())
			}
			bv.root.SetTransform(held)
			target = math.Min(2.1, math.Max(0, pose.Tilt-0.7)*1.6)
		case b.Collected:
			bv.root.SetTransform(kerb)
			target = 0.28
		default:
			bv.root.SetTransform(kerb)
		}
		bv.wasHeld = b.Held
		bv.lidAngle += (target - bv.lidAngle) * 0.25
		bv.lid.SetBasis(xBasis(bv.lidAngle))

		show := !b.Collected && !b.Held
		bv.marker.SetVisible(show)
		if show {
			scale := 1.0
			if b.Colour == s.Bonus {
				scale = 1.45 + 0.15*math.Sin(clock*6)
			}
			bob := 2.0 + 0.15*math.Sin(clock*2.6+float64(i))
			bs := Basis.Scaled(yBasis(clock*1.8+float64(i)), vec(scale, scale, scale))
			bv.marker.SetTransform(xform(bs, vec(b.Pos.X, bob, b.Pos.Y)))
		}
	}
}

// titleCamera orbits the suburb for the menu screen.
func titleCamera(cam Camera3D.Instance, town *sim.Town, clock float64) {
	c := town.Min.Lerp(town.Max, 0.5)
	a := clock * 0.05
	r := 0.38 * town.Max.Dist(town.Min)
	eye := vec(c.X+r*math.Cos(a), 95, c.Y+r*math.Sin(a))
	t := Transform3D.BasisOrigin{Basis: Basis.New(), Origin: eye}
	cam.AsNode3D().SetGlobalTransform(Transform3D.LookingAt(t, vec(c.X, 0, c.Y), Vector3.Up))
}
