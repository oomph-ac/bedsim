package block

import (
	dfblock "github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/world"
)

type SoulSand struct{}

func (SoulSand) Matches(b world.Block, name string) bool {
	if _, ok := b.(dfblock.SoulSand); ok {
		return true
	}
	return name == "minecraft:soul_sand"
}

func (SoulSand) Apply(s *MovementSemantics) {
	s.GroundFriction *= SoulGroundFrictionMultiplier
}

type SoulSoil struct{}

func (SoulSoil) Matches(b world.Block, name string) bool {
	if _, ok := b.(dfblock.SoulSoil); ok {
		return true
	}
	return name == "minecraft:soul_soil"
}

func (SoulSoil) Apply(s *MovementSemantics) {
	s.GroundFriction *= SoulGroundFrictionMultiplier
}
