package bedsim

import (
	"testing"

	"github.com/chewxy/math32"
	"github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/go-gl/mathgl/mgl32"
)

// climbWorld serves named blocks plus full-cube solids that collide.
type climbWorld struct {
	blocks   map[cube.Pos]world.Block
	solids   map[cube.Pos]bool
	contexts *[]MovementCollisionContext
}

func (w climbWorld) Block(pos cube.Pos) world.Block {
	if b, ok := w.blocks[pos]; ok {
		return b
	}
	if w.solids[pos] {
		return block.Stone{}
	}
	return block.Air{}
}

func (w climbWorld) BlockCollisions(pos cube.Pos) []cube.BBox32 {
	if w.solids[pos] {
		return []cube.BBox32{cube.Box32(0, 0, 0, 1, 1, 1)}
	}
	return nil
}

func (w climbWorld) GetNearbyBBoxes(aabb cube.BBox32) []cube.BBox32 {
	var boxes []cube.BBox32
	for pos := range w.solids {
		box := cube.Box32(0, 0, 0, 1, 1, 1).Translate(posVec3(pos))
		if aabb.IntersectsWith(box) {
			boxes = append(boxes, box)
		}
	}
	return boxes
}

func (w climbWorld) GetMovementBBoxes(aabb cube.BBox32, context MovementCollisionContext) []cube.BBox32 {
	if w.contexts != nil {
		*w.contexts = append(*w.contexts, context)
	}
	return w.GetNearbyBBoxes(aabb)
}

func (climbWorld) IsChunkLoaded(int32, int32) bool { return true }

func climbingState(pos mgl32.Vec3) *MovementState {
	state := newBaseState()
	state.Pos = pos
	state.Client.Pos = pos
	state.Gravity = NormalGravity
	return state
}

func assertFloat(t *testing.T, what string, got, want float32) {
	t.Helper()
	if math32.Abs(got-want) > 1e-6 {
		t.Fatalf("%s = %v, want %v", what, got, want)
	}
}

// A held jump at a ladder base climbs instead of jumping: no 0.42, sprint impulse or jump delay.
func TestLadderJumpReplacesGroundJump(t *testing.T) {
	w := climbWorld{
		blocks: map[cube.Pos]world.Block{{0, 0, 0}: block.Ladder{Facing: cube.West}},
		solids: map[cube.Pos]bool{{0, -1, 0}: true},
	}
	state := climbingState(mgl32.Vec3{0.5, 0, 0.5})
	state.OnGround = true
	state.Sprinting = true
	state.Jumping, state.PressingJump, state.EffectiveJumping = true, true, true

	(&Simulator{World: w}).SimulateState(state)

	assertFloat(t, "climb velocity", state.Vel.Y(), (ClimbSpeed-NormalGravity)*NormalGravityMultiplier)
	if state.Vel.X() != 0 || state.Vel.Z() != 0 {
		t.Fatalf("ladder jump applied a sprint impulse: %v", state.Vel)
	}
	if state.JumpDelay != 0 {
		t.Fatalf("ladder jump set jump delay %d", state.JumpDelay)
	}
}

// A held jump inside scaffolding ascends at 0.15 with a jump delay and no ground jump.
func TestScaffoldingJumpReplacesGroundJump(t *testing.T) {
	w := climbWorld{
		blocks: map[cube.Pos]world.Block{{0, 0, 0}: semanticsNamedBlock{name: "minecraft:scaffolding"}},
		solids: map[cube.Pos]bool{{0, -1, 0}: true},
	}
	state := climbingState(mgl32.Vec3{0.5, 0, 0.5})
	state.OnGround = true
	state.Sprinting = true
	state.Jumping, state.PressingJump, state.EffectiveJumping = true, true, true

	(&Simulator{World: w, BlockSemantics: encodedBlockSemantics{}}).SimulateState(state)

	assertFloat(t, "scaffolding ascent", state.Vel.Y(), (ScaffoldingSpeed-NormalGravity)*NormalGravityMultiplier)
	if state.Vel.X() != 0 || state.Vel.Z() != 0 {
		t.Fatalf("scaffolding jump applied a sprint impulse: %v", state.Vel)
	}
	if state.JumpDelay != JumpDelayTicks {
		t.Fatalf("scaffolding jump delay = %d, want %d", state.JumpDelay, JumpDelayTicks)
	}
}

