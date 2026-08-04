package block

import "github.com/df-mc/dragonfly/server/world"

type frictionBlock struct {
	name     string
	friction float32
}

func (b frictionBlock) Matches(_ world.Block, name string) bool {
	return name == b.name
}

func (frictionBlock) Apply(*MovementSemantics) {}

func (b frictionBlock) Friction() (float32, bool) {
	return b.friction, true
}
