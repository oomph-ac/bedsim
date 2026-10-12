package bedsim

import (
	"math"

	"github.com/chewxy/math32"
	"testing"

	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

func TestSoulSpeedSkipsSoulSandSlowdown(t *testing.T) {
	w := environmentWorld{blocks: map[cube.Pos]world.Block{
		{0, 0, 0}: semanticsNamedBlock{name: "minecraft:soul_sand"},
	}}
	base := newBaseState()
	base.Pos = mgl32.Vec3{0.5, 1, 0.5}
	base.OnGround = true
	base.HasGravity = false

	without := *base
	(&Simulator{World: w, BlockSemantics: encodedBlockSemantics{}}).Simulate(&without, InputState{MoveVector: mgl32.Vec2{0, 1}})

	with := *base
	(&Simulator{World: w, BlockSemantics: encodedBlockSemantics{}, Equipment: fixedEquipment{EnchantmentSoulSpeed: 1}}).Simulate(&with, InputState{MoveVector: mgl32.Vec2{0, 1}})

	if with.Vel.Z() <= without.Vel.Z() {
		t.Fatalf("expected Soul Speed to bypass soul-sand slowdown: with=%v without=%v", with.Vel.Z(), without.Vel.Z())
	}
}

func TestSwiftSneakAppliesAfterTwoSlowdownTicks(t *testing.T) {
	sim := &Simulator{Equipment: fixedEquipment{EnchantmentSwiftSneak: 3}}
	state := newBaseState()
	input := InputState{SneakDown: true, MoveVector: mgl32.Vec2{0, 1}}

	sim.applyInput(state, input)
	if want := float32(0.3 * 0.98); math32.Abs(state.Impulse.Y()-want) > 1e-6 {
		t.Fatalf("expected first-tick sneak impulse %v, got %v", want, state.Impulse.Y())
	}
	sim.applyInput(state, input)
	sim.applyInput(state, input)
	if want := float32(0.75 * 0.98); math32.Abs(state.Impulse.Y()-want) > 1e-6 {
		t.Fatalf("expected Swift Sneak impulse %v after two ticks, got %v", want, state.Impulse.Y())
	}
}

func TestItemUseAndInventoryActionInputRules(t *testing.T) {
	tests := []struct {
		name  string
		input InputState
		want  float32
	}{
		{name: "using item", input: InputState{UsingItem: true, MoveVector: mgl32.Vec2{0, 1}}, want: MaxConsumingImpulse * 0.98},
		{name: "using spear", input: InputState{UsingItem: true, UsingSpear: true, MoveVector: mgl32.Vec2{0, 1}}, want: 0.98},
		{name: "inventory action", input: InputState{InventoryAction: true, MoveVector: mgl32.Vec2{0, 1}}, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := newBaseState()
			(&Simulator{}).applyInput(state, tt.input)
			if math32.Abs(state.Impulse.Y()-tt.want) > 1e-6 {
				t.Fatalf("expected impulse %v, got %v", tt.want, state.Impulse.Y())
			}
		})
	}
}

func TestCrawlingUpdatesPoseAndSlowdown(t *testing.T) {
	state := newBaseState()
	sim := &Simulator{World: staticWorld{chunkLoaded: true, boxes: []cube.BBox32{
		cube.Box32(-1, 0.7, -1, 1, 2, 1),
	}}}
	sim.applyInput(state, InputState{StartCrawling: true, MoveVector: mgl32.Vec2{0, 1}})

	if !state.Crawling || state.Size.Y() != 0.6 {
		t.Fatalf("expected crawling pose, got crawling=%v size=%v", state.Crawling, state.Size)
	}
	if want := float32(0.3 * 0.98); math32.Abs(state.Impulse.Y()-want) > 1e-6 {
		t.Fatalf("expected crawling slowdown %v, got %v", want, state.Impulse.Y())
	}
}

func TestLevitationStopsGliding(t *testing.T) {
	sim := &Simulator{
		World:     mockWorld{},
		Inventory: mockInventory{hasElytra: true},
		Effects:   fixedEffects{packet.EffectLevitation: 0},
	}
	state := newBaseState()
	state.Gliding = true
	state.HasGravity = true

	sim.SimulateState(state)

	if state.Gliding {
		t.Fatal("expected levitation to stop gliding")
	}
	if state.Vel.Y() <= 0 {
		t.Fatalf("expected levitation velocity after glide stops, got %v", state.Vel)
	}
}

func TestStoppingGlideDoesNotCancelActiveBoost(t *testing.T) {
	state := newBaseState()
	state.Gliding = true
	state.GlideBoostTicks = 10

	(&Simulator{}).applyInput(state, InputState{StopGliding: true})

	if state.GlideBoostTicks != 10 {
		t.Fatalf("expected glide boost to keep ticking independently, got %d", state.GlideBoostTicks)
	}
}

func TestValidatedRocketUseArmsGlideBoost(t *testing.T) {
	state := newBaseState()
	state.Gliding = true

	(&Simulator{}).applyInput(state, InputState{StartGlideBoost: true})

	if state.GlideBoostTicks != GlideBoostTicks {
		t.Fatalf("GlideBoostTicks = %d, want %d", state.GlideBoostTicks, GlideBoostTicks)
	}
}

func TestRocketUseWithoutGlideDoesNotArmBoost(t *testing.T) {
	state := newBaseState()
	state.Gliding = false

	(&Simulator{}).applyInput(state, InputState{StartGlideBoost: true})

	if state.GlideBoostTicks != 0 {
		t.Fatalf("GlideBoostTicks = %d, want 0 without an active glide", state.GlideBoostTicks)
	}
}

