package main

import (
	"math"

	"graphics.gd/variant/Object"

	"graphics.gd/classdb/Camera3D"
	"graphics.gd/classdb/DirectionalLight3D"
	"graphics.gd/classdb/Environment"
	"graphics.gd/classdb/GeometryInstance3D"
	"graphics.gd/classdb/Light3D"
	"graphics.gd/classdb/Mesh"
	"graphics.gd/classdb/MultiMesh"
	"graphics.gd/classdb/MultiMeshInstance3D"
	"graphics.gd/classdb/Node"
	"graphics.gd/classdb/Node3D"
	"graphics.gd/classdb/RenderingServer"
	"graphics.gd/classdb/Shader"
	"graphics.gd/classdb/ShaderMaterial"
	"graphics.gd/classdb/Sky"
	"graphics.gd/classdb/Viewport"
	"graphics.gd/classdb/WorldEnvironment"
	"graphics.gd/variant/Basis"
	"graphics.gd/variant/Color"
	"graphics.gd/variant/Euler"
	"graphics.gd/variant/Transform3D"
	"graphics.gd/variant/Vector3"

	"github.com/jsok/garbage-truck-simulator/internal/meshgen"
	"github.com/jsok/garbage-truck-simulator/internal/sim"
)

// Quality selects how much rendering work the scene does.
type Quality int

const (
	QualityLow Quality = iota
	QualityHigh
)

func (q Quality) String() string {
	if q == QualityLow {
		return "LOW"
	}
	return "HIGH"
}

// Scene lighting that the quality setting can adjust later.
type lighting struct {
	env *Environment.Instance
	sun DirectionalLight3D.Instance
}

// addEnvironment sets up a sunny suburban morning.
func addEnvironment(parent Node.Instance) *lighting {
	sh := Shader.New()
	sh.SetCode(skyShader)
	skyMat := ShaderMaterial.New()
	skyMat.SetShader(sh)
	s := Sky.New()
	s.SetSkyMaterial(skyMat.AsMaterial())
	s.SetRadianceSize(Sky.RadianceSize256)

	env := Environment.New()
	env.SetBackgroundMode(Environment.BgSky)
	env.SetSky(s)
	env.SetAmbientLightSource(Environment.AmbientSourceSky)
	env.SetAmbientLightEnergy(0.38)
	env.SetReflectedLightSource(Environment.ReflectionSourceSky)
	env.SetTonemapMode(Environment.ToneMapperAgx)
	env.SetTonemapExposure(1.05)
	env.SetAdjustmentEnabled(true)
	env.SetAdjustmentSaturation(1.08)
	env.SetAdjustmentContrast(1.06)
	env.SetFogEnabled(true)
	env.SetFogMode(Environment.FogModeExponential)
	env.SetFogLightColor(Color.RGBA{R: 0.7, G: 0.78, B: 0.88, A: 1})
	env.SetFogDensity(0.0008)
	env.SetFogAerialPerspective(0.55)
	env.SetFogSunScatter(0.25)
	env.SetFogSkyAffect(0.25)
	env.SetSsaoEnabled(true)
	env.SetSsaoRadius(1.2)
	env.SetSsaoIntensity(1.6)
	env.SetGlowIntensity(0.45)
	env.SetGlowBloom(0.03)
	env.SetGlowHdrThreshold(1.1)
	env.SetGlowBlendMode(Environment.GlowBlendModeSoftlight)
	env.SetSsilRadius(3)
	env.SetSsilIntensity(0.8)
	we := WorldEnvironment.New()
	we.SetEnvironment(env)
	addChild(parent, we.AsNode())

	sun := DirectionalLight3D.New()
	sun.AsNode3D().SetRotationDegrees(Euler.Degrees{X: -42, Y: -35})
	l := sun.AsLight3D()
	l.SetShadowEnabled(true)
	l.SetLightEnergy(1.9)
	l.SetLightColor(Color.RGBA{R: 1, G: 0.94, B: 0.84, A: 1})
	l.SetShadowBlur(1.0)
	l.SetShadowNormalBias(1.2)
	sun.SetDirectionalShadowBlendSplits(true)
	Light3D.Advanced(l).SetParam(Light3D.ParamShadowMaxDistance, 180)
	addChild(parent, sun.AsNode())
	return &lighting{env: &env, sun: sun}
}

