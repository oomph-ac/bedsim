package bedsim

import (
	"github.com/chewxy/math32"
	"github.com/go-gl/mathgl/mgl32"
)

// SprintMovementBlocked reports vanilla's stalled dominant-axis condition.
// requested is the previous world-space movement request; actual is the position
// displacement it produced, not the post-drag velocity. A glancing collision
// does not stop sprint while movement along the requested dominant axis continues.
func SprintMovementBlocked(requested, actual mgl32.Vec3) bool {
	const minimumMovement = float32(0.00005)
	x, z := math32.Abs(requested[0]), math32.Abs(requested[2])
	return (x > z && math32.Abs(actual[0]) < minimumMovement) ||
		(z > x && math32.Abs(actual[2]) < minimumMovement)
}
