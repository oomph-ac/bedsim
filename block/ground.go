package block

import (
	dfblock "github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/world"
)

// SoulSand owns the ground-friction adjustment shared by Soul Sand and its
// Soul Soil counterpart in vanilla movement.
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

// SoulSoil owns the same ground-friction adjustment as Soul Sand.
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
