package block

import "github.com/df-mc/dragonfly/server/world"

type environmentRule struct {
	name      string
	inside    InsideMovement
	traversal Traversal
}

func (r environmentRule) Matches(_ world.Block, name string) bool {
	return name == r.name
}

func (r environmentRule) Apply(s *resolution) {
	s.InsideMovement = r.inside
	s.Traversal = r.traversal
}
