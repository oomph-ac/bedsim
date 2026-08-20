package bedsim

import (
	"github.com/chewxy/math32"
	"testing"

	"github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/go-gl/mathgl/mgl32"
	movementblock "github.com/oomph-ac/bedsim/block"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

type fixedEffects map[int32]int32

func (e fixedEffects) GetEffect(effectID int32) (int32, bool) {
	amplifier, ok := e[effectID]
	return amplifier, ok
}

func TestJumpBoostUsesZeroBasedEffectAmplifier(t *testing.T) {
	sim := &Simulator{Effects: fixedEffects{packet.EffectJumpBoost: 0}}
	state := newBaseState()

	sim.applyInput(state, InputState{})

	if want := float32(0.52); math32.Abs(state.JumpHeight-want) > 1e-6 {
		t.Fatalf("expected jump boost I height %v, got %v", want, state.JumpHeight)
	}
}

func TestLevitationUsesZeroBasedEffectAmplifier(t *testing.T) {
	sim := &Simulator{World: mockWorld{}, Effects: fixedEffects{packet.EffectLevitation: 0}}
	state := newBaseState()
	state.HasGravity = false

	sim.SimulateState(state)

	if want := float32(0.01); math32.Abs(state.Vel.Y()-want) > 1e-6 {
		t.Fatalf("expected levitation I velocity %v, got %v", want, state.Vel.Y())
	}
}

func TestSlowFallingOnlyChangesGravityWhileDescending(t *testing.T) {
	sim := &Simulator{World: mockWorld{}, Effects: fixedEffects{packet.EffectSlowFalling: 0}}
	state := newBaseState()
	state.Vel = mgl32.Vec3{0, 0.2}
	state.Gravity = NormalGravity
	state.SlowFalling = true

	sim.SimulateState(state)

	if want := float32((0.2 - NormalGravity) * NormalGravityMultiplier); math32.Abs(state.Vel.Y()-want) > 1e-6 {
		t.Fatalf("expected normal gravity while ascending, want %v, got %v", want, state.Vel.Y())
	}
}

func TestBedrockStepHeight(t *testing.T) {
	if want := float32(0.5625); StepHeight != want {
		t.Fatalf("expected Bedrock step height %v, got %v", want, StepHeight)
	}
}

func TestBedBounceUsesVanillaRestitutionWithoutCap(t *testing.T) {
	sim := &Simulator{BlockSemantics: overrideBlockSemantics{semantics: movementblock.MovementSemantics{Bounce: movementblock.BounceBed}}}
	state := newBaseState()
	state.Vel = mgl32.Vec3{0, -2}

	sim.landOnBlock(state, state.Vel, block.Air{})

	if want := float32(1.5); math32.Abs(state.Vel.Y()-want) > 1e-6 {
		t.Fatalf("expected bed bounce %v, got %v", want, state.Vel.Y())
	}

	state.Vel = mgl32.Vec3{0, -1}
	sim.landOnBlock(state, state.Vel, block.Air{})
	if want := float32(0.75); math32.Abs(state.Vel.Y()-want) > 1e-6 {
		t.Fatalf("expected bed bounce %v, got %v", want, state.Vel.Y())
	}
}

func TestTinyVelocityIsNotDiscardedPrematurely(t *testing.T) {
	sim := &Simulator{World: mockWorld{}}
	state := newBaseState()
	state.HasGravity = false
	state.Vel = mgl32.Vec3{1e-7, 0, 0}

	sim.SimulateState(state)

	if state.Vel.X() == 0 {
		t.Fatal("expected Bedrock-scale tiny velocity to remain non-zero")
	}
}

func TestSlowFallingChangesGlideGravity(t *testing.T) {
	sim := &Simulator{World: mockWorld{}, Inventory: mockInventory{hasElytra: true}}
	state := newBaseState()
	state.Gliding = true
	state.Gravity = NormalGravity
	state.SlowFalling = true
	state.Vel[1] = -0.01

	sim.SimulateState(state)

	if want := float32(-0.011025); math32.Abs(state.Vel.Y()-want) > 1e-6 {
		t.Fatalf("expected slow-falling glide velocity %v, got %v", want, state.Vel.Y())
	}
}

func TestSneakEdgeProtectionRequiresGround(t *testing.T) {
	sim := &Simulator{World: staticWorld{chunkLoaded: true, boxes: []cube.BBox32{
		cube.Box32(-1, -1, -1, 0, 0, 1),
	}}}
	state := newBaseState()
	state.Sneaking = true
	state.OnGround = false
	state.FallDistance = 0.1
	state.Vel = mgl32.Vec3{0.5, 0, 0}

	sim.avoidEdge(state)

	if state.Vel.X() != 0.5 {
		t.Fatalf("edge protection ran while airborne: %v", state.Vel)
	}
}
