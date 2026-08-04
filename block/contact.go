package block

import "github.com/df-mc/dragonfly/server/world"

type Cobweb struct{}

func (Cobweb) Matches(_ world.Block, name string) bool {
	return name == "minecraft:web" || name == "minecraft:cobweb"
}

func (Cobweb) Apply(s *MovementSemantics) {
	s.Cobweb = true
}
