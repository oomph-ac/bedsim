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

func (s *MovementState) collisionDimensions() mgl32.Vec2 {
	scale := s.Size[2]
	height := s.Size[1] * scale
	if s.SwimPose() {
		height = s.Size[0] * scale
	}
	return mgl32.Vec2{(s.Size[0] * 0.5) * scale, height}
}

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
	width, height := dimensions[0], dimensions[1]
	yOffset := float32(0)
	if useSlideOffset {
		yOffset = s.SlideOffset.Y()
	}
	shape := s.collisionShape
	if shape.valid && shape.pos == s.Pos && shape.dimensions == dimensions && shape.yOffset == yOffset {
		return shape.box
	}

	return cube.Box32(
		s.Pos[0]-width,
		s.Pos[1]+yOffset,
		s.Pos[2]-width,
		s.Pos[0]+width,
		s.Pos[1]+height+yOffset,
		s.Pos[2]+width,
	)
}

// ClientBoundingBox returns the bounding box translated to the client's position.
func (s *MovementState) ClientBoundingBox(useSlideOffset bool) cube.BBox32 {
	dimensions := s.collisionDimensions()
	width, height := dimensions[0], dimensions[1]
	yOffset := float32(0)
	if useSlideOffset {
		yOffset = s.SlideOffset.Y()
	}

	return cube.Box32(
		s.Client.Pos[0]-width,
		s.Client.Pos[1]+yOffset,
		s.Client.Pos[2]-width,
		s.Client.Pos[0]+width,
		s.Client.Pos[1]+height+yOffset,
		s.Client.Pos[2]+width,
	)
}
