package bedsim

import (
	"github.com/go-gl/mathgl/mgl32"
	"testing"
)

func TestJumpImpulse(t *testing.T) {
	for _, tc := range []struct {
		name     string
		velocity mgl32.Vec3
		yaw      float32
		sprint   bool
		want     mgl32.Vec3
	}{
		{"walking", mgl32.Vec3{.1, .3, -.2}, 0, false, mgl32.Vec3{.1, .42, -.2}},
		{"sprint counters backward impulse", mgl32.Vec3{0, .3, -.2}, 0, true, mgl32.Vec3{0, .42, 0}},
		{"sprint rotated", mgl32.Vec3{.2, .3, 0}, 90, true, mgl32.Vec3{0, .42, 0}},
		{"strong upward impulse retained", mgl32.Vec3{0, .7, -.2}, 0, true, mgl32.Vec3{0, .7, 0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := JumpImpulse(tc.velocity, .42, tc.yaw, tc.sprint)
			if got.Sub(tc.want).Len() > 1e-5 {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}
