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

// BoundingBox returns the entity bounding box translated to the current position.
func (s *MovementState) BoundingBox(useSlideOffset bool) cube.BBox32 {
	scale := s.Size[2]
	width := (s.Size[0] * 0.5) * scale
	height := s.Size[1] * scale
	if s.SwimPose() {
		height = s.Size[0] * scale
	}
	yOffset := float32(0)
	if useSlideOffset {
		yOffset = s.SlideOffset.Y()
	}

	return cube.Box32(
		s.Pos[0]-width,
		s.Pos[1]+yOffset,
		s.Pos[2]-width,
		s.Pos[0]+width,
		s.Pos[1]+height+yOffset,
		s.Pos[2]+width,
	).GrowVec3(mgl32.Vec3{-1e-4, 0, -1e-4})
}

// ClientBoundingBox returns the bounding box translated to the client's position.
func (s *MovementState) ClientBoundingBox(useSlideOffset bool) cube.BBox32 {
	scale := s.Size[2]
	width := (s.Size[0] * 0.5) * scale
	height := s.Size[1] * scale
	if s.SwimPose() {
		height = s.Size[0] * scale
	}
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
	).GrowVec3(mgl32.Vec3{-1e-4, 0, -1e-4})
}
