package bedsim

import (
	"testing"

	"github.com/chewxy/math32"
	"github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

func TestZeroValueStateHasNoSyntheticEvents(t *testing.T) {
	var state MovementState
	if state.HasKnockback() {
		t.Fatal("zero state must not report knockback")
	}
	if state.HasTeleport() {
		t.Fatal("zero state must not report teleport")
	}
}

func TestSimulationRejectsNonFiniteInputAndState(t *testing.T) {
	state := newBaseState()
	state.Pos = mgl32.Vec3{1, 2, 3}
	result := (&Simulator{World: mockWorld{}}).Simulate(state, InputState{Pitch: math32.NaN()})
	if result.Outcome != SimulationOutcomeInvalidInput || !result.NeedsCorrection {
		t.Fatalf("invalid input result = %+v", result)
	}
	if state.Pos != (mgl32.Vec3{1, 2, 3}) {
		t.Fatalf("invalid input mutated state position to %v", state.Pos)
	}

	state = newBaseState()
	state.Vel[0] = math32.Inf(1)
	result = (&Simulator{World: mockWorld{}}).SimulateState(state)
	if result.Outcome != SimulationOutcomeInvalidInput || !result.NeedsCorrection {
		t.Fatalf("invalid state result = %+v", result)
	}
}

func TestMountedStateSkipsMovement(t *testing.T) {
	state := newBaseState()
	state.InVehicle = true
	state.Pos = mgl32.Vec3{10, 70, 10}
	state.Vel = mgl32.Vec3{1, 2, 3}
	state.Client.Pos = mgl32.Vec3{4, 5, 6}
	state.Client.Vel = mgl32.Vec3{0.1, 0.2, 0.3}

	result := (&Simulator{World: mockWorld{}}).SimulateState(state)
	if result.Outcome != SimulationOutcomeMounted {
		t.Fatalf("outcome = %v, want mounted", result.Outcome)
	}
	if state.Pos != state.Client.Pos || state.Vel != state.Client.Vel {
		t.Fatalf("mounted state was simulated: pos=%v vel=%v", state.Pos, state.Vel)
	}
}

func TestSimulateStateLeavesTransientInputForCaller(t *testing.T) {
	state := newBaseState()
	state.RiptideReady = true
	(&Simulator{World: mockWorld{}}).SimulateState(state)
	if !state.RiptideReady {
		t.Fatal("SimulateState must not consume caller-managed transient input")
	}
}

func TestActiveRiptideAppliesImpulseWithoutOrdinaryPhysics(t *testing.T) {
	state := newBaseState()
	state.RiptideTicks = 5
	state.Vel = mgl32.Vec3{0, 0.8, 0}
	state.Impulse = mgl32.Vec2{0, 1}
	state.Gravity = NormalGravity

	(&Simulator{World: mockWorld{}, Equipment: fixedEquipment{EnchantmentRiptide: 2}}).SimulateState(state)
	if math32.Abs(state.Pos.Z()-2.25) > 1e-6 {
		t.Fatalf("riptide tick did not apply directional displacement: %v", state.Pos)
	}
	if math32.Abs(state.Vel.Y()-0.8) > 1e-6 || math32.Abs(state.Vel.Z()-2.25) > 1e-6 {
		t.Fatalf("riptide tick applied ordinary acceleration: %v", state.Vel)
	}
}

func TestMovementSpeedUsesEffectiveAttribute(t *testing.T) {
	withoutEffect := newBaseState()
	withoutEffect.MovementSpeed = 0.12
	withoutEffect.DefaultMovementSpeed = 0.12
	withoutEffect.Impulse = mgl32.Vec2{0, 1}

	withEffect := *withoutEffect

	base := (&Simulator{World: mockWorld{}}).SimulateState(withoutEffect)
	withSpeedEffect := (&Simulator{World: mockWorld{}, Effects: fixedEffects{packet.EffectSpeed: 0}}).SimulateState(&withEffect)
	if base.Velocity != withSpeedEffect.Velocity {
		t.Fatalf("effective movement speed was modified by a second effect pass: base=%v with_effect=%v", base.Velocity, withSpeedEffect.Velocity)
	}
}

func TestTeleportDoesNotApplyJumpImpulse(t *testing.T) {
	state := newBaseState()
	state.OnGround = true
	state.Jumping = true
	state.QueueTeleport(mgl32.Vec3{10, 20, 30}, false, 0)

	result := (&Simulator{World: mockWorld{}}).SimulateState(state)
	if result.Outcome != SimulationOutcomeTeleport {
		t.Fatalf("outcome = %v, want teleport", result.Outcome)
	}
	if state.Vel != (mgl32.Vec3{}) {
		t.Fatalf("teleport applied jump/other velocity: %v", state.Vel)
	}
	if state.HasTeleport() {
		t.Fatal("completed hard teleport remained active")
	}
}

func TestGlideAtVerticalPitchRemainsFinite(t *testing.T) {
	state := newBaseState()
	state.Gliding = true
	state.OnGround = false
	state.Rotation = mgl32.Vec3{-90, 0, 0}
	state.Vel = mgl32.Vec3{1, 0, 0}

	(&Simulator{World: mockWorld{}, Inventory: mockInventory{hasElytra: true}}).SimulateState(state)
	for axis, value := range state.Vel {
		if !finiteFloat(value) {
			t.Fatalf("glide velocity axis %d is not finite: %v", axis, state.Vel)
		}
	}
}

func TestShallowLiquidBelowPlayerIsNotContact(t *testing.T) {
	w := newLiquidWorld().set(cube.Pos{0, 0, 0}, block.Water{Depth: 0, Still: true})
	sim := newLiquidSim(w)
	state := submergedState()

	if got := len(sim.touchingLiquidBlocks(state, liquidWater)); got != 0 {
		t.Fatalf("shallow liquid blocks = %d, want no contact above its surface", got)
	}
}

func TestMovementChecksSweptChunks(t *testing.T) {
	state := newBaseState()
	state.Pos = mgl32.Vec3{15.5, 0, 0.5}
	state.Vel = mgl32.Vec3{1, 0, 0}

	result := (&Simulator{World: selectiveChunkWorld{}}).SimulateState(state)
	if result.Outcome != SimulationOutcomeUnloadedChunk {
		t.Fatalf("outcome = %v, want unloaded chunk for swept movement", result.Outcome)
	}
}

func TestUnloadedTickDoesNotCommitPoseChanges(t *testing.T) {
	state := newBaseState()
	state.Pos = mgl32.Vec3{16.5, 0, 0.5}
	state.Swimming = true
	state.Size[1] = state.StandingHeight
	originalSize := state.Size

	result := (&Simulator{World: selectiveChunkWorld{}}).Simulate(state, InputState{StopSwimming: true})
	if result.Outcome != SimulationOutcomeUnloadedChunk {
		t.Fatalf("outcome = %v, want unloaded chunk", result.Outcome)
	}
	if !state.Swimming || state.Size != originalSize {
		t.Fatalf("unloaded tick committed pose change: swimming=%v size=%v", state.Swimming, state.Size)
	}
}

func TestAdjacentClimbableContactIsDetected(t *testing.T) {
	w := environmentWorld{blocks: map[cube.Pos]world.Block{
		{1, 0, 0}: block.Ladder{Facing: cube.West},
	}}
	state := newBaseState()
	state.Pos = mgl32.Vec3{0.8, 0, 0.5}
	state.Client.Pos = state.Pos
	state.EffectiveJumping = true
	state.Gravity = NormalGravity

	sim := &Simulator{World: w}
	sim.SimulateState(state)
	if state.Vel.Y() <= 0 {
		t.Fatalf("adjacent ladder did not provide climb velocity: %v", state.Vel)
	}
}

type selectiveChunkWorld struct{}

func (selectiveChunkWorld) Block(cube.Pos) world.Block { return block.Air{} }

func (selectiveChunkWorld) BlockCollisions(cube.Pos) []cube.BBox32 { return nil }

func (selectiveChunkWorld) GetNearbyBBoxes(cube.BBox32) []cube.BBox32 { return nil }

func (selectiveChunkWorld) IsChunkLoaded(chunkX, chunkZ int32) bool {
	return chunkX == 0 && chunkZ == 0
}
