package bedsim

import (
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"strings"
)

// MovementSpeedWithoutSprint returns the effective walking attribute, retaining
// Speed/Slowness and custom movement modifiers. BDS sends Value with the sprint
// multiplier already applied, whereas DefaultMovementSpeed must exclude it.
// The modifier identity and operation are confirmed by Lens 1.26.50.26,
// RVA 0x3693cd0, and the real BDS 1.26.45.1 UpdateAttributes packet.
func MovementSpeedWithoutSprint(attribute protocol.Attribute) float32 {
	for _, modifier := range attribute.Modifiers {
		if strings.EqualFold(modifier.ID, "d208fc00-42aa-4aad-9276-d5446530de43") &&
			modifier.Operation == protocol.AttributeModifierOperationMultiplyTotal &&
			modifier.Operand == protocol.AttributeModifierOperandCurrent {
			return attribute.Value / (1 + modifier.Amount)
		}
	}
	return attribute.Value
}
