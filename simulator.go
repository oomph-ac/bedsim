package bedsim

// SimulationMode defines how strict the simulator should be with client corrections.
type SimulationMode uint8

const (
	// SimulationModeAuthoritative flags corrections when either position or velocity drift exceeds thresholds.
	SimulationModeAuthoritative SimulationMode = iota
	// SimulationModePermissive only flags positional drift and ignores velocity-only divergence.
	SimulationModePermissive
	// SimulationModePassive never flags corrections; simulation still runs and returns deltas.
	SimulationModePassive
)

// SprintTiming defines when movement speed changes apply relative to simulation.
type SprintTiming uint8

const (
	SprintTimingModern SprintTiming = iota
	SprintTimingLegacy
)

// SimulationOptions define simulator behavior and correction thresholds.
type SimulationOptions struct {
	Mode SimulationMode

	PositionCorrectionThreshold float64
	VelocityCorrectionThreshold float64

	UseSlideOffset bool
	SprintTiming   SprintTiming

	LimitAllVelocity          bool
	LimitAllVelocityThreshold float64

	// IgnoreClientStepTiebreaker, when true, skips the client-alignment
	// tie-breaker in the step-up collision logic. Pathfinders that drive their
	// own movement should set this so that legitimate step-ups are never
	// rejected due to the client position matching the pre-step position.
	IgnoreClientStepTiebreaker bool

	// Debugf receives internal simulation trace logs for callers that need deep diagnostics.
	Debugf func(format string, args ...any)
}

// Simulator orchestrates movement simulation using the provided adapters.
type Simulator struct {
	World     WorldProvider
	Effects   EffectsProvider
	Inventory InventoryProvider
	Options   SimulationOptions
}