func TestRocketUseOnDeployTickArmsGlideBoost(t *testing.T) {
	state := newBaseState()

	(&Simulator{}).applyInput(state, InputState{StartGliding: true, StartGlideBoost: true})

	if !state.Gliding {
		t.Fatal("expected the elytra to deploy")
	}
	if state.GlideBoostTicks != GlideBoostTicks {
		t.Fatalf("GlideBoostTicks = %d, want %d on the deploy tick", state.GlideBoostTicks, GlideBoostTicks)
	}
}

func TestStoppingGlideOnTheSameTickRefusesBoost(t *testing.T) {
	state := newBaseState()
	state.Gliding = true

	(&Simulator{}).applyInput(state, InputState{StopGliding: true, StartGlideBoost: true})

	if state.GlideBoostTicks != 0 {
		t.Fatalf("GlideBoostTicks = %d, want 0 once the glide ends", state.GlideBoostTicks)
	}
}

func TestRocketUseRearmsPartiallySpentGlideBoost(t *testing.T) {
	state := newBaseState()
	state.Gliding = true
	state.GlideBoostTicks = 3

	(&Simulator{}).applyInput(state, InputState{StartGlideBoost: true})

	if state.GlideBoostTicks != GlideBoostTicks {
		t.Fatalf("GlideBoostTicks = %d, want a full %d window", state.GlideBoostTicks, GlideBoostTicks)
	}
}

// glidingBoostSimulator returns a simulator and airborne gliding state facing
// +Z, which is the look direction for zero yaw and pitch.
func glidingBoostSimulator() (*Simulator, *MovementState) {
	sim := &Simulator{World: mockWorld{}, Inventory: mockInventory{hasElytra: true}}
	state := newBaseState()
	state.Gliding = true
	state.OnGround = false
	state.Pos = mgl32.Vec3{0, 64, 0}
	state.Client.Pos = state.Pos
	return sim, state
}

func TestGlideBoostThrustsTowardLook(t *testing.T) {
	sim, boosted := glidingBoostSimulator()
	_, unboosted := glidingBoostSimulator()

	sim.Simulate(boosted, InputState{StartGlideBoost: true})
	sim.Simulate(unboosted, InputState{})

	if boosted.Vel.Z() <= unboosted.Vel.Z() {
		t.Fatalf("boosted Z velocity %v did not exceed unboosted %v", boosted.Vel.Z(), unboosted.Vel.Z())
	}
	if boosted.Vel.Y() <= unboosted.Vel.Y() {
		t.Fatalf("boosted Y velocity %v did not exceed unboosted %v", boosted.Vel.Y(), unboosted.Vel.Y())
	}
}

func TestGlideBoostThrustsForTheFullWindow(t *testing.T) {
	sim, state := glidingBoostSimulator()

	sim.Simulate(state, InputState{StartGlideBoost: true})
	boostedTicks := 1
	for state.GlideBoostTicks > 0 {
		sim.Simulate(state, InputState{})
		boostedTicks++
	}

	if boostedTicks != GlideBoostTicks {
		t.Fatalf("boost thrust for %d ticks, want %d", boostedTicks, GlideBoostTicks)
	}

	// The window is spent: a further tick must not thrust again.
	before := state.Vel
	sim.Simulate(state, InputState{})
	if state.Vel.Z() > before.Z() {
		t.Fatalf("Z velocity rose from %v to %v after the boost window ended", before.Z(), state.Vel.Z())
	}
}

// TestGlideMatchesVanillaFloat32 pins whole-tick glide results, including the
// wrapped previous rotation, slow falling, rocket thrust and a vertical climb.
func TestGlideMatchesVanillaFloat32(t *testing.T) {
	for _, tc := range []struct {
		rot, prev   mgl32.Vec2 // pitch, yaw
		vel         mgl32.Vec3
		slow, boost bool
		want        [3]uint32
	}{
		{mgl32.Vec2{-12.5, 37.25}, mgl32.Vec2{-11.75, 33.5}, mgl32.Vec3{0.31, -0.42, 0.87}, false, false, [3]uint32{0x3e4f460d, 0xbebc2c28, 0x3f5f49a6}},
		{mgl32.Vec2{24, -170}, mgl32.Vec2{22, 175}, mgl32.Vec3{-0.6, -0.9, 0.2}, true, false, [3]uint32{0xbf031693, 0xbf4fcee8, 0x3d4e41e4}},
		{mgl32.Vec2{-35, 91.3}, mgl32.Vec2{-35, 91.3}, mgl32.Vec3{1.1, 0.05, -0.3}, false, true, [3]uint32{0xbe79be45, 0x3f061758, 0xbe19dd88}},
		{mgl32.Vec2{-90, 0}, mgl32.Vec2{-90, 0}, mgl32.Vec3{0.5, 0, 0}, false, false, [3]uint32{0x3ee41893, 0xbc807356, 0x3d01c2e3}},
	} {
		state := newBaseState()
		state.Gravity = NormalGravity
		state.Rotation = mgl32.Vec3{tc.rot[0], tc.rot[1], tc.rot[1]}
		state.LastRotation = mgl32.Vec3{tc.prev[0], tc.prev[1], tc.prev[1]}
		state.Vel = tc.vel
		state.SlowFalling = tc.slow
		if tc.boost {
			state.GlideBoostTicks = 1
		}
		(&Simulator{}).simulateGlide(state)
		for axis := range 3 {
			if got := math.Float32bits(state.Vel[axis]); got != tc.want[axis] {
				t.Fatalf("rot %v prev %v: velocity %v axis %d = %#08x, want %#08x", tc.rot, tc.prev, state.Vel, axis, got, tc.want[axis])
			}
		}
	}
}
