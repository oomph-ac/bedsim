package bedsim

const (
	DefaultJumpHeight           = 0.42
	DefaultAirFriction          = 0.91
	DefaultBlockFriction        = 0.6
	NormalGravityMultiplier     = 0.98
	LevitationGravityMultiplier = 0.05
	NormalGravity               = 0.08
	SlowFallingGravity          = 0.01
	StepHeight                  = 0.6
	SlideOffsetMultiplier       = 0.4
	SlimeBounceMultiplier       = -1.0
	BedBounceMultiplier         = -0.66
	// This can be validated in Mob::ascendLadder().
	ClimbSpeed           = 0.2
	MaxConsumingImpulse  = 0.1225
	MaxSneakImpulse      = 0.3
	MaxNormalizedImpulse = 0.70710678118 // 1/sqrt(2)

	DefaultPlayerHeightOffset  = 1.62
	SneakingPlayerHeightOffset = 1.27

	// TerminalVelocity is the natural convergence of the gravity formula:
	// (v - 0.08) * 0.98 = v → v = -3.92. This is not explicitly clamped;
	// it emerges from the per-tick gravity and drag multipliers.
	TerminalVelocity = -3.92

	JumpDelayTicks  = 10
	GlideBoostTicks = 20
)