// This tick's wall collision lifts a climber to 0.2 with no gravity or vertical drag.
func TestAutoClimbFollowsThisTicksWallCollision(t *testing.T) {
	w := climbWorld{
		blocks: map[cube.Pos]world.Block{{0, 0, 0}: block.Ladder{Facing: cube.West}},
		solids: map[cube.Pos]bool{{1, 0, 0}: true, {1, 1, 0}: true},
	}
	state := climbingState(mgl32.Vec3{0.65, 0, 0.5})
	state.Vel = mgl32.Vec3{0.1, 0, 0}

	(&Simulator{World: w}).SimulateState(state)

	if !state.CollideX {
		t.Fatal("expected a wall collision")
	}
	if state.Vel.Y() != ClimbSpeed {
		t.Fatalf("auto-climb velocity = %v, want exactly %v", state.Vel.Y(), ClimbSpeed)
	}
}

// Leaving the wall at the ladder top keeps no climb lift from the previous tick's collision.
func TestPreviousWallCollisionDoesNotLiftClimber(t *testing.T) {
	w := climbWorld{blocks: map[cube.Pos]world.Block{{0, 0, 0}: block.Ladder{Facing: cube.West}}}
	state := climbingState(mgl32.Vec3{0.5, 0, 0.5})
	state.CollideX = true

	(&Simulator{World: w}).SimulateState(state)

	if state.Pos.Y() != 0 {
		t.Fatalf("stale collision moved the climber to y=%v", state.Pos.Y())
	}
	assertFloat(t, "vertical velocity", state.Vel.Y(), -NormalGravity*NormalGravityMultiplier)
}

// Sneaking on a scaffold bridge over air does not descend through it.
func TestScaffoldingBridgeOverAirDoesNotDescend(t *testing.T) {
	var contexts []MovementCollisionContext
	w := climbWorld{
		blocks:   map[cube.Pos]world.Block{{0, 0, 0}: semanticsNamedBlock{name: "minecraft:scaffolding"}},
		contexts: &contexts,
	}
	support := cube.Pos{0, 0, 0}
	state := climbingState(mgl32.Vec3{0.5, 1, 0.5})
	state.OnGround = true
	state.SupportingBlockPos = &support
	state.PressingDescend = true

	(&Simulator{World: w, BlockSemantics: encodedBlockSemantics{}}).SimulateState(state)

	assertFloat(t, "vertical velocity", state.Vel.Y(), -NormalGravity*NormalGravityMultiplier)
	for _, context := range contexts {
		if context.DescendThroughBlock {
			t.Fatal("bridge over air reported a descent through the block")
		}
	}
}

// A supported scaffold column descends 0.15 per tick, keeping vertical drag but no gravity.
func TestScaffoldingColumnDescentKeepsDrag(t *testing.T) {
	var contexts []MovementCollisionContext
	w := climbWorld{
		blocks: map[cube.Pos]world.Block{
			{0, 0, 0}: semanticsNamedBlock{name: "minecraft:scaffolding"},
			{0, 1, 0}: semanticsNamedBlock{name: "minecraft:scaffolding"},
		},
		solids:   map[cube.Pos]bool{{0, -1, 0}: true},
		contexts: &contexts,
	}
	state := climbingState(mgl32.Vec3{0.5, 1.5, 0.5})
	state.PressingDescend = true
	state.FallDistance = 4

	(&Simulator{World: w, BlockSemantics: encodedBlockSemantics{}}).SimulateState(state)

	assertFloat(t, "position", state.Pos.Y(), 1.35)
	if want := -ScaffoldingSpeed * NormalGravityMultiplier; state.Vel.Y() != want {
		t.Fatalf("descent velocity = %v, want %v", state.Vel.Y(), want)
	}
	if state.FallDistance != 0 {
		t.Fatalf("scaffolding descent left fall distance %v", state.FallDistance)
	}
	if len(contexts) == 0 || !contexts[len(contexts)-1].DescendThroughBlock {
		t.Fatalf("move collision context did not report the descent: %+v", contexts)
	}
}

