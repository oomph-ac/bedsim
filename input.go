package bedsim

import "github.com/go-gl/mathgl/mgl64"

// InputState represents a single tick's client input and reported state.
type InputState struct {
	MoveVector mgl64.Vec2

	Pitch   float64
	Yaw     float64
	HeadYaw float64

	ClientPos mgl64.Vec3
	ClientVel mgl64.Vec3

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

	UsingConsumable bool
	UsingItem       bool
	UsingSpear      bool
	InventoryAction bool

	StartSpinAttack bool
	StopSpinAttack  bool
}
