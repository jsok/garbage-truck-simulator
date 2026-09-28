package main

import (
	"math"

	"graphics.gd/variant/Object"

	"graphics.gd/classdb/ArrayMesh"
	"graphics.gd/classdb/BaseMaterial3D"
	"graphics.gd/classdb/GeometryInstance3D"
	"graphics.gd/classdb/Material"
	"graphics.gd/classdb/Mesh"
	"graphics.gd/classdb/MeshInstance3D"
	"graphics.gd/classdb/Node"
	"graphics.gd/classdb/Node3D"
	"graphics.gd/classdb/StandardMaterial3D"
	"graphics.gd/variant/Basis"
	"graphics.gd/variant/Color"
	"graphics.gd/variant/Float"
	"graphics.gd/variant/Transform3D"
	"graphics.gd/variant/Vector3"

	"github.com/jsok/garbage-truck-simulator/internal/meshgen"
	"github.com/jsok/garbage-truck-simulator/internal/sim"
)

// Render layers. The cab interior is only drawn for the driver, the truck's
// exterior only for the fork camera, and bin markers only for the driver.
const (
	layerWorld   = 1 << 0
	layerCab     = 1 << 1
	layerTruck   = 1 << 2
	layerMarkers = 1 << 3
)

func toArrayMesh(m *meshgen.Mesh) ArrayMesh.Instance {
	am := ArrayMesh.New()
	if m.Empty() {
		return am
	}
	verts := make([]Vector3.XYZ, len(m.P))
	norms := make([]Vector3.XYZ, len(m.N))
	cols := make([]Color.RGBA, len(m.C))
	for i := range m.P {
		verts[i] = Vector3.XYZ(m.P[i])
		norms[i] = Vector3.XYZ(m.N[i])
		cols[i] = Color.RGBA(m.C[i])
	}
	arrays := make([]any, Mesh.ArrayMax)
	arrays[Mesh.ArrayVertex] = verts
	arrays[Mesh.ArrayNormal] = norms
	arrays[Mesh.ArrayColor] = cols
	am.AddSurfaceFromArrays(Mesh.PrimitiveTriangles, arrays)
	return am
}

var vertexColourMaterial, unshadedVertexMaterial Material.Instance

func materials() {
	if vertexColourMaterial != Material.Nil {
		return
	}
	m := StandardMaterial3D.New()
	m.AsBaseMaterial3D().SetVertexColorUseAsAlbedo(true)
	m.AsBaseMaterial3D().SetRoughness(0.85)
	m.AsBaseMaterial3D().SetVertexColorIsSrgb(true)
	vertexColourMaterial = Object.Leak(m.AsMaterial())

	u := StandardMaterial3D.New()
	u.AsBaseMaterial3D().SetVertexColorUseAsAlbedo(true)
	u.AsBaseMaterial3D().SetShadingMode(BaseMaterial3D.ShadingModeUnshaded)
	u.AsBaseMaterial3D().SetVertexColorIsSrgb(true)
	unshadedVertexMaterial = Object.Leak(u.AsMaterial())
}

// meshNode wraps generated geometry in a MeshInstance3D on the given layers.
func meshNode(m *meshgen.Mesh, layers int, shadows bool) MeshInstance3D.Instance {
	return meshInstance(toArrayMesh(m).AsMesh(), layers, shadows)
}

func meshInstance(mesh Mesh.Instance, layers int, shadows bool) MeshInstance3D.Instance {
	materials()
	mi := MeshInstance3D.New()
	mi.SetMesh(mesh)
	gi := mi.AsGeometryInstance3D()
	gi.SetMaterialOverride(vertexColourMaterial)
	if !shadows {
		gi.SetCastShadow(GeometryInstance3D.ShadowCastingSettingOff)
	}
	mi.AsVisualInstance3D().SetLayers(layers)
	return mi
}

func addChild(parent Node.Instance, child Node.Instance) { parent.AddChild(child) }

func vec(x, y, z float64) Vector3.XYZ { return Vector3.New(x, y, z) }

// yawBasis rotates local -Z to face sim heading h.
func yawBasis(h float64) Basis.XYZ {
	th := -h - math.Pi/2
	s, c := math.Sincos(th)
	return Basis.XYZ{
		X: vec(c, 0, -s),
		Y: vec(0, 1, 0),
		Z: vec(s, 0, c),
	}
}

// groundXform places something at sim position p, facing heading h.
func groundXform(p sim.V2, h float64, y float64) Transform3D.BasisOrigin {
	return Transform3D.BasisOrigin{Basis: yawBasis(h), Origin: vec(p.X, y, p.Y)}
}

// rollBasis rotates about local Z by angle a.
func rollBasis(a float64) Basis.XYZ {
	s, c := math.Sincos(a)
	return Basis.XYZ{X: vec(c, s, 0), Y: vec(-s, c, 0), Z: vec(0, 0, 1)}
}

// yBasis rotates about local Y by angle a.
func yBasis(a float64) Basis.XYZ {
	s, c := math.Sincos(a)
	return Basis.XYZ{X: vec(c, 0, -s), Y: vec(0, 1, 0), Z: vec(s, 0, c)}
}

// xBasis rotates about local X by angle a.
func xBasis(a float64) Basis.XYZ {
	s, c := math.Sincos(a)
	return Basis.XYZ{X: vec(1, 0, 0), Y: vec(0, c, s), Z: vec(0, -s, c)}
}

func xform(b Basis.XYZ, o Vector3.XYZ) Transform3D.BasisOrigin {
	return Transform3D.BasisOrigin{Basis: b, Origin: o}
}

func setXform(n Node3D.Instance, t Transform3D.BasisOrigin) { n.SetTransform(t) }

func f32(x float64) Float.X { return Float.X(x) }