// apply switches the expensive effects on or off.
func (lt *lighting) apply(q Quality, vp Viewport.Instance, fork Viewport.Instance) {
	high := q == QualityHigh
	lt.env.SetSsilEnabled(high)
	lt.env.SetGlowEnabled(high)
	angular := 0.0
	msaa := Viewport.Msaa2x
	if high {
		angular = 0.6 // soft, contact-hardening shadows
		msaa = Viewport.Msaa4x
	}
	Light3D.Advanced(lt.sun.AsLight3D()).SetParam(Light3D.ParamSize, angular)
	vp.SetMsaa3d(msaa)
	fork.SetMsaa3d(Viewport.Msaa2x)
	size := 4096
	if high {
		size = 8192
	}
	RenderingServer.DirectionalShadowAtlasSetSize(size, true)
	filter := RenderingServer.ShadowQualitySoftLow
	if high {
		filter = RenderingServer.ShadowQualitySoftHigh
	}
	RenderingServer.DirectionalSoftShadowFilterSetQuality(filter)
}

// WorldView renders a town: static scenery plus the live bins.
type WorldView struct {
	root  Node3D.Instance
	seed  int64
	bins  []binView
	grass []Node3D.Instance

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
	addChild(w.root.AsNode(), meshNode(tm.Far, layerWorld, false).AsNode())
	w.addGrass(town)
	for _, c := range sim.Colours {
		w.bodies[c] = Object.Leak(toArrayMesh(meshgen.BinBody(c)).AsMesh())
		w.lids[c] = Object.Leak(toArrayMesh(meshgen.BinLid(c)).AsMesh())
		w.markers[c] = Object.Leak(toArrayMesh(meshgen.Marker(meshgen.BinColours[c].Shade(1.15))).AsMesh())
	}
	w.bindBins(town)
	return w
}

// addGrass scatters instanced grass tufts, one MultiMesh per chunk so that
// distant chunks are culled.
func (w *WorldView) addGrass(town *sim.Town) {
	tuft := toArrayMesh(meshgen.GrassTuft()).AsMesh()
	for key, tufts := range meshgen.GrassTufts(town) {
		cx := (float64(key[0]) + 0.5) * meshgen.GrassChunk
		cz := (float64(key[1]) + 0.5) * meshgen.GrassChunk
		buf := make([]float32, 0, 12*len(tufts))
		for _, t := range tufts {
			s, c := math.Sincos(float64(t.Yaw))
			k := float64(t.Scale)
			// Row-major 3x4: basis rows then origin.
			buf = append(buf,
				float32(c*k), 0, float32(s*k), float32(float64(t.X)-cx),
				0, float32(k*(0.8+0.4*s*s)), 0, 0,
				float32(-s*k), 0, float32(c*k), float32(float64(t.Z)-cz))
		}
		mm := MultiMesh.New()
		mm.SetTransformFormat(MultiMesh.Transform3d)
		mm.SetMesh(tuft)
		mm.SetInstanceCount(len(tufts))
		mm.SetBuffer(buf)
		mi := MultiMeshInstance3D.New()
		mi.SetMultimesh(mm)
		gi := mi.AsGeometryInstance3D()
		gi.SetMaterialOverride(vertexColourMaterial)
		gi.SetCastShadow(GeometryInstance3D.ShadowCastingSettingOff)
		gi.SetVisibilityRangeEnd(80)
		mi.AsVisualInstance3D().SetLayers(layerWorld)
		mi.AsNode3D().SetPosition(vec(cx, 0, cz))
		addChild(w.root.AsNode(), mi.AsNode())
		w.grass = append(w.grass, mi.AsNode3D())
	}
}

// showGrass turns the grass on or off.
func (w *WorldView) showGrass(on bool) {
	for _, g := range w.grass {
		g.SetVisible(on)
	}
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
	for _, g := range w.grass {
		Object.Use(g)
	}
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
