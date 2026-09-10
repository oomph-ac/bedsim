package bedsim

import (
	"testing"

	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/go-gl/mathgl/mgl32"
)

// TestBoundingBoxRoundedWallSlide preserves the swept face through center rounding.
func TestBoundingBoxRoundedWallSlide(t *testing.T) {
	state := newBaseState()
	state.Pos = mgl32.Vec3{256.5, 1, 0.5}
	state.Client.Pos = state.Pos
	state.OnGround = true
	// Rebuilding a full-width float32 box at x=256.3 puts its minimum at
	// 255.9999847. That rounding must not turn a parallel wall slide into
	// a horizontal collision or mark the player as stuck inside the wall.
	sim := Simulator{World: staticWorld{chunkLoaded: true, boxes: []cube.BBox32{
		cube.Box32(255, 0, -10, 258, 1, 10),
		cube.Box32(255, 0, -10, 256, 4, 10),
	}}}
	// Approach the wall through the same sweep that creates vanilla's AABB.
	state.Vel = mgl32.Vec3{-.3, -.0784, 0}
	if !sim.tryCollisions(state) {
		t.Fatal("approach unavailable")
	}
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

func TestBoundingBox_VanillaWallContact(t *testing.T) {
	for _, tc := range []struct {
		name     string
		pos      mgl32.Vec3
		wall     cube.BBox32
		velocity mgl32.Vec3
		axis     int
		want     float32
	}{
		{"captured climb", mgl32.Vec3{186.61009, -59.580002, 4.5}, cube.Box32(187, -61, 3, 188, -59, 6), mgl32.Vec3{.28, 0, 0}, 0, 186.7},
		{"captured wall", mgl32.Vec3{26.5, -59, 5.611176}, cube.Box32(24, -60, 6, 29, -58, 7), mgl32.Vec3{0, 0, .2584}, 2, 5.7},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := newBaseState()
			state.Pos, state.Client.Pos = tc.pos, tc.pos
			for _, box := range []cube.BBox32{state.BoundingBox(false), state.ClientBoundingBox(false)} {
				clipped := BBClipCollide(tc.wall, box, tc.velocity, false, nil)
				moved := box.Translate(clipped)
				center := moved.Min().Add(moved.Max()).Mul(.5)
				if center[tc.axis] != tc.want {
					t.Fatalf("contact = %.9g, captured BDS position %.9g", center[tc.axis], tc.want)
				}
			}
		})
	}
}

func TestBBClipCollide_ContactEpsilonDoesNotShrinkSweeps(t *testing.T) {
	wall := cube.Box32(-1, -1, -1, 0, 3, 3)
	for _, tc := range []struct {
		gap     float32
		blocked bool
	}{{-5e-7, false}, {-2e-6, true}, {5e-7, false}} {
		box := cube.Box32(tc.gap, 0, 0, .6+tc.gap, 1.8, .6)
		penetration := mgl32.Vec3{}
		velocity := BBClipCollide(wall, box, mgl32.Vec3{0, 0, .1}, false, &penetration)
		if (penetration.X() > 0) != tc.blocked || (velocity.X() > 0) != tc.blocked {
			t.Fatalf("gap %g: penetration %v, velocity %v", tc.gap, penetration, velocity)
		}
	}
}

func TestBoundingBox_RetainedShapeInvalidatesForExternalChanges(t *testing.T) {
	state := newBaseState()
	state.Pos = mgl32.Vec3{256.3, 1, .5}
	box := cube.Box32(256, 1, .2, 256.6, 2.8, .8)
	state.rememberCollisionBox(box, false)
	clone := state.Clone()
	if state.BoundingBox(false) != box {
		t.Fatal("lost exact swept endpoints")
	}
	state.SetPos(mgl32.Vec3{10, 2, 3})
	if state.BoundingBox(false).Min().X() != float32(9.7) {
		t.Fatal("retained old position")
	}
	if clone.BoundingBox(false) != box {
		t.Fatal("clone shared mutable geometry")
	}
	clone.Size[0] = 1
	if clone.BoundingBox(false).Min().X() != clone.Pos.X()-.5 {
		t.Fatal("retained old width")
	}
	clone = state.Clone()
	clone.rememberCollisionBox(clone.BoundingBox(false), false)
	clone.Crawling, clone.Size[1] = true, .6
	if clone.BoundingBox(false).Max().Y() != float32(2.6) {
		t.Fatal("retained standing height")
	}
}
