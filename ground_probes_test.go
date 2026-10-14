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

// groundWorld is a sparse world whose block-local collision boxes back both
// per-block and nearby-box queries.
type groundWorld struct {
	blocks map[cube.Pos]world.Block
	boxes  map[cube.Pos][]cube.BBox32
}

func (w groundWorld) Block(pos cube.Pos) world.Block {
	if b, ok := w.blocks[pos]; ok {
		return b
	}
	return block.Air{}
}

func (w groundWorld) BlockCollisions(pos cube.Pos) []cube.BBox32 { return w.boxes[pos] }

func (w groundWorld) GetNearbyBBoxes(aabb cube.BBox32) []cube.BBox32 {
	var out []cube.BBox32
	for pos, boxes := range w.boxes {
		for _, box := range boxes {
			if box = box.Translate(posVec3(pos)); strictlyIntersects(aabb, box) {
				out = append(out, box)
			}
		}
	}
	return out
}

func (groundWorld) IsChunkLoaded(int32, int32) bool { return true }

func strictlyIntersects(a, b cube.BBox32) bool {
	for axis := range 3 {
		if a.Max()[axis] <= b.Min()[axis] || b.Max()[axis] <= a.Min()[axis] {
			return false
		}
	}
	return true
}

// place sets b at pos with a full-width box of the given height.
func (w groundWorld) place(pos cube.Pos, b world.Block, height float32) {
	w.blocks[pos] = b
	w.boxes[pos] = []cube.BBox32{cube.Box32(0, 0, 0, 1, height, 1)}
}

// mul32 multiplies at runtime so constants round like simulation arithmetic.
func mul32(a, b float32) float32 { return a * b }

func newGroundWorld() groundWorld {
	return groundWorld{blocks: map[cube.Pos]world.Block{}, boxes: map[cube.Pos][]cube.BBox32{}}
}

func groundState(pos mgl32.Vec3) *MovementState {
	state := newBaseState()
	state.Pos = pos
	state.Client.Pos = pos
	state.OnGround = true
	state.Gravity = NormalGravity
	return state
}

// Ground friction comes from the cell 0.1 below the feet, not 0.5 below.
func TestGroundFrictionProbesJustBelowFeet(t *testing.T) {
	w := newGroundWorld()
	w.place(cube.Pos{0, 0, 0}, block.PackedIce{}, 1)
	w.place(cube.Pos{0, 1, 0}, semanticsNamedBlock{"minecraft:snow_layer"}, 0.25)
	sim := &Simulator{World: w, BlockSemantics: encodedBlockSemantics{}}
	state := groundState(mgl32.Vec3{0.5, 1.25, 0.5})
	state.Vel = mgl32.Vec3{0.1, 0, 0}

	sim.SimulateState(state)

	if want := float32(0.1) * mul32(DefaultAirFriction, DefaultBlockFriction); state.Vel.X() != want {
		t.Fatalf("drag over snow layer on ice: got %v, want %v", state.Vel.X(), want)
	}
}

// Air under the feet keeps the default friction even when a custom registry
// assigns air another value.
func TestGroundFrictionIgnoresAirSemantics(t *testing.T) {
	sim := &Simulator{
		World:          mockWorld{},
		BlockSemantics: overrideBlockSemantics{semantics: movementblock.MovementSemantics{GroundFriction: 0.98, GroundAccelerationFrictionMultiplier: 1}},
	}
	state := groundState(mgl32.Vec3{0.5, 1, 0.5})
	state.HasGravity = false
	state.Vel = mgl32.Vec3{0.1, 0, 0}

	sim.SimulateState(state)

	if want := float32(0.1) * mul32(DefaultAirFriction, DefaultBlockFriction); state.Vel.X() != want {
		t.Fatalf("drag over air: got %v, want %v", state.Vel.X(), want)
	}
}

// Horizontal drag clears components at or below float epsilon.
func TestHorizontalDragClearsEpsilonVelocity(t *testing.T) {
	sim := &Simulator{World: mockWorld{}}
	state := newBaseState()
	state.HasGravity = false
	state.Vel = mgl32.Vec3{1e-7, 0, -1e-7}

	sim.SimulateState(state)

	if state.Vel.X() != 0 || state.Vel.Z() != 0 {
		t.Fatalf("expected epsilon velocity cleared, got %v", state.Vel)
	}
}

