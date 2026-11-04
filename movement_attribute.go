package bedsim

import (
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"strings"
)

// MovementSpeedWithoutSprint returns the effective walking attribute, retaining
// Speed/Slowness and custom movement modifiers. BDS sends Value with the sprint
// multiplier already applied, whereas DefaultMovementSpeed must exclude it.
// When modifier metadata is absent, sprinting is the resolved server-facing
// sprint state associated with the attribute update.
func MovementSpeedWithoutSprint(attribute protocol.Attribute, sprinting bool) float32 {
	for _, modifier := range attribute.Modifiers {
		if strings.EqualFold(modifier.ID, "d208fc00-42aa-4aad-9276-d5446530de43") &&
			modifier.Operation == protocol.AttributeModifierOperationMultiplyTotal &&
			modifier.Operand == protocol.AttributeModifierOperandCurrent {
			return attribute.Value / (1 + modifier.Amount)
		}
	}
	if len(attribute.Modifiers) == 0 && sprinting {
		return attribute.Value / SprintSpeedMultiplier
	}
	return attribute.Value
}
