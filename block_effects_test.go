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

	if !sim.attemptJump(state) {
		t.Fatal("expected jump to be applied")
	}
	if want := float32(DefaultJumpHeight * 0.6); math32.Abs(state.Vel.Y()-want) > 1e-6 {
		t.Fatalf("expected honey jump velocity %v, got %v", want, state.Vel.Y())
	}
}

func TestHoneyWallSlideAppliesOnSolidSideContact(t *testing.T) {
	w := honeyWallWorld{
		// Honey collision is inset by 1/16 on each horizontal side.
		staticWorld: staticWorld{chunkLoaded: true, boxes: []cube.BBox32{
			cube.Box32(1.0625, -1, 0.0625, 1.9375, 2, 0.9375),
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

func TestHoneySideSlideResetsFallDistance(t *testing.T) {
	w := environmentWorld{blocks: map[cube.Pos]world.Block{
		{0, 0, 0}: semanticsNamedBlock{name: "minecraft:honey_block"},
	}}
	state := newBaseState()
	state.Pos = mgl32.Vec3{1.25, 0, 0.5}
	state.Vel = mgl32.Vec3{0, -0.2, 0}
	state.FallDistance = 4

	(&Simulator{World: w, BlockSemantics: encodedBlockSemantics{}}).applyHoneyWallSlide(state)

	if state.FallDistance != 0 {
		t.Fatalf("honey side slide left fall distance = %v", state.FallDistance)
	}
}

func TestHoneyTopContactPreservesFallDistance(t *testing.T) {
	w := environmentWorld{blocks: map[cube.Pos]world.Block{
		{0, 0, 0}: semanticsNamedBlock{name: "minecraft:honey_block"},
	}}
	state := newBaseState()
	state.Pos = mgl32.Vec3{0.5, 0, 0.5}
	state.Vel = mgl32.Vec3{0, -0.2, 0}
	state.FallDistance = 4

	(&Simulator{World: w, BlockSemantics: encodedBlockSemantics{}}).applyHoneyWallSlide(state)

	if state.FallDistance != 4 {
		t.Fatalf("honey top contact changed fall distance to %v", state.FallDistance)
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

	sim.SimulateState(state)
	if want := (mgl32.Vec3{0.5, 0.25, 0.5}); state.StuckSpeedMultiplier != want {
		t.Fatalf("expected Weaving web multiplier %v, got %v", want, state.StuckSpeedMultiplier)
	}
	want := state.Vel.X() * 0.5
	result := sim.SimulateState(state)

	if math32.Abs(result.Movement.X()-want) > 1e-6 {
		t.Fatalf("expected Weaving web movement %v, got %v", want, result.Movement.X())
	}
}

func TestGlidingIntoWebUsesSlowdownMultiplier(t *testing.T) {
	w := environmentWorld{blocks: map[cube.Pos]world.Block{
		{0, 0, 0}: semanticsNamedBlock{name: "minecraft:web"},
	}}
	sim := &Simulator{World: w, BlockSemantics: encodedBlockSemantics{}, Inventory: mockInventory{hasElytra: true}}
	state := newBaseState()
	state.Pos = mgl32.Vec3{0.5, 0, 0.5}
	state.Gliding = true
	state.Gravity = NormalGravity
	state.Vel = mgl32.Vec3{0, 0, 0.5}

	sim.SimulateState(state)
	if want := (mgl32.Vec3{0.25, 0.05, 0.25}); state.StuckSpeedMultiplier != want {
		t.Fatalf("gliding web contact queued %v, want %v", state.StuckSpeedMultiplier, want)
	}
	if !state.Gliding {
		t.Fatal("expected glide to continue")
	}
	sim.SimulateState(state)
	if state.Vel != (mgl32.Vec3{}) {
		t.Fatalf("expected web slowdown to clear gliding velocity, got %v", state.Vel)
	}
}

func TestOverlappingSlowdownBlocksUseWeakestAxisNotProduct(t *testing.T) {
	w := environmentWorld{blocks: map[cube.Pos]world.Block{
		{0, 0, 0}: semanticsNamedBlock{name: "minecraft:web"},
		{0, 1, 0}: semanticsNamedBlock{name: "minecraft:sweet_berry_bush"},
	}}
	sim := &Simulator{World: w, BlockSemantics: encodedBlockSemantics{}}
	state := newBaseState()
	state.Pos = mgl32.Vec3{0.5, 0, 0.5}
	state.HasGravity = false

	sim.SimulateState(state)
	if want := (mgl32.Vec3{0.25, 0.05, 0.25}); state.StuckSpeedMultiplier != want {
		t.Fatalf("web and berry bush queued %v, want %v", state.StuckSpeedMultiplier, want)
	}
	state.Vel = mgl32.Vec3{0.4, 0, 0}
	result := sim.SimulateState(state)
	if want := float32(0.4 * 0.25); math32.Abs(result.Movement.X()-want) > 1e-6 {
		t.Fatalf("web and berry bush moved %v, want %v", result.Movement.X(), want)
	}
}

func TestWeavingKeepsWebMultiplierInsideOtherSlowdownBlocks(t *testing.T) {
	w := environmentWorld{blocks: map[cube.Pos]world.Block{
		{0, 0, 0}: semanticsNamedBlock{name: "minecraft:web"},
		{0, 1, 0}: semanticsNamedBlock{name: "minecraft:powder_snow"},
	}}
	sim := &Simulator{World: w, BlockSemantics: encodedBlockSemantics{}, Effects: fixedEffects{EffectWeaving: 0}}
	state := newBaseState()
	state.Pos = mgl32.Vec3{0.5, 0, 0.5}

	sim.applyInsideBlockEffects(state)

	if want := (mgl32.Vec3{0.25, 0.05, 0.25}); state.StuckSpeedMultiplier != want {
		t.Fatalf("Weaving in web and powder snow queued %v, want %v", state.StuckSpeedMultiplier, want)
	}
}

func TestSlowdownBlockContactResetsFallDistance(t *testing.T) {
	w := environmentWorld{blocks: map[cube.Pos]world.Block{
		{0, 0, 0}: semanticsNamedBlock{name: "minecraft:sweet_berry_bush"},
	}}
	state := newBaseState()
	state.Pos = mgl32.Vec3{0.5, 0, 0.5}
	state.FallDistance = 4

	(&Simulator{World: w, BlockSemantics: encodedBlockSemantics{}}).applyInsideBlockEffects(state)

	if state.FallDistance != 0 {
		t.Fatalf("berry bush contact left fall distance = %v", state.FallDistance)
	}
}

func TestHoneyDoesNotSlowEntityInWater(t *testing.T) {
	w := environmentWorld{blocks: map[cube.Pos]world.Block{
		{1, 0, 0}: semanticsNamedBlock{name: "minecraft:honey_block"},
	}}
	state := newBaseState()
	state.Pos = mgl32.Vec3{0.75, 0, 0.5}
	state.Vel = mgl32.Vec3{0.1, -0.2, 0.1}
	state.swimWaterContact = true

	(&Simulator{World: w, BlockSemantics: encodedBlockSemantics{}}).applyInsideBlockEffects(state)

	if want := (mgl32.Vec3{0.1, -0.2, 0.1}); state.Vel != want {
		t.Fatalf("honey changed in-water velocity to %v", state.Vel)
	}
}

func TestInsideBlocksIgnoreMarginalOverlap(t *testing.T) {
	w := environmentWorld{
		bubbles: map[cube.Pos]BubbleColumnDirection{{1, 0, 0}: BubbleColumnUp},
		blocks: map[cube.Pos]world.Block{
			{0, 0, 1}: semanticsNamedBlock{name: "minecraft:honey_block"},
		},
	}
	state := newBaseState()
	// The box overlaps the bubble column (x) and honey (z) cells by 0.0005.
	state.Pos = mgl32.Vec3{0.7005, 0, 0.7005}
	state.Vel = mgl32.Vec3{0.1, -0.2, 0.1}

	(&Simulator{World: w, BlockSemantics: encodedBlockSemantics{}}).applyInsideBlockEffects(state)

	if want := (mgl32.Vec3{0.1, -0.2, 0.1}); state.Vel != want {
		t.Fatalf("marginal overlap changed velocity to %v", state.Vel)
	}
}

func TestBubbleColumnAppliesBeforeHoney(t *testing.T) {
	w := environmentWorld{
		bubbles: map[cube.Pos]BubbleColumnDirection{{0, -1, 0}: BubbleColumnUp},
		blocks: map[cube.Pos]world.Block{
			{0, 0, 0}: semanticsNamedBlock{name: "minecraft:honey_block"},
		},
	}
	state := newBaseState()
	state.Pos = mgl32.Vec3{0.5, 0, 0.5}
	state.Vel = mgl32.Vec3{0, -1, 0}
	state.HasGravity = false

	(&Simulator{World: w, BlockSemantics: encodedBlockSemantics{}}).SimulateState(state)

	// The submerged column gives -0.94; honey then caps the descent at -0.12.
	if state.Vel.Y() != -0.12 {
		t.Fatalf("bubble column and honey velocity = %v, want -0.12", state.Vel.Y())
	}
}
