package bedsim

import (
	"github.com/chewxy/math32"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/go-gl/mathgl/mgl32"
	movementblock "github.com/oomph-ac/bedsim/block"
)

// ScaffoldingSpeed is the vertical speed for ascending or descending through
// scaffolding and, with leather boots, powder snow.
const ScaffoldingSpeed = float32(0.15)

// climberFlags are the per-tick climbing states derived before travel.
type climberFlags struct {
	inScaffolding, overScaffolding bool
	// inAscendable and overDescendable cover scaffolding standing on a non-air,
	// non-water block, and powder snow when leather boots let the player stand on it.
	inAscendable, overDescendable bool
	// climbable is a ladder-path block in the single feet cell.
	climbable bool
	// holdOnSneak reports whether sneaking stops the descent on that block.
	holdOnSneak bool
}

// climberFlags samples the AABB footprint at the feet layer and the layer below.
func (s *Simulator) climberFlags(state *MovementState, leatherBoots bool) climberFlags {
	var flags climberFlags
	bb := state.BoundingBox(s.Options.UseSlideOffset)
	feetY := bb.Min().Y()
	flags.inScaffolding, flags.inAscendable = s.climberLayer(bb, int(math32.Floor(feetY)), leatherBoots)
	flags.overScaffolding, flags.overDescendable = s.climberLayer(bb, int(math32.Floor(feetY+(-1))), leatherBoots)
	flags.climbable, flags.holdOnSneak = s.climbableAtFeet(state, leatherBoots)
	return flags
}

// climberLayer reports scaffolding and ascendable blocks in one footprint layer.
func (s *Simulator) climberLayer(bb cube.BBox32, y int, leatherBoots bool) (scaffolding, ascendable bool) {
	if s.World == nil {
		return false, false
	}
	minX, maxX := int(math32.Floor(bb.Min().X())), int(math32.Floor(bb.Max().X()))
	minZ, maxZ := int(math32.Floor(bb.Min().Z())), int(math32.Floor(bb.Max().Z()))
	for x := minX; x <= maxX; x++ {
		for z := minZ; z <= maxZ; z++ {
			pos := cube.Pos{x, y, z}
			switch s.blockMovementSemantics(s.World.Block(pos)).Traversal {
			case movementblock.TraversalScaffolding:
				scaffolding = true
				if !ascendable {
					below := s.World.Block(pos.Side(cube.FaceDown))
					name := BlockName(below)
					ascendable = !s.blockAir(below) && name != "minecraft:water" && name != "minecraft:flowing_water"
				}
			case movementblock.TraversalPowderSnow:
				ascendable = ascendable || leatherBoots
			}
		}
	}
	return scaffolding, ascendable
}

// climbableAtFeet reports a ladder-path block in the feet cell; powder snow
// qualifies only with leather boots and never holds a sneaking player.
func (s *Simulator) climbableAtFeet(state *MovementState, leatherBoots bool) (climbable, holdOnSneak bool) {
	semantics := s.blockMovementSemantics(s.blockAtPos(posFromVec3(state.Pos)))
	if s.climbableContact(state, semantics.Climbable) {
		return true, true
	}
	return leatherBoots && semantics.Traversal == movementblock.TraversalPowderSnow, false
}

// applyClimbJump runs the held-jump climbing branches that take precedence over
// a ground jump, and reports whether one of them consumed the jump.
func applyClimbJump(state *MovementState, flags climberFlags) bool {
	if !state.EffectiveJumping && !state.PressingAscend && !state.Jumping {
		return false
	}
	velocity := state.Vel
	switch {
	case (flags.inScaffolding || flags.inAscendable) && !state.descendThroughBlock:
		velocity[1] = ScaffoldingSpeed
		state.JumpDelay = JumpDelayTicks
	case flags.climbable:
		velocity[1] = ClimbSpeed
	default:
		return false
	}
	state.SetVel(velocity)
	return true
}

// applyLadderTravel clamps descent on a climbable block and lets sneaking hold position.
func applyLadderTravel(state *MovementState, flags climberFlags) {
	if !flags.climbable {
		return
	}
	velocity := state.Vel
	if velocity[1] < -ClimbSpeed {
		velocity[1] = -ClimbSpeed
	}
	if flags.holdOnSneak && state.Sneaking && velocity[1] < 0 {
		velocity[1] = 0
	}
	state.SetVel(velocity)
}

// applyAutoClimb lifts a player pushing into a wall from a climbable block and
// reports whether it did, which suppresses that tick's gravity and vertical drag.
func (s *Simulator) applyAutoClimb(state *MovementState) bool {
	if !state.CollideX && !state.CollideZ {
		return false
	}
	leatherBoots := s.Equipment != nil && s.Equipment.WearingLeatherBoots()
	if climbable, _ := s.climbableAtFeet(state, leatherBoots); !climbable {
		return false
	}
	state.SetVel(mgl32.Vec3{state.Vel.X(), ClimbSpeed, state.Vel.Z()})
	return true
}
