package bedsim

import (
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/go-gl/mathgl/mgl32"
)

// ClientState holds non-authoritative movement data sent by the client.
type ClientState struct {
	Pos, LastPos mgl32.Vec3
	Vel, LastVel mgl32.Vec3
	Mov, LastMov mgl32.Vec3

	HorizontalCollision bool
	VerticalCollision   bool
	ToggledFly          bool
}

// MovementState holds the authoritative movement state for a single entity.
type MovementState struct {
	Client ClientState

	Pos, LastPos mgl32.Vec3
	Vel, LastVel mgl32.Vec3
	Mov, LastMov mgl32.Vec3

	Rotation, LastRotation mgl32.Vec3

	SlideOffset mgl32.Vec2
	Impulse     mgl32.Vec2
	Size        mgl32.Vec3
	// StuckSpeedMultiplier is the strongest queued berry-bush or powder-snow
	// multiplier. It applies to one displacement, then clears along with
	// persistent velocity.
	StuckSpeedMultiplier mgl32.Vec3
	// StandingHeight, SneakingHeight, and CrawlingHeight preserve custom entity
	// dimensions across pose transitions. Zero values use the current standing
	// height and vanilla player pose heights respectively.
	StandingHeight float32
	SneakingHeight float32
	CrawlingHeight float32

	SupportingBlockPos *cube.Pos

	Gravity float32
	// JumpHeight is an output derived by Simulate from JumpStrength and active
	// effects; set JumpStrength to customize the base jump velocity.
	JumpHeight float32
	// JumpStrength is the base jump velocity. Zero uses DefaultJumpHeight.
	JumpStrength float32
	FallDistance float32

	// MovementSpeed is the effective movement attribute used by travel. Include
	// movement effects in this value before passing the state to BedSim.
	MovementSpeed float32
	// DefaultMovementSpeed is the effective non-sprinting movement attribute
	// used when sprinting is toggled.
	DefaultMovementSpeed float32
	// AirSpeed is the air acceleration speed, which does not track the movement
	// attribute. Simulate sets it from the sprint state; SimulateState callers
	// provide it as part of the current state.
	AirSpeed                float32
	UnderwaterMovementSpeed float32
	LavaMovementSpeed       float32
	// SwimSpeedMultiplier scales swimming acceleration; zero means the default.
	SwimSpeedMultiplier float32
	// DolphinBoostTicks is the remaining dolphin-boost duration.
	DolphinBoostTicks  int64
	ServerUpdatedSpeed bool

	Knockback           mgl32.Vec3
	TicksSinceKnockback uint64
	KnockbackPending    bool

	PendingTeleportPos mgl32.Vec3
	PendingTeleports   int

	TeleportPos             mgl32.Vec3
	TicksSinceTeleport      uint64
	TeleportCompletionTicks uint64
	TeleportIsSmoothed      bool
	TeleportPending         bool

	Sprinting, PressingSprint         bool
	ServerSprint, ServerSprintApplied bool

	Sneaking, PressingSneak bool
	PressingAscend          bool
	PressingDescend         bool

	Jumping, PressingJump bool
	EffectiveJumping      bool
	JumpDelay             uint64

	Swimming   bool
	SwimAmount float32
	// StoppedSwimmingThisTick selects the client's fast water drag on the
	// transition out of swimming.
	StoppedSwimmingThisTick bool
	// SwimWaterGraceTicks retains recent server-observed water contact.
	SwimWaterGraceTicks    int64
	AutoJumpingInWater     bool
	WantDown, WantDownSlow bool

	CollideX, CollideY, CollideZ bool
	OnGround                     bool

	PenetratedLastFrame, StuckInCollider bool

	Immobile bool
	NoClip   bool

	Gliding         bool
	GlideBoostTicks int64

	HasGravity bool
	// SlowFalling reports whether the slow-falling effect is active. Its lower
	// gravity only applies while descending.
	SlowFalling bool

	Crawling              bool
	TicksSinceCanSlowdown int
	RiptideTicks          int
	StartingSpinAttack    bool
	// RiptideReady is a one-tick trusted latch set after validating a charged
	// Riptide trident release. RiptideCollision is set after a server-observed
	// entity collision and authorizes the matching stop/reversal.
	RiptideReady     bool
	RiptideCollision bool
	// RiptideInRain is server-observed weather exposure that permits a
	// validated Riptide release without direct block-water contact.
	RiptideInRain bool
	// InVehicle is server-authoritative mounted state. Riptide cannot start
	// while the player is riding a vehicle.
	InVehicle bool

	Flying, MayFly, TrustFlyStatus bool
	JustDisabledFlight             bool

	AllowedInputs int64
	HasFirstInput bool

	PendingCorrections   int
	InCorrectionCooldown bool

	Ready bool
	Alive bool

	GameMode int32
}

func (s *MovementState) ensurePoseHeights() {
	if s.StandingHeight <= 0 {
		if !s.Sneaking && !s.Crawling && s.Size.Y() > 0 {
			s.StandingHeight = s.Size.Y()
		} else {
			s.StandingHeight = 1.8
		}
	}
	if s.SneakingHeight <= 0 {
		s.SneakingHeight = 1.49
	}
	if s.CrawlingHeight <= 0 {
		s.CrawlingHeight = 0.6
	}
}

func (s *MovementState) SetPos(newPos mgl32.Vec3) {
	s.LastPos = s.Pos
	s.Pos = newPos
}

func (s *MovementState) SetVel(newVel mgl32.Vec3) {
	s.LastVel = s.Vel
	s.Vel = newVel
}

func (s *MovementState) SetMov(newMov mgl32.Vec3) {
	s.LastMov = s.Mov
	s.Mov = newMov
}

func (s *MovementState) SetRotation(newRot mgl32.Vec3) {
	s.LastRotation = s.Rotation
	s.Rotation = newRot
}

func (s *MovementState) HasKnockback() bool {
	return s.KnockbackPending || (s.TicksSinceKnockback == 0 && s.Knockback != (mgl32.Vec3{}))
}

func (s *MovementState) HasTeleport() bool {
	if s.TeleportPending || s.PendingTeleports > 0 {
		return true
	}
	if s.TeleportCompletionTicks == 0 {
		return s.TicksSinceTeleport == 0 && s.TeleportPos != (mgl32.Vec3{})
	}
	return s.TicksSinceTeleport <= s.TeleportCompletionTicks
}

func (s *MovementState) RemainingTeleportTicks() int {
	if !s.HasTeleport() || s.TicksSinceTeleport >= s.TeleportCompletionTicks {
		return 0
	}
	remaining := s.TeleportCompletionTicks - s.TicksSinceTeleport
	maxInt := uint64(^uint(0) >> 1)
	if remaining > maxInt {
		return int(maxInt)
	}
	return int(remaining)
}

// QueueKnockback schedules one authoritative velocity replacement.
func (s *MovementState) QueueKnockback(velocity mgl32.Vec3) {
	s.Knockback = velocity
	s.KnockbackPending = true
	s.TicksSinceKnockback = 0
}

// QueueTeleport schedules one authoritative teleport.
func (s *MovementState) QueueTeleport(pos mgl32.Vec3, smoothed bool, completionTicks uint64) {
	s.PendingTeleportPos = pos
	s.TeleportPos = pos
	s.TeleportIsSmoothed = smoothed
	s.TeleportCompletionTicks = completionTicks
	s.TicksSinceTeleport = 0
	s.TeleportPending = true
}
