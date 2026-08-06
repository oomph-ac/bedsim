package bedsim

import (
	"testing"

	"github.com/chewxy/math32"
	"github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/go-gl/mathgl/mgl32"
	movementblock "github.com/oomph-ac/bedsim/block"
)

type encodedBlockSemantics struct{}

func (encodedBlockSemantics) BlockMovementSemantics(b world.Block) movementblock.MovementSemantics {
	return movementblock.Resolve(b, BlockName(b))
}

type honeyWallWorld struct {
	staticWorld
	pos cube.Pos
}

func (w honeyWallWorld) Block(pos cube.Pos) world.Block {
	if pos == w.pos {
		return semanticsNamedBlock{name: "minecraft:honey_block"}
	}
	return block.Air{}
}

func TestStuckMovementMultiplierKeepsStrongestOverlappingEffect(t *testing.T) {
	state := newBaseState()
	state.Vel = mgl32.Vec3{1, -1, 1}

	applyInsideBlockMovement(state, movementblock.InsideMovementPowderSnow)
	applyInsideBlockMovement(state, movementblock.InsideMovementSweetBerryBush)
	applyInsideBlockMovement(state, movementblock.InsideMovementSweetBerryBush)

	if want := (mgl32.Vec3{0.8, 0.75, 0.8}); state.StuckSpeedMultiplier != want {
		t.Fatalf("expected strongest multiplier %v without compounding, got %v", want, state.StuckSpeedMultiplier)
	}
	if want := (mgl32.Vec3{1, -1, 1}); state.Vel != want {
		t.Fatalf("inside-block scan changed persistent velocity: got %v, want %v", state.Vel, want)
	}
}

func TestStuckMovementMultiplierAppliesOnceAndClearsVelocity(t *testing.T) {
	sim := &Simulator{World: mockWorld{}}
	state := newBaseState()
	state.HasGravity = false
	state.Vel = mgl32.Vec3{1, -1, 1}
	state.StuckSpeedMultiplier = mgl32.Vec3{0.8, 0.75, 0.8}

	result := sim.SimulateState(state)

	if want := (mgl32.Vec3{0.8, -0.75, 0.8}); result.Movement != want {
		t.Fatalf("expected one scaled displacement %v, got %v", want, result.Movement)
	}
	if state.Vel != (mgl32.Vec3{}) {
		t.Fatalf("expected persistent velocity to clear after stuck movement, got %v", state.Vel)
	}
	if state.StuckSpeedMultiplier != (mgl32.Vec3{}) {
		t.Fatalf("expected pending multiplier to clear, got %v", state.StuckSpeedMultiplier)
	}
}

func TestNoClipDiscardsQueuedStuckMovement(t *testing.T) {
	state := newBaseState()
	state.NoClip = true
	state.StuckSpeedMultiplier = mgl32.Vec3{0.8, 0.75, 0.8}

	if applyStuckSpeedMultiplier(state) {
		t.Fatal("expected no-clip movement not to consume a multiplier")
	}
	if state.StuckSpeedMultiplier != (mgl32.Vec3{}) {
		t.Fatalf("expected no-clip movement to discard the queued effect, got %v", state.StuckSpeedMultiplier)
	}
}

func TestStuckMovementDoesNotBounce(t *testing.T) {
	sim := &Simulator{
		World: staticWorld{chunkLoaded: true, boxes: []cube.BBox32{
			cube.Box32(-1, 0, -1, 1, 1, 1),
		}},
		BlockSemantics: overrideBlockSemantics{semantics: movementblock.MovementSemantics{
			Bounce: movementblock.BounceSlime,
		}},
	}
	state := newBaseState()
	state.Pos = mgl32.Vec3{0, 1, 0}
	state.Vel = mgl32.Vec3{0, -1, 0}
	state.Gravity = NormalGravity
	state.StuckSpeedMultiplier = mgl32.Vec3{0.8, 0.75, 0.8}

	sim.SimulateState(state)

	if state.Vel.Y() > 0 {
		t.Fatalf("expected stuck movement to suppress bounce, got y velocity %v", state.Vel.Y())
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

func TestHoneyWallSlideAppliesOnSolidSideContact(t *testing.T) {
	w := honeyWallWorld{
		staticWorld: staticWorld{chunkLoaded: true, boxes: []cube.BBox32{
			cube.Box32(1, -1, 0, 2, 2, 1),
		}},
		pos: cube.Pos{1, 0, 0},
	}
	sim := &Simulator{World: w, BlockSemantics: encodedBlockSemantics{}}
	state := newBaseState()
	state.Pos = mgl32.Vec3{0.7, 0, 0.5}
	state.Vel = mgl32.Vec3{1, -0.2, 0.1}
	state.HasGravity = false

	sim.SimulateState(state)

	if !state.CollideX {
		t.Fatal("expected horizontal collision with honey wall")
	}
	if state.Vel.Y() != -0.12 {
		t.Fatalf("expected honey slide downward cap -0.12, got %v", state.Vel.Y())
	}
	if want := float32(0.1 * DefaultAirFriction * 0.4); math32.Abs(state.Vel.Z()-want) > 1e-6 {
		t.Fatalf("expected honey slide lateral slowdown %v, got %v", want, state.Vel.Z())
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
