package block

import (
	"testing"

	"github.com/df-mc/dragonfly/server/world"
)

type countingRule struct {
	matches int
}

func (r *countingRule) Matches(world.Block, string) bool {
	r.matches++
	return true
}

func (*countingRule) Apply(s resolution) resolution {
	s.GroundFriction = 0.7
	s.groundFrictionSet = true
	return s
}

func TestResolveVisitsEachRuleOnce(t *testing.T) {
	original := rules[0]
	rule := &countingRule{}
	rules[0] = rule
	t.Cleanup(func() { rules[0] = original })

	Resolve(namedBlock{"minecraft:test"}, "minecraft:test")
	if rule.matches != 1 {
		t.Fatalf("rule matches called %d times, want 1", rule.matches)
	}
}

type namedBlock struct {
	name string
}

func (b namedBlock) EncodeBlock() (string, map[string]any) {
	return b.name, nil
}

func (namedBlock) Hash() (uint64, uint64) {
	return 0, 0
}

func (namedBlock) Model() world.BlockModel {
	return nil
}
