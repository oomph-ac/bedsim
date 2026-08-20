package bedsim

import (
	"math"
	"reflect"
	"testing"

	"github.com/go-gl/mathgl/mgl32"
)

type numericStateField struct {
	name  string
	index []int
	typ   reflect.Type
}

func TestFiniteMovementStateRejectsEveryFloatField(t *testing.T) {
	base := newBaseState()
	fields := collectNumericStateFields(reflect.TypeOf(*base), nil, "MovementState")
	if len(fields) == 0 {
		t.Fatal("no numeric movement fields discovered")
	}

	for _, field := range fields {
		t.Run(field.name, func(t *testing.T) {
			state := *base
			value := reflect.ValueOf(&state).Elem().FieldByIndex(field.index)
			switch field.typ {
			case reflect.TypeFor[float32]():
				value.SetFloat(math.NaN())
			case reflect.TypeFor[mgl32.Vec2](), reflect.TypeFor[mgl32.Vec3]():
				value.Index(0).SetFloat(math.NaN())
			default:
				t.Fatalf("unsupported numeric field type %v", field.typ)
			}
			if finiteMovementState(&state) {
				t.Fatalf("finiteMovementState accepted NaN in %s", field.name)
			}
		})
	}
}

// collectNumericStateFields returns every float or movement-vector field in a
// movement state, including fields nested in value structs.
func collectNumericStateFields(t reflect.Type, prefix []int, name string) []numericStateField {
	floatType := reflect.TypeFor[float32]()
	vec2Type := reflect.TypeFor[mgl32.Vec2]()
	vec3Type := reflect.TypeFor[mgl32.Vec3]()
	fields := make([]numericStateField, 0)
	for i := range t.NumField() {
		field := t.Field(i)
		index := append(append([]int(nil), prefix...), i)
		fieldName := name + "." + field.Name
		switch field.Type {
		case floatType, vec2Type, vec3Type:
			fields = append(fields, numericStateField{name: fieldName, index: index, typ: field.Type})
		default:
			if field.Type.Kind() == reflect.Struct {
				fields = append(fields, collectNumericStateFields(field.Type, index, fieldName)...)
			}
		}
	}
	return fields
}
