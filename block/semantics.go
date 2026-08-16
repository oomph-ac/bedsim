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

// InsideMovement identifies velocity changes applied while an entity overlaps
// a block's volume.
type InsideMovement uint8

const (
	InsideMovementNone InsideMovement = iota
	InsideMovementSweetBerryBush
	InsideMovementPowderSnow
)

// Traversal identifies input-driven vertical movement supported by a block.
type Traversal uint8

const (
	TraversalNone Traversal = iota
	TraversalScaffolding
	TraversalPowderSnow
)

// MovementSemantics is the movement behavior resolved for a block.
type MovementSemantics struct {
	GroundFriction                           float32
	GroundAccelerationFrictionMultiplier     float32
	Climbable                                bool
	Cobweb                                   bool
	Honey                                    bool
	Bounce                                   Bounce
	InsideMovement                           InsideMovement
	Traversal                                Traversal
	SoulSpeedNeutralizesAccelerationFriction bool
}

// rule contributes movement behavior for a block family.
type rule interface {
	Matches(world.Block, string) bool
	Apply(resolution) resolution
}

type resolution struct {
	MovementSemantics
	groundFrictionSet bool
}

var rules = [...]rule{
	soulSand{},
	climbableBlock{},
	cobweb{},
	slime{},
	bed{},
	environmentRule{name: "minecraft:honey_block", honey: true},
	environmentRule{name: "minecraft:sweet_berry_bush", inside: InsideMovementSweetBerryBush},
	environmentRule{name: "minecraft:powder_snow", inside: InsideMovementPowderSnow, traversal: TraversalPowderSnow},
	environmentRule{name: "minecraft:scaffolding", traversal: TraversalScaffolding},
	frictionBlock{name: "minecraft:ice", friction: 0.98},
	frictionBlock{name: "minecraft:packed_ice", friction: 0.98},
	frictionBlock{name: "minecraft:blue_ice", friction: 0.989},
}

// Resolve returns built-in movement semantics for b.
func Resolve(b world.Block, name string) MovementSemantics {
	result := resolution{
		MovementSemantics: MovementSemantics{
			GroundAccelerationFrictionMultiplier: 1,
		},
	}
	if f, ok := b.(dfblock.Frictional); ok {
		result.GroundFriction = float32(f.Friction())
		result.groundFrictionSet = true
	}
	for _, rule := range rules {
		if rule.Matches(b, name) {
			result = rule.Apply(result)
		}
	}
	if !result.groundFrictionSet {
		result.GroundFriction = 0.6
	}
	return result.MovementSemantics
}

// Friction returns a block's ordinary friction.
func Friction(b world.Block, name string) float32 {
	return Resolve(b, name).GroundFriction
}
