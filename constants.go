package bedsim

const (
	DefaultJumpHeight           = float32(0.42)
	DefaultAirFriction          = float32(0.91)
	DefaultBlockFriction        = float32(0.6)
	NormalGravityMultiplier     = float32(0.98)
	LevitationGravityMultiplier = float32(0.05)
	NormalGravity               = float32(0.08)
	SlowFallingGravity          = float32(0.01)
	StepHeight                  = float32(0.5625)
	SlideOffsetMultiplier       = float32(0.4)
	SlimeBounceMultiplier       = float32(-1)
	BedBounceMultiplier         = float32(-0.75)
	// This can be validated in Mob::ascendLadder().
	ClimbSpeed          = float32(0.2)
	MaxConsumingImpulse = float32(0.1225)
	MaxSneakImpulse     = float32(0.3)
	// Deprecated: MaxNormalizedImpulse is unused by the simulator. The
	// diagonal-impulse normalization it was intended for is disabled upstream
	// as well. It is retained only for API compatibility.
	MaxNormalizedImpulse           = float32(0.70710678118) // 1/sqrt(2)
	DefaultUnderwaterMovementSpeed = float32(0.02)
	DefaultLavaMovementSpeed       = float32(0.02)
	DefaultSwimSpeedMultiplier     = float32(1)
	GlideHorizontalLookEpsilon     = float32(1e-4)
	// WalkAirSpeed and SprintAirSpeed are the air acceleration pair vanilla
	// selects on the sprint flag alone; neither scales with the movement
	// attribute.
	WalkAirSpeed   = float32(0.02)
	SprintAirSpeed = float32(0.026)
	// WaterDrag is the ordinary horizontal water drag; sprinting uses 0.9.
	WaterDrag = float32(0.8)

	DefaultPlayerHeightOffset  = float32(1.62)
	SneakingPlayerHeightOffset = float32(1.27)

	// TerminalVelocity is the natural convergence of the gravity formula:
	// (v - 0.08) * 0.98 = v → v = -3.92. This is not explicitly clamped;
	// it emerges from the per-tick gravity and drag multipliers.
	TerminalVelocity = float32(-3.92)

	JumpDelayTicks  = 10
	GlideBoostTicks = 20

	// DefaultSwimWaterGraceTicks bounds retained server-observed water contact.
	DefaultSwimWaterGraceTicks = 10

	// EffectWeaving is the Bedrock effect ID for Weaving. Gophertunnel's
	// current named effect constants predate the trial effects.
	EffectWeaving int32 = 33
)
