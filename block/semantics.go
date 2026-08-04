// Package block owns BedSim's built-in movement semantics for blocks.
//
// It deliberately does not register blocks in Dragonfly's world registry.
// Registry ownership belongs to the application because registration affects
// runtime IDs and must happen before the registry is finalized.
package block

import (
	dfblock "github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/world"
)

// SoulGroundFrictionMultiplier is the native ground-friction adjustment used
// by SoulSandBlock::calcGroundFriction when Soul Speed is not active.
const SoulGroundFrictionMultiplier float32 = 1.225000023841858

// Bounce identifies the vanilla vertical response when an entity lands on a
// block.
type Bounce uint8

const (
	BounceNone Bounce = iota
	BounceSlime
	BounceBed
)

// MovementSemantics is the movement behavior owned by one resolved block.
// GroundFriction is supplied by Resolve so frictional blocks and block-owned
// adjustments are composed in one place.
type MovementSemantics struct {
	GroundFriction float32
	Climbable      bool
	Cobweb         bool
	Bounce         Bounce
}

// Owner contributes movement behavior for one family of blocks.
type Owner interface {
	Matches(world.Block, string) bool
	Apply(*MovementSemantics)
}

var owners = [...]Owner{
	SoulSand{},
	SoulSoil{},
	ClimbableBlock{},
	Cobweb{},
	Slime{},
	Bed{},
	frictionBlock{name: "minecraft:ice", friction: 0.98},
	frictionBlock{name: "minecraft:packed_ice", friction: 0.98},
	frictionBlock{name: "minecraft:blue_ice", friction: 0.99},
}

// Resolve returns the built-in movement semantics for b. name should be the
// caller's cached canonical block name; it lets custom block implementations
// participate without pretending to be Dragonfly concrete types.
func Resolve(b world.Block, name string, groundFriction float32) MovementSemantics {
	semantics := MovementSemantics{GroundFriction: groundFriction}
	for _, owner := range owners {
		if owner.Matches(b, name) {
			owner.Apply(&semantics)
		}
	}
	return semantics
}

// Friction returns the ordinary block friction before block-specific ground
// adjustments are applied.
func Friction(b world.Block, name string) float32 {
	if f, ok := b.(dfblock.Frictional); ok {
		return float32(f.Friction())
	}

	for _, owner := range owners {
		frictionOwner, ok := owner.(interface{ Friction() (float32, bool) })
		if !ok || !owner.Matches(b, name) {
			continue
		}
		if friction, ok := frictionOwner.Friction(); ok {
			return friction
		}
	}
	return 0.6
}
