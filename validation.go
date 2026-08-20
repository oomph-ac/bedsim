package bedsim

import (
	"github.com/chewxy/math32"
	"github.com/go-gl/mathgl/mgl32"
)

// finiteFloat reports whether value is neither NaN nor infinite.
func finiteFloat(value float32) bool {
	return !math32.IsNaN(value) && !math32.IsInf(value, 0)
}

// finiteVec2 reports whether every component is finite.
func finiteVec2(value mgl32.Vec2) bool {
	for axis := range 2 {
		if !finiteFloat(value[axis]) {
			return false
		}
	}
	return true
}

// finiteVec3 reports whether every component is finite.
func finiteVec3(value mgl32.Vec3) bool {
	for axis := range 3 {
		if !finiteFloat(value[axis]) {
			return false
		}
	}
	return true
}

// finiteInput reports whether all numeric input fields are finite.
func finiteInput(input InputState) bool {
	return finiteVec2(input.MoveVector) &&
		finiteVec3(input.ClientPos) &&
		finiteVec3(input.ClientVel) &&
		finiteFloat(input.Pitch) && finiteFloat(input.Yaw) && finiteFloat(input.HeadYaw)
}

// finiteMovementState reports whether all simulated numeric state is finite.
func finiteMovementState(state *MovementState) bool {
	if state == nil {
		return false
	}
	for _, value := range []mgl32.Vec3{
		state.Client.Pos, state.Client.LastPos, state.Client.Vel, state.Client.LastVel,
		state.Client.Mov, state.Client.LastMov, state.Pos, state.LastPos, state.Vel,
		state.LastVel, state.Mov, state.LastMov, state.Rotation, state.LastRotation,
		state.Knockback, state.TeleportPos, state.PendingTeleportPos, state.Size,
		state.StuckSpeedMultiplier,
	} {
		if !finiteVec3(value) {
			return false
		}
	}
	if !finiteVec2(state.SlideOffset) || !finiteVec2(state.Impulse) {
		return false
	}
	for _, value := range []float32{
		state.StandingHeight, state.SneakingHeight, state.CrawlingHeight,
		state.Gravity, state.JumpHeight, state.JumpStrength, state.FallDistance,
		state.MovementSpeed, state.DefaultMovementSpeed, state.AirSpeed,
		state.UnderwaterMovementSpeed, state.LavaMovementSpeed, state.SwimSpeedMultiplier,
		state.SwimAmount,
	} {
		if !finiteFloat(value) {
			return false
		}
	}
	return true
}