// Landings slower than the restitution threshold do not bounce on slime.
func TestSlimeIgnoresSlowLandings(t *testing.T) {
	w := newGroundWorld()
	w.place(cube.Pos{0, 0, 0}, block.Slime{}, 1)
	sim := &Simulator{World: w}
	state := groundState(mgl32.Vec3{0.5, 1.05, 0.5})
	state.OnGround = false
	state.Vel = mgl32.Vec3{0, -0.07, 0}

	sim.SimulateState(state)

	if want := mul32(-NormalGravity, NormalGravityMultiplier); state.Vel.Y() != want || !state.OnGround {
		t.Fatalf("slow slime landing: vy=%v onGround=%v, want vy=%v grounded", state.Vel.Y(), state.OnGround, want)
	}
}

// The sneaking pose, not the held key, suppresses a bounce.
func TestSneakingPoseSuppressesBounce(t *testing.T) {
	w := newGroundWorld()
	w.place(cube.Pos{0, 0, 0}, block.Slime{}, 1)
	sim := &Simulator{World: w}
	state := groundState(mgl32.Vec3{0.5, 1.2, 0.5})
	state.OnGround = false
	state.Sneaking = true
	state.Size[1] = 1.49
	state.Vel = mgl32.Vec3{0, -0.5, 0}

	sim.SimulateState(state)

	if want := mul32(-NormalGravity, NormalGravityMultiplier); state.Vel.Y() != want {
		t.Fatalf("sneaking slime landing: vy=%v, want %v", state.Vel.Y(), want)
	}
}

// A carpet on slime still bounces: the landing block is the highest shape
// centred at least 0.2 below the feet.
func TestCarpetOnSlimeBounces(t *testing.T) {
	w := newGroundWorld()
	w.place(cube.Pos{0, 0, 0}, block.Slime{}, 1)
	w.place(cube.Pos{0, 1, 0}, semanticsNamedBlock{"minecraft:white_carpet"}, 0.0625)
	sim := &Simulator{World: w, BlockSemantics: encodedBlockSemantics{}}
	state := groundState(mgl32.Vec3{0.5, 1.4, 0.5})
	state.OnGround = false
	state.HasGravity = false
	state.Vel = mgl32.Vec3{0, -0.5, 0}

	sim.SimulateState(state)

	if want := float32(0.5); state.Vel.Y() != want {
		t.Fatalf("carpet over slime landing: vy=%v, want %v", state.Vel.Y(), want)
	}
}

// Slime and honey damp horizontal motion after gravity and drag, using the
// post-gravity vertical speed.
func TestStandOnDampingFollowsGravityAndDrag(t *testing.T) {
	for name, b := range map[string]world.Block{
		"slime": block.Slime{},
		"honey": semanticsNamedBlock{"minecraft:honey_block"},
	} {
		t.Run(name, func(t *testing.T) {
			w := newGroundWorld()
			w.place(cube.Pos{0, 0, 0}, b, 1)
			sim := &Simulator{World: w, BlockSemantics: encodedBlockSemantics{}}
			state := groundState(mgl32.Vec3{0.5, 1, 0.5})
			state.Vel = mgl32.Vec3{0.2, 0, 0}

			sim.SimulateState(state)

			vy := mul32(-NormalGravity, NormalGravityMultiplier)
			factor := float32(-vy*0.2) + 0.4
			want := float32(0.2) * mul32(DefaultAirFriction, 0.8) * factor
			if state.Vel.X() != want || state.Vel.Y() != vy {
				t.Fatalf("stand-on velocity %v, want x=%v y=%v", state.Vel, want, vy)
			}
		})
	}
}

