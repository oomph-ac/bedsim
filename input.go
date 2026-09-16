package bedsim

import "github.com/go-gl/mathgl/mgl32"

// InputState represents a single tick's client input and reported state.
type InputState struct {
	MoveVector mgl32.Vec2
	// MoveVectorIsRaw asks BedSim to apply client movement multipliers to raw
	// controls. False preserves validation of an already processed packet vector.
	// Raw controls always apply slowdown, regardless of UpstreamImpulseClamping.
	MoveVectorIsRaw bool

	Pitch   float32
	Yaw     float32
	HeadYaw float32

	ClientPos mgl32.Vec3
	ClientVel mgl32.Vec3

	HorizontalCollision bool
	VerticalCollision   bool

	StartFlying bool
	StopFlying  bool

	StartSprinting bool
	StopSprinting  bool
	SprintDown     bool

	StartSneaking bool
	StopSneaking  bool
	SneakDown     bool
	Sneaking      bool

	StartJumping       bool
	Jumping            bool
	AutoJumpingInWater bool
	AscendBlock        bool

	StartSwimming bool
	StopSwimming  bool
	WantDown      bool
	WantDownSlow  bool
	StartCrawling bool
	StopCrawling  bool
	DescendBlock  bool

	StopGliding  bool
	StartGliding bool

	// ItemUseMovementModifier overrides item-use slowdown with a finite value in [0, 1].
	// Nil derives the modifier from the item-use flags. The pointed-to value must
	// remain immutable while this input is used or retained. Pose slowdown is separate.
	ItemUseMovementModifier *float32

	UsingConsumable bool
	UsingItem       bool
	UsingSpear      bool
	InventoryAction bool

	StartSpinAttack bool
	StopSpinAttack  bool
}
