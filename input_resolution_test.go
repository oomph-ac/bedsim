package bedsim

import (
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
