package bedsim

import (
	"github.com/chewxy/math32"
	"testing"

	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/go-gl/mathgl/mgl32"
	movementblock "github.com/oomph-ac/bedsim/block"
)

type encodedBlockSemantics struct{}

func (encodedBlockSemantics) BlockMovementSemantics(b world.Block) movementblock.MovementSemantics {
	return movementblock.Resolve(b, BlockName(b))
}

type namedOverrideSemantics struct {
	name      string
	semantics movementblock.MovementSemantics
}

func (s namedOverrideSemantics) BlockMovementSemantics(b world.Block) movementblock.MovementSemantics {
	if BlockName(b) == s.name {
		return s.semantics
	}
	return movementblock.Resolve(b, BlockName(b))
}

func TestInsideBlockMovementMultipliers(t *testing.T) {
	tests := []struct {
		name     string
		movement movementblock.InsideMovement
		want     mgl32.Vec3
	}{
		{name: "honey", movement: movementblock.InsideMovementHoney, want: mgl32.Vec3{0.4, -0.12, 0.4}},
		{name: "sweet berry bush", movement: movementblock.InsideMovementSweetBerryBush, want: mgl32.Vec3{0.8, -0.75, 0.8}},
		{name: "powder snow", movement: movementblock.InsideMovementPowderSnow, want: mgl32.Vec3{0.9, -1.5, 0.9}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := newBaseState()
			state.Vel = mgl32.Vec3{1, -1, 1}

			applyInsideBlockMovement(state, tt.movement)

			if state.Vel != tt.want {
				t.Fatalf("expected velocity %v, got %v", tt.want, state.Vel)
			}
		})
	}
}

func TestHoneyBlockReducesJumpPower(t *testing.T) {
	sim := &Simulator{
		World: environmentWorld{blocks: map[cube.Pos]world.Block{
			{0, 0, 0}: semanticsNamedBlock{"minecraft:honey_block"},
		}},
	}
	state := newBaseState()
	state.OnGround = true
	state.Jumping = true
	state.JumpHeight = DefaultJumpHeight

	if !sim.attemptJump(state, nil) {
		t.Fatal("expected jump to be applied")
	}
	if want := float32(DefaultJumpHeight * 0.6); math32.Abs(state.Vel.Y()-want) > 1e-6 {
		t.Fatalf("expected honey jump velocity %v, got %v", want, state.Vel.Y())
	}
}

func TestScaffoldingAscendAndDescendSpeeds(t *testing.T) {
	state := newBaseState()
	state.PressingAscend = true
	applyAscendableMovement(state, movementblock.TraversalScaffolding, false)
	if state.Vel.Y() != 0.15 {
		t.Fatalf("expected scaffolding ascend velocity 0.15, got %v", state.Vel.Y())
	}

	state.PressingAscend = false
	state.PressingDescend = true
	applyAscendableMovement(state, movementblock.TraversalScaffolding, false)
	if state.Vel.Y() != -0.15 {
		t.Fatalf("expected scaffolding descend velocity -0.15, got %v", state.Vel.Y())
	}
}

func TestPowderSnowTraversalRequiresLeatherBoots(t *testing.T) {
	state := newBaseState()
	state.PressingAscend = true
	applyAscendableMovement(state, movementblock.TraversalPowderSnow, false)
	if state.Vel.Y() != 0 {
		t.Fatalf("expected no powder-snow ascent without leather boots, got %v", state.Vel.Y())
	}

	applyAscendableMovement(state, movementblock.TraversalPowderSnow, true)
	if state.Vel.Y() != 0.2 {
		t.Fatalf("expected leather-boots powder-snow ascent 0.2, got %v", state.Vel.Y())
	}
}

func TestHoneyWalkSlowdownMatchesSlime(t *testing.T) {
	sim := &Simulator{}
	state := newBaseState()
	state.OnGround = true
	state.Vel = mgl32.Vec3{1, 0.05, 1}

	sim.walkOnBlock(state, semanticsNamedBlock{"minecraft:honey_block"})

	if want := float32(0.41); math32.Abs(state.Vel.X()-want) > 1e-6 || math32.Abs(state.Vel.Z()-want) > 1e-6 {
		t.Fatalf("expected honey walk slowdown %v, got %v", want, state.Vel)
	}
}

func TestSimulationAppliesInsideBlockMovementEffect(t *testing.T) {
	w := environmentWorld{blocks: map[cube.Pos]world.Block{
		{0, 0, 0}: semanticsNamedBlock{name: "custom:sticky"},
	}}
	sim := &Simulator{
		World: w,
		BlockSemantics: namedOverrideSemantics{name: "custom:sticky", semantics: movementblock.MovementSemantics{
			GroundFriction:                       DefaultBlockFriction,
			GroundAccelerationFrictionMultiplier: 1,
			InsideMovement:                       movementblock.InsideMovementHoney,
		}},
	}
	state := newBaseState()
	state.Pos = mgl32.Vec3{0.5, 0, 0.5}
	state.Vel = mgl32.Vec3{0.1, 0, 0}
	state.HasGravity = false

	sim.SimulateState(state)

	if want := float32(0.1 * DefaultAirFriction * 0.4); math32.Abs(state.Vel.X()-want) > 1e-6 {
		t.Fatalf("expected integrated honey slowdown %v, got %v", want, state.Vel.X())
	}
}

func TestSimulationAppliesScaffoldingTraversal(t *testing.T) {
	w := environmentWorld{blocks: map[cube.Pos]world.Block{
		{0, 0, 0}: semanticsNamedBlock{name: "minecraft:scaffolding"},
	}}
	sim := &Simulator{World: w, BlockSemantics: encodedBlockSemantics{}}
	state := newBaseState()
	state.Pos = mgl32.Vec3{0.5, 0, 0.5}
	state.HasGravity = false

	sim.Simulate(state, InputState{AscendBlock: true})

	if state.Vel.Y() != 0.15 {
		t.Fatalf("expected integrated scaffolding ascent 0.15, got %v", state.Vel.Y())
	}
}

func TestSimulationDetectsNonSolidWebAndAppliesWeaving(t *testing.T) {
	w := environmentWorld{blocks: map[cube.Pos]world.Block{
		{0, 0, 0}: semanticsNamedBlock{name: "minecraft:web"},
	}}
	sim := &Simulator{
		World:          w,
		BlockSemantics: encodedBlockSemantics{},
		Effects:        fixedEffects{EffectWeaving: 0},
	}
	state := newBaseState()
	state.Pos = mgl32.Vec3{0.5, 0, 0.5}
	state.Vel = mgl32.Vec3{0.1, 0, 0}
	state.HasGravity = false

	result := sim.SimulateState(state)

	if want := float32(0.05); math32.Abs(result.Movement.X()-want) > 1e-6 {
		t.Fatalf("expected Weaving web movement %v, got %v", want, result.Movement.X())
	}
}
