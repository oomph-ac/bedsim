package bedsim

import (
	"github.com/chewxy/math32"

	"github.com/df-mc/dragonfly/server/block/cube"
	movementblock "github.com/oomph-ac/bedsim/block"
)

func applyInsideBlockMovement(state *MovementState, movement movementblock.InsideMovement) {
	velocity := state.Vel
	switch movement {
	case movementblock.InsideMovementHoney:
		velocity[0] *= 0.4
		velocity[1] = max(-0.12, velocity[1])
		velocity[2] *= 0.4
	case movementblock.InsideMovementSweetBerryBush:
		velocity[0] *= 0.8
		velocity[1] *= 0.75
		velocity[2] *= 0.8
	case movementblock.InsideMovementPowderSnow:
		velocity[0] *= 0.9
		velocity[1] *= 1.5
		velocity[2] *= 0.9
	}
	state.SetVel(velocity)
}

func applyAscendableMovement(state *MovementState, traversal movementblock.Traversal, leatherBoots bool) {
	velocity := state.Vel
	switch traversal {
	case movementblock.TraversalScaffolding:
		if state.PressingDescend {
			velocity[1] = -0.15
		} else if state.PressingAscend {
			velocity[1] = 0.15
		}
	case movementblock.TraversalPowderSnow:
		if state.PressingDescend {
			velocity[1] = -0.15
		} else if state.PressingAscend && leatherBoots {
			velocity[1] = 0.2
		}
	}
	state.SetVel(velocity)
}

func (s *Simulator) applyInsideBlockEffects(state *MovementState) {
	if s.World == nil {
		return
	}
	bb := state.BoundingBox(s.Options.UseSlideOffset)
	min, maxPoint := bb.Min(), bb.Max()
	for x := int(math32.Floor(min.X())); x < int(math32.Ceil(maxPoint.X())); x++ {
		for y := int(math32.Floor(min.Y())); y < int(math32.Ceil(maxPoint.Y())); y++ {
			for z := int(math32.Floor(min.Z())); z < int(math32.Ceil(maxPoint.Z())); z++ {
				pos := cube.Pos{x, y, z}
				if !bb.IntersectsWith(cube.Box32(0, 0, 0, 1, 1, 1).Translate(posVec3(pos))) {
					continue
				}
				b := s.World.Block(pos)
				semantics := s.blockMovementSemantics(b)
				if semantics.Cobweb {
					continue
				}
				applyInsideBlockMovement(state, semantics.InsideMovement)
			}
		}
	}
}
