package block

import (
	dfblock "github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/world"
)

type soulSand struct{}

func (soulSand) Matches(b world.Block, name string) bool {
	if _, ok := b.(dfblock.SoulSand); ok {
		return true
	}
	return name == "minecraft:soul_sand"
}

func (soulSand) Apply(s *resolution) {
	s.GroundAccelerationFrictionMultiplier *= SoulSandAccelerationFrictionMultiplier
}
