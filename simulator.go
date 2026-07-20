package bedsim

import (
	"math"

	"github.com/df-mc/dragonfly/server/world"
)

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

	// RequireLiquidLayer refuses simulation without second-layer liquid data.
	RequireLiquidLayer bool

	// SwimWaterGraceTicks bounds retained water contact. Zero uses the default;
	// a negative value disables retention.
	SwimWaterGraceTicks int64

	// UpstreamImpulseClamping opts into oomph PR #145's unclamped impulses.
	UpstreamImpulseClamping bool

	// Debugf receives internal simulation trace logs for callers that need deep diagnostics.
	Debugf func(format string, args ...any)
}

// Simulator orchestrates movement simulation using the provided adapters.
type Simulator struct {
	World WorldProvider
	// BlockSemantics optionally resolves movement-specific block behavior from
	// the same world snapshot as World. Nil uses DefaultBlockSemantics.
	BlockSemantics BlockSemanticsProvider
	// Liquids exposes second-layer liquids. World is used when it implements
	// LiquidProvider; otherwise waterlogged blocks are invisible.
	Liquids   LiquidProvider
	Effects   EffectsProvider
	Inventory InventoryProvider
	Options   SimulationOptions
}

func (DefaultBlockSemantics) BlockName(b world.Block) string {
	return BlockName(b)
}

func (DefaultBlockSemantics) BlockFriction(b world.Block) float64 {
	return BlockFriction(b)
}

func (DefaultBlockSemantics) BlockClimbable(b world.Block) bool {
	return BlockClimbable(b)
}

// swimWaterGraceTicks resolves the configured grace window: zero means the
// default, and any negative value disables the grace entirely.
func (s *Simulator) swimWaterGraceTicks() int64 {
	switch {
	case s.Options.SwimWaterGraceTicks < 0:
		return 0
	case s.Options.SwimWaterGraceTicks == 0:
		return DefaultSwimWaterGraceTicks
	default:
		return s.Options.SwimWaterGraceTicks
	}
}

func (s *Simulator) blockName(b world.Block) string {
	if s.BlockSemantics != nil {
		return s.BlockSemantics.BlockName(b)
	}
	return BlockName(b)
}

func (s *Simulator) blockFriction(b world.Block) float64 {
	if s.BlockSemantics != nil {
		if friction := s.BlockSemantics.BlockFriction(b); friction > 0 && !math.IsInf(friction, 1) {
			return friction
		}
	}
	return BlockFriction(b)
}

func (s *Simulator) blockClimbable(b world.Block) bool {
	if s.BlockSemantics != nil {
		return s.BlockSemantics.BlockClimbable(b)
	}
	return BlockClimbable(b)
}
