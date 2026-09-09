package bedsim

import (
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/go-gl/mathgl/mgl32"
	"testing"
)

func TestBoundingBox_VanillaWallContact(t *testing.T) {
	state := newBaseState()
	state.Pos = mgl32.Vec3{26.5, -59, 5.611176}
	state.Client.Pos = state.Pos
	// Real BDS 1.26.45.1 clips the 0.6-wide player at z=5.7 against
	// the wall at z=6. Shrinking X/Z by 1e-4 instead penetrates to 5.7001.
	wall := cube.Box32(24, -60, 6, 29, -58, 7)
	for name, box := range map[string]cube.BBox32{"simulated": state.BoundingBox(false), "client": state.ClientBoundingBox(false)} {
		t.Run(name, func(t *testing.T) {
			var penetration mgl32.Vec3
			clipped := BBClipCollide(wall, box, mgl32.Vec3{0, 0, 0.2584}, false, &penetration)
			center := box.Translate(clipped).Min().Add(box.Translate(clipped).Max()).Mul(0.5)
			if center.Z() != float32(5.7) {
				t.Fatalf("wall contact = %.9g, vanilla 5.7", center.Z())
			}
		})
	}
}
