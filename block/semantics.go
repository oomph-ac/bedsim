// Package block provides BedSim's built-in movement semantics.
package block

import (
	dfblock "github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/world"
)

// SoulGroundFrictionMultiplier is the vanilla soul-ground adjustment.
const SoulGroundFrictionMultiplier float32 = 1.225000023841858

// Bounce identifies a block's landing response.
type Bounce uint8

const (
	BounceNone Bounce = iota
	BounceSlime
	BounceBed
)

// MovementSemantics is the movement behavior resolved for a block.
type MovementSemantics struct {
	GroundFriction float32
	Climbable      bool
	Cobweb         bool
	Bounce         Bounce
}

// Owner contributes movement behavior for a block family.
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

// Resolve returns built-in movement semantics for b.
func Resolve(b world.Block, name string) MovementSemantics {
	semantics := MovementSemantics{GroundFriction: Friction(b, name)}
	for _, owner := range owners {
		if owner.Matches(b, name) {
			owner.Apply(&semantics)
		}
	}
	return semantics
}

// Friction returns a block's ordinary friction.
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
