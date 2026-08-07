package bedsim

import (
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/oomph-ac/bedsim/block"
)

// WorldProvider bridges the world/chunk system for collision and block lookups.
type WorldProvider interface {
	Block(pos cube.Pos) world.Block
	// BlockCollisions returns block-local collision boxes at pos.
	BlockCollisions(pos cube.Pos) []cube.BBox32
	GetNearbyBBoxes(aabb cube.BBox32) []cube.BBox32
	IsChunkLoaded(chunkX, chunkZ int32) bool
}

// MovementAreaProvider can provide a precise loaded/known check for a swept
// movement volume in world space. Worlds that only expose chunk loading use BedSim's
// conservative chunk-range fallback.
type MovementAreaProvider interface {
	IsMovementAreaLoaded(aabb cube.BBox32) bool
}

// LiquidProvider returns liquids from either block layer at a position.
type LiquidProvider interface {
	Liquid(pos cube.Pos) (world.Liquid, bool)
}

// MovementCollisionContext contains player-dependent state needed by dynamic
// collision shapes such as scaffolding and powder snow.
type MovementCollisionContext struct {
	Position     [3]float32
	Sneaking     bool
	Descending   bool
	WantDown     bool
	LeatherBoots bool
}

// MovementCollisionProvider optionally resolves collision boxes whose shape
// depends on current player input or equipment.
type MovementCollisionProvider interface {
	GetMovementBBoxes(aabb cube.BBox32, context MovementCollisionContext) []cube.BBox32
}

// ClimbableContactProvider resolves orientation-aware ladder and vine contact.
// aabb is in world space.
// The built-in fallback scans intersecting block volumes when this is absent.
type ClimbableContactProvider interface {
	HasClimbableContact(aabb cube.BBox32) bool
}

// MovementSupportProvider resolves the exact support block for dynamic shapes.
// aabb is in world space.
// It is optional because a generic collision provider may not retain source
// block identities.
type MovementSupportProvider interface {
	SupportingBlock(aabb cube.BBox32, context MovementCollisionContext) (cube.Pos, bool)
}

// BlockMovementSemanticsProvider resolves the complete movement behavior for a
// block from a custom world registry or block data. GroundFriction and
// GroundAccelerationFrictionMultiplier must be finite and positive; invalid
// values fall back to BedSim's built-in semantics. Boolean and enum values are
// used as returned, including their zero values.
type BlockMovementSemanticsProvider interface {
	BlockMovementSemantics(world.Block) block.MovementSemantics
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

// MovementEnchantment identifies enchantments that directly affect movement.
type MovementEnchantment uint8

const (
	EnchantmentDepthStrider MovementEnchantment = iota
	EnchantmentSoulSpeed
	EnchantmentSwiftSneak
	EnchantmentRiptide
)

// MovementEquipmentProvider exposes equipment and enchantments whose effects
// are part of client movement physics.
type MovementEquipmentProvider interface {
	EnchantmentLevel(enchantment MovementEnchantment) int
	WearingLeatherBoots() bool
}
