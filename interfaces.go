package bedsim

import (
	dfcube "github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
)

// WorldProvider bridges the world/chunk system for collision and block lookups.
type WorldProvider interface {
	Block(pos dfcube.Pos) world.Block
	BlockCollisions(pos dfcube.Pos) []dfcube.BBox32
	GetNearbyBBoxes(aabb dfcube.BBox32) []dfcube.BBox32
	IsChunkLoaded(chunkX, chunkZ int32) bool
}

// LiquidProvider returns liquids from either block layer at a position.
type LiquidProvider interface {
	Liquid(pos dfcube.Pos) (world.Liquid, bool)
}

// BlockSemanticsProvider resolves movement-relevant block behavior. Implement
// this when names, friction, or climbability come from a per-world registry or
// custom block data instead of Dragonfly's default block types.
type BlockSemanticsProvider interface {
	BlockName(world.Block) string
	BlockFriction(world.Block) float32
	BlockClimbable(world.Block) bool
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
