package bedsim

import (
	"github.com/chewxy/math32"
	"testing"

	"github.com/df-mc/dragonfly/server/block"
	dfcube "github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/ethaniccc/float32-cube/cube"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

// waterSource and lavaSource are full still source blocks, the most common
// liquid a player is submerged in.
var (
	waterSource = block.Water{Depth: 8, Still: true}
	lavaSource  = block.Lava{Depth: 8, Still: true}
)

// liquidWorld is a WorldProvider backed by explicit block placements. It
// deliberately does not implement LiquidProvider, so simulations against it
// exercise the fallback path through WorldProvider.Block.
type liquidWorld struct {
	blocks      map[dfcube.Pos]world.Block
	chunkLoaded bool
}

func newLiquidWorld() *liquidWorld {
	return &liquidWorld{blocks: map[dfcube.Pos]world.Block{}, chunkLoaded: true}
}

func (w *liquidWorld) set(pos dfcube.Pos, b world.Block) *liquidWorld {
	w.blocks[pos] = b
	return w
}

// fill places b in the inclusive cuboid between min and max.
func (w *liquidWorld) fill(min, max dfcube.Pos, b world.Block) *liquidWorld {
	for x := min[0]; x <= max[0]; x++ {
		for y := min[1]; y <= max[1]; y++ {
			for z := min[2]; z <= max[2]; z++ {
				w.blocks[dfcube.Pos{x, y, z}] = b
			}
		}
	}
	return w
}

func (w *liquidWorld) Block(pos dfcube.Pos) world.Block {
	if b, ok := w.blocks[pos]; ok {
		return b
	}
	return block.Air{}
}

func (w *liquidWorld) BlockCollisions(pos dfcube.Pos) []cube.BBox {
	b := w.Block(pos)
	if _, air := b.(block.Air); air {
		return nil
	}
	if _, liquid := b.(world.Liquid); liquid {
		return nil
	}
	return []cube.BBox{cube.Box(0, 0, 0, 1, 1, 1).Translate(posVec3(pos))}
}

func (w *liquidWorld) GetNearbyBBoxes(aabb cube.BBox) []cube.BBox {
	min, max := aabb.Min(), aabb.Max()
	var out []cube.BBox
	for x := int(math32.Floor(min.X())); x <= int(math32.Floor(max.X())); x++ {
		for y := int(math32.Floor(min.Y())); y <= int(math32.Floor(max.Y())); y++ {
			for z := int(math32.Floor(min.Z())); z <= int(math32.Floor(max.Z())); z++ {
				for _, bb := range w.BlockCollisions(dfcube.Pos{x, y, z}) {
					if bb.IntersectsWith(aabb) {
						out = append(out, bb)
					}
				}
			}
		}
	}
	return out
}

func (w *liquidWorld) IsChunkLoaded(chunkX, chunkZ int32) bool {
	return w.chunkLoaded
}

// layeredLiquidWorld implements LiquidProvider, exposing liquids stored in the
// second block layer (waterlogged blocks) as well as the main layer.
type layeredLiquidWorld struct {
	*liquidWorld
	layer map[dfcube.Pos]world.Liquid
}

func newLayeredLiquidWorld() *layeredLiquidWorld {
	return &layeredLiquidWorld{liquidWorld: newLiquidWorld(), layer: map[dfcube.Pos]world.Liquid{}}
}

func (w *layeredLiquidWorld) waterlog(pos dfcube.Pos, b world.Block, liquid world.Liquid) *layeredLiquidWorld {
	w.blocks[pos] = b
	w.layer[pos] = liquid
	return w
}

func (w *layeredLiquidWorld) Liquid(pos dfcube.Pos) (world.Liquid, bool) {
	if liquid, ok := w.layer[pos]; ok {
		return liquid, true
	}
	liquid, ok := w.Block(pos).(world.Liquid)
	return liquid, ok
}

type levitationEffects struct {
	amplifier int32
}

func (e levitationEffects) GetEffect(effectID int32) (int32, bool) {
	if effectID == packet.EffectLevitation {
		return e.amplifier, true
	}
	return 0, false
}

type depthStriderInventory struct {
	level int
}

func (depthStriderInventory) HasElytra() bool { return false }

func (i depthStriderInventory) DepthStriderLevel() int { return i.level }

func newLiquidSim(w WorldProvider) *Simulator {
	return &Simulator{
		World:   w,
		Effects: mockEffects{},
		Options: SimulationOptions{PositionCorrectionThreshold: 0.3},
	}
}

// submergedState returns a state standing inside a liquid column at 0.5/0.5/0.5.
func submergedState() *MovementState {
	state := newBaseState()
	state.Pos = mgl32.Vec3{0.5, 0.5, 0.5}
	state.Client.Pos = state.Pos
	return state
}

// filledColumn returns a world where the 3x3 column around the origin is filled
// with the given liquid from y=0 to y=3, so the player is fully submerged and
// the liquid gradient is uniform (no flow).
func filledColumn(b world.Block) *liquidWorld {
	return newLiquidWorld().fill(dfcube.Pos{-2, 0, -2}, dfcube.Pos{2, 3, 2}, b)
}

func approxEqual(a, b float32) bool {
	return math32.Abs(a-b) < 1e-6
}

func assertVec(t *testing.T, got, want mgl32.Vec3) {
	t.Helper()
	if !approxEqual(got.X(), want.X()) || !approxEqual(got.Y(), want.Y()) || !approxEqual(got.Z(), want.Z()) {
		t.Fatalf("velocity = %v, want %v", got, want)
	}
}

// A swimming player's hitbox collapses to a width-sized cube, matching the
// client's swim pose. This drives collision, liquid detection and exit probing.
func TestSwimmingBoundingBoxUsesWidthAsHeight(t *testing.T) {
	state := newBaseState()
	state.Pos = mgl32.Vec3{0.5, 10, 0.5}

	standing := state.BoundingBox(false)
	if height := standing.Height(); !approxEqual(height, 1.8) {
		t.Fatalf("standing height = %v, want 1.8", height)
	}

	state.Swimming = true
	state.SwimWaterGraceTicks = DefaultSwimWaterGraceTicks
	swimming := state.BoundingBox(false)
	if height := swimming.Height(); !approxEqual(height, 0.6) {
		t.Fatalf("swimming height = %v, want 0.6", height)
	}
	if width := swimming.Width(); !approxEqual(width, standing.Width()) {
		t.Fatalf("swimming width = %v, want unchanged %v", width, standing.Width())
	}
}

// The swimming flag alone must not shrink the server-side hitbox: without
// server-observed water contact a client could otherwise claim the swim pose in
// open air and fit through gaps a standing player cannot.
func TestSwimmingFlagAloneDoesNotShrinkHitbox(t *testing.T) {
	state := newBaseState()
	state.Pos = mgl32.Vec3{0.5, 10, 0.5}
	state.Swimming = true
	state.SwimWaterGraceTicks = 0

	if state.SwimPose() {
		t.Fatal("swim pose must require water evidence")
	}
	if height := state.BoundingBox(false).Height(); !approxEqual(height, 1.8) {
		t.Fatalf("height = %v, want the full standing hitbox", height)
	}
	if height := state.ClientBoundingBox(false).Height(); !approxEqual(height, 1.8) {
		t.Fatalf("client height = %v, want the full standing hitbox", height)
	}
}

// A spoofed swimming flag with no water anywhere must not let the player pass
// through a gap that only the collapsed swim hitbox fits.
func TestSpoofedSwimmingCannotFitThroughCeilingGap(t *testing.T) {
	sim := newLiquidSim(newLiquidWorld().set(dfcube.Pos{0, 2, 0}, block.Stone{}))
	state := newBaseState()
	state.Pos = mgl32.Vec3{0.5, 0, 0.5}
	state.Client.Pos = state.Pos
	state.Swimming = true
	state.Vel = mgl32.Vec3{0, 0.5, 0}

	sim.SimulateState(state)
	if !state.CollideY {
		t.Fatal("a spoofed swim pose must still collide with the ceiling")
	}
}

func TestSwimmingClientBoundingBoxUsesWidthAsHeight(t *testing.T) {
	state := newBaseState()
	state.Client.Pos = mgl32.Vec3{0.5, 10, 0.5}

	if height := state.ClientBoundingBox(false).Height(); !approxEqual(height, 1.8) {
		t.Fatalf("standing client height = %v, want 1.8", height)
	}
	state.Swimming = true
	state.SwimWaterGraceTicks = DefaultSwimWaterGraceTicks
	if height := state.ClientBoundingBox(false).Height(); !approxEqual(height, 0.6) {
		t.Fatalf("swimming client height = %v, want 0.6", height)
	}
}

// The swim hitbox must scale with the entity size, not use a hardcoded 0.6.
func TestSwimmingBoundingBoxRespectsScale(t *testing.T) {
	state := newBaseState()
	state.Pos = mgl32.Vec3{0.5, 10, 0.5}
	state.Size = mgl32.Vec3{0.6, 1.8, 2}
	state.Swimming = true
	state.SwimWaterGraceTicks = DefaultSwimWaterGraceTicks

	if height := state.BoundingBox(false).Height(); !approxEqual(height, 1.2) {
		t.Fatalf("scaled swimming height = %v, want 1.2", height)
	}
}

// Jumping is edge-triggered by StartJumping alone. A held jump key sets
// PressingJump and EffectiveJumping, but must not re-arm the ground jump.
func TestJumpingIsEdgeTriggeredByStartJumping(t *testing.T) {
	sim := newLiquidSim(newLiquidWorld())
	state := newBaseState()

	sim.applyInput(state, InputState{Jumping: true})
	if state.Jumping {
		t.Fatal("held jump must not set Jumping without StartJumping")
	}
	if !state.PressingJump {
		t.Fatal("held jump must set PressingJump")
	}
	if !state.EffectiveJumping {
		t.Fatal("held jump must set EffectiveJumping")
	}

	sim.applyInput(state, InputState{StartJumping: true})
	if !state.Jumping {
		t.Fatal("StartJumping must set Jumping")
	}
}

func TestEffectiveJumpingSources(t *testing.T) {
	sim := newLiquidSim(newLiquidWorld())
	for name, input := range map[string]InputState{
		"jumping":            {Jumping: true},
		"autoJumpingInWater": {AutoJumpingInWater: true},
		"ascendBlock":        {AscendBlock: true},
	} {
		state := newBaseState()
		sim.applyInput(state, input)
		if !state.EffectiveJumping {
			t.Fatalf("%s must set EffectiveJumping", name)
		}
	}

	state := newBaseState()
	sim.applyInput(state, InputState{StartJumping: true})
	if state.EffectiveJumping {
		t.Fatal("StartJumping alone must not set EffectiveJumping")
	}
}

// SwimAmount interpolates by 0.1 per tick, using the swimming state as it was
// at the start of the tick.
func TestSwimAmountInterpolation(t *testing.T) {
	sim := newLiquidSim(newLiquidWorld())
	state := newBaseState()

	// The first StartSwimming tick still decays, because wasSwimming is false.
	sim.applyInput(state, InputState{StartSwimming: true})
	if !state.Swimming {
		t.Fatal("StartSwimming must set Swimming")
	}
	if !approxEqual(state.SwimAmount, 0) {
		t.Fatalf("SwimAmount = %v, want 0", state.SwimAmount)
	}

	for i := 1; i <= 3; i++ {
		sim.applyInput(state, InputState{})
		if want := float32(float32(i) * 0.1); !approxEqual(state.SwimAmount, want) {
			t.Fatalf("tick %d: SwimAmount = %v, want %v", i, state.SwimAmount, want)
		}
	}

	sim.applyInput(state, InputState{StopSwimming: true})
	if state.Swimming {
		t.Fatal("StopSwimming must clear Swimming")
	}
	if !approxEqual(state.SwimAmount, 0.4) {
		t.Fatalf("SwimAmount = %v, want 0.4 (still rising on the stop tick)", state.SwimAmount)
	}
	sim.applyInput(state, InputState{})
	if !approxEqual(state.SwimAmount, 0.3) {
		t.Fatalf("SwimAmount = %v, want 0.3", state.SwimAmount)
	}
}

func TestSwimAmountClampedToUnitRange(t *testing.T) {
	sim := newLiquidSim(newLiquidWorld())
	state := newBaseState()
	state.Swimming = true
	for range 30 {
		sim.applyInput(state, InputState{})
	}
	if !approxEqual(state.SwimAmount, 1) {
		t.Fatalf("SwimAmount = %v, want clamped to 1", state.SwimAmount)
	}

	state.Swimming = false
	for range 30 {
		sim.applyInput(state, InputState{})
	}
	if !approxEqual(state.SwimAmount, 0) {
		t.Fatalf("SwimAmount = %v, want clamped to 0", state.SwimAmount)
	}
}

// StartSwimming cancels sneaking, since the two poses are mutually exclusive.
func TestStartSwimmingClearsSneaking(t *testing.T) {
	sim := newLiquidSim(newLiquidWorld())
	state := newBaseState()
	sim.applyInput(state, InputState{SneakDown: true, StartSneaking: true, StartSwimming: true})
	if state.Sneaking {
		t.Fatal("StartSwimming must clear Sneaking")
	}
}

// StopSwimming wins when both flags arrive in the same tick.
func TestStopSwimmingTakesPriority(t *testing.T) {
	sim := newLiquidSim(newLiquidWorld())
	state := newBaseState()
	state.Swimming = true
	sim.applyInput(state, InputState{StartSwimming: true, StopSwimming: true})
	if state.Swimming {
		t.Fatal("StopSwimming must take priority over StartSwimming")
	}
}

// A player resting in still water sinks at the water gravity rate, with water
// drag applied before gravity.
func TestWaterDragAndGravity(t *testing.T) {
	sim := newLiquidSim(filledColumn(waterSource))
	state := submergedState()

	sim.SimulateState(state)
	assertVec(t, state.Vel, mgl32.Vec3{0, -0.005, 0})

	// Second tick: previous velocity is dragged by 0.8, then gravity applies.
	sim.SimulateState(state)
	assertVec(t, state.Vel, mgl32.Vec3{0, -0.005*0.8 - 0.005, 0})
}

// Sprinting in water raises horizontal drag from 0.8 to 0.9.
func TestWaterSprintDrag(t *testing.T) {
	sim := newLiquidSim(filledColumn(waterSource))

	normal := submergedState()
	normal.Vel = mgl32.Vec3{0.5, 0, 0}
	sim.SimulateState(normal)

	sprinting := submergedState()
	sprinting.Vel = mgl32.Vec3{0.5, 0, 0}
	sprinting.Sprinting = true
	sim.SimulateState(sprinting)

	if !approxEqual(normal.Vel.X(), 0.5*0.8) {
		t.Fatalf("walking drag: X = %v, want %v", normal.Vel.X(), 0.5*0.8)
	}
	if !approxEqual(sprinting.Vel.X(), 0.5*0.9) {
		t.Fatalf("sprinting drag: X = %v, want %v", sprinting.Vel.X(), 0.5*0.9)
	}
}

// Vertical drag in water is always 0.8, independent of sprinting.
func TestWaterVerticalDragIndependentOfSprint(t *testing.T) {
	sim := newLiquidSim(filledColumn(waterSource))
	state := submergedState()
	state.Vel = mgl32.Vec3{0, 0.5, 0}
	state.Sprinting = true
	sim.SimulateState(state)

	assertVec(t, state.Vel, mgl32.Vec3{0, 0.5*0.8 - 0.005, 0})
}

// Lava uses a flat 0.5 drag on every axis and a heavier 0.02 gravity.
func TestLavaDragAndGravity(t *testing.T) {
	sim := newLiquidSim(filledColumn(lavaSource))
	state := submergedState()
	state.Vel = mgl32.Vec3{0.4, 0.4, 0.4}

	sim.SimulateState(state)
	assertVec(t, state.Vel, mgl32.Vec3{0.2, 0.4*0.5 - 0.02, 0.2})
}

// Swimming removes water gravity entirely.
func TestSwimmingCancelsWaterGravity(t *testing.T) {
	sim := newLiquidSim(filledColumn(waterSource))
	state := submergedState()
	state.Swimming = true
	state.Rotation = mgl32.Vec3{0, 0, 0}

	sim.SimulateState(state)
	if !approxEqual(state.Vel.Y(), 0) {
		t.Fatalf("swimming vertical velocity = %v, want 0", state.Vel.Y())
	}
}

// Gravity is skipped entirely when the state has no gravity.
func TestNoGravityInLiquid(t *testing.T) {
	sim := newLiquidSim(filledColumn(waterSource))
	state := submergedState()
	state.HasGravity = false

	sim.SimulateState(state)
	assertVec(t, state.Vel, mgl32.Vec3{})
}

// Levitation replaces liquid gravity with a pull toward the levitation target.
func TestLevitationOverridesLiquidGravity(t *testing.T) {
	sim := newLiquidSim(filledColumn(waterSource))
	sim.Effects = levitationEffects{amplifier: 0}
	state := submergedState()

	sim.SimulateState(state)
	// target = 0.05 * (0+1); vel += (target - vel) * 0.2
	assertVec(t, state.Vel, mgl32.Vec3{0, 0.05 * 0.2, 0})
}

func TestLevitationAmplifierScales(t *testing.T) {
	sim := newLiquidSim(filledColumn(waterSource))
	sim.Effects = levitationEffects{amplifier: 3}
	state := submergedState()

	sim.SimulateState(state)
	assertVec(t, state.Vel, mgl32.Vec3{0, (LevitationGravityMultiplier * 4) * 0.2, 0})
}

// A nil effects provider must not panic and must fall back to gravity.
func TestNilEffectsProviderFallsBackToGravity(t *testing.T) {
	sim := newLiquidSim(filledColumn(waterSource))
	sim.Effects = nil
	state := submergedState()

	sim.SimulateState(state)
	assertVec(t, state.Vel, mgl32.Vec3{0, -0.005, 0})
}

// Falling into liquid clears accumulated fall distance.
func TestLiquidResetsFallDistance(t *testing.T) {
	sim := newLiquidSim(filledColumn(waterSource))
	state := submergedState()
	state.FallDistance = 12

	sim.SimulateState(state)
	if state.FallDistance != 0 {
		t.Fatalf("FallDistance = %v, want 0", state.FallDistance)
	}
}

// Holding jump underwater adds a fixed 0.04 ascent impulse.
func TestEffectiveJumpingAscendsInWater(t *testing.T) {
	sim := newLiquidSim(filledColumn(waterSource))
	state := submergedState()
	state.EffectiveJumping = true

	sim.SimulateState(state)
	assertVec(t, state.Vel, mgl32.Vec3{0, 0.04*0.8 - 0.005, 0})
}

// Mid-transition into the swim pose zeroes the ascent instead of applying it.
func TestSwimTransitionZeroesJumpAscent(t *testing.T) {
	sim := newLiquidSim(filledColumn(waterSource))
	state := submergedState()
	state.EffectiveJumping = true
	state.SwimAmount = 0.5

	sim.SimulateState(state)
	assertVec(t, state.Vel, mgl32.Vec3{0, -0.005, 0})
}

// A fully-transitioned swimmer still ascends normally.
func TestFullSwimAmountStillAscends(t *testing.T) {
	sim := newLiquidSim(filledColumn(waterSource))
	state := submergedState()
	state.EffectiveJumping = true
	state.SwimAmount = 1

	sim.SimulateState(state)
	if state.Vel.Y() <= 0 {
		t.Fatalf("vertical velocity = %v, want positive ascent", state.Vel.Y())
	}
}

// WantDown and WantDownSlow each sink the player by 0.04 before drag.
func TestWantDownSinksInWater(t *testing.T) {
	for name, apply := range map[string]func(*MovementState){
		"wantDown":     func(s *MovementState) { s.WantDown = true },
		"wantDownSlow": func(s *MovementState) { s.WantDownSlow = true },
	} {
		t.Run(name, func(t *testing.T) {
			sim := newLiquidSim(filledColumn(waterSource))
			state := submergedState()
			apply(state)

			sim.SimulateState(state)
			assertVec(t, state.Vel, mgl32.Vec3{0, -0.04*0.8 - 0.005, 0})
		})
	}
}

// The descend inputs are water-only; lava ignores them.
func TestWantDownIgnoredInLava(t *testing.T) {
	sim := newLiquidSim(filledColumn(lavaSource))
	state := submergedState()
	state.WantDown = true

	sim.SimulateState(state)
	assertVec(t, state.Vel, mgl32.Vec3{0, -0.02, 0})
}

// The descend inputs must not alter the sneak impulse clamp. Upstream dropped
// the clamp entirely in this PR, but that is a breaking, non-liquid API change
// that bedsim deliberately does not follow, so behavior stays as in v0.1.3.
func TestDescendInputsDoNotChangeSneakImpulseClamp(t *testing.T) {
	sim := newLiquidSim(filledColumn(waterSource))

	sneaking := newBaseState()
	sim.applyInput(sneaking, InputState{SneakDown: true, MoveVector: mgl32.Vec2{0, 1}})

	descending := newBaseState()
	sim.applyInput(descending, InputState{SneakDown: true, WantDown: true, MoveVector: mgl32.Vec2{0, 1}})

	if !approxEqual(descending.Impulse.Y(), sneaking.Impulse.Y()) {
		t.Fatalf("descending impulse %v must match sneaking impulse %v",
			descending.Impulse.Y(), sneaking.Impulse.Y())
	}
	if !approxEqual(sneaking.Impulse.Y(), MaxSneakImpulse*0.98) {
		t.Fatalf("sneaking impulse = %v, want %v", sneaking.Impulse.Y(), MaxSneakImpulse*0.98)
	}
}

// While swimming, pitch steers vertical velocity toward -sin(pitch).
func TestSwimTravelFollowsPitch(t *testing.T) {
	sim := newLiquidSim(filledColumn(waterSource))
	state := submergedState()
	state.Swimming = true
	state.Rotation = mgl32.Vec3{-90, 0, 0} // looking straight up

	sim.SimulateState(state)
	// targetY = -sin(-90deg) = 1; vel += (1 - 0) * 0.06, then drag 0.8.
	assertVec(t, state.Vel, mgl32.Vec3{0, 0.06 * 0.8, 0})
}

// A steep downward pitch uses the faster 0.085 interpolation rate.
func TestSwimTravelUsesFasterRateWhenDivingSteeply(t *testing.T) {
	sim := newLiquidSim(filledColumn(waterSource))
	state := submergedState()
	state.Swimming = true
	state.Rotation = mgl32.Vec3{90, 0, 0} // looking straight down

	sim.SimulateState(state)
	// targetY = -sin(90deg) = -1, below -0.2 so rate is 0.085.
	assertVec(t, state.Vel, mgl32.Vec3{0, -0.085 * 0.8, 0})
}

// Swim travel is suppressed while jumping, letting the jump impulse win.
func TestSwimTravelSkippedWhileJumping(t *testing.T) {
	sim := newLiquidSim(filledColumn(waterSource))
	state := submergedState()
	state.Swimming = true
	state.EffectiveJumping = true
	state.SwimAmount = 1
	state.Rotation = mgl32.Vec3{90, 0, 0}

	sim.SimulateState(state)
	// Pitch steering skipped, so only the 0.04 jump impulse applies.
	assertVec(t, state.Vel, mgl32.Vec3{0, 0.04 * 0.8, 0})
}

// Swimming upward at the surface stops the climb once the head clears the
// liquid, preventing the player from swimming out into open air.
func TestSwimTravelStopsAtSurface(t *testing.T) {
	// Liquid only below the player's head-check probes.
	w := newLiquidWorld().fill(dfcube.Pos{-2, -4, -2}, dfcube.Pos{2, 0, 2}, waterSource)
	sim := newLiquidSim(w)
	state := submergedState()
	// Both head probes (+0.52 and +0.42) clear the liquid surface at y=1.
	state.Pos = mgl32.Vec3{0.5, 1.5, 0.5}
	state.Swimming = true
	state.Rotation = mgl32.Vec3{-90, 0, 0}
	state.Vel = mgl32.Vec3{0, 0.5, 0}
	// The hitbox has just left the water, so water travel is still in its
	// grace window.
	state.SwimWaterGraceTicks = DefaultSwimWaterGraceTicks

	sim.SimulateState(state)
	// Vertical velocity is zeroed at the surface, and swimming has no gravity.
	if !approxEqual(state.Vel.Y(), 0) {
		t.Fatalf("surface vertical velocity = %v, want 0", state.Vel.Y())
	}
}

// While the head is still submerged the climb continues normally.
func TestSwimTravelContinuesWhileHeadSubmerged(t *testing.T) {
	w := newLiquidWorld().fill(dfcube.Pos{-2, -4, -2}, dfcube.Pos{2, 0, 2}, waterSource)
	sim := newLiquidSim(w)
	state := submergedState()
	state.Swimming = true
	state.Rotation = mgl32.Vec3{-90, 0, 0}
	state.Vel = mgl32.Vec3{0, 0.5, 0}

	sim.SimulateState(state)
	if approxEqual(state.Vel.Y(), 0) {
		t.Fatal("a submerged head must not trigger the surface clamp")
	}
}

// WantDownSlow suppresses the surface clamp so the player can hover.
func TestSwimTravelSurfaceClampSkippedWhenWantDownSlow(t *testing.T) {
	w := newLiquidWorld().fill(dfcube.Pos{-2, -4, -2}, dfcube.Pos{2, 0, 2}, waterSource)
	sim := newLiquidSim(w)
	state := submergedState()
	state.Pos = mgl32.Vec3{0.5, 1.5, 0.5}
	state.Swimming = true
	state.Rotation = mgl32.Vec3{-90, 0, 0}
	state.WantDownSlow = true
	state.Vel = mgl32.Vec3{0, 0.5, 0}
	state.SwimWaterGraceTicks = DefaultSwimWaterGraceTicks

	sim.SimulateState(state)
	if approxEqual(state.Vel.Y(), 0) {
		t.Fatal("WantDownSlow must skip the surface clamp")
	}
}

// Depth Strider lowers the horizontal drag coefficient toward 0.546, so
// existing momentum decays faster rather than slower.
func TestDepthStriderLowersDragCoefficient(t *testing.T) {
	base := newLiquidSim(filledColumn(waterSource))
	baseState := submergedState()
	baseState.Vel = mgl32.Vec3{0.5, 0, 0}
	base.SimulateState(baseState)

	strider := newLiquidSim(filledColumn(waterSource))
	strider.Inventory = depthStriderInventory{level: 3}
	striderState := submergedState()
	striderState.Vel = mgl32.Vec3{0.5, 0, 0}
	striderState.OnGround = true
	strider.SimulateState(striderState)

	if !approxEqual(baseState.Vel.X(), 0.5*0.8) {
		t.Fatalf("base drag X = %v, want %v", baseState.Vel.X(), 0.5*0.8)
	}
	// Level 3 on the ground: drag = 0.8 + (0.54600006 - 0.8) * 1.
	if !approxEqual(striderState.Vel.X(), 0.5*0.54600006) {
		t.Fatalf("depth strider drag X = %v, want %v", striderState.Vel.X(), 0.5*0.54600006)
	}
	if !(striderState.Vel.X() < baseState.Vel.X()) {
		t.Fatalf("depth strider X = %v must decay faster than base X = %v",
			striderState.Vel.X(), baseState.Vel.X())
	}
}

// Depth Strider raises water acceleration from the underwater speed toward the
// player's full movement speed.
func TestDepthStriderIncreasesAcceleration(t *testing.T) {
	base := newLiquidSim(filledColumn(waterSource))
	baseState := submergedState()
	baseState.Impulse = mgl32.Vec2{0, 0.98}
	base.SimulateState(baseState)

	strider := newLiquidSim(filledColumn(waterSource))
	strider.Inventory = depthStriderInventory{level: 3}
	striderState := submergedState()
	striderState.Impulse = mgl32.Vec2{0, 0.98}
	striderState.OnGround = true
	strider.SimulateState(striderState)

	if !(math32.Abs(striderState.Vel.Z()) > math32.Abs(baseState.Vel.Z())) {
		t.Fatalf("depth strider Z = %v must exceed base Z = %v",
			striderState.Vel.Z(), baseState.Vel.Z())
	}
}

// Depth Strider is halved while airborne (not standing on the floor).
func TestDepthStriderHalvedWhenAirborne(t *testing.T) {
	sim := newLiquidSim(filledColumn(waterSource))
	sim.Inventory = depthStriderInventory{level: 3}
	state := submergedState()
	state.Vel = mgl32.Vec3{0.5, 0, 0}
	state.OnGround = false

	sim.SimulateState(state)
	// level 1.5 -> fraction 0.5 -> drag = 0.8 + (0.54600006 - 0.8) * 0.5.
	want := float32(0.5 * (0.8 + (0.54600006-0.8)*0.5))
	if !approxEqual(state.Vel.X(), want) {
		t.Fatalf("airborne depth strider X = %v, want %v", state.Vel.X(), want)
	}
}

// Depth Strider levels above the enchantment maximum are clamped to 3.
func TestDepthStriderClampedToMaxLevel(t *testing.T) {
	clamped := newLiquidSim(filledColumn(waterSource))
	clamped.Inventory = depthStriderInventory{level: 99}
	clampedState := submergedState()
	clampedState.Vel = mgl32.Vec3{0.5, 0, 0}
	clampedState.OnGround = true
	clamped.SimulateState(clampedState)

	if !approxEqual(clampedState.Vel.X(), 0.5*0.54600006) {
		t.Fatalf("clamped depth strider X = %v, want level-3 behavior", clampedState.Vel.X())
	}
}

// A negative level must not amplify movement.
func TestDepthStriderNegativeLevelIgnored(t *testing.T) {
	sim := newLiquidSim(filledColumn(waterSource))
	sim.Inventory = depthStriderInventory{level: -5}
	state := submergedState()
	state.Vel = mgl32.Vec3{0.5, 0, 0}

	sim.SimulateState(state)
	if !approxEqual(state.Vel.X(), 0.5*0.8) {
		t.Fatalf("negative depth strider X = %v, want plain water drag", state.Vel.X())
	}
}

// Depth Strider only applies in water, never in lava.
func TestDepthStriderIgnoredInLava(t *testing.T) {
	sim := newLiquidSim(filledColumn(lavaSource))
	sim.Inventory = depthStriderInventory{level: 3}
	state := submergedState()
	state.Vel = mgl32.Vec3{0.5, 0, 0}

	sim.SimulateState(state)
	if !approxEqual(state.Vel.X(), 0.5*0.5) {
		t.Fatalf("lava X = %v, want flat 0.5 drag", state.Vel.X())
	}
}

// An inventory that does not implement DepthStriderProvider must be ignored.
func TestInventoryWithoutDepthStriderProvider(t *testing.T) {
	sim := newLiquidSim(filledColumn(waterSource))
	sim.Inventory = mockInventory{}
	state := submergedState()
	state.Vel = mgl32.Vec3{0.5, 0, 0}

	sim.SimulateState(state)
	if !approxEqual(state.Vel.X(), 0.5*0.8) {
		t.Fatalf("X = %v, want plain water drag", state.Vel.X())
	}
}

func TestZeroEquipmentDepthStriderFallsBackToLegacyInventory(t *testing.T) {
	sim := newLiquidSim(filledColumn(waterSource))
	sim.Inventory = depthStriderInventory{level: 3}
	sim.Equipment = fixedEquipment{}
	state := submergedState()
	state.Vel = mgl32.Vec3{0.5, 0, 0}
	state.OnGround = true

	sim.SimulateState(state)

	if !approxEqual(state.Vel.X(), 0.5*0.54600006) {
		t.Fatalf("legacy depth strider X = %v, want level-3 behavior", state.Vel.X())
	}
}

// A dolphin boost raises the swim speed multiplier, which only takes effect
// while actually swimming.
func TestSwimSpeedMultiplierRequiresSwimming(t *testing.T) {
	boosted := newLiquidSim(filledColumn(waterSource))
	boostedState := submergedState()
	boostedState.Swimming = true
	boostedState.SwimSpeedMultiplier = 2
	boostedState.Impulse = mgl32.Vec2{0, 0.98}
	boosted.SimulateState(boostedState)

	plain := newLiquidSim(filledColumn(waterSource))
	plainState := submergedState()
	plainState.Swimming = true
	plainState.SwimSpeedMultiplier = 1
	plainState.Impulse = mgl32.Vec2{0, 0.98}
	plain.SimulateState(plainState)

	if !(math32.Abs(boostedState.Vel.Z()) > math32.Abs(plainState.Vel.Z())) {
		t.Fatalf("boosted Z = %v must exceed plain Z = %v", boostedState.Vel.Z(), plainState.Vel.Z())
	}

	notSwimming := newLiquidSim(filledColumn(waterSource))
	notSwimmingState := submergedState()
	notSwimmingState.SwimSpeedMultiplier = 2
	notSwimmingState.Impulse = mgl32.Vec2{0, 0.98}
	notSwimming.SimulateState(notSwimmingState)

	if !approxEqual(notSwimmingState.Vel.Z(), plainState.Vel.Z()) {
		t.Fatalf("non-swimming Z = %v, want unboosted %v", notSwimmingState.Vel.Z(), plainState.Vel.Z())
	}
}

// The dolphin boost expires on its own tick counter and restores the default
// multiplier when it runs out.
func TestDolphinBoostTicksExpire(t *testing.T) {
	sim := newLiquidSim(filledColumn(waterSource))
	state := submergedState()
	state.Swimming = true
	state.DolphinBoostTicks = 2
	state.SwimSpeedMultiplier = 2

	sim.Simulate(state, InputState{})
	if state.DolphinBoostTicks != 1 {
		t.Fatalf("DolphinBoostTicks = %d, want 1", state.DolphinBoostTicks)
	}
	if !approxEqual(state.SwimSpeedMultiplier, 2) {
		t.Fatalf("SwimSpeedMultiplier = %v, want still boosted", state.SwimSpeedMultiplier)
	}

	sim.Simulate(state, InputState{})
	if state.DolphinBoostTicks != 0 {
		t.Fatalf("DolphinBoostTicks = %d, want 0", state.DolphinBoostTicks)
	}
	if !approxEqual(state.SwimSpeedMultiplier, DefaultSwimSpeedMultiplier) {
		t.Fatalf("SwimSpeedMultiplier = %v, want reset to %v", state.SwimSpeedMultiplier, DefaultSwimSpeedMultiplier)
	}
}

// An unset multiplier must not zero out water acceleration.
func TestZeroSwimSpeedMultiplierTreatedAsDefault(t *testing.T) {
	sim := newLiquidSim(filledColumn(waterSource))
	state := submergedState()
	state.Swimming = true
	state.SwimSpeedMultiplier = 0
	state.Impulse = mgl32.Vec2{0, 0.98}
	sim.SimulateState(state)

	explicit := newLiquidSim(filledColumn(waterSource))
	explicitState := submergedState()
	explicitState.Swimming = true
	explicitState.SwimSpeedMultiplier = DefaultSwimSpeedMultiplier
	explicitState.Impulse = mgl32.Vec2{0, 0.98}
	explicit.SimulateState(explicitState)

	assertVec(t, state.Vel, explicitState.Vel)
}

// Unset movement speeds fall back to the documented defaults.
func TestZeroMovementSpeedsUseDefaults(t *testing.T) {
	sim := newLiquidSim(filledColumn(waterSource))
	state := submergedState()
	state.UnderwaterMovementSpeed = 0
	state.Impulse = mgl32.Vec2{0, 0.98}
	sim.SimulateState(state)

	explicit := newLiquidSim(filledColumn(waterSource))
	explicitState := submergedState()
	explicitState.UnderwaterMovementSpeed = DefaultUnderwaterMovementSpeed
	explicitState.Impulse = mgl32.Vec2{0, 0.98}
	explicit.SimulateState(explicitState)

	assertVec(t, state.Vel, explicitState.Vel)
}

// Water is detected through a shallow vertical offset, so a player standing on
// top of a water block is still considered to be in water.
func TestWaterDetectedAtFeet(t *testing.T) {
	sim := newLiquidSim(newLiquidWorld().set(dfcube.Pos{0, 0, 0}, waterSource))
	state := submergedState()

	if got := len(sim.touchingLiquidBlocks(state, liquidWater)); got != 1 {
		t.Fatalf("water blocks = %d, want 1", got)
	}
}

// Lava uses a wider horizontal shrink than water, so a player at the very edge
// of a lava block touches water but not lava.
func TestLavaUsesWiderHorizontalMargin(t *testing.T) {
	w := newLiquidWorld().set(dfcube.Pos{0, 0, 0}, waterSource).set(dfcube.Pos{1, 0, 0}, lavaSource)
	sim := newLiquidSim(w)
	state := submergedState()
	// Position the player so the box only just reaches into x=1.
	state.Pos = mgl32.Vec3{0.75, 0.5, 0.5}

	water := sim.touchingLiquidBlocks(state, liquidWater)
	lava := sim.touchingLiquidBlocks(state, liquidLava)
	if len(water) == 0 {
		t.Fatal("expected water contact")
	}
	if len(lava) != 0 {
		t.Fatalf("lava blocks = %d, want 0 due to the wider lava margin", len(lava))
	}
}

// Liquid type filtering must not mix water and lava.
func TestLiquidTypeFiltering(t *testing.T) {
	sim := newLiquidSim(filledColumn(lavaSource))
	state := submergedState()

	if got := len(sim.touchingLiquidBlocks(state, liquidWater)); got != 0 {
		t.Fatalf("water blocks = %d, want 0 in a lava column", got)
	}
	if got := len(sim.touchingLiquidBlocks(state, liquidLava)); got == 0 {
		t.Fatal("expected lava contact")
	}
}

// Water travel takes priority when a player touches both liquids.
func TestWaterTakesPriorityOverLava(t *testing.T) {
	w := newLiquidWorld().
		fill(dfcube.Pos{-2, 0, -2}, dfcube.Pos{2, 3, 2}, waterSource).
		set(dfcube.Pos{0, 0, 0}, lavaSource)
	sim := newLiquidSim(w)
	state := submergedState()

	sim.SimulateState(state)
	// Water gravity (0.005), not lava gravity (0.02).
	assertVec(t, state.Vel, mgl32.Vec3{0, -0.005, 0})
}

// Without a LiquidProvider, liquids are read from WorldProvider.Block.
func TestLiquidFallsBackToBlockProvider(t *testing.T) {
	sim := newLiquidSim(filledColumn(waterSource))
	state := submergedState()

	sim.SimulateState(state)
	assertVec(t, state.Vel, mgl32.Vec3{0, -0.005, 0})
}

// A LiquidProvider exposes waterlogged blocks whose main layer is a solid.
func TestLiquidProviderDetectsWaterloggedBlocks(t *testing.T) {
	w := newLayeredLiquidWorld()
	for y := range 4 {
		w.waterlog(dfcube.Pos{0, y, 0}, block.Air{}, waterSource)
	}
	sim := newLiquidSim(w)
	state := submergedState()

	if got := len(sim.touchingLiquidBlocks(state, liquidWater)); got == 0 {
		t.Fatal("expected waterlogged blocks to register as water")
	}
	sim.SimulateState(state)
	assertVec(t, state.Vel, mgl32.Vec3{0, -0.005, 0})
}

// A world with no liquids at all must run normal (non-liquid) physics.
func TestNoLiquidRunsNormalPhysics(t *testing.T) {
	sim := newLiquidSim(newLiquidWorld())
	state := submergedState()
	// Gravity is normally seeded by applyInput; SimulateState uses state as-is.
	state.Gravity = NormalGravity

	sim.SimulateState(state)
	// Normal air gravity subtracts first and then applies the 0.98 multiplier,
	// unlike the liquid path which applies drag before gravity.
	if approxEqual(state.Vel.Y(), -0.005) {
		t.Fatal("dry world must not use liquid gravity")
	}
	if !approxEqual(state.Vel.Y(), -NormalGravity*NormalGravityMultiplier) {
		t.Fatalf("vertical velocity = %v, want normal gravity", state.Vel.Y())
	}
}

// Unloaded chunks must fail safe and cancel movement rather than simulating.
func TestUnloadedChunkCancelsLiquidSimulation(t *testing.T) {
	w := filledColumn(waterSource)
	w.chunkLoaded = false
	sim := newLiquidSim(w)
	state := submergedState()
	state.Vel = mgl32.Vec3{0.5, 0.5, 0.5}

	result := sim.SimulateState(state)
	if result.Outcome != SimulationOutcomeUnloadedChunk {
		t.Fatalf("outcome = %v, want unloaded chunk", result.Outcome)
	}
	assertVec(t, state.Vel, mgl32.Vec3{})
}

// Being inside a liquid is a reliable scenario; v0.1.3 bailed out here.
func TestLiquidIsReliableScenario(t *testing.T) {
	sim := newLiquidSim(filledColumn(waterSource))
	state := submergedState()

	result := sim.SimulateState(state)
	if result.Outcome == SimulationOutcomeUnreliable {
		t.Fatal("liquid contact must not mark the simulation unreliable")
	}
}

// Flying players ignore liquid physics entirely.
// Flying is covered by TestFlyingIsUnreliableBeforePhysics and
// TestLiquidGateExcludesFlying in liquid_hardening_test.go, which separate the
// reliability bail-out from the liquid gate itself.

// Entering water cancels an active glide and its boost.
func TestWaterCancelsGliding(t *testing.T) {
	sim := newLiquidSim(filledColumn(waterSource))
	state := submergedState()
	state.Gliding = true
	state.GlideBoostTicks = 15

	sim.SimulateState(state)
	if state.Gliding {
		t.Fatal("water must cancel gliding")
	}
	if state.GlideBoostTicks != 0 {
		t.Fatalf("GlideBoostTicks = %d, want 0", state.GlideBoostTicks)
	}
}

// A glide boost held by a player who is not gliding survives water contact.
func TestWaterPreservesGlideBoostWhenNotGliding(t *testing.T) {
	sim := newLiquidSim(filledColumn(waterSource))
	state := submergedState()
	state.Gliding = false
	state.GlideBoostTicks = 15

	sim.SimulateState(state)
	if state.GlideBoostTicks != 15 {
		t.Fatalf("GlideBoostTicks = %d, want 15 preserved when not gliding", state.GlideBoostTicks)
	}
}

// Lava does not cancel gliding; only water travel does.
func TestLavaDoesNotCancelGliding(t *testing.T) {
	sim := newLiquidSim(filledColumn(lavaSource))
	state := submergedState()
	state.Gliding = true
	state.GlideBoostTicks = 15

	sim.SimulateState(state)
	if !state.Gliding {
		t.Fatal("lava must not cancel gliding")
	}
	if state.GlideBoostTicks != 15 {
		t.Fatalf("GlideBoostTicks = %d, want 15", state.GlideBoostTicks)
	}
}

// A swimming player keeps water physics even after leaving the water blocks,
// until the client reports that swimming stopped.
func TestSwimmingPreservesWaterTravelOutsideWater(t *testing.T) {
	sim := newLiquidSim(newLiquidWorld())
	state := submergedState()
	state.Swimming = true
	state.Rotation = mgl32.Vec3{0, 0, 0}
	state.SwimWaterGraceTicks = DefaultSwimWaterGraceTicks
	// Seeded so that falling back to normal physics would be visible as a
	// gravity pull rather than an indistinguishable zero.
	state.Gravity = NormalGravity

	sim.SimulateState(state)
	// Water travel with swimming applies no gravity at all.
	if !approxEqual(state.Vel.Y(), 0) {
		t.Fatalf("vertical velocity = %v, want water travel (0)", state.Vel.Y())
	}
}

// Jumping while swimming outside the water is suppressed, so the player cannot
// gain height in open air.
func TestSwimmingOutsideWaterSuppressesJump(t *testing.T) {
	sim := newLiquidSim(newLiquidWorld())
	state := submergedState()
	state.Swimming = true
	state.SwimAmount = 1
	state.EffectiveJumping = true
	state.SwimWaterGraceTicks = DefaultSwimWaterGraceTicks
	state.Gravity = NormalGravity

	sim.SimulateState(state)
	// Exactly zero: the jump impulse is suppressed and swimming cancels
	// gravity, which distinguishes this from both an ascent and a normal fall.
	if !approxEqual(state.Vel.Y(), 0) {
		t.Fatalf("vertical velocity = %v, want exactly 0 outside water", state.Vel.Y())
	}

	// The same state actually in water does ascend, confirming the assertion
	// above is not vacuous.
	wet := newLiquidSim(filledColumn(waterSource))
	wetState := submergedState()
	wetState.Swimming = true
	wetState.SwimAmount = 1
	wetState.EffectiveJumping = true
	wet.SimulateState(wetState)
	if wetState.Vel.Y() <= 0 {
		t.Fatalf("in-water vertical velocity = %v, want an ascent", wetState.Vel.Y())
	}
}

// Flowing water pushes the player toward the lower-depth neighbour.
func TestLiquidFlowPushesTowardLowerDepth(t *testing.T) {
	w := newLiquidWorld().
		set(dfcube.Pos{0, 0, 0}, block.Water{Depth: 8}).
		set(dfcube.Pos{1, 0, 0}, block.Water{Depth: 7}).
		set(dfcube.Pos{-1, 0, 0}, block.Water{Depth: 8}).
		set(dfcube.Pos{0, 0, 1}, block.Water{Depth: 8}).
		set(dfcube.Pos{0, 0, -1}, block.Water{Depth: 8})
	sim := newLiquidSim(w)
	state := submergedState()

	sim.SimulateState(state)
	if !(state.Vel.X() > 0) {
		t.Fatalf("X velocity = %v, want a positive push toward the shallower neighbour", state.Vel.X())
	}
}

// Water flow strength is 0.014 per tick along the normalized flow vector.
func TestWaterFlowStrength(t *testing.T) {
	w := newLiquidWorld().
		set(dfcube.Pos{0, 0, 0}, block.Water{Depth: 8}).
		set(dfcube.Pos{1, 0, 0}, block.Water{Depth: 7})
	sim := newLiquidSim(w)
	state := submergedState()

	sim.applyLiquidFlow(state, sim.touchingLiquidBlocks(state, liquidWater), liquidWater)
	if !approxEqual(state.Vel.X(), 0.014) {
		t.Fatalf("flow X = %v, want 0.014", state.Vel.X())
	}
}

// Lava flow is much weaker than water flow.
func TestLavaFlowStrength(t *testing.T) {
	w := newLiquidWorld().
		set(dfcube.Pos{0, 0, 0}, block.Lava{Depth: 8}).
		set(dfcube.Pos{1, 0, 0}, block.Lava{Depth: 7})
	sim := newLiquidSim(w)
	state := submergedState()

	sim.applyLiquidFlow(state, sim.touchingLiquidBlocks(state, liquidLava), liquidLava)
	if !approxEqual(state.Vel.X(), 0.0035) {
		t.Fatalf("flow X = %v, want 0.0035", state.Vel.X())
	}
}

// A uniform liquid body produces no flow at all.
func TestUniformLiquidHasNoFlow(t *testing.T) {
	sim := newLiquidSim(filledColumn(waterSource))
	state := submergedState()

	sim.applyLiquidFlow(state, sim.touchingLiquidBlocks(state, liquidWater), liquidWater)
	assertVec(t, state.Vel, mgl32.Vec3{})
}

// Falling liquid against a solid neighbour gains a strong downward component.
func TestFallingLiquidFlowsDownwardAlongSolids(t *testing.T) {
	w := newLiquidWorld().
		set(dfcube.Pos{0, 0, 0}, block.Water{Depth: 8, Falling: true}).
		set(dfcube.Pos{1, 0, 0}, block.Stone{})
	sim := newLiquidSim(w)

	flow := sim.liquidFlow(dfcube.Pos{0, 0, 0}, block.Water{Depth: 8, Falling: true})
	if !(flow.Y() < 0) {
		t.Fatalf("falling liquid flow Y = %v, want negative", flow.Y())
	}
}

// Non-falling liquid never gains the downward push.
func TestNonFallingLiquidHasNoDownwardFlow(t *testing.T) {
	w := newLiquidWorld().
		set(dfcube.Pos{0, 0, 0}, block.Water{Depth: 8}).
		set(dfcube.Pos{1, 0, 0}, block.Stone{})
	sim := newLiquidSim(w)

	flow := sim.liquidFlow(dfcube.Pos{0, 0, 0}, block.Water{Depth: 8})
	if flow.Y() < 0 {
		t.Fatalf("non-falling liquid flow Y = %v, want no downward push", flow.Y())
	}
}

// A solid neighbour blocks flow in that direction rather than contributing.
func TestSolidNeighbourBlocksFlow(t *testing.T) {
	w := newLiquidWorld().
		set(dfcube.Pos{0, 0, 0}, block.Water{Depth: 8}).
		set(dfcube.Pos{1, 0, 0}, block.Stone{}).
		set(dfcube.Pos{1, -1, 0}, block.Water{Depth: 8})
	sim := newLiquidSim(w)

	flow := sim.liquidFlow(dfcube.Pos{0, 0, 0}, block.Water{Depth: 8})
	if !approxEqual(flow.X(), 0) {
		t.Fatalf("flow X = %v, want 0 through a solid neighbour", flow.X())
	}
}

// An open neighbour with liquid below pulls the flow into the drop.
func TestFlowFallsIntoOpenDrop(t *testing.T) {
	w := newLiquidWorld().
		set(dfcube.Pos{0, 0, 0}, block.Water{Depth: 8}).
		set(dfcube.Pos{1, -1, 0}, block.Water{Depth: 8})
	sim := newLiquidSim(w)

	flow := sim.liquidFlow(dfcube.Pos{0, 0, 0}, block.Water{Depth: 8})
	if !(flow.X() > 0) {
		t.Fatalf("flow X = %v, want a positive pull into the drop", flow.X())
	}
}

// Flow below the epsilon threshold is discarded rather than applied.
func TestNegligibleFlowIgnored(t *testing.T) {
	sim := newLiquidSim(filledColumn(waterSource))
	state := submergedState()
	before := state.Vel

	sim.applyLiquidFlow(state, nil, liquidWater)
	assertVec(t, state.Vel, before)
}

// Falling liquid is treated as full-height with zero decay.
func TestFallingLiquidDecayAndHeight(t *testing.T) {
	falling := block.Water{Depth: 3, Falling: true}
	if got := liquidDecay(falling); got != 0 {
		t.Fatalf("falling decay = %d, want 0", got)
	}
	if got := liquidHeight(falling); !approxEqual(got, 1) {
		t.Fatalf("falling height = %v, want 1", got)
	}

	still := block.Water{Depth: 8}
	if got := liquidDecay(still); got != 0 {
		t.Fatalf("source decay = %d, want 0", got)
	}
	if got := liquidHeight(still); !approxEqual(got, 1) {
		t.Fatalf("source height = %v, want 1", got)
	}

	shallow := block.Water{Depth: 1}
	if got := liquidDecay(shallow); got != 7 {
		t.Fatalf("shallow decay = %d, want 7", got)
	}
	if got := liquidHeight(shallow); !approxEqual(got, 2.0/9.0) {
		t.Fatalf("shallow height = %v, want 2/9", got)
	}
}

// Colliding horizontally with a ledge that has clear air above lets the player
// hop out of the liquid.
func TestLiquidExitProbeBoostsOverLedge(t *testing.T) {
	w := newLiquidWorld().
		fill(dfcube.Pos{-1, 0, -1}, dfcube.Pos{0, 0, 1}, waterSource).
		set(dfcube.Pos{1, 0, 0}, block.Stone{})
	sim := newLiquidSim(w)
	state := submergedState()
	state.Vel = mgl32.Vec3{0.5, 0, 0}
	state.Impulse = mgl32.Vec2{0, 0.98}

	sim.SimulateState(state)
	if !state.CollideX {
		t.Fatal("expected a horizontal collision against the ledge")
	}
	if !approxEqual(state.Vel.Y(), 0.3) {
		t.Fatalf("exit boost Y = %v, want 0.3", state.Vel.Y())
	}
}

// A wall that continues above the player blocks the exit boost.
// Isolates the collision term of the exit probe. The two cases differ only by
// the block overhanging the probe box; in both, the probe box is verified to
// contain no liquid, so the liquid term cannot be what changes the result.
// The swim hitbox is used so the player fits below the overhang without
// colliding with it directly.
func TestLiquidExitProbeBlockedByCollisionAlone(t *testing.T) {
	build := func(overhang bool) (*Simulator, *MovementState) {
		w := newLiquidWorld().
			fill(dfcube.Pos{-1, 0, -1}, dfcube.Pos{0, 0, 1}, waterSource).
			set(dfcube.Pos{1, 0, 0}, block.Stone{})
		if overhang {
			w.set(dfcube.Pos{0, 1, 0}, block.Stone{})
		}
		state := submergedState()
		state.Pos = mgl32.Vec3{0.5, 0.4, 0.5}
		state.Client.Pos = state.Pos
		state.Swimming = true
		state.SwimWaterGraceTicks = DefaultSwimWaterGraceTicks
		state.Vel = mgl32.Vec3{0.5, 0, 0}
		return newLiquidSim(w), state
	}

	for _, overhang := range []bool{false, true} {
		sim, state := build(overhang)
		raised := state.BoundingBox(false).Translate(mgl32.Vec3{0, 0.6, 0})
		if sim.containsAnyLiquid(raised) {
			t.Fatalf("overhang=%t: probe box must contain no liquid to isolate the collision term", overhang)
		}

		sim.SimulateState(state)
		if !state.CollideX {
			t.Fatalf("overhang=%t: expected a horizontal collision against the ledge", overhang)
		}
		if state.CollideY {
			t.Fatalf("overhang=%t: the hitbox itself must not touch the overhang", overhang)
		}

		boosted := approxEqual(state.Vel.Y(), 0.3)
		if overhang && boosted {
			t.Fatal("a block above the probe must deny the exit boost")
		}
		if !overhang && !boosted {
			t.Fatalf("clear probe must grant the exit boost, got %v", state.Vel.Y())
		}
	}
}

// Liquid above the probe also blocks the exit boost, since the player is still
// submerged rather than at the surface.
func TestLiquidExitProbeBlockedByLiquidAbove(t *testing.T) {
	w := newLiquidWorld().
		fill(dfcube.Pos{-1, 0, -1}, dfcube.Pos{0, 4, 1}, waterSource).
		set(dfcube.Pos{1, 0, 0}, block.Stone{})
	sim := newLiquidSim(w)
	state := submergedState()
	state.Vel = mgl32.Vec3{0.5, 0, 0}

	sim.SimulateState(state)
	if approxEqual(state.Vel.Y(), 0.3) {
		t.Fatal("liquid above the probe must block the exit boost")
	}
}

// Without a horizontal collision there is no exit probe at all.
func TestNoExitProbeWithoutHorizontalCollision(t *testing.T) {
	sim := newLiquidSim(filledColumn(waterSource))
	state := submergedState()

	sim.SimulateState(state)
	if approxEqual(state.Vel.Y(), 0.3) {
		t.Fatal("exit boost must require a horizontal collision")
	}
}

// Climbing is driven by EffectiveJumping, so the automatic ascent inputs climb
// a ladder just like a held jump key does.
func TestClimbUsesEffectiveJumping(t *testing.T) {
	for name, apply := range map[string]func(*MovementState){
		"pressingJump":       func(s *MovementState) { s.PressingJump = true; s.EffectiveJumping = true },
		"autoJumpingInWater": func(s *MovementState) { s.EffectiveJumping = true },
		"none":               func(s *MovementState) {},
	} {
		t.Run(name, func(t *testing.T) {
			sim := newLiquidSim(newLiquidWorld())
			sim.BlockSemantics = overrideBlockSemantics{
				name:      "minecraft:ladder",
				friction:  DefaultBlockFriction,
				climbable: true,
			}
			state := submergedState()
			apply(state)

			sim.SimulateState(state)
			// Climb speed is set during the tick, then the end-of-tick gravity
			// multiplier applies.
			climbing := approxEqual(state.Vel.Y(), ClimbSpeed*NormalGravityMultiplier)
			if name == "none" && climbing {
				t.Fatal("no ascent input must not climb")
			}
			if name != "none" && !climbing {
				t.Fatalf("vertical velocity = %v, want ClimbSpeed", state.Vel.Y())
			}
		})
	}
}

// Determinism is covered by TestLiquidGoldenScenario in
// liquid_hardening_test.go, which pins the same mixed-liquid scenario to exact
// expected values rather than comparing the implementation against itself.

// Shrinking a box past its own size collapses it to its midpoint instead of
// inverting it.
func TestShrinkLiquidBoxCollapsesToMidpoint(t *testing.T) {
	box := cube.Box(0, 0, 0, 1, 0.2, 1)
	shrunk := shrinkLiquidBox(box, mgl32.Vec3{0.001, 0.401, 0.001})

	if !approxEqual(shrunk.Min().Y(), 0.1) || !approxEqual(shrunk.Max().Y(), 0.1) {
		t.Fatalf("collapsed Y = [%v %v], want [0.1 0.1]", shrunk.Min().Y(), shrunk.Max().Y())
	}
	if !approxEqual(shrunk.Min().X(), 0.001) || !approxEqual(shrunk.Max().X(), 0.999) {
		t.Fatalf("X = [%v %v], want [0.001 0.999]", shrunk.Min().X(), shrunk.Max().X())
	}
}
