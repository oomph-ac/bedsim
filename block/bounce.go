package block

import (
	dfblock "github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/world"
)

type Slime struct{}

func (Slime) Matches(b world.Block, name string) bool {
	if _, ok := b.(dfblock.Slime); ok {
		return true
	}
	return name == "minecraft:slime"
}

func (Slime) Apply(s *MovementSemantics) {
	s.Bounce = BounceSlime
}

func (Slime) Friction() (float32, bool) { return 0.8, true }

type Bed struct{}

func (Bed) Matches(b world.Block, name string) bool {
	if _, ok := b.(dfblock.Bed); ok {
		return true
	}
	return name == "minecraft:bed"
}

func (Bed) Apply(s *MovementSemantics) {
	s.Bounce = BounceBed
}
