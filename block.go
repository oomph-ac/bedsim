package bedsim

import (
	"math"
	"sync"

	"github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
)

// SoulSandGroundFrictionMultiplier is the native friction adjustment used by
// SoulSandBlock::calcGroundFriction when Soul Speed is not active. Soul sand
// and soul soil share this ground-friction path.
const SoulSandGroundFrictionMultiplier float32 = 1.225000023841858

type blockNameKey struct {
	base, state uint64
}

var blockNameCache sync.Map

// BlockName returns the canonical name of a block.
func BlockName(b world.Block) string {
	base, state := b.Hash()
	if base == 0 && state == math.MaxUint64 {
		name, _ := b.EncodeBlock()
		return name
	}

	key := blockNameKey{base: base, state: state}
	if name, ok := blockNameCache.Load(key); ok {
		return name.(string)
	}

	name, _ := b.EncodeBlock()
	stored, _ := blockNameCache.LoadOrStore(key, name)
	return stored.(string)
}

// BlockFriction returns the friction of the block.
func BlockFriction(b world.Block) float32 {
	if f, ok := b.(block.Frictional); ok {
		return float32(f.Friction())
	}

	switch BlockName(b) {
	case "minecraft:slime":
		return 0.8
	case "minecraft:ice", "minecraft:packed_ice":
		return 0.98
	case "minecraft:blue_ice":
		return 0.99
	default:
		return 0.6
	}
}

// BlockGroundFriction returns the friction used by ordinary grounded travel.
// It is intentionally separate from BlockFriction: most blocks expose their
// ordinary friction directly, while soul sand and soul soil apply a movement
// specific adjustment in the native travel path.
func BlockGroundFriction(b world.Block) float32 {
	friction := BlockFriction(b)
	if isSoulGroundBlock(b, BlockName(b)) {
		friction *= SoulSandGroundFrictionMultiplier
	}
	return friction
}

func isSoulGroundBlock(b world.Block, name string) bool {
	switch b.(type) {
	case block.SoulSand, block.SoulSoil:
		return true
	}
	return name == "minecraft:soul_sand" || name == "minecraft:soul_soil"
}

// BlockClimbable returns whether the given block is climbable.
func BlockClimbable(b world.Block) bool {
	switch b.(type) {
	case block.Ladder:
		return true
	}

	switch BlockName(b) {
	case "minecraft:vine", "minecraft:cave_vines", "minecraft:cave_vines_body_with_berries", "minecraft:cave_vines_head_with_berries",
		"minecraft:twisting_vines", "minecraft:weeping_vines":
		return true
	default:
		return false
	}
}

// BlockCobweb reports whether the block applies the cobweb movement slowdown.
// "web" is the canonical Bedrock identifier; accepting "cobweb" as well
// keeps custom registries and older adapters interoperable.
func BlockCobweb(b world.Block) bool {
	switch BlockName(b) {
	case "minecraft:web", "minecraft:cobweb":
		return true
	default:
		return false
	}
}

// MovementBounce identifies the vanilla vertical response when an entity
// lands on a block.
type MovementBounce uint8

const (
	MovementBounceNone MovementBounce = iota
	MovementBounceSlime
	MovementBounceBed
)

// MovementBlockSemantics is the complete set of block properties consumed by
// the movement integrator. A custom registry must return all properties from
// one world-consistent snapshot; zero values mean ordinary block behaviour.
type MovementBlockSemantics struct {
	GroundFriction float32
	Climbable      bool
	Cobweb         bool
	Bounce         MovementBounce

	// Unsupported marks blocks whose collision/contact behaviour is not safe
	// for authoritative simulation. Bamboo is the built-in example.
	Unsupported bool
}

// DefaultMovementBlockSemantics resolves the vanilla movement properties for
// a block using the built-in Dragonfly-backed registry.
func DefaultMovementBlockSemantics(b world.Block) MovementBlockSemantics {
	semantics := MovementBlockSemantics{
		GroundFriction: BlockGroundFriction(b),
		Climbable:      BlockClimbable(b),
		Cobweb:         BlockCobweb(b),
	}

	switch BlockName(b) {
	case "minecraft:slime":
		semantics.Bounce = MovementBounceSlime
	case "minecraft:bed":
		semantics.Bounce = MovementBounceBed
	case "minecraft:bamboo":
		semantics.Unsupported = true
	}
	return semantics
}

// BlockSupportHeight returns the effective standing surface height for a ground
// block by sampling its collision boxes at the block centre (0.5, 0.5).
// This handles slabs, stairs, and any other sub-block geometry correctly.
func BlockSupportHeight(b world.Block, pos cube.Pos, src world.BlockSource) float32 {
	boxes := b.Model().BBox(pos, src)
	maxY := float32(-1)
	for _, box := range boxes {
		min := box.Min()
		max := box.Max()
		if min[0] <= 0.5 && max[0] >= 0.5 && min[2] <= 0.5 && max[2] >= 0.5 {
			if top := float32(max[1]); top > maxY {
				maxY = top
			}
		}
	}
	if maxY >= 0 {
		return maxY
	}
	return 1.0
}

// IsFence returns true if the block is a fence.
func IsFence(b world.Block) bool {
	switch b.(type) {
	case block.WoodFence, block.WoodFenceGate, block.NetherBrickFence:
		return true
	default:
		return false
	}
}

// IsWall returns true if the block is a wall.
func IsWall(b world.Block) bool {
	_, ok := b.(block.Wall)
	return ok
}
