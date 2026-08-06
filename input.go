package bedsim

import "github.com/go-gl/mathgl/mgl32"

// InputState represents a single tick's client input and reported state.
type InputState struct {
	MoveVector mgl32.Vec2

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

	UsingConsumable bool
	UsingItem       bool
	UsingSpear      bool
	InventoryAction bool

	StartSpinAttack bool
	StopSpinAttack  bool
}
