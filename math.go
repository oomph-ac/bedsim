package bedsim

import (
	"math"

	"github.com/chewxy/math32"

	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/go-gl/mathgl/mgl32"
)

var mcSinTable []float32

func init() {
	mcSinTable = make([]float32, 65536)
	for i := range 65536 {
		mcSinTable[i] = float32(math.Sin(float64(i) * math.Pi * 2 / 65536))
	}
}

// MCSin returns the Minecraft sin of the given angle.
func MCSin(val float32) float32 {
	return mcSinTable[uint16(val*10430.378)&65535]
}

// MCCos returns the Minecraft cos of the given angle.
func MCCos(val float32) float32 {
	return mcSinTable[uint16(val*10430.378+16384.0)&65535]
}

// ClampFloat clamps the given value to the given range.
func ClampFloat(num, min, max float32) float32 {
	if num < min {
		return min
	}
	return math32.Min(num, max)
}

// Vec3HzDistSqr returns the squared horizontal distance in a vector.
func Vec3HzDistSqr(vec3 mgl32.Vec3) float32 {
	return vec3.X()*vec3.X() + vec3.Z()*vec3.Z()
}

func posFromVec3(vec mgl32.Vec3) cube.Pos {
	return cube.Pos{int(math32.Floor(vec.X())), int(math32.Floor(vec.Y())), int(math32.Floor(vec.Z()))}
}

func posVec3(pos cube.Pos) mgl32.Vec3 {
	return mgl32.Vec3{float32(pos.X()), float32(pos.Y()), float32(pos.Z())}
}
