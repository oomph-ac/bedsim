package block

import (
	dfblock "github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/world"
)

type slime struct{}

func (slime) Matches(b world.Block, name string) bool {
	if _, ok := b.(dfblock.Slime); ok {
		return true
	}
	return name == "minecraft:slime"
}

func (slime) Apply(s resolution) resolution {
	if !s.groundFrictionSet {
		s.GroundFriction = 0.8
		s.groundFrictionSet = true
	}
	s.Bounce = BounceSlime
	return s
}

type bed struct{}

func (bed) Matches(b world.Block, name string) bool {
	if _, ok := b.(dfblock.Bed); ok {
		return true
	}
	return name == "minecraft:bed"
}

func (bed) Apply(s resolution) resolution {
	s.Bounce = BounceBed
	return s
}