// Powder snow without leather boots has no descend traversal.
func TestPowderSnowDescentRequiresLeatherBoots(t *testing.T) {
	w := climbWorld{blocks: map[cube.Pos]world.Block{{0, 0, 0}: semanticsNamedBlock{name: "minecraft:powder_snow"}}}
	state := climbingState(mgl32.Vec3{0.5, 0.5, 0.5})
	state.PressingDescend = true

	(&Simulator{World: w, BlockSemantics: encodedBlockSemantics{}}).SimulateState(state)

	assertFloat(t, "vertical velocity", state.Vel.Y(), -NormalGravity*NormalGravityMultiplier)
}

// Leather boots let a player sink into powder snow below; gravity still applies.
func TestLeatherBootsDescendIntoPowderSnow(t *testing.T) {
	w := climbWorld{blocks: map[cube.Pos]world.Block{{0, 0, 0}: semanticsNamedBlock{name: "minecraft:powder_snow"}}}
	state := climbingState(mgl32.Vec3{0.5, 1, 0.5})
	state.PressingDescend = true

	(&Simulator{World: w, BlockSemantics: encodedBlockSemantics{}, Equipment: leatherEquipment{}}).SimulateState(state)

	assertFloat(t, "vertical velocity", state.Vel.Y(), (-ScaffoldingSpeed-NormalGravity)*NormalGravityMultiplier)
}

// Jumping in powder snow with leather boots takes the 0.15 ascendable branch, not the ladder 0.2.
func TestLeatherBootsPowderSnowJumpAscends(t *testing.T) {
	w := climbWorld{blocks: map[cube.Pos]world.Block{{0, 0, 0}: semanticsNamedBlock{name: "minecraft:powder_snow"}}}
	state := climbingState(mgl32.Vec3{0.5, 0.5, 0.5})
	state.PressingJump, state.EffectiveJumping = true, true

	(&Simulator{World: w, BlockSemantics: encodedBlockSemantics{}, Equipment: leatherEquipment{}}).SimulateState(state)

	assertFloat(t, "vertical velocity", state.Vel.Y(), (ScaffoldingSpeed-NormalGravity)*NormalGravityMultiplier)
	if state.JumpDelay != JumpDelayTicks {
		t.Fatalf("jump delay = %d, want %d", state.JumpDelay, JumpDelayTicks)
	}
}

// floorLoadedWorld reports every area below its floor as unloaded.
type floorLoadedWorld struct {
	climbWorld
	floor float32
}

func (w floorLoadedWorld) IsMovementAreaLoaded(aabb cube.BBox32) bool {
	return aabb.Min().Y() >= w.floor
}

// A scaffold support below the loaded area makes the tick unknown rather than normal.
func TestScaffoldingSupportProbeRequiresLoadedArea(t *testing.T) {
	w := floorLoadedWorld{
		climbWorld: climbWorld{blocks: map[cube.Pos]world.Block{{0, 0, 0}: semanticsNamedBlock{name: "minecraft:scaffolding"}}},
		floor:      0,
	}
	state := climbingState(mgl32.Vec3{0.5, 1.5, 0.5})

	result := (&Simulator{World: w, BlockSemantics: encodedBlockSemantics{}}).SimulateState(state)

	if result.Outcome != SimulationOutcomeUnloadedChunk {
		t.Fatalf("outcome = %v, want unloaded chunk", result.Outcome)
	}
}
