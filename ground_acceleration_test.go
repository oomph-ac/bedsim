package bedsim

import (
	"testing"

	"github.com/df-mc/dragonfly/server/block"
	"github.com/go-gl/mathgl/mgl32"
)

func TestSimulator_VanillaGroundAcceleration(t *testing.T) {
	// Stationary-to-forward PAI velocity from the unmodified 1.26.45.1
	// client walking on grass. These are post-physics velocities, not the
	// larger position displacements. Lens 1.26.50.26 RVA 0x70d9630 confirms
	// speed * ratio * ratio * ratio with ratio = 0.546000063419342 / friction.
	want := []float32{0.05350801, 0.08272339, 0.09867499, 0.10738456, 0.112139985}
	state := newBaseState()
	state.Pos = mgl32.Vec3{0.5, 1, 0.5}
	state.OnGround = true
	state.HasGravity = false
	sim := Simulator{World: blockMovementWorld{b: block.Grass{}}}
	for tick, velocity := range want {
		result := sim.Simulate(state, InputState{MoveVector: mgl32.Vec2{0, 1}})
		if result.Velocity.Z() != velocity {
			t.Fatalf("tick %d: velocity %.9g, vanilla %.9g", tick+1, result.Velocity.Z(), velocity)
		}
	}
}

func TestSimulator_VanillaSoulSandAcceleration(t *testing.T) {
	state := newBaseState()
	state.Pos = mgl32.Vec3{0.5, 0.875, 0.5}
	state.OnGround = true
	state.HasGravity = false
	sim := Simulator{World: blockMovementWorld{b: block.SoulSand{}}}
	result := sim.Simulate(state, InputState{MoveVector: mgl32.Vec2{0, 1}})
	// Real BDS response and Lens 1.26.50.26 multiply the soul-sand
	// factor into block friction before applying air friction.
	if result.Velocity.Z() != float32(0.029107885) {
		t.Fatalf("soul sand velocity %.9g, want 0.029107885", result.Velocity.Z())
	}
}
