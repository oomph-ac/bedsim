package bedsim

import (
	"math"
	"testing"

	"github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/go-gl/mathgl/mgl32"
	movementblock "github.com/oomph-ac/bedsim/block"
)

func TestBlockGroundFrictionSoulBlocks(t *testing.T) {
	want := DefaultBlockFriction * movementblock.SoulGroundFrictionMultiplier

	for name, b := range map[string]world.Block{
		"soul sand": block.SoulSand{},
		"soul soil": block.SoulSoil{},
	} {
		t.Run(name, func(t *testing.T) {
			if got := movementblock.Resolve(b, BlockName(b)).GroundFriction; math.Abs(float64(got-want)) > 1e-6 {
				t.Fatalf("ground friction = %.8f, want %.8f", got, want)
			}
		})
	}
}

func TestDefaultMovementBlockSemantics(t *testing.T) {
	tests := []struct {
		name       string
		block      world.Block
		climbable  bool
		cobweb     bool
		bounce     movementblock.Bounce
		groundWant float32
	}{
		{
			name:       "air",
			block:      block.Air{},
			groundWant: DefaultBlockFriction,
		},
		{
			name:       "ladder",
			block:      block.Ladder{},
			climbable:  true,
			groundWant: DefaultBlockFriction,
		},
		{
			name:       "vines",
			block:      block.Vines{},
			climbable:  true,
			groundWant: DefaultBlockFriction,
		},
		{
			name:       "soul soil",
			block:      block.SoulSoil{},
			groundWant: DefaultBlockFriction * movementblock.SoulGroundFrictionMultiplier,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := movementblock.Resolve(tt.block, BlockName(tt.block))
			if math.Abs(float64(got.GroundFriction-tt.groundWant)) > 1e-6 {
				t.Fatalf("ground friction = %.8f, want %.8f", got.GroundFriction, tt.groundWant)
			}
			if got.Climbable != tt.climbable {
				t.Fatalf("climbable = %v, want %v", got.Climbable, tt.climbable)
			}
			if got.Cobweb != tt.cobweb {
				t.Fatalf("cobweb = %v, want %v", got.Cobweb, tt.cobweb)
			}
			if got.Bounce != tt.bounce {
				t.Fatalf("bounce = %v, want %v", got.Bounce, tt.bounce)
			}
		})
	}
}

func TestDefaultMovementBlockSemanticsSpecialBlocks(t *testing.T) {
	for name, want := range map[string]struct {
		block  world.Block
		bounce movementblock.Bounce
	}{
		"slime":  {block: semanticsNamedBlock{"minecraft:slime"}, bounce: movementblock.BounceSlime},
		"bed":    {block: semanticsNamedBlock{"minecraft:bed"}, bounce: movementblock.BounceBed},
		"bamboo": {block: semanticsNamedBlock{"minecraft:bamboo"}},
		"cobweb": {block: semanticsNamedBlock{"minecraft:web"}},
	} {
		t.Run(name, func(t *testing.T) {
			got := movementblock.Resolve(want.block, BlockName(want.block))
			if got.Bounce != want.bounce {
				t.Fatalf("semantics = %+v, want bounce=%v", got, want.bounce)
			}
			if name == "cobweb" && !got.Cobweb {
				t.Fatalf("expected cobweb semantics")
			}
		})
	}
}

// semanticsNamedBlock is enough to exercise name-based semantics without depending on
// a particular Dragonfly block implementation being present in the registry.
type semanticsNamedBlock struct{ name string }

func (b semanticsNamedBlock) Hash() (uint64, uint64) { return 0, math.MaxUint64 }
func (b semanticsNamedBlock) EncodeBlock() (string, map[string]any) {
	return b.name, nil
}
func (b semanticsNamedBlock) Model() world.BlockModel { return block.Air{}.Model() }

type extendedMovementSemantics struct {
	groundFriction float32
	climbable      bool
	cobweb         bool
	bounce         movementblock.Bounce
}

func (s extendedMovementSemantics) BlockMovementSemantics(world.Block) movementblock.MovementSemantics {
	return movementblock.MovementSemantics{
		GroundFriction: s.groundFriction,
		Climbable:      s.climbable,
		Cobweb:         s.cobweb,
		Bounce:         s.bounce,
	}
}

func TestSimulatorCompleteBlockSemanticsProvider(t *testing.T) {
	sim := &Simulator{
		BlockSemantics: extendedMovementSemantics{
			groundFriction: 0.37,
			climbable:      true,
			cobweb:         true,
			bounce:         movementblock.BounceBed,
		},
	}

	got := sim.blockMovementSemantics(block.Air{})
	if got.GroundFriction != 0.37 || !got.Climbable || !got.Cobweb ||
		got.Bounce != movementblock.BounceBed {
		t.Fatalf("got incomplete semantic bundle: %+v", got)
	}
}

func TestBambooDoesNotInvalidateSimulation(t *testing.T) {
	sim := &Simulator{World: blockMovementWorld{b: block.Bamboo{}}}
	if !sim.simulationIsReliable(newBaseState()) {
		t.Fatal("bamboo should use ordinary collision simulation")
	}
}

func TestSimulatorInvalidBlockSemanticsFrictionFallsBack(t *testing.T) {
	for name, friction := range map[string]float32{
		"zero":              0,
		"negative":          -0.42,
		"nan":               float32(math.NaN()),
		"positive infinity": float32(math.Inf(1)),
		"negative infinity": float32(math.Inf(-1)),
	} {
		t.Run(name, func(t *testing.T) {
			sim := &Simulator{
				BlockSemantics: extendedMovementSemantics{groundFriction: friction},
			}
			got := sim.blockMovementSemantics(block.Air{}).GroundFriction
			if got != DefaultBlockFriction {
				t.Fatalf("ground friction = %v, want %v", got, DefaultBlockFriction)
			}
		})
	}
}

type blockMovementWorld struct {
	b world.Block
}

func (w blockMovementWorld) Block(cube.Pos) world.Block {
	return w.b
}

func (blockMovementWorld) BlockCollisions(cube.Pos) []cube.BBox32 {
	return nil
}

func (blockMovementWorld) GetNearbyBBoxes(cube.BBox32) []cube.BBox32 {
	return nil
}

func (blockMovementWorld) IsChunkLoaded(int32, int32) bool {
	return true
}

func TestSimulateGroundUsesSoulSoilFriction(t *testing.T) {
	sim := &Simulator{
		World:   blockMovementWorld{b: block.SoulSoil{}},
		Effects: mockEffects{},
	}

	state := newBaseState()
	state.Pos = mgl32.Vec3{0, 1, 0}
	state.Client.Pos = state.Pos
	state.OnGround = true
	state.HasGravity = false
	state.Impulse = mgl32.Vec2{0, 0.98}

	result := sim.SimulateState(state)
	groundFriction := DefaultAirFriction * movementblock.Resolve(block.SoulSoil{}, BlockName(block.SoulSoil{})).GroundFriction
	moveRelativeSpeed := state.MovementSpeed *
		(0.16277136 / (groundFriction * groundFriction * groundFriction))
	wantZ := 0.98 * moveRelativeSpeed * groundFriction

	if math.Abs(float64(result.Velocity.Z()-wantZ)) > 1e-5 {
		t.Fatalf("ground velocity Z = %.8f, want %.8f", result.Velocity.Z(), wantZ)
	}
}
