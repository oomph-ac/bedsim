package block

import "github.com/df-mc/dragonfly/server/world"

type cobweb struct{}

func (cobweb) Matches(_ world.Block, name string) bool {
	return name == "minecraft:web"
}

func (cobweb) Apply(s *resolution) {
	s.Cobweb = true
}
