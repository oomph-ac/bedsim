package bedsim

import (
	"testing"

	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/go-gl/mathgl/mgl32"
)

func TestNativeFloat32Surface(t *testing.T) {
	var sin func(float32) float32 = MCSin
	state := MovementState{Pos: mgl32.Vec3{1, 2, 3}}
	if got := sin(state.Pos.X()); got != MCSin(1) {
		t.Fatalf("MCSin(%v) = %v, want %v", state.Pos.X(), got, MCSin(1))
	}
}

func TestBBoxFromDragonflyRoundsAtProviderBoundary(t *testing.T) {
	got := BBoxFromDragonfly(cube.Box(0.1, 0.2, 0.3, 0.9, 1.8, 0.7))
	want := cube.Box32(float32(0.1), float32(0.2), float32(0.3), float32(0.9), float32(1.8), float32(0.7))
	if got != want {
		t.Fatalf("BBoxFromDragonfly() = %v, want %v", got, want)
	}
}