// The honey jump factor reads the feet cell, and the cell below only from a
// block boundary or a carpet.
func TestHoneyJumpFactorBlock(t *testing.T) {
	for name, tt := range map[string]struct {
		feet   world.Block
		feetY  float32
		height float32
	}{
		"thin block over honey": {semanticsNamedBlock{"minecraft:test_plate"}, 1.05, 1},
		"carpet over honey":     {semanticsNamedBlock{"minecraft:red_carpet"}, 1.0625, 0.6},
		"honey boundary":        {block.Air{}, 1, 0.6},
	} {
		t.Run(name, func(t *testing.T) {
			w := newGroundWorld()
			w.blocks[cube.Pos{0, 0, 0}] = semanticsNamedBlock{"minecraft:honey_block"}
			w.blocks[cube.Pos{0, 1, 0}] = tt.feet
			sim := &Simulator{World: w, BlockSemantics: encodedBlockSemantics{}}
			state := groundState(mgl32.Vec3{0.5, tt.feetY, 0.5})
			state.Jumping = true
			state.JumpHeight = DefaultJumpHeight

			sim.attemptJump(state)

			if want := DefaultJumpHeight * tt.height; state.Vel.Y() != want {
				t.Fatalf("jump velocity %v, want %v", state.Vel.Y(), want)
			}
		})
	}
}

// Sneaking clips the jump tick too, and clipping shortens only displacement.
func TestSneakEdgeClipsDisplacementOnly(t *testing.T) {
	w := newGroundWorld()
	w.place(cube.Pos{0, 0, 0}, block.Stone{}, 1)
	sim := &Simulator{World: w}
	state := groundState(mgl32.Vec3{0.5, 1, 0.5})
	state.Sneaking = true
	state.Size[1] = 1.49
	state.HasGravity = false
	state.Vel = mgl32.Vec3{1, 0.42, 0}

	sim.SimulateState(state)

	if state.Pos.X() >= 1.3 {
		t.Fatalf("jump-tick sneak move left supported ground: x=%v", state.Pos.X())
	}
	if want := float32(1) * mul32(DefaultAirFriction, DefaultBlockFriction); state.Vel.X() != want {
		t.Fatalf("retained velocity %v, want %v", state.Vel.X(), want)
	}
}

// A vertical clip larger than float epsilon is a collision and grounds a fall.
func TestSmallVerticalClipGrounds(t *testing.T) {
	w := newGroundWorld()
	w.place(cube.Pos{0, 0, 0}, block.Stone{}, 1)
	sim := &Simulator{World: w}
	state := groundState(mgl32.Vec3{0.5, 1.01, 0.5})
	state.OnGround = false
	state.HasGravity = false
	state.Vel = mgl32.Vec3{0, -0.010003, 0}

	sim.SimulateState(state)

	if !state.OnGround || !state.CollideY {
		t.Fatalf("expected landing collision, onGround=%v collideY=%v pos=%v", state.OnGround, state.CollideY, state.Pos)
	}
}

// orderedBoxWorld returns its boxes in a fixed provider order.
type orderedBoxWorld struct {
	groundWorld
	order []cube.Pos
}

func (w orderedBoxWorld) GetNearbyBBoxes(aabb cube.BBox32) []cube.BBox32 {
	var out []cube.BBox32
	for _, pos := range w.order {
		for _, box := range w.boxes[pos] {
			if box = box.Translate(posVec3(pos)); strictlyIntersects(aabb, box) {
				out = append(out, box)
			}
		}
	}
	return out
}

// An exact landing tie resolves in vanilla's x, z, y gathering order, not the
// provider's order.
func TestLandingTieUsesVanillaGatheringOrder(t *testing.T) {
	slime, stone := cube.Pos{0, 0, 0}, cube.Pos{1, 0, 0}
	for name, order := range map[string][]cube.Pos{
		"slime first": {slime, stone},
		"stone first": {stone, slime},
	} {
		t.Run(name, func(t *testing.T) {
			w := orderedBoxWorld{groundWorld: newGroundWorld(), order: order}
			w.place(slime, block.Slime{}, 1)
			w.place(stone, block.Stone{}, 1)
			sim := &Simulator{World: w}
			state := groundState(mgl32.Vec3{1, 1.2, 0.5})
			state.OnGround = false
			state.HasGravity = false
			state.Vel = mgl32.Vec3{0, -0.5, 0}

			sim.SimulateState(state)

			if want := float32(0.5); state.Vel.Y() != want {
				t.Fatalf("boundary landing vy=%v, want slime bounce %v", state.Vel.Y(), want)
			}
		})
	}
}

