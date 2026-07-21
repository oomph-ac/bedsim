package bedsim

import (
	"github.com/df-mc/dragonfly/server/block"
	dfcube "github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/ethaniccc/float32-cube/cube"
)

type environmentWorld struct {
	bubbles map[dfcube.Pos]BubbleColumnDirection
	solids  map[dfcube.Pos]bool
	blocks  map[dfcube.Pos]world.Block
}

func (w environmentWorld) Block(pos dfcube.Pos) world.Block {
	if b, ok := w.blocks[pos]; ok {
		return b
	}
	if w.solids[pos] {
		return block.Stone{}
	}
	return block.Air{}
}

func (w environmentWorld) BlockCollisions(pos dfcube.Pos) []cube.BBox {
	if w.solids[pos] {
		return []cube.BBox{cube.Box(0, 0, 0, 1, 1, 1).Translate(posVec3(pos))}
	}
	return nil
}

func (w environmentWorld) GetNearbyBBoxes(cube.BBox) []cube.BBox { return nil }
func (w environmentWorld) IsChunkLoaded(int32, int32) bool       { return true }

func (w environmentWorld) Liquid(pos dfcube.Pos) (world.Liquid, bool) {
	liquid, ok := w.Block(pos).(world.Liquid)
	return liquid, ok
}

func (w environmentWorld) BubbleColumn(pos dfcube.Pos) (BubbleColumnDirection, bool) {
	direction, ok := w.bubbles[pos]
	return direction, ok
}

type fixedEquipment map[MovementEnchantment]int

func (e fixedEquipment) EnchantmentLevel(enchantment MovementEnchantment) int {
	return e[enchantment]
}

func (fixedEquipment) WearingLeatherBoots() bool { return false }
