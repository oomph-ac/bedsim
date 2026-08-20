package bedsim

import (
	"github.com/chewxy/math32"

	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/go-gl/mathgl/mgl32"
)

type clipCollideResult struct {
	depenetratingAxis     int
	penetration           float32
	clippedVelocity       mgl32.Vec3
	depenetratingVelocity mgl32.Vec3
}

// BBClipCollide clips or depenetrates a moving bounding box against a stationary one.
func BBClipCollide(this, c cube.BBox32, vel mgl32.Vec3, oneWay bool, penetration *mgl32.Vec3) mgl32.Vec3 {
	result := doBBClipCollide(this, c, vel)
	if penetration != nil && penetration[result.depenetratingAxis] < result.penetration {
		penetration[result.depenetratingAxis] = result.penetration
	}

	if oneWay {
		return result.clippedVelocity
	}
	return result.depenetratingVelocity
}

func doBBClipCollide(stationary, moving cube.BBox32, velocity mgl32.Vec3) (result clipCollideResult) {
	result.clippedVelocity = velocity
	result.depenetratingVelocity = velocity

	if BBHasZeroVolume(stationary) {
		return
	}

	axisPenetrations := [3]float32{}
	axisPenetrationsSigned := [3]float32{}
	normalDirs := [3]float32{}
	separatingAxes, separatingAxis := 0, 0
	resultPenetration := float32(math32.MaxFloat32 - 1)

	for i := range 3 {
		minPenetration := moving.Max()[i] - stationary.Min()[i]
		maxPenetration := stationary.Max()[i] - moving.Min()[i]

		if math32.Abs(minPenetration) <= 1e-7 {
			minPenetration = 0
		}
		if math32.Abs(maxPenetration) <= 1e-7 {
			maxPenetration = 0
		}

		minPositive := math32.Max(0, minPenetration)
		maxPositive := math32.Max(0, maxPenetration)

		if minPositive == 0 {
			axisPenetrations[i] = 0
			axisPenetrationsSigned[i] = minPenetration
			normalDirs[i] = -1
			separatingAxes++
			separatingAxis = i
		} else if maxPositive == 0 {
			axisPenetrations[i] = 0
			axisPenetrationsSigned[i] = maxPenetration
			normalDirs[i] = 1
			separatingAxes++
			separatingAxis = i
		} else if minPositive < maxPositive {
			axisPenetrations[i] = minPositive
			axisPenetrationsSigned[i] = minPositive
			normalDirs[i] = -1
		} else {
			axisPenetrations[i] = maxPositive
			axisPenetrationsSigned[i] = maxPositive
			normalDirs[i] = 1
		}

		if separatingAxes > 1 {
			return
		}
		resultPenetration = math32.Min(resultPenetration, axisPenetrations[i])
	}

	// No separating axes means a collision.
	if separatingAxes == 0 {
		result.penetration = resultPenetration
		bestAxis := 0
		for i := 1; i < 3; i++ {
			if axisPenetrations[i] < axisPenetrations[bestAxis] {
				bestAxis = i
			}
		}

		desiredVelocity := axisPenetrations[bestAxis] * normalDirs[bestAxis]
		if desiredVelocity > 0 {
			result.depenetratingVelocity[bestAxis] = math32.Max(desiredVelocity, velocity[bestAxis])
		} else {
			result.depenetratingVelocity[bestAxis] = math32.Min(desiredVelocity, velocity[bestAxis])
		}
		result.depenetratingAxis = bestAxis
		return
	}

	sweptPenetration := axisPenetrationsSigned[separatingAxis] - (normalDirs[separatingAxis] * velocity[separatingAxis])
	if sweptPenetration <= 0 {
		return
	}

	resolvedVelocity := axisPenetrationsSigned[separatingAxis] * normalDirs[separatingAxis]
	result.clippedVelocity[separatingAxis] = resolvedVelocity
	result.depenetratingVelocity[separatingAxis] = resolvedVelocity
	return
}

// BBHasZeroVolume returns true for empty or invalid bounding boxes.
func BBHasZeroVolume(bb cube.BBox32) bool {
	min, max := bb.Min(), bb.Max()
	for axis := range 3 {
		if !finiteFloat(min[axis]) || !finiteFloat(max[axis]) || min[axis] >= max[axis] {
			return true
		}
	}
	return false
}
