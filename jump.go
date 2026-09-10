package bedsim

import (
	"github.com/chewxy/math32"
	"github.com/go-gl/mathgl/mgl32"
)

// JumpImpulse applies the velocity change of an accepted vanilla jump. yaw is in
// degrees. Callers must enforce ground contact, jump cooldown, and jump eligibility;
// this function does not advance movement or apply input acceleration and drag.
func JumpImpulse(velocity mgl32.Vec3, jumpHeight, yaw float32, sprinting bool) mgl32.Vec3 {
	velocity[1] = math32.Max(jumpHeight, velocity[1])
	if sprinting {
		force := yaw * 0.017453292
		velocity[0] -= MCSin(force) * 0.2
		velocity[2] += MCCos(force) * 0.2
	}
	return velocity
}
