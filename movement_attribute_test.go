package bedsim

import (
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"testing"
)

func TestMovementSpeedWithoutSprint(t *testing.T) {
	for _, tt := range []struct {
		name        string
		value, want float32
		modifiers   []protocol.AttributeModifier
	}{
		{name: "walking", value: 0.1, want: 0.1},
		{name: "custom speed without sprint", value: 0.13, want: 0.13},
		{name: "sprinting", value: 0.13, want: 0.1, modifiers: []protocol.AttributeModifier{{ID: "d208fc00-42aa-4aad-9276-d5446530de43", Amount: 0.3, Operation: 2, Operand: 2}}},
		{name: "speed effect and sprint", value: 0.156, want: 0.120000005, modifiers: []protocol.AttributeModifier{{ID: "D208FC00-42AA-4AAD-9276-D5446530DE43", Amount: 0.3, Operation: 2, Operand: 2}}},
		{name: "name alone is not identity", value: 0.13, want: 0.13, modifiers: []protocol.AttributeModifier{{ID: "custom", Name: "Sprinting speed boost", Amount: 0.3, Operation: 2, Operand: 2}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := MovementSpeedWithoutSprint(protocol.Attribute{AttributeValue: protocol.AttributeValue{Value: tt.value}, Modifiers: tt.modifiers})
			if got != tt.want {
				t.Fatalf("got %.9g want %.9g", got, tt.want)
			}
		})
	}
}
