package bedsim

import (
	"github.com/chewxy/math32"

	"github.com/df-mc/dragonfly/server/world"
	"github.com/oomph-ac/bedsim/block"
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

	PositionCorrectionThreshold float32
	VelocityCorrectionThreshold float32

	UseSlideOffset bool
	SprintTiming   SprintTiming

	LimitAllVelocity          bool
	LimitAllVelocityThreshold float32

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
	BlockSemantics BlockMovementSemanticsProvider
	// Liquids exposes second-layer liquids. World is used when it implements
	// LiquidProvider; otherwise waterlogged blocks are invisible.
	Liquids   LiquidProvider
	Effects   EffectsProvider
	Inventory InventoryProvider
	Options   SimulationOptions
}

func (DefaultBlockSemantics) BlockMovementSemantics(b world.Block) block.MovementSemantics {
	return block.Resolve(b, BlockName(b))
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

func validGroundFriction(friction float32) bool {
	return friction > 0 && !math32.IsInf(friction, 1)
}

func (s *Simulator) blockMovementSemantics(b world.Block) block.MovementSemantics {
	if s.BlockSemantics != nil {
		semantics := s.BlockSemantics.BlockMovementSemantics(b)
		if !validGroundFriction(semantics.GroundFriction) {
			semantics.GroundFriction = block.Resolve(b, BlockName(b)).GroundFriction
		}
		return semantics
	}
	return block.Resolve(b, BlockName(b))
}
