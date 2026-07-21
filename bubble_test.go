package bedsim

import (
	"math"
	"testing"

	"github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/go-gl/mathgl/mgl64"
)

func TestBubbleColumnUsesBoarImpulsesAndCaps(t *testing.T) {
	tests := []struct {
		name      string
		direction BubbleColumnDirection
		surface   bool
		initial   float64
		want      float64
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
			if math.Abs(state.Vel.Y()-tt.want) > 1e-12 {
				t.Fatalf("expected y velocity %v, got %v", tt.want, state.Vel.Y())
			}
		})
	}
}

func TestBubbleColumnSurfaceAcceptsRegistryBackedAir(t *testing.T) {
	w := environmentWorld{
		bubbles: map[cube.Pos]BubbleColumnDirection{{0, 0, 0}: BubbleColumnUp},
		blocks:  map[cube.Pos]world.Block{{0, 1, 0}: namedBlock{name: "minecraft:air"}},
	}
	state := newBaseState()
	state.Pos = mgl64.Vec3{0.5, 0, 0.5}

	(&Simulator{World: w, BlockSemantics: encodedBlockSemantics{}}).applyBubbleColumns(state)

	if state.Vel.Y() != 0.1 {
		t.Fatalf("expected surface bubble impulse above registry-backed air, got %v", state.Vel.Y())
	}
}

func TestRiptideLaunchesInWaterAndStartsSpinAttack(t *testing.T) {
	w := environmentWorld{blocks: map[cube.Pos]world.Block{{0, 0, 0}: block.Water{Still: true, Depth: 8}}}
	sim := &Simulator{World: w, Equipment: fixedEquipment{EnchantmentRiptide: 2}}
	state := newBaseState()
	state.Pos = mgl64.Vec3{0.5, 0, 0.5}
	state.Gravity = NormalGravity

	sim.Simulate(state, InputState{StartSpinAttack: true})

	if want := 1.8; math.Abs(state.Vel.Z()-want) > 1e-9 {
		t.Fatalf("expected riptide velocity %v, got %v", want, state.Vel.Z())
	}
	if state.RiptideTicks != 19 {
		t.Fatalf("expected 19 riptide ticks after the launch tick, got %d", state.RiptideTicks)
	}
}
