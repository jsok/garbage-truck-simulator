// Bin Day is a garbage truck simulator: drive a side-loader around a
// procedurally generated suburb and empty as many wheelie bins as you can
// before the shift ends, lining each one up with the fork camera screen on
// the dashboard.
package main

import (
	"graphics.gd/classdb"
	"graphics.gd/classdb/SceneTree"
	"graphics.gd/startup"
)

func main() {
	classdb.Register[Game]()
	classdb.Register[HUD]()
	classdb.Register[Minimap]()
	classdb.Register[ForkOverlay]()
	startup.LoadingScene()
	SceneTree.Add(new(Game))
	startup.Scene()
}
