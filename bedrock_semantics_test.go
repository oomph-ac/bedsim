package bedsim

import (
	"github.com/chewxy/math32"
	"testing"

	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/go-gl/mathgl/mgl32"
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
			if got := sim.blockAir(semanticsNamedBlock{name: tt.name}); got != tt.want {
				t.Fatalf("blockAir(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestJavaWebIdentifierHasNoBedrockMovementEffect(t *testing.T) {
	w := environmentWorld{blocks: map[cube.Pos]world.Block{
		{0, 0, 0}: semanticsNamedBlock{name: "minecraft:cobweb"},
	}}
	state := newBaseState()
	state.Pos = mgl32.Vec3{0.5, 0, 0.5}
	state.Vel = mgl32.Vec3{0.1, 0, 0}
	state.HasGravity = false

	result := (&Simulator{World: w, BlockSemantics: encodedBlockSemantics{}}).SimulateState(state)

	if want := float32(0.1); math32.Abs(result.Movement.X()-want) > 1e-6 {
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
