package bedsim

import "github.com/go-gl/mathgl/mgl32"

// SimulationOutcome describes which path the simulator took for the current tick.
type SimulationOutcome uint8

const (
	SimulationOutcomeNormal SimulationOutcome = iota
	SimulationOutcomeTeleport
	SimulationOutcomeUnreliable
	SimulationOutcomeUnloadedChunk
	SimulationOutcomeImmobileOrNotReady
	SimulationOutcomeMounted
	SimulationOutcomeInvalidInput
)

// SimulationResult captures the outcome of a single simulation tick.
type SimulationResult struct {
	Position mgl32.Vec3
	Velocity mgl32.Vec3
	Movement mgl32.Vec3

	// InputMoveVector is the processed primary input used by Simulate, before
	// the 0.98 movement-impulse factor. SimulateState does not resolve input and
	// leaves this zero. Raw controls and analogue vectors remain caller-owned.
	InputMoveVector mgl32.Vec2

	OnGround bool
	CollideX bool
	CollideY bool
	CollideZ bool

	PositionDelta   mgl32.Vec3
	VelocityDelta   mgl32.Vec3
	NeedsCorrection bool

	Outcome SimulationOutcome
}
