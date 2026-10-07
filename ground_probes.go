package bedsim

import (
	"strings"

	"github.com/chewxy/math32"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/go-gl/mathgl/mgl32"
	movementblock "github.com/oomph-ac/bedsim/block"
)

const (
	// floatEpsilon is the float32 machine epsilon vanilla uses for collision
	// flags, horizontal drag clearing and sneak-edge velocity clearing.
	floatEpsilon = float32(1.1920929e-7)
	// minRestitutionSpeed is the slowest downward landing that can bounce.
	minRestitutionSpeed = float32(0.08000011742115021)
	// groundFrictionProbeOffset and standingOnProbeOffset are added to the
	// collision box's minimum Y before sampling the block underneath.
	groundFrictionProbeOffset = float32(-0.1)
	standingOnProbeOffset     = float32(-0.2)
)

// groundFrictionSemantics returns the semantics of the block that sets ground
// friction: the cell at (floor x, floor(minY-0.1), floor z), with air keeping
// the default friction and no acceleration adjustment.
func (s *Simulator) groundFrictionSemantics(state *MovementState) (world.Block, movementblock.MovementSemantics) {
	minY := state.BoundingBox(s.Options.UseSlideOffset).Min().Y()
	pos := cube.Pos{
		int(math32.Floor(state.Pos.X())),
		int(math32.Floor(minY + groundFrictionProbeOffset)),
		int(math32.Floor(state.Pos.Z())),
	}
	b := s.blockAtPos(pos)
	if s.blockAir(b) {
		return b, movementblock.MovementSemantics{
			GroundFriction:                       DefaultBlockFriction,
			GroundAccelerationFrictionMultiplier: 1,
		}
	}
	return b, s.blockMovementSemantics(b)
}

// landingBlock returns the block a downward collision lands on: among the
// move's collision boxes whose centre is at or below minY-0.2, the highest
// centre wins, then the centre nearest the probe plane's centre.
func (s *Simulator) landingBlock(box cube.BBox32, boxes []cube.BBox32) (world.Block, bool) {
	planeY := box.Min().Y() + standingOnProbeOffset
	centerX := (box.Max().X()-box.Min().X())*0.5 + box.Min().X()
	centerZ := (box.Max().Z()-box.Min().Z())*0.5 + box.Min().Z()
	best := -1
	bestGap := float32(math32.MaxFloat32)
	bestDist := float32(0)
	for i, candidate := range boxes {
		min, max := candidate.Min(), candidate.Max()
		cy := (max.Y()-min.Y())*0.5 + min.Y()
		gap := planeY - cy
		if gap < 0 {
			continue
		}
		dx := (max.X()-min.X())*0.5 + min.X() - centerX
		dy := cy - planeY
		dz := (max.Z()-min.Z())*0.5 + min.Z() - centerZ
		dist := float32(float32(dz*dz)+float32(dy*dy)) + float32(dx*dx)
		if gap < bestGap || best >= 0 && gap == bestGap && dist < bestDist {
			best, bestGap, bestDist = i, gap, dist
		}
	}
	if best < 0 || BBHasZeroVolume(boxes[best]) {
		return nil, false
	}
	min := boxes[best].Min()
	pos := cube.Pos{int(math32.Floor(min.X())), int(math32.Floor(min.Y())), int(math32.Floor(min.Z()))}
	return s.blockAtPos(pos), true
}

// restitution returns the vertical speed a downward landing rebounds with.
// Slime returns the impact speed, beds three quarters of it and honey cancels
// any bounce; landings slower than minRestitutionSpeed or while sneaking stop.
func (s *Simulator) restitution(state *MovementState, impactY float32, landedOn world.Block, found bool) float32 {
	if state.Sneaking || impactY >= 0 || math32.Abs(impactY) < minRestitutionSpeed || !found {
		return 0
	}
	semantics := s.blockMovementSemantics(landedOn)
	bounciness := float32(0)
	switch {
	case semantics.Honey:
		return 0
	case semantics.Bounce == movementblock.BounceSlime:
		bounciness = -SlimeBounceMultiplier
	case semantics.Bounce == movementblock.BounceBed:
		bounciness = -BedBounceMultiplier
	}
	return max(bounciness*-impactY, 0)
}

