package bedsim

import (
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
)

// WorldProvider bridges the world/chunk system for collision and block lookups.
type WorldProvider interface {
	Block(pos cube.Pos) world.Block
	// BlockCollisions returns block-local collision boxes at pos.
	BlockCollisions(pos cube.Pos) []cube.BBox32
	GetNearbyBBoxes(aabb cube.BBox32) []cube.BBox32
	IsChunkLoaded(chunkX, chunkZ int32) bool
}

// LiquidProvider returns liquids from either block layer at a position.
type LiquidProvider interface {
	Liquid(pos cube.Pos) (world.Liquid, bool)
}

// BlockMovementSemanticsProvider resolves the complete movement-relevant
// behavior for a block. Implement this when block properties come from a
// per-world registry or custom block data instead of Dragonfly's defaults.
//
// GroundFriction is the post-adjustment value used by grounded travel. A
// non-positive or non-finite value falls back to bedsim's default resolution.
type BlockMovementSemanticsProvider interface {
	BlockMovementSemantics(world.Block) MovementBlockSemantics
}

// DefaultBlockSemantics uses bedsim's built-in Dragonfly-backed block helpers.
type DefaultBlockSemantics struct{}

// EffectsProvider bridges effect tracking (jump boost, levitation, slow falling, etc.).
type EffectsProvider interface {
	GetEffect(effectID int32) (amplifier int32, ok bool)
}

// InventoryProvider exposes equipment checks needed by movement (elytra, etc.).
type InventoryProvider interface {
	HasElytra() bool
}

// DepthStriderProvider exposes the equipped Depth Strider level.
type DepthStriderProvider interface {
	DepthStriderLevel() int
}
