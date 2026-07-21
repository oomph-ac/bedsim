package bedsim

import (
	"github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
)

type environmentWorld struct {
	bubbles map[cube.Pos]BubbleColumnDirection
	solids  map[cube.Pos]bool
	blocks  map[cube.Pos]world.Block
}

func (w environmentWorld) Block(pos cube.Pos) world.Block {
	if b, ok := w.blocks[pos]; ok {
		return b
	}
	if w.solids[pos] {
		return block.Stone{}
	}
	return block.Air{}
}

func (w environmentWorld) BlockCollisions(pos cube.Pos) []cube.BBox {
	if w.solids[pos] {
		return []cube.BBox{cube.Box(0, 0, 0, 1, 1, 1).Translate(pos.Vec3())}
	}
	return nil
}

func (w environmentWorld) GetNearbyBBoxes(cube.BBox) []cube.BBox { return nil }
func (w environmentWorld) IsChunkLoaded(int32, int32) bool       { return true }

func (w environmentWorld) Liquid(pos cube.Pos) (world.Liquid, bool) {
	liquid, ok := w.Block(pos).(world.Liquid)
	return liquid, ok
}

func (w environmentWorld) BubbleColumn(pos cube.Pos) (BubbleColumnDirection, bool) {
	direction, ok := w.bubbles[pos]
	return direction, ok
}

type fixedEquipment map[MovementEnchantment]int

func (e fixedEquipment) EnchantmentLevel(enchantment MovementEnchantment) int {
	return e[enchantment]
}

func (fixedEquipment) WearingLeatherBoots() bool { return false }
