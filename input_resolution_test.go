package bedsim

import (
	"math"
	"reflect"
	"testing"

	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/go-gl/mathgl/mgl32"
)

// TestRawMovementResolution preserves analogue magnitude and combines item and
// pose slowdown once, even when received packet vectors use unclamped semantics.
func TestRawMovementResolution(t *testing.T) {
	for _, upstream := range []bool{false, true} {
		for _, tc := range []struct {
			name  string
			input InputState
			want  float32
		}{
			{"normal", InputState{}, 1},
			{"consuming", InputState{UsingConsumable: true}, MaxConsumingImpulse},
			{"held item", InputState{UsingItem: true}, MaxConsumingImpulse},
			{"spear", InputState{UsingItem: true, UsingSpear: true}, 1},
			{"sneaking", InputState{SneakDown: true}, MaxSneakImpulse},
			{"consuming sneak", InputState{UsingConsumable: true, SneakDown: true}, MaxConsumingImpulse * MaxSneakImpulse},
			{"inventory", InputState{InventoryAction: true}, 0},
		} {
			t.Run(tc.name, func(t *testing.T) {
				state := newBaseState()
				sim := Simulator{Options: SimulationOptions{UpstreamImpulseClamping: upstream}}
				in := tc.input
				in.MoveVector, in.MoveVectorIsRaw = mgl32.Vec2{.25, -.5}, true
				result := sim.Simulate(state, in)
				want := in.MoveVector.Mul(tc.want)
				if result.InputMoveVector != want || state.Impulse != want.Mul(.98) {
					t.Fatalf("upstream=%v: input %v impulse %v, want %v", upstream, result.InputMoveVector, state.Impulse, want)
				}
			})
		}
	}
}

// TestRawMovementSwiftSneakUsesSimulationHistory keeps delayed equipment rules
// at the same owner for both generated and received movement input.
func TestRawMovementSwiftSneakUsesSimulationHistory(t *testing.T) {
	state := newBaseState()
	sim := Simulator{Equipment: fixedEquipment{EnchantmentSwiftSneak: 3}, Options: SimulationOptions{UpstreamImpulseClamping: true}}
	for tick, scale := range []float32{.3, .3, .75} {
		result := sim.Simulate(state, InputState{SneakDown: true, MoveVector: mgl32.Vec2{0, .5}, MoveVectorIsRaw: true})
		if result.InputMoveVector != (mgl32.Vec2{0, .5 * scale}) {
			t.Fatalf("tick %d: got %v, want scale %v", tick, result.InputMoveVector, scale)
		}
	}
}

// TestMovementStateCloneDetachesReferences protects speculative state forks and
// requires an ownership decision whenever state gains another reference field.
func TestMovementStateCloneDetachesReferences(t *testing.T) {
	var check func(reflect.Type, string)
	check = func(typ reflect.Type, path string) {
		switch typ.Kind() {
		case reflect.Struct:
			for field := range typ.NumField() {
				f := typ.Field(field)
				check(f.Type, path+"."+f.Name)
			}
		case reflect.Array:
			check(typ.Elem(), path+"[]")
		case reflect.Pointer:
			if path != ".SupportingBlockPos" || typ != reflect.TypeFor[*cube.Pos]() {
				t.Fatalf("new reference field %s needs Clone ownership coverage", path)
			}
		case reflect.Map, reflect.Slice, reflect.Interface, reflect.Func, reflect.Chan, reflect.UnsafePointer:
			t.Fatalf("new reference field %s needs Clone ownership coverage", path)
		}
	}
	check(reflect.TypeFor[MovementState](), "")
	original := newBaseState()
	pos := cube.Pos{1, 2, 3}
	original.SupportingBlockPos = &pos
	copy := original.Clone()
	if !reflect.DeepEqual(copy, *original) {
		t.Fatal("clone changed movement state")
	}
	copy.SupportingBlockPos[0] = 9
	if original.SupportingBlockPos[0] != 1 {
		t.Fatal("clone retained supporting-block reference")
	}
}

