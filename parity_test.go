package bedsim

import (
	"math"
	"testing"

	"github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/go-gl/mathgl/mgl64"
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

	if want := 0.52; math.Abs(state.JumpHeight-want) > 1e-12 {
		t.Fatalf("expected jump boost I height %v, got %v", want, state.JumpHeight)
	}
}

func TestLevitationUsesZeroBasedEffectAmplifier(t *testing.T) {
	sim := &Simulator{World: mockWorld{}, Effects: fixedEffects{packet.EffectLevitation: 0}}
	state := newBaseState()
	state.HasGravity = false

	sim.SimulateState(state)

	if want := 0.01; math.Abs(state.Vel.Y()-want) > 1e-12 {
		t.Fatalf("expected levitation I velocity %v, got %v", want, state.Vel.Y())
	}
}

func TestSlowFallingOnlyChangesGravityWhileDescending(t *testing.T) {
	sim := &Simulator{World: mockWorld{}, Effects: fixedEffects{packet.EffectSlowFalling: 0}}
	state := newBaseState()
	state.Vel = mgl64.Vec3{0, 0.2}
	state.Gravity = NormalGravity
	state.SlowFalling = true

	sim.SimulateState(state)

	if want := (0.2 - NormalGravity) * NormalGravityMultiplier; math.Abs(state.Vel.Y()-want) > 1e-12 {
		t.Fatalf("expected normal gravity while ascending, want %v, got %v", want, state.Vel.Y())
	}
}

func TestBedrockStepHeight(t *testing.T) {
	if want := 0.5625; StepHeight != want {
		t.Fatalf("expected Bedrock step height %v, got %v", want, StepHeight)
	}
}

func TestBedBounceUsesBedrockRestitutionAndCap(t *testing.T) {
	sim := &Simulator{BlockSemantics: overrideBlockSemantics{name: "minecraft:bed"}}
	state := newBaseState()
	state.Vel = mgl64.Vec3{0, -2}

	sim.landOnBlock(state, state.Vel, block.Air{})

	if want := 0.75; state.Vel.Y() != want {
		t.Fatalf("expected bed bounce %v, got %v", want, state.Vel.Y())
	}
}

func TestTinyVelocityIsNotDiscardedPrematurely(t *testing.T) {
	sim := &Simulator{World: mockWorld{}}
	state := newBaseState()
	state.HasGravity = false
	state.Vel = mgl64.Vec3{1e-7, 0, 0}

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

	if want := -0.011025; math.Abs(state.Vel.Y()-want) > 1e-9 {
		t.Fatalf("expected slow-falling glide velocity %v, got %v", want, state.Vel.Y())
	}
}

func TestSneakEdgeProtectionWhileSlightlyAboveGround(t *testing.T) {
	sim := &Simulator{World: staticWorld{chunkLoaded: true, boxes: []cube.BBox{
		cube.Box(-1, -1, -1, 0, 0, 1),
	}}}
	state := newBaseState()
	state.Sneaking = true
	state.OnGround = false
	state.FallDistance = 0.1
	state.Vel = mgl64.Vec3{0.5, 0, 0}

	sim.avoidEdge(state)

	if state.Vel.X() >= 0.5 {
		t.Fatalf("expected edge protection above nearby ground, got velocity %v", state.Vel)
	}
}
