package bedsim

import (
	"math"
	"sync"

	"github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
)

type blockNameKey struct {
	base, state uint64
}

var blockNameCache sync.Map

// BlockName returns the canonical name of a block.
func BlockName(b world.Block) string {
	base, state := b.Hash()
	if state == math.MaxUint64 {
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
