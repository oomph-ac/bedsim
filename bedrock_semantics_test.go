package bedsim

import (
	"math"
	"testing"

	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/go-gl/mathgl/mgl64"
)

func TestBlockAirRecognisesOnlyBedrockAirIdentifier(t *testing.T) {
	sim := &Simulator{BlockSemantics: encodedBlockSemantics{}}
	tests := []struct {
		name string
		want bool
	}{
		{name: "minecraft:air", want: true},
		{name: "minecraft:cave_air", want: false},
		{name: "minecraft:void_air", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sim.blockAir(namedBlock{name: tt.name}); got != tt.want {
				t.Fatalf("blockAir(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestJavaWebIdentifierHasNoBedrockMovementEffect(t *testing.T) {
	w := environmentWorld{blocks: map[cube.Pos]world.Block{
		{0, 0, 0}: namedBlock{name: "minecraft:cobweb"},
	}}
	state := newBaseState()
	state.Pos = mgl64.Vec3{0.5, 0, 0.5}
	state.Vel = mgl64.Vec3{0.1, 0, 0}
	state.HasGravity = false

	result := (&Simulator{World: w, BlockSemantics: encodedBlockSemantics{}}).SimulateState(state)

	if want := 0.1; math.Abs(result.Movement.X()-want) > 1e-12 {
		t.Fatalf("Java web identifier changed Bedrock movement: got %v, want %v", result.Movement.X(), want)
	}
}

func TestDefaultSneakingHeightMatchesDragonflyBedrockPlayer(t *testing.T) {
	state := newBaseState()

	(&Simulator{}).applyInput(state, InputState{StartSneaking: true})

	if state.Size.Y() != 1.49 {
		t.Fatalf("sneaking height = %v, want 1.49", state.Size.Y())
	}
}

func TestCrawlingCannotStartInOpenAir(t *testing.T) {
	state := newBaseState()

	(&Simulator{World: environmentWorld{}}).applyInput(state, InputState{StartCrawling: true})

	if state.Crawling || state.Size.Y() != 1.8 {
		t.Fatalf("open-air crawl was accepted: crawling=%v size=%v", state.Crawling, state.Size)
	}
}
