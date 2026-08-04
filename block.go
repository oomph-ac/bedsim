package bedsim

import (
	"math"
	"sync"

	"github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
	movementblock "github.com/oomph-ac/bedsim/block"
)

// SoulSandGroundFrictionMultiplier is retained as a root-package alias for
// the owner constant in bedsim/block.
const SoulSandGroundFrictionMultiplier = movementblock.SoulGroundFrictionMultiplier

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
	return movementblock.Friction(b, BlockName(b))
}

// BlockGroundFriction returns the friction used by ordinary grounded travel.
// It is intentionally separate from BlockFriction: most blocks expose their
// ordinary friction directly, while soul sand and soul soil apply a movement
// specific adjustment in the native travel path.
func BlockGroundFriction(b world.Block) float32 {
	return DefaultMovementBlockSemantics(b).GroundFriction
}

// BlockClimbable returns whether the given block is climbable.
func BlockClimbable(b world.Block) bool {
	return DefaultMovementBlockSemantics(b).Climbable
}

// BlockCobweb reports whether the block applies the cobweb movement slowdown.
// "web" is the canonical Bedrock identifier; accepting "cobweb" as well
// keeps custom registries and older adapters interoperable.
func BlockCobweb(b world.Block) bool {
	return DefaultMovementBlockSemantics(b).Cobweb
}

// MovementBounce identifies the vanilla vertical response when an entity
// lands on a block.
type MovementBounce = movementblock.Bounce

const (
	MovementBounceNone  = movementblock.BounceNone
	MovementBounceSlime = movementblock.BounceSlime
	MovementBounceBed   = movementblock.BounceBed
)

// MovementBlockSemantics is the complete set of block properties consumed by
// the movement integrator. A custom registry must return all properties from
// one world-consistent snapshot; zero values mean ordinary block behaviour.
type MovementBlockSemantics = movementblock.MovementSemantics

// DefaultMovementBlockSemantics resolves the vanilla movement properties for
// a block using the built-in Dragonfly-backed registry.
func DefaultMovementBlockSemantics(b world.Block) MovementBlockSemantics {
	return movementblock.Resolve(b, BlockName(b), BlockFriction(b))
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
