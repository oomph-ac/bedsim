package bedsim

import (
	"math"
	"testing"

	"github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/go-gl/mathgl/mgl64"
)

type namedBlock struct {
	block.Air
	name string
}

func (b namedBlock) EncodeBlock() (string, map[string]any) { return b.name, nil }

type encodedBlockSemantics struct{}

func (encodedBlockSemantics) BlockName(b world.Block) string {
	name, _ := b.EncodeBlock()
	return name
}
func (encodedBlockSemantics) BlockFriction(world.Block) float64 { return DefaultBlockFriction }
func (encodedBlockSemantics) BlockClimbable(world.Block) bool   { return false }

func TestInsideBlockMovementMultipliers(t *testing.T) {
	tests := []struct {
		name      string
		blockName string
		want      mgl64.Vec3
	}{
		{name: "honey", blockName: "minecraft:honey_block", want: mgl64.Vec3{0.4, -0.12, 0.4}},
		{name: "sweet berry bush", blockName: "minecraft:sweet_berry_bush", want: mgl64.Vec3{0.8, -0.75, 0.8}},
		{name: "powder snow", blockName: "minecraft:powder_snow", want: mgl64.Vec3{0.9, -1.5, 0.9}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := newBaseState()
			state.Vel = mgl64.Vec3{1, -1, 1}

			applyInsideBlockMovement(state, tt.blockName)

			if state.Vel != tt.want {
				t.Fatalf("expected velocity %v, got %v", tt.want, state.Vel)
			}
		})
	}
}

func TestHoneyBlockReducesJumpPower(t *testing.T) {
	sim := &Simulator{
		World:          mockWorld{},
		BlockSemantics: overrideBlockSemantics{name: "minecraft:honey_block"},
	}
	state := newBaseState()
	state.OnGround = true
	state.Jumping = true
	state.JumpHeight = DefaultJumpHeight

	if !sim.attemptJump(state, nil) {
		t.Fatal("expected jump to be applied")
	}
	if want := DefaultJumpHeight * 0.6; math.Abs(state.Vel.Y()-want) > 1e-12 {
		t.Fatalf("expected honey jump velocity %v, got %v", want, state.Vel.Y())
	}
}

func TestScaffoldingAscendAndDescendSpeeds(t *testing.T) {
	state := newBaseState()
	state.PressingAscend = true
	applyAscendableMovement(state, "minecraft:scaffolding", false)
	if state.Vel.Y() != 0.15 {
		t.Fatalf("expected scaffolding ascend velocity 0.15, got %v", state.Vel.Y())
	}

	state.PressingAscend = false
	state.PressingDescend = true
	applyAscendableMovement(state, "minecraft:scaffolding", false)
	if state.Vel.Y() != -0.15 {
		t.Fatalf("expected scaffolding descend velocity -0.15, got %v", state.Vel.Y())
	}
}

func TestPowderSnowTraversalRequiresLeatherBoots(t *testing.T) {
	state := newBaseState()
	state.PressingAscend = true
	applyAscendableMovement(state, "minecraft:powder_snow", false)
	if state.Vel.Y() != 0 {
		t.Fatalf("expected no powder-snow ascent without leather boots, got %v", state.Vel.Y())
	}

	applyAscendableMovement(state, "minecraft:powder_snow", true)
	if state.Vel.Y() != 0.2 {
		t.Fatalf("expected leather-boots powder-snow ascent 0.2, got %v", state.Vel.Y())
	}
}

func TestHoneyWalkSlowdownMatchesSlime(t *testing.T) {
	sim := &Simulator{BlockSemantics: overrideBlockSemantics{name: "minecraft:honey_block"}}
	state := newBaseState()
	state.OnGround = true
	state.Vel = mgl64.Vec3{1, 0.05, 1}

	sim.walkOnBlock(state, block.Air{})

	if want := 0.41; math.Abs(state.Vel.X()-want) > 1e-12 || math.Abs(state.Vel.Z()-want) > 1e-12 {
		t.Fatalf("expected honey walk slowdown %v, got %v", want, state.Vel)
	}
}

func TestSimulationAppliesInsideBlockMovementEffect(t *testing.T) {
	w := environmentWorld{blocks: map[cube.Pos]world.Block{
		{0, 0, 0}: namedBlock{name: "minecraft:honey_block"},
	}}
	sim := &Simulator{World: w, BlockSemantics: encodedBlockSemantics{}}
	state := newBaseState()
	state.Pos = mgl64.Vec3{0.5, 0, 0.5}
	state.Vel = mgl64.Vec3{0.1, 0, 0}
	state.HasGravity = false

	sim.SimulateState(state)

	if want := 0.1 * DefaultAirFriction * 0.4; math.Abs(state.Vel.X()-want) > 1e-12 {
		t.Fatalf("expected integrated honey slowdown %v, got %v", want, state.Vel.X())
	}
}

func TestSimulationAppliesScaffoldingTraversal(t *testing.T) {
	w := environmentWorld{blocks: map[cube.Pos]world.Block{
		{0, 0, 0}: namedBlock{name: "minecraft:scaffolding"},
	}}
	sim := &Simulator{World: w, BlockSemantics: encodedBlockSemantics{}}
	state := newBaseState()
	state.Pos = mgl64.Vec3{0.5, 0, 0.5}
	state.HasGravity = false

	sim.Simulate(state, InputState{AscendBlock: true})

	if state.Vel.Y() != 0.15 {
		t.Fatalf("expected integrated scaffolding ascent 0.15, got %v", state.Vel.Y())
	}
}

func TestSimulationDetectsNonSolidWebAndAppliesWeaving(t *testing.T) {
	w := environmentWorld{blocks: map[cube.Pos]world.Block{
		{0, 0, 0}: namedBlock{name: "minecraft:web"},
	}}
	sim := &Simulator{
		World:          w,
		BlockSemantics: encodedBlockSemantics{},
		Effects:        fixedEffects{EffectWeaving: 0},
	}
	state := newBaseState()
	state.Pos = mgl64.Vec3{0.5, 0, 0.5}
	state.Vel = mgl64.Vec3{0.1, 0, 0}
	state.HasGravity = false

	result := sim.SimulateState(state)

	if want := 0.05; math.Abs(result.Movement.X()-want) > 1e-12 {
		t.Fatalf("expected Weaving web movement %v, got %v", want, result.Movement.X())
	}
}