// standingOnBlock returns the block whose collision shape has the highest top
// crossing the plane at minY-0.2 under the player's footprint; ties go to the
// shape whose centre is nearest the plane's centre.
func (s *Simulator) standingOnBlock(state *MovementState) (world.Block, bool) {
	if s.World == nil {
		return nil, false
	}
	box := state.BoundingBox(s.Options.UseSlideOffset)
	min, max := box.Min(), box.Max()
	planeY := min.Y() + standingOnProbeOffset
	center := mgl32.Vec3{(max.X()-min.X())*0.5 + min.X(), planeY, (max.Z()-min.Z())*0.5 + min.Z()}
	topY := int(math32.Floor(planeY))
	var (
		bestPos  cube.Pos
		found    bool
		bestTop  = float32(-math32.MaxFloat32)
		bestDist float32
	)
	for x := int(math32.Floor(min.X())); x <= int(math32.Floor(max.X())); x++ {
		for z := int(math32.Floor(min.Z())); z <= int(math32.Floor(max.Z())); z++ {
			// Taller shapes such as fences reach the plane from the cell below.
			for y := topY; y >= topY-1; y-- {
				pos := cube.Pos{x, y, z}
				shape, ok := blockCollisionShape(s.World.BlockCollisions(pos))
				if !ok {
					continue
				}
				shape = shape.Translate(posVec3(pos))
				smin, smax := shape.Min(), shape.Max()
				if smax.X() <= min.X() || smin.X() >= max.X() || smax.Z() <= min.Z() || smin.Z() >= max.Z() ||
					smax.Y() <= planeY || smin.Y() >= planeY || bestTop > smax.Y() {
					continue
				}
				d := mgl32.Vec3{
					center.X() - ((smax.X()-smin.X())*0.5 + smin.X()),
					center.Y() - ((smax.Y()-smin.Y())*0.5 + smin.Y()),
					center.Z() - ((smax.Z()-smin.Z())*0.5 + smin.Z()),
				}
				dist := float32(float32(d.Z()*d.Z())+float32(d.Y()*d.Y())) + float32(d.X()*d.X())
				if bestTop < smax.Y() || dist < bestDist {
					bestTop, bestDist, found = smax.Y(), dist, true
					bestPos = cube.Pos{int(math32.Floor(smin.X())), int(math32.Floor(smin.Y())), int(math32.Floor(smin.Z()))}
				}
			}
		}
	}
	if !found {
		return nil, false
	}
	return s.blockAtPos(bestPos), true
}

// blockCollisionShape returns the bounds of a block's collision boxes.
func blockCollisionShape(boxes []cube.BBox32) (cube.BBox32, bool) {
	var shape cube.BBox32
	found := false
	for _, b := range boxes {
		if BBHasZeroVolume(b) {
			continue
		}
		if !found {
			shape, found = b, true
			continue
		}
		smin, smax := shape.Min(), shape.Max()
		bmin, bmax := b.Min(), b.Max()
		shape = cube.Box32(
			min(smin.X(), bmin.X()), min(smin.Y(), bmin.Y()), min(smin.Z(), bmin.Z()),
			max(smax.X(), bmax.X()), max(smax.Y(), bmax.Y()), max(smax.Z(), bmax.Z()),
		)
	}
	return shape, found
}

// applyStandOnDamping slows horizontal motion on slime and honey after drag.
func (s *Simulator) applyStandOnDamping(state *MovementState) {
	if !state.OnGround || state.Sneaking || state.Vel.Y() >= 0.1 {
		return
	}
	b, ok := s.standingOnBlock(state)
	if !ok {
		return
	}
	semantics := s.blockMovementSemantics(b)
	if semantics.Bounce != movementblock.BounceSlime && !semantics.Honey {
		return
	}
	factor := float32(math32.Abs(state.Vel.Y())*0.2) + 0.4
	state.Vel[0] *= factor
	state.Vel[2] *= factor
}

// jumpFactorBlockHoney reports whether the jump-factor block is honey: the
// feet cell, or the cell below when the feet cell is a carpet or the feet rest
// exactly on a block boundary.
func (s *Simulator) jumpFactorBlockHoney(state *MovementState) bool {
	minY := state.BoundingBox(s.Options.UseSlideOffset).Min().Y()
	feetY := math32.Floor(minY)
	pos := cube.Pos{int(math32.Floor(state.Pos.X())), int(feetY), int(math32.Floor(state.Pos.Z()))}
	feet := s.blockAtPos(pos)
	if s.blockMovementSemantics(feet).Honey {
		return true
	}
	if minY > feetY && !isCarpet(feet) {
		return false
	}
	return s.blockMovementSemantics(s.blockAtPos(pos.Side(cube.FaceDown))).Honey
}

// isCarpet reports whether b is a carpet, which defers block-below checks.
func isCarpet(b world.Block) bool {
	name := BlockName(b)
	return name == "minecraft:carpet" || strings.HasSuffix(name, "_carpet")
}

// dampHorizontal applies horizontal drag, clearing components at or below
// float epsilon first.
func dampHorizontal(v, drag float32) float32 {
	if math32.Abs(v) <= floatEpsilon {
		return 0
	}
	return v * drag
}
