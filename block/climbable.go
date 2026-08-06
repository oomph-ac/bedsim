package block

import (
	dfblock "github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/world"
)

type climbableBlock struct{}

func (climbableBlock) Matches(b world.Block, name string) bool {
	if _, ok := b.(dfblock.Ladder); ok {
		return true
	}

	switch name {
	case "minecraft:ladder", "minecraft:vine", "minecraft:cave_vines", "minecraft:cave_vines_body_with_berries", "minecraft:cave_vines_head_with_berries",
		"minecraft:twisting_vines", "minecraft:weeping_vines":
		return true
	default:
		return false
	}
}

func (climbableBlock) Apply(s *resolution) {
	s.Climbable = true
}
