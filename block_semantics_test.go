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

func TestSoulBlocksKeepOrdinaryGroundFriction(t *testing.T) {
	for name, tt := range map[string]struct {
		block                world.Block
		accelScale           float32
		soulSpeedNeutralizes bool
	}{
		"soul sand": {block: block.SoulSand{}, accelScale: movementblock.SoulSandAccelerationFrictionMultiplier, soulSpeedNeutralizes: true},
		"soul soil": {block: block.SoulSoil{}, accelScale: 1},
	} {
		t.Run(name, func(t *testing.T) {
			got := movementblock.Resolve(tt.block, BlockName(tt.block))
			if got.GroundFriction != DefaultBlockFriction {
				t.Fatalf("ground friction = %.8f, want %.8f", got.GroundFriction, DefaultBlockFriction)
			}
			if got.GroundAccelerationFrictionMultiplier != tt.accelScale {
				t.Fatalf("ground acceleration friction multiplier = %.8f, want %.8f", got.GroundAccelerationFrictionMultiplier, tt.accelScale)
			}
			if got.SoulSpeedNeutralizesAccelerationFriction != tt.soulSpeedNeutralizes {
				t.Fatalf("soul-speed neutralization = %t, want %t", got.SoulSpeedNeutralizesAccelerationFriction, tt.soulSpeedNeutralizes)
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
			name:       "registry-backed ladder",
			block:      semanticsNamedBlock{"minecraft:ladder"},
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
			groundWant: DefaultBlockFriction,
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

func TestBlueIceFrictionMatchesAcrossBlockRepresentations(t *testing.T) {
	for name, b := range map[string]world.Block{
		"dragonfly":       block.BlueIce{},
		"registry-backed": semanticsNamedBlock{"minecraft:blue_ice"},
	} {
		t.Run(name, func(t *testing.T) {
			if got := movementblock.Resolve(b, BlockName(b)).GroundFriction; got != 0.989 {
				t.Fatalf("blue ice friction = %.8f, want 0.989", got)
			}
		})
	}
}

func TestFrostedIceFrictionMatchesVanillaIce(t *testing.T) {
	if got := movementblock.Resolve(semanticsNamedBlock{"minecraft:frosted_ice"}, "minecraft:frosted_ice").GroundFriction; got != 0.98 {
		t.Fatalf("frosted ice friction = %.8f, want 0.98", got)
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

func TestEnvironmentMovementSemantics(t *testing.T) {
	tests := []struct {
		name      string
		inside    movementblock.InsideMovement
		traversal movementblock.Traversal
		honey     bool
	}{
		{name: "minecraft:honey_block", honey: true},
		{name: "minecraft:sweet_berry_bush", inside: movementblock.InsideMovementSweetBerryBush},
		{name: "minecraft:powder_snow", inside: movementblock.InsideMovementPowderSnow, traversal: movementblock.TraversalPowderSnow},
		{name: "minecraft:scaffolding", traversal: movementblock.TraversalScaffolding},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := movementblock.Resolve(semanticsNamedBlock{tt.name}, tt.name)
			if got.InsideMovement != tt.inside || got.Traversal != tt.traversal || got.Honey != tt.honey {
				t.Fatalf("semantics = %+v, want inside=%v traversal=%v honey=%v", got, tt.inside, tt.traversal, tt.honey)
			}
		})
	}
}

func TestHoneyBlockFrictionMatchesVanilla(t *testing.T) {
	got := movementblock.Resolve(semanticsNamedBlock{"minecraft:honey_block"}, "minecraft:honey_block")
	if got.GroundFriction != 0.8 {
		t.Fatalf("honey block friction = %.8f, want 0.8", got.GroundFriction)
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
	groundFriction                       float32
	groundAccelerationFrictionMultiplier float32
	climbable                            bool
	cobweb                               bool
	bounce                               movementblock.Bounce
}

func (s extendedMovementSemantics) BlockMovementSemantics(world.Block) movementblock.MovementSemantics {
	return movementblock.MovementSemantics{
		GroundFriction:                       s.groundFriction,
		GroundAccelerationFrictionMultiplier: s.groundAccelerationFrictionMultiplier,
		Climbable:                            s.climbable,
		Cobweb:                               s.cobweb,
		Bounce:                               s.bounce,
	}
}

func TestSimulatorCompleteBlockSemanticsProvider(t *testing.T) {
	sim := &Simulator{
		BlockSemantics: extendedMovementSemantics{
			groundFriction:                       0.37,
			groundAccelerationFrictionMultiplier: 1.25,
			climbable:                            true,
			cobweb:                               true,
			bounce:                               movementblock.BounceBed,
		},
	}

	got := sim.blockMovementSemantics(block.Air{})
	if got.GroundFriction != 0.37 || got.GroundAccelerationFrictionMultiplier != 1.25 || !got.Climbable || !got.Cobweb ||
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

func TestSimulateSoulGroundSeparatesAccelerationFromDrag(t *testing.T) {
	for name, tt := range map[string]struct {
		block                      world.Block
		accelerationFrictionFactor float32
	}{
		"soul sand": {block: block.SoulSand{}, accelerationFrictionFactor: 1.225000023841858},
		"soul soil": {block: block.SoulSoil{}, accelerationFrictionFactor: 1},
	} {
		t.Run(name, func(t *testing.T) {
			sim := &Simulator{
				World:   blockMovementWorld{b: tt.block},
				Effects: mockEffects{},
			}

			state := newBaseState()
			state.Pos = mgl32.Vec3{0, 1, 0}
			state.Client.Pos = state.Pos
			state.OnGround = true
			state.HasGravity = false
			state.Impulse = mgl32.Vec2{0, 0.98}

			result := sim.SimulateState(state)
			groundFriction := DefaultAirFriction * DefaultBlockFriction
			accelerationFriction := groundFriction * tt.accelerationFrictionFactor
			moveRelativeSpeed := state.MovementSpeed *
				(0.16277136 / (accelerationFriction * accelerationFriction * accelerationFriction))
			wantZ := 0.98 * moveRelativeSpeed * groundFriction

			if math.Abs(float64(result.Velocity.Z()-wantZ)) > 1e-5 {
				t.Fatalf("ground velocity Z = %.8f, want %.8f", result.Velocity.Z(), wantZ)
			}
		})
	}
}
