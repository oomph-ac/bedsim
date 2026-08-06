package block

import "github.com/df-mc/dragonfly/server/world"

type frictionBlock struct {
	name     string
	friction float32
}

func (b frictionBlock) Matches(_ world.Block, name string) bool {
	return name == b.name
}

func (b frictionBlock) Apply(s *resolution) {
	if !s.groundFrictionSet {
		s.GroundFriction = b.friction
		s.groundFrictionSet = true
	}
}