// bounceLanding drops a player at gap above a full surface block with a
// -0.5 vertical speed and runs the landing tick.
func bounceLanding(t *testing.T, surface string, gap float32, configure func(*MovementState)) (*Simulator, *MovementState) {
	t.Helper()
	w := newGroundWorld()
	w.place(cube.Pos{0, 0, 0}, semanticsNamedBlock{surface}, 1)
	sim := &Simulator{World: w, BlockSemantics: encodedBlockSemantics{}}
	state := groundState(mgl32.Vec3{0.5, 1 + gap, 0.5})
	state.OnGround = false
	state.Vel = mgl32.Vec3{0, -0.5, 0}
	if configure != nil {
		configure(state)
	}
	sim.SimulateState(state)
	if !state.CollideY {
		t.Fatalf("%s gap %v: expected a landing collision", surface, gap)
	}
	return sim, state
}

// A rebound tick applies only the gravity left after impact, then drag; the
// next tick is ordinary.
func TestBounceGravityUsesTravelBeforeImpact(t *testing.T) {
	gaps := []float32{0.0625, 0.25, 0.4375}
	for surface, want := range map[string][]float32{
		"minecraft:slime": {0.42130393, 0.44934615, 0.47597167},
		"minecraft:bed":   {0.29880393, 0.32684615, 0.35347167},
	} {
		for i, gap := range gaps {
			sim, state := bounceLanding(t, surface, gap, nil)
			if state.Pos.Y() != 1 || state.Mov.Y() != -gap {
				t.Fatalf("%s gap %v: landed at y=%v after moving %v", surface, gap, state.Pos.Y(), state.Mov.Y())
			}
			landed := state.Vel.Y()
			if math32.Abs(landed-want[i]) > floatEpsilon {
				t.Fatalf("%s gap %v: rebound vy=%v, want %v", surface, gap, landed, want[i])
			}

			sim.SimulateState(state)
			if state.CollideY || state.Mov.Y() != landed {
				t.Fatalf("%s gap %v: rising tick collideY=%v mov=%v, want free move of %v", surface, gap, state.CollideY, state.Mov.Y(), landed)
			}
			if next := (landed - NormalGravity) * NormalGravityMultiplier; state.Vel.Y() != next {
				t.Fatalf("%s gap %v: rising tick vy=%v, want full gravity %v", surface, gap, state.Vel.Y(), next)
			}
		}
	}
}

// The correction scales a custom gravity, and an actor without gravity keeps
// its rebound untouched.
func TestBounceGravityFollowsCustomAndDisabledGravity(t *testing.T) {
	for surface, tt := range map[string]struct{ rebound, customGravity float32 }{
		"minecraft:slime": {0.5, 0.47002295},
		"minecraft:bed":   {0.375, 0.34752294},
	} {
		_, state := bounceLanding(t, surface, 0.25, func(state *MovementState) { state.HasGravity = false })
		if state.Vel.Y() != tt.rebound {
			t.Fatalf("%s without gravity: vy=%v, want %v", surface, state.Vel.Y(), tt.rebound)
		}
		_, state = bounceLanding(t, surface, 0.25, func(state *MovementState) { state.Gravity = 0.04 })
		if math32.Abs(state.Vel.Y()-tt.customGravity) > floatEpsilon {
			t.Fatalf("%s with gravity 0.04: vy=%v, want %v", surface, state.Vel.Y(), tt.customGravity)
		}
	}
}

// A rebound with no measurable travel before impact keeps a full tick of
// gravity; an epsilon gap clips to no travel.
func TestBounceWithNegligibleTravelUsesFullGravity(t *testing.T) {
	for _, gap := range []float32{0, floatEpsilon} {
		_, state := bounceLanding(t, "minecraft:slime", gap, nil)
		if want := (float32(0.5) - NormalGravity) * NormalGravityMultiplier; state.Vel.Y() != want {
			t.Fatalf("gap %v: vy=%v, want %v", gap, state.Vel.Y(), want)
		}
	}
}
