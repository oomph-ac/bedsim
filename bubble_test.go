package bedsim

import (
	"github.com/chewxy/math32"
	"testing"

	"github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/go-gl/mathgl/mgl32"
)

func TestBubbleColumnUsesBoarImpulsesAndCaps(t *testing.T) {
	tests := []struct {
		name      string
		direction BubbleColumnDirection
		surface   bool
		initial   float32
		want      float32
	}{
		{name: "submerged up", direction: BubbleColumnUp, initial: 0, want: 0.06},
		{name: "submerged up cap", direction: BubbleColumnUp, initial: 0.69, want: 0.70},
		{name: "surface up", direction: BubbleColumnUp, surface: true, initial: 0, want: 0.10},
		{name: "surface up cap", direction: BubbleColumnUp, surface: true, initial: 1.79, want: 1.80},
		{name: "submerged down", direction: BubbleColumnDown, initial: 0, want: -0.03},
		{name: "submerged down cap", direction: BubbleColumnDown, initial: -0.29, want: -0.30},
		{name: "surface down", direction: BubbleColumnDown, surface: true, initial: 0, want: -0.03},
		{name: "surface down cap", direction: BubbleColumnDown, surface: true, initial: -0.89, want: -0.90},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := newBaseState()
			state.Vel[1] = tt.initial
			applyBubbleColumn(state, tt.direction, tt.surface)
			if math32.Abs(state.Vel.Y()-tt.want) > 1e-6 {
				t.Fatalf("expected y velocity %v, got %v", tt.want, state.Vel.Y())
			}
		})
	}
}

func TestBubbleColumnSurfaceAcceptsRegistryBackedAir(t *testing.T) {
	w := environmentWorld{
		bubbles: map[cube.Pos]BubbleColumnDirection{{0, 0, 0}: BubbleColumnUp},
		blocks:  map[cube.Pos]world.Block{{0, 1, 0}: semanticsNamedBlock{name: "minecraft:air"}},
	}
	state := newBaseState()
	state.Pos = mgl32.Vec3{0.5, 0, 0.5}

	(&Simulator{World: w, BlockSemantics: encodedBlockSemantics{}}).applyBubbleColumns(state)

	if state.Vel.Y() != 0.1 {
		t.Fatalf("expected surface bubble impulse above registry-backed air, got %v", state.Vel.Y())
	}
}

func TestBubbleColumnAppliesOnceAcrossMultipleCells(t *testing.T) {
	w := environmentWorld{bubbles: map[cube.Pos]BubbleColumnDirection{
		{0, 0, 0}: BubbleColumnUp,
		{0, 1, 0}: BubbleColumnUp,
	}}
	state := newBaseState()
	state.Pos = mgl32.Vec3{0.5, 0, 0.5}

	(&Simulator{World: w}).applyBubbleColumns(state)

	if want := float32(0.1); math32.Abs(state.Vel.Y()-want) > 1e-6 {
		t.Fatalf("bubble-column velocity = %v, want one impulse %v", state.Vel.Y(), want)
	}
}

func TestRiptideLaunchesInWaterAndStartsSpinAttack(t *testing.T) {
	w := environmentWorld{blocks: map[cube.Pos]world.Block{{0, 0, 0}: block.Water{Still: true, Depth: 8}}}
	sim := &Simulator{World: w, Equipment: fixedEquipment{EnchantmentRiptide: 2}}
	state := newBaseState()
	state.Pos = mgl32.Vec3{0.5, 0, 0.5}
	state.Gravity = NormalGravity

	sim.Simulate(state, InputState{StartSpinAttack: true})

	if want := float32(1.8); math32.Abs(state.Vel.Z()-want) > 1e-6 {
		t.Fatalf("expected riptide velocity %v, got %v", want, state.Vel.Z())
	}
	if state.RiptideTicks != 19 {
		t.Fatalf("expected 19 riptide ticks after the launch tick, got %d", state.RiptideTicks)
	}
}
