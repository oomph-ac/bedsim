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

func TestBubbleColumnAppliesForEachOccupiedCell(t *testing.T) {
	w := environmentWorld{
		bubbles: map[cube.Pos]BubbleColumnDirection{
			{0, 0, 0}: BubbleColumnUp,
			{0, 1, 0}: BubbleColumnUp,
		},
		blocks: map[cube.Pos]world.Block{
			{0, 0, 0}: block.Water{Still: true, Depth: 8},
			{0, 1, 0}: block.Water{Still: true, Depth: 8},
		},
	}
	state := newBaseState()
	state.Pos = mgl32.Vec3{0.5, 0, 0.5}

	(&Simulator{World: w}).applyBubbleColumns(state)

	if want := float32(0.16); math32.Abs(state.Vel.Y()-want) > 1e-6 {
		t.Fatalf("bubble-column velocity = %v, want per-cell impulses totaling %v", state.Vel.Y(), want)
	}
}

func TestBubbleColumnAppliesOutsideLiquidTravel(t *testing.T) {
	w := environmentWorld{
		bubbles: map[cube.Pos]BubbleColumnDirection{{0, 0, 0}: BubbleColumnUp},
		blocks:  map[cube.Pos]world.Block{},
	}
	state := newBaseState()
	state.Pos = mgl32.Vec3{0.5, 0, 0.5}
	state.HasGravity = false

	(&Simulator{World: w}).SimulateState(state)

	if state.Vel.Y() != 0.1 {
		t.Fatalf("normal movement missed surface bubble impulse: %v", state.Vel.Y())
	}
}

func TestRiptideLaunchesInWaterAndStartsSpinAttack(t *testing.T) {
	w := environmentWorld{blocks: map[cube.Pos]world.Block{{0, 0, 0}: block.Water{Still: true, Depth: 8}}}
	sim := &Simulator{World: w, Equipment: fixedEquipment{EnchantmentRiptide: 2}}
	state := newBaseState()
	state.Pos = mgl32.Vec3{0.5, 0, 0.5}
	state.Gravity = NormalGravity
	state.RiptideReady = true

	sim.Simulate(state, InputState{StartSpinAttack: true})

	if want := float32(1.8); math32.Abs(state.Vel.Z()-want) > 1e-6 {
		t.Fatalf("expected riptide velocity %v, got %v", want, state.Vel.Z())
	}
	if state.RiptideTicks != 19 {
		t.Fatalf("expected 19 riptide ticks after the launch tick, got %d", state.RiptideTicks)
	}
}

func TestRiptideDoesNotLaunchInLava(t *testing.T) {
	w := environmentWorld{blocks: map[cube.Pos]world.Block{{0, 0, 0}: block.Lava{Still: true, Depth: 8}}}
	sim := &Simulator{World: w, Equipment: fixedEquipment{EnchantmentRiptide: 2}}
	state := newBaseState()
	state.Pos = mgl32.Vec3{0.5, 0, 0.5}
	state.Gravity = NormalGravity
	state.RiptideReady = true

	sim.Simulate(state, InputState{StartSpinAttack: true})

	if state.RiptideTicks != 0 {
		t.Fatalf("expected lava not to start riptide, got %d ticks", state.RiptideTicks)
	}
}

func TestRiptideDoesNotLaunchWhileFlying(t *testing.T) {
	w := environmentWorld{blocks: map[cube.Pos]world.Block{{0, 0, 0}: block.Water{Still: true, Depth: 8}}}
	sim := &Simulator{World: w, Equipment: fixedEquipment{EnchantmentRiptide: 2}}
	state := newBaseState()
	state.Pos = mgl32.Vec3{0.5, 0, 0.5}
	state.Flying = true
	state.Gravity = NormalGravity
	state.RiptideReady = true

	sim.Simulate(state, InputState{StartSpinAttack: true})

	if state.RiptideTicks != 0 {
		t.Fatalf("expected flying not to start riptide, got %d ticks", state.RiptideTicks)
	}
}

func TestRiptideDoesNotLaunchWhileMounted(t *testing.T) {
	w := environmentWorld{blocks: map[cube.Pos]world.Block{{0, 0, 0}: block.Water{Still: true, Depth: 8}}}
	sim := &Simulator{World: w, Equipment: fixedEquipment{EnchantmentRiptide: 2}}
	state := newBaseState()
	state.Pos = mgl32.Vec3{0.5, 0, 0.5}
	state.Gravity = NormalGravity
	state.InVehicle = true
	state.RiptideReady = true

	sim.Simulate(state, InputState{StartSpinAttack: true})

	if state.RiptideTicks != 0 {
		t.Fatalf("expected mounted player not to start riptide, got %d ticks", state.RiptideTicks)
	}
}

func TestRiptideRequiresValidatedTridentRelease(t *testing.T) {
	w := environmentWorld{blocks: map[cube.Pos]world.Block{{0, 0, 0}: block.Water{Still: true, Depth: 8}}}
	sim := &Simulator{World: w, Equipment: fixedEquipment{EnchantmentRiptide: 2}}
	state := newBaseState()
	state.Pos = mgl32.Vec3{0.5, 0, 0.5}
	state.Gravity = NormalGravity

	sim.Simulate(state, InputState{StartSpinAttack: true})

	if state.RiptideTicks != 0 {
		t.Fatalf("expected unvalidated start flag not to launch, got %d ticks", state.RiptideTicks)
	}
}

func TestRiptideLaunchesInRainWithoutBlockWater(t *testing.T) {
	sim := &Simulator{World: mockWorld{}, Equipment: fixedEquipment{EnchantmentRiptide: 2}}
	state := newBaseState()
	state.Gravity = NormalGravity
	state.RiptideReady = true
	state.RiptideInRain = true

	sim.Simulate(state, InputState{StartSpinAttack: true})

	if state.RiptideTicks != 19 {
		t.Fatalf("expected rain-authorized riptide launch, got %d ticks", state.RiptideTicks)
	}
}

func TestRiptideStopsOnNormalMovementWallCollision(t *testing.T) {
	sim := &Simulator{World: staticWorld{chunkLoaded: true, boxes: []cube.BBox32{
		cube.Box32(0.5, -1, -1, 1.5, 2, 1),
	}}}
	state := newBaseState()
	state.Vel = mgl32.Vec3{1, 0, 0}
	state.RiptideTicks = 10
	state.HasGravity = false

	sim.SimulateState(state)

	if state.RiptideTicks != 0 {
		t.Fatalf("expected wall collision to stop riptide, got %d ticks", state.RiptideTicks)
	}
}

func TestRiptideStopRequiresValidatedEntityCollision(t *testing.T) {
	sim := &Simulator{}
	state := newBaseState()
	state.RiptideTicks = 10
	state.Vel = mgl32.Vec3{1, 0, 0}

	sim.applyInput(state, InputState{StopSpinAttack: true})
	if state.RiptideTicks != 10 || state.Vel.X() != 1 {
		t.Fatalf("spoofed stop changed riptide: ticks=%d vel=%v", state.RiptideTicks, state.Vel)
	}

	state.RiptideCollision = true
	sim.applyInput(state, InputState{StopSpinAttack: true})
	if state.RiptideTicks != 0 || state.Vel.X() != -0.2 {
		t.Fatalf("validated stop was not applied: ticks=%d vel=%v", state.RiptideTicks, state.Vel)
	}
}
