package bedsim

import (
	"github.com/go-gl/mathgl/mgl32"
	"testing"
)

// TestSprintMovementBlocked checks stalled axes, wall slides, and the movement threshold.
func TestSprintMovementBlocked(t *testing.T) {
	for _, tt := range []struct {
		name              string
		requested, actual mgl32.Vec3
		want              bool
	}{
		{"blocked z", mgl32.Vec3{0, 0, 1}, mgl32.Vec3{0, .33, 0}, true},
		{"blocked x", mgl32.Vec3{-1, 0, 0}, mgl32.Vec3{0, .33, 0}, true},
		{"wall slide", mgl32.Vec3{.2, 0, 1}, mgl32.Vec3{0, 0, .12}, false},
		{"threshold", mgl32.Vec3{0, 0, 1}, mgl32.Vec3{0, 0, .00005}, false},
		{"stationary request", mgl32.Vec3{}, mgl32.Vec3{}, false},
		{"equal axes", mgl32.Vec3{1, 0, 1}, mgl32.Vec3{}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := SprintMovementBlocked(tt.requested, tt.actual); got != tt.want {
				t.Fatalf("blocked=%v want %v", got, tt.want)
			}
		})
	}
}
