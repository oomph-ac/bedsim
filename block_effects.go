package bedsim

import (
	"github.com/chewxy/math32"

	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/go-gl/mathgl/mgl32"
	movementblock "github.com/oomph-ac/bedsim/block"
)

func applyInsideBlockMovement(state *MovementState, movement movementblock.InsideMovement) {
	switch movement {
	case movementblock.InsideMovementSweetBerryBush:
		queueStuckSpeedMultiplier(state, mgl32.Vec3{0.8, 0.75, 0.8})
	case movementblock.InsideMovementPowderSnow:
		queueStuckSpeedMultiplier(state, mgl32.Vec3{0.9, 1.5, 0.9})
	}
}

func queueStuckSpeedMultiplier(state *MovementState, multiplier mgl32.Vec3) {
	queued := state.StuckSpeedMultiplier
	if queued.LenSqr() <= 1e-7 {
		state.StuckSpeedMultiplier = multiplier
		return
	}
	for axis := range 3 {
		queued[axis] = min(queued[axis], multiplier[axis])
	}
	state.StuckSpeedMultiplier = queued
}

func applyStuckSpeedMultiplier(state *MovementState) bool {
	multiplier := state.StuckSpeedMultiplier
	if multiplier.LenSqr() <= 1e-7 {
		return false
	}
	if state.NoClip {
		state.StuckSpeedMultiplier = mgl32.Vec3{}
		return false
	}
	state.SetVel(mgl32.Vec3{
		state.Vel.X() * multiplier.X(),
		state.Vel.Y() * multiplier.Y(),
		state.Vel.Z() * multiplier.Z(),
	})
	state.StuckSpeedMultiplier = mgl32.Vec3{}
	return true
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
				if s.blockAir(b) {
					continue
				}
				semantics := s.blockMovementSemantics(b)
				applyInsideBlockMovement(state, semantics.InsideMovement)
			}
		}
	}
	s.applyHoneyWallSlide(state)
}

func (s *Simulator) applyHoneyWallSlide(state *MovementState) {
	if !state.CollideX && !state.CollideZ {
		return
	}
	bb := state.BoundingBox(s.Options.UseSlideOffset).GrowVec3(mgl32.Vec3{1e-3, 0, 1e-3})
	min, maxPoint := bb.Min(), bb.Max()
	for x := int(math32.Floor(min.X())); x < int(math32.Ceil(maxPoint.X())); x++ {
		for y := int(math32.Floor(min.Y())); y < int(math32.Ceil(maxPoint.Y())); y++ {
			for z := int(math32.Floor(min.Z())); z < int(math32.Ceil(maxPoint.Z())); z++ {
				pos := cube.Pos{x, y, z}
				if !bb.IntersectsWith(cube.Box32(0, 0, 0, 1, 1, 1).Translate(posVec3(pos))) {
					continue
				}
				if s.blockMovementSemantics(s.World.Block(pos)).Honey {
					velocity := state.Vel
					velocity[0] *= 0.4
					velocity[1] = max(-0.12, velocity[1])
					velocity[2] *= 0.4
					state.SetVel(velocity)
					return
				}
			}
		}
	}
}
