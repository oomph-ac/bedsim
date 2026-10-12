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

// insideCellRange returns the inclusive block range an entity occupies. Cells
// overlapped by 0.001 or less on any axis are not inside.
func insideCellRange(bb cube.BBox32) (minPos, maxPos cube.Pos) {
	min, max := bb.Min(), bb.Max()
	for axis := range 3 {
		minPos[axis] = int(math32.Floor(min[axis] + 0.001))
		maxPos[axis] = int(math32.Floor(max[axis] - 0.001))
	}
	return minPos, maxPos
}

// applyInsideBlockEffects applies post-move block contact: bubble columns,
// then honey, then the slowdown multiplier for the next move.
func (s *Simulator) applyInsideBlockEffects(state *MovementState) {
	if s.World == nil {
		return
	}
	s.applyBubbleColumns(state)
	if !state.swimWaterContact {
		s.applyHoneyWallSlide(state)
	}
	minPos, maxPos := insideCellRange(state.BoundingBox(s.Options.UseSlideOffset))
	var berry, powderSnow, web bool
	for x := minPos.X(); x <= maxPos.X(); x++ {
		for y := minPos.Y(); y <= maxPos.Y(); y++ {
			for z := minPos.Z(); z <= maxPos.Z(); z++ {
				b := s.World.Block(cube.Pos{x, y, z})
				if s.blockAir(b) {
					continue
				}
				semantics := s.blockMovementSemantics(b)
				berry = berry || semantics.InsideMovement == movementblock.InsideMovementSweetBerryBush
				powderSnow = powderSnow || semantics.InsideMovement == movementblock.InsideMovementPowderSnow
				web = web || semantics.Cobweb
			}
		}
	}
	if powderSnow {
		applyInsideBlockMovement(state, movementblock.InsideMovementPowderSnow)
	}
	if web {
		multiplier := mgl32.Vec3{0.25, 0.05, 0.25}
		// Weaving replaces the web multiplier only when no other slowdown block is entered.
		if !berry && !powderSnow && s.Effects != nil {
			if _, weaving := s.Effects.GetEffect(EffectWeaving); weaving {
				multiplier = mgl32.Vec3{0.5, 0.25, 0.5}
			}
		}
		queueStuckSpeedMultiplier(state, multiplier)
	}
	if berry {
		applyInsideBlockMovement(state, movementblock.InsideMovementSweetBerryBush)
	}
	if berry || powderSnow || web {
		state.FallDistance = 0
	}
}

// applyHoneyWallSlide slows the entity once per occupied honey cell, including
// the cell it stands on. Overlapping two cells compounds the horizontal factor.
func (s *Simulator) applyHoneyWallSlide(state *MovementState) {
	minPos, maxPos := insideCellRange(state.BoundingBox(s.Options.UseSlideOffset))
	for x := minPos.X(); x <= maxPos.X(); x++ {
		for y := minPos.Y(); y <= maxPos.Y(); y++ {
			for z := minPos.Z(); z <= maxPos.Z(); z++ {
				pos := cube.Pos{x, y, z}
				if !s.blockMovementSemantics(s.World.Block(pos)).Honey {
					continue
				}
				velocity := state.Vel
				velocity[0] *= 0.4
				velocity[1] = max(-0.12, velocity[1])
				velocity[2] *= 0.4
				state.SetVel(velocity)
				if honeySlideResetsFallDistance(state, pos) {
					state.FallDistance = 0
				}
			}
		}
	}
}

// honeySlideResetsFallDistance reports whether contact is with a honey side
// rather than the top surface.
func honeySlideResetsFallDistance(state *MovementState, pos cube.Pos) bool {
	if state.Vel.Y() >= 0 || state.Pos.Y() > float32(pos.Y())+0.9375 {
		return false
	}
	radius := state.Size.X()*state.Size.Z()*0.5 + 0.43125
	centerX, centerZ := float32(pos.X())+0.5, float32(pos.Z())+0.5
	return math32.Abs(centerX-state.Pos.X()) > radius || math32.Abs(centerZ-state.Pos.Z()) > radius
}
