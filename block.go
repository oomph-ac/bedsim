package bedsim

import (
	"math"
	"sync"

	"github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
)

var (
	blockNameMapping     map[uint64]string
	blockNameMappingOnce sync.Once
)

func initBlockNameMapping() {
	blockNameMapping = make(map[uint64]string, len(world.Blocks()))
	for _, b := range world.Blocks() {
		x, y := b.Hash()
		if x == 0 && y == math.MaxUint64 {
			continue
		}
		name, _ := b.EncodeBlock()
		blockNameMapping[world.BlockHash(b)] = name
	}
}

// BlockName returns the canonical name of a block.
func BlockName(b world.Block) string {
	blockNameMappingOnce.Do(initBlockNameMapping)
	if n, ok := blockNameMapping[world.BlockHash(b)]; ok {
		return n
	}
	n, _ := b.EncodeBlock()
	return n
}

// BlockFriction returns the friction of the block.
func BlockFriction(b world.Block) float64 {
	if f, ok := b.(block.Frictional); ok {
		return f.Friction()
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
