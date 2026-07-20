package bedsim

import (
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/go-gl/mathgl/mgl64"
)

// SwimPose reports whether the collapsed swim hitbox applies. It requires the
// client's swimming flag *and* recent server-observed water contact, because
// the flag alone is client-controlled: without the second condition a client
// could shrink its server-side hitbox from 1.8 to 0.6 in open air and walk
// through gaps a standing player cannot fit. See SwimWaterGraceTicks.
//
// The grace budget is clamped to its configured bound at the start of a tick
// and decremented at the end, so this stays constant for the whole tick and
// collision, liquid detection and exit probing always agree on one hitbox.
// Entering water therefore adopts the swim pose one tick later than upstream,
// which errs toward the larger, more conservative box.
func (s *MovementState) SwimPose() bool {
	return s.Swimming && s.SwimWaterGraceTicks > 0
}

// BoundingBox returns the entity bounding box translated to the current position.
func (s *MovementState) BoundingBox(useSlideOffset bool) cube.BBox {
	scale := s.Size[2]
	width := (s.Size[0] * 0.5) * scale
	height := s.Size[1] * scale
	if s.SwimPose() {
		// The swim pose collapses the hitbox to a width-sized cube.
		height = s.Size[0] * scale
	}
	yOffset := 0.0
	if useSlideOffset {
		yOffset = s.SlideOffset.Y()
	}

	return cube.Box(
		s.Pos[0]-width,
		s.Pos[1]+yOffset,
		s.Pos[2]-width,
		s.Pos[0]+width,
		s.Pos[1]+height+yOffset,
		s.Pos[2]+width,
	).GrowVec3(mgl64.Vec3{-1e-4, 0, -1e-4})
}

// ClientBoundingBox returns the bounding box translated to the client's position.
func (s *MovementState) ClientBoundingBox(useSlideOffset bool) cube.BBox {
	scale := s.Size[2]
	width := (s.Size[0] * 0.5) * scale
	height := s.Size[1] * scale
	if s.SwimPose() {
		// The swim pose collapses the hitbox to a width-sized cube.
		height = s.Size[0] * scale
	}
	yOffset := 0.0
	if useSlideOffset {
		yOffset = s.SlideOffset.Y()
	}

	return cube.Box(
		s.Client.Pos[0]-width,
		s.Client.Pos[1]+yOffset,
		s.Client.Pos[2]-width,
		s.Client.Pos[0]+width,
		s.Client.Pos[1]+height+yOffset,
		s.Client.Pos[2]+width,
	).GrowVec3(mgl64.Vec3{-1e-4, 0, -1e-4})
}
