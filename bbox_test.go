package bedsim

import (
	"testing"

	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/go-gl/mathgl/mgl32"
)

// TestBoundingBoxRoundedWallSlide preserves tolerance for rounded client contact positions.
func TestBoundingBoxRoundedWallSlide(t *testing.T) {
	state := newBaseState()
	state.Pos = mgl32.Vec3{256.3, 1, 0.5}
	state.Client.Pos = state.Pos
	state.OnGround = true
	// Rebuilding a full-width float32 box at x=256.3 puts its minimum at
	// 255.9999847. That rounding must not turn a parallel wall slide into
	// a horizontal collision or mark the player as stuck inside the wall.
	sim := Simulator{World: staticWorld{chunkLoaded: true, boxes: []cube.BBox32{
		cube.Box32(255, 0, -10, 258, 1, 10),
		cube.Box32(255, 0, -10, 256, 4, 10),
	}}}
	for tick := 0; tick < 3; tick++ {
		state.Vel = mgl32.Vec3{0, -0.0784, 0.1}
		previousZ := state.Pos.Z()
		if !sim.tryCollisions(state) {
			t.Fatal("collision simulation could not complete")
		}
		if state.CollideX || state.CollideZ || state.PenetratedLastFrame || state.StuckInCollider {
			t.Errorf("tick %d: wall slide acquired collision or penetration: x=%v z=%v penetrated=%v stuck=%v",
				tick, state.CollideX, state.CollideZ, state.PenetratedLastFrame, state.StuckInCollider)
		}
		if !state.OnGround || state.Pos.Z() <= previousZ {
			t.Errorf("tick %d: wall slide stopped moving along the floor: pos=%v grounded=%v", tick, state.Pos, state.OnGround)
		}
	}
}
