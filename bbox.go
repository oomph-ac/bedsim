package bedsim

import (
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/go-gl/mathgl/mgl32"
)

// BBoxFromDragonfly returns a simulation bounding box rounded to float32 coordinates.
func BBoxFromDragonfly(box cube.BBox) cube.BBox32 {
	min, max := box.Min(), box.Max()
	return cube.Box32(
		float32(min.X()), float32(min.Y()), float32(min.Z()),
		float32(max.X()), float32(max.Y()), float32(max.Z()),
	)
}

// SwimPose reports whether recent server-observed water contact permits the
// client-requested collapsed hitbox.
func (s *MovementState) SwimPose() bool {
	return s.Swimming && s.SwimWaterGraceTicks > 0
}

// EyePosition returns the vanilla player eye attachment position for the
// current pose. Compact swimming, crawling, gliding, and active Riptide poses
// use the same offset. The offset follows the entity scale used by BoundingBox.
func (s *MovementState) EyePosition() mgl32.Vec3 {
	if s == nil {
		return mgl32.Vec3{}
	}
	offset := DefaultPlayerHeightOffset
	switch {
	case s.SwimPose() || s.Crawling || s.Gliding || s.RiptideTicks > 0:
		offset = CompactPlayerHeightOffset
	case s.Sneaking:
		offset = SneakingPlayerHeightOffset
	}
	scale := s.Size.Z()
	if scale <= 0 {
		scale = 1
	}
	return s.Pos.Add(mgl32.Vec3{0, offset * scale, 0})
}

// collisionShape retains native AABB endpoints after a sweep. Reconstructing a
// box from its rounded center can move a flush face inside the touching block.
// Pose changes and external position changes invalidate the retained endpoints.
type collisionShape struct {
	box        cube.BBox32
	pos        mgl32.Vec3
	dimensions mgl32.Vec2
	yOffset    float32
	valid      bool
}

// collisionDimensions returns the scaled half-width and pose height.
func (s *MovementState) collisionDimensions() mgl32.Vec2 {
	scale := s.Size[2]
	height := s.Size[1] * scale
	if s.SwimPose() {
		height = s.Size[0] * scale
	}
	return mgl32.Vec2{(s.Size[0] * 0.5) * scale, height}
}

// rememberCollisionBox retains endpoints with the state that produced them.
func (s *MovementState) rememberCollisionBox(box cube.BBox32, useSlideOffset bool) {
	offset := float32(0)
	if useSlideOffset {
		offset = s.SlideOffset.Y()
	}
	s.collisionShape = collisionShape{box: box, pos: s.Pos, dimensions: s.collisionDimensions(), yOffset: offset, valid: true}
}

// BoundingBox returns the entity bounding box translated to the current position.
func (s *MovementState) BoundingBox(useSlideOffset bool) cube.BBox32 {
	dimensions := s.collisionDimensions()
	yOffset := float32(0)
	if useSlideOffset {
		yOffset = s.SlideOffset.Y()
	}
	shape := s.collisionShape
	if shape.valid && shape.pos == s.Pos && shape.dimensions == dimensions && shape.yOffset == yOffset {
		return shape.box
	}

	return s.collisionBoxAt(s.Pos, useSlideOffset)
}

// ClientBoundingBox returns the bounding box translated to the client's position.
func (s *MovementState) ClientBoundingBox(useSlideOffset bool) cube.BBox32 {
	return s.collisionBoxAt(s.Client.Pos, useSlideOffset)
}

// collisionBoxAt reconstructs native geometry for a position or pose change.
func (s *MovementState) collisionBoxAt(pos mgl32.Vec3, useSlideOffset bool) cube.BBox32 {
	dimensions := s.collisionDimensions()
	width, height := dimensions[0], dimensions[1]
	yOffset := float32(0)
	if useSlideOffset {
		yOffset = s.SlideOffset.Y()
	}
	return cube.Box32(pos[0]-width, pos[1]+yOffset, pos[2]-width,
		pos[0]+width, pos[1]+height+yOffset, pos[2]+width)
}

// prepareCollisionBox recovers unknown contact endpoints from a supplied rounded
// position once the area is loaded. Known sweeps and native pose changes keep
// their geometry. QueueTeleport establishes native geometry at its destination.
func (s *Simulator) prepareCollisionBox(state *MovementState) {
	useSlideOffset := s.Options.UseSlideOffset
	shape := state.collisionShape
	dimensions := state.collisionDimensions()
	offset := float32(0)
	if useSlideOffset {
		offset = state.SlideOffset.Y()
	}
	sameDimensions := shape.dimensions == dimensions && shape.yOffset == offset
	if shape.valid && sameDimensions && shape.pos == state.Pos {
		return
	}
	box := state.collisionBoxAt(state.Pos, useSlideOffset)
	if shape.valid && !sameDimensions {
		// Native resizing rebuilds all endpoints, even when only height changes.
		state.rememberCollisionBox(box, useSlideOffset)
		return
	}
	if s.World == nil || !s.movementAreaLoaded(box) {
		return
	}
	box = recoverRoundedContacts(box, state.Pos, s.nearbyBBoxes(state, box))
	state.rememberCollisionBox(box, useSlideOffset)
}

// recoverRoundedContacts restores horizontal contact faces only when the box's
// float32 center still equals the supplied position. It never moves the position
// or applies a tolerance to subsequent sweeps. Combined faces are checked again
// so the result does not depend on collision-box enumeration order.
func recoverRoundedContacts(box cube.BBox32, pos mgl32.Vec3, boxes []cube.BBox32) cube.BBox32 {
	originalMin, originalMax := box.Min(), box.Max()
	low, high := originalMin, originalMax
	for _, other := range boxes {
		if !box.IntersectsWith(other) {
			continue
		}
		for _, axis := range [...]int{0, 2} {
			face := other.Max()[axis]
			if face > originalMin[axis] && face < originalMax[axis] &&
				(face+originalMax[axis])*.5 == pos[axis] {
				low[axis] = max(low[axis], face)
			}
			face = other.Min()[axis]
			if face < originalMax[axis] && face > originalMin[axis] &&
				(originalMin[axis]+face)*.5 == pos[axis] {
				high[axis] = min(high[axis], face)
			}
		}
	}
	for _, axis := range [...]int{0, 2} {
		if low[axis] >= high[axis] || (low[axis]+high[axis])*.5 != pos[axis] {
			low[axis], high[axis] = originalMin[axis], originalMax[axis]
		}
	}
	return cube.Box32(low[0], low[1], low[2], high[0], high[1], high[2])
}
