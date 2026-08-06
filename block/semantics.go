// Package block provides BedSim's built-in movement semantics.
package block

import (
	dfblock "github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/world"
)

// SoulSandAccelerationFrictionMultiplier is the native adjustment used when
// calculating grounded acceleration on soul sand. Ordinary drag still uses the
// block's unadjusted friction. This exact native value replaces the legacy
// 0.543 speed approximation.
const SoulSandAccelerationFrictionMultiplier float32 = 1.225000023841858

// Bounce identifies a block's landing response.
type Bounce uint8

const (
	BounceNone Bounce = iota
	BounceSlime
	BounceBed
)

// MovementSemantics is the movement behavior resolved for a block.
type MovementSemantics struct {
	GroundFriction                       float32
	GroundAccelerationFrictionMultiplier float32
	Climbable                            bool
	Cobweb                               bool
	Bounce                               Bounce
}

// Owner contributes movement behavior for a block family.
type Owner interface {
	Matches(world.Block, string) bool
	Apply(*MovementSemantics)
}

var owners = [...]Owner{
	SoulSand{},
	ClimbableBlock{},
	Cobweb{},
	Slime{},
	Bed{},
	frictionBlock{name: "minecraft:ice", friction: 0.98},
	frictionBlock{name: "minecraft:packed_ice", friction: 0.98},
	frictionBlock{name: "minecraft:blue_ice", friction: 0.989},
}

// Resolve returns built-in movement semantics for b.
func Resolve(b world.Block, name string) MovementSemantics {
	semantics := MovementSemantics{
		GroundFriction:                       Friction(b, name),
		GroundAccelerationFrictionMultiplier: 1,
	}
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