// TestItemUseMovementModifier checks explicit overrides and rejects invalid values before mutation.
func TestItemUseMovementModifier(t *testing.T) {
	for _, value := range []float32{0, MaxConsumingImpulse, .5, 1} {
		for _, sneak := range []bool{false, true} {
			for _, upstream := range []bool{false, true} {
				state := newBaseState()
				sim := Simulator{Options: SimulationOptions{UpstreamImpulseClamping: upstream}}
				input := InputState{MoveVector: mgl32.Vec2{.25, -.5}, MoveVectorIsRaw: true, UsingConsumable: true, UsingItem: true, SneakDown: sneak, ItemUseMovementModifier: &value}
				result := sim.Simulate(state, input)
				scale := value
				if sneak {
					scale *= MaxSneakImpulse
				}
				if want := input.MoveVector.Mul(scale); result.InputMoveVector != want {
					t.Fatalf("modifier %v sneak %v upstream %v: got %v want %v", value, sneak, upstream, result.InputMoveVector, want)
				}
			}
		}
	}
	for _, value := range []float32{-1, 1.1, float32(math.NaN()), float32(math.Inf(1)), float32(math.Inf(-1))} {
		state := newBaseState()
		before := state.Clone()
		result := (&Simulator{}).Simulate(state, InputState{ItemUseMovementModifier: &value})
		if result.Outcome != SimulationOutcomeInvalidInput || !reflect.DeepEqual(*state, before) {
			t.Fatalf("invalid modifier %v mutated state or accepted: %v", value, result.Outcome)
		}
	}
}

// TestRawMovementSlowdownRoundsPoseThenItem pins vanilla's float32 order: pose
// slowdown first, then the 0.35 modifier squared, each product rounded.
func TestRawMovementSlowdownRoundsPoseThenItem(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input InputState
		want  uint32
	}{
		{"consuming", InputState{UsingConsumable: true}, 0x3d96872b},
		{"consuming sneak", InputState{UsingConsumable: true, SneakDown: true}, 0x3cb4a234},
	} {
		in := tc.input
		in.MoveVector, in.MoveVectorIsRaw = mgl32.Vec2{0, .6}, true
		result := (&Simulator{}).Simulate(newBaseState(), in)
		if got := math.Float32bits(result.InputMoveVector.Y()); got != tc.want {
			t.Fatalf("%s: forward %#08x, want %#08x", tc.name, got, tc.want)
		}
	}
}

// TestRawMovementNormalisesOntoUnitCircle keeps diagonal raw input at unit length.
func TestRawMovementNormalisesOntoUnitCircle(t *testing.T) {
	result := (&Simulator{}).Simulate(newBaseState(), InputState{MoveVector: mgl32.Vec2{1, 1}, MoveVectorIsRaw: true})
	want := float32(1 / math.Sqrt2)
	if result.InputMoveVector != (mgl32.Vec2{want, want}) {
		t.Fatalf("diagonal raw input = %v, want %v on both axes", result.InputMoveVector, want)
	}
}

// TestSneakSlowdownFollowsVanillaGate covers held descend, gliding (no trigger
// and no exemption) and the swimming and in-water exemptions.
func TestSneakSlowdownFollowsVanillaGate(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(*MovementState)
		input InputState
		want  float32
	}{
		{"held descend", func(*MovementState) {}, InputState{DescendBlock: true}, MaxSneakImpulse},
		{"gliding", func(s *MovementState) { s.Gliding = true }, InputState{}, 1},
		{"gliding held sneak", func(s *MovementState) { s.Gliding = true }, InputState{SneakDown: true}, MaxSneakImpulse},
		{"gliding held descend", func(s *MovementState) { s.Gliding = true }, InputState{DescendBlock: true}, MaxSneakImpulse},
		{"swimming", func(s *MovementState) { s.Swimming = true }, InputState{SneakDown: true}, 1},
		{"in water", func(s *MovementState) { s.swimWaterContact = true }, InputState{SneakDown: true}, 1},
	} {
		state := newBaseState()
		tc.setup(state)
		in := tc.input
		in.MoveVector, in.MoveVectorIsRaw = mgl32.Vec2{0, 1}, true
		(&Simulator{}).applyInput(state, in)
		if want := tc.want * 0.98; state.Impulse.Y() != want {
			t.Fatalf("%s: impulse %v, want %v", tc.name, state.Impulse.Y(), want)
		}
	}
}
