package bedsim

import (
	"github.com/chewxy/math32"
	"testing"

	"github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/go-gl/mathgl/mgl32"
)

func dryState() *MovementState {
	state := submergedState()
	state.Gravity = NormalGravity
	return state
}

// A client that latches the swimming flag while in open air must not keep water
// travel — and therefore zero gravity — indefinitely.
func TestSpoofedSwimmingInAirStopsHovering(t *testing.T) {
	sim := newLiquidSim(newLiquidWorld())
	state := dryState()
	state.Swimming = true

	startY := state.Pos.Y()
	for range 60 {
		sim.Simulate(state, InputState{})
	}

	if state.Vel.Y() > -0.5 {
		t.Fatalf("vertical velocity = %v, want a clear fall after the grace expires", state.Vel.Y())
	}
	if state.Pos.Y() >= startY {
		t.Fatalf("position Y = %v, want below the start %v", state.Pos.Y(), startY)
	}
}

// The grace window exists so a swimmer at the surface, whose hitbox briefly
// stops overlapping a water block, keeps water travel across the transition.
func TestSwimmingPreservesWaterTravelAcrossSurfaceTransition(t *testing.T) {
	sim := newLiquidSim(newLiquidWorld())
	state := dryState()
	state.Swimming = true
	// Simulate prior water contact by seeding the grace the same way a real
	// contact tick would.
	state.SwimWaterGraceTicks = DefaultSwimWaterGraceTicks

	sim.SimulateState(state)
	// Water travel while swimming applies no gravity at all.
	if !approxEqual(state.Vel.Y(), 0) {
		t.Fatalf("vertical velocity = %v, want water travel (0) inside the grace window", state.Vel.Y())
	}
}

// Real water contact refills the grace to its full window every tick.
func TestSwimWaterGraceRefillsOnWaterContact(t *testing.T) {
	sim := newLiquidSim(filledColumn(waterSource))
	state := submergedState()
	state.Swimming = true
	state.SwimWaterGraceTicks = 1

	sim.SimulateState(state)
	if state.SwimWaterGraceTicks != DefaultSwimWaterGraceTicks {
		t.Fatalf("grace = %d, want refilled to %d", state.SwimWaterGraceTicks, DefaultSwimWaterGraceTicks)
	}
}

// The grace decays by exactly one tick per dry tick and expires on schedule.
func TestSwimWaterGraceExpiresDeterministically(t *testing.T) {
	sim := newLiquidSim(newLiquidWorld())
	sim.Options.SwimWaterGraceTicks = 3
	state := dryState()
	state.Swimming = true
	state.SwimWaterGraceTicks = 3

	for tick := 1; tick <= 3; tick++ {
		sim.SimulateState(state)
		if !approxEqual(state.Vel.Y(), 0) {
			t.Fatalf("tick %d: vertical velocity = %v, want water travel", tick, state.Vel.Y())
		}
		if want := int64(3 - tick); state.SwimWaterGraceTicks != want {
			t.Fatalf("tick %d: grace = %d, want %d", tick, state.SwimWaterGraceTicks, want)
		}
	}

	sim.SimulateState(state)
	if approxEqual(state.Vel.Y(), 0) {
		t.Fatal("water travel must stop once the grace is exhausted")
	}
}

// A caller can widen the grace window through options.
func TestSwimWaterGraceOptionOverride(t *testing.T) {
	sim := newLiquidSim(filledColumn(waterSource))
	sim.Options.SwimWaterGraceTicks = 25
	state := submergedState()
	state.Swimming = true

	sim.SimulateState(state)
	if state.SwimWaterGraceTicks != 25 {
		t.Fatalf("grace = %d, want 25", state.SwimWaterGraceTicks)
	}
}

// A negative option disables the grace entirely, requiring water contact on
// every tick for water travel.
func TestSwimWaterGraceCanBeDisabled(t *testing.T) {
	sim := newLiquidSim(filledColumn(waterSource))
	sim.Options.SwimWaterGraceTicks = -1
	state := submergedState()
	state.Swimming = true

	sim.SimulateState(state)
	if state.SwimWaterGraceTicks != 0 {
		t.Fatalf("grace = %d, want 0 when disabled", state.SwimWaterGraceTicks)
	}

	dry := newLiquidSim(newLiquidWorld())
	dry.Options.SwimWaterGraceTicks = -1
	dryS := dryState()
	dryS.Swimming = true
	dryS.SwimWaterGraceTicks = 5 // stale value must not grant travel

	dry.SimulateState(dryS)
	if approxEqual(dryS.Vel.Y(), 0) {
		t.Fatal("disabled grace must not permit water travel out of water")
	}
}

// An unreliable frame resets the retained proof, so it cannot be carried across
// a correction.
func TestSwimWaterGraceResetOnUnreliableFrame(t *testing.T) {
	sim := newLiquidSim(newLiquidWorld())
	state := dryState()
	state.Swimming = true
	state.SwimWaterGraceTicks = DefaultSwimWaterGraceTicks
	state.Alive = false // forces the unreliable path

	result := sim.SimulateState(state)
	if result.Outcome != SimulationOutcomeUnreliable {
		t.Fatalf("outcome = %v, want unreliable", result.Outcome)
	}
	if state.SwimWaterGraceTicks != 0 {
		t.Fatalf("grace = %d, want reset to 0", state.SwimWaterGraceTicks)
	}
}

// An unloaded chunk cannot prove water contact, so the retained proof is
// cleared rather than carried across the gap.
func TestSwimWaterGraceResetOnUnloadedChunk(t *testing.T) {
	w := newLiquidWorld()
	w.chunkLoaded = false
	sim := newLiquidSim(w)
	state := dryState()
	state.Swimming = true
	state.SwimWaterGraceTicks = DefaultSwimWaterGraceTicks

	sim.SimulateState(state)
	if state.SwimWaterGraceTicks != 0 {
		t.Fatalf("grace = %d, want reset to 0", state.SwimWaterGraceTicks)
	}
}

// The grace never exceeds its configured bound, however long the player swims.
func TestSwimWaterGraceIsBounded(t *testing.T) {
	sim := newLiquidSim(filledColumn(waterSource))
	state := submergedState()
	state.Swimming = true

	for range 100 {
		sim.SimulateState(state)
		if state.SwimWaterGraceTicks > DefaultSwimWaterGraceTicks {
			t.Fatalf("grace = %d, want at most %d", state.SwimWaterGraceTicks, DefaultSwimWaterGraceTicks)
		}
	}
}

// Real water contact still takes priority over the swimming flag: a player in
// water gets water travel whether or not the client claims to be swimming.
func TestRealWaterContactDoesNotNeedSwimmingFlag(t *testing.T) {
	sim := newLiquidSim(filledColumn(waterSource))
	state := submergedState()
	state.Swimming = false

	sim.SimulateState(state)
	assertVec(t, state.Vel, mgl32.Vec3{0, -0.005, 0})
}

// The security window's default is pinned so a regression cannot silently
// widen it. Every other grace test compares against the symbol or overrides it
// through options, so without this the constant is unconstrained.
func TestDefaultSwimWaterGraceTicksValue(t *testing.T) {
	if DefaultSwimWaterGraceTicks != 10 {
		t.Fatalf("DefaultSwimWaterGraceTicks = %d, want 10; widening this enlarges "+
			"the window in which a client-controlled flag alone drives water travel",
			DefaultSwimWaterGraceTicks)
	}
}

// A teleport relocates the player without observing the destination, so
// retained water contact from the origin must not survive it.
func TestSwimWaterGraceResetOnTeleport(t *testing.T) {
	sim := newLiquidSim(newLiquidWorld())
	state := dryState()
	state.Swimming = true
	state.SwimWaterGraceTicks = DefaultSwimWaterGraceTicks
	state.TeleportPos = mgl32.Vec3{50, 50, 50}
	state.TeleportCompletionTicks = 3
	state.TicksSinceTeleport = 0

	result := sim.SimulateState(state)
	if result.Outcome != SimulationOutcomeTeleport {
		t.Fatalf("outcome = %v, want teleport", result.Outcome)
	}
	if state.SwimWaterGraceTicks != 0 {
		t.Fatalf("grace = %d, want reset to 0 across a teleport", state.SwimWaterGraceTicks)
	}
}

// Frozen ticks observe nothing, so the budget must not pause and resume later.
func TestSwimWaterGraceResetWhenImmobile(t *testing.T) {
	sim := newLiquidSim(newLiquidWorld())
	state := dryState()
	state.Swimming = true
	state.SwimWaterGraceTicks = DefaultSwimWaterGraceTicks
	state.Immobile = true

	result := sim.SimulateState(state)
	if result.Outcome != SimulationOutcomeImmobileOrNotReady {
		t.Fatalf("outcome = %v, want immobile", result.Outcome)
	}
	if state.SwimWaterGraceTicks != 0 {
		t.Fatalf("grace = %d, want reset to 0 while immobile", state.SwimWaterGraceTicks)
	}
}

// Lava the player is demonstrably standing in must win over a stale water
// grace, so a swimming flag cannot cancel lava gravity.
func TestLavaWinsOverStaleWaterGrace(t *testing.T) {
	sim := newLiquidSim(filledColumn(lavaSource))
	state := submergedState()
	state.Swimming = true
	state.SwimWaterGraceTicks = DefaultSwimWaterGraceTicks

	sim.SimulateState(state)
	// Lava gravity, not water travel's zero gravity for a swimmer.
	assertVec(t, state.Vel, mgl32.Vec3{0, -0.02, 0})
}

// The swim-speed multiplier branch scales acceleration by
// (0.7 + depthStriderFraction*0.3) * multiplier. Comparing full Depth Strider
// against none pins that expression, which the golden cannot reach because it
// runs with a multiplier of 1.
func TestSwimSpeedMultiplierDepthStriderScaling(t *testing.T) {
	run := func(level int) float32 {
		sim := newLiquidSim(filledColumn(waterSource))
		sim.Inventory = depthStriderInventory{level: level}
		state := submergedState()
		state.Swimming = true
		state.SwimWaterGraceTicks = DefaultSwimWaterGraceTicks
		state.SwimSpeedMultiplier = 2
		state.OnGround = true
		state.Impulse = mgl32.Vec2{0, 0.98}
		sim.SimulateState(state)
		return state.Vel.Z()
	}

	none, full := run(0), run(3)
	if approxEqual(none, 0) {
		t.Fatal("baseline velocity must be non-zero")
	}
	// fraction 0 -> 0.7; fraction 1 -> 1.0. Drag is 0.8 in both cases because
	// the Depth Strider drag term is gated on multiplier <= 1.
	if ratio := full / none; math32.Abs(ratio-1/0.7) > 1e-6 {
		t.Fatalf("full/none acceleration ratio = %.17g, want %.17g", ratio, 1/0.7)
	}
}

type explicitLiquids struct {
	layer map[cube.Pos]world.Liquid
}

func (p explicitLiquids) Liquid(pos cube.Pos) (world.Liquid, bool) {
	liquid, ok := p.layer[pos]
	return liquid, ok
}

func TestHasLiquidLayerReportsExplicitProvider(t *testing.T) {
	sim := newLiquidSim(newLiquidWorld())
	if sim.HasLiquidLayer() {
		t.Fatal("a plain world must not report liquid layer support")
	}

	sim.Liquids = explicitLiquids{layer: map[cube.Pos]world.Liquid{}}
	if !sim.HasLiquidLayer() {
		t.Fatal("an explicit Liquids provider must report support")
	}
}

// A world that implements LiquidProvider still works, preserving v0.1.3-era
// integrations that relied on the type assertion.
func TestHasLiquidLayerAcceptsWorldProvider(t *testing.T) {
	sim := newLiquidSim(newLayeredLiquidWorld())
	if !sim.HasLiquidLayer() {
		t.Fatal("a world implementing LiquidProvider must report support")
	}
}

// The explicit field wins over the world assertion when both are present.
func TestExplicitLiquidsFieldTakesPrecedence(t *testing.T) {
	w := newLayeredLiquidWorld()
	w.waterlog(cube.Pos{0, 0, 0}, block.Air{}, waterSource)

	sim := newLiquidSim(w)
	sim.Liquids = explicitLiquids{layer: map[cube.Pos]world.Liquid{}}

	state := submergedState()
	if got := len(sim.touchingLiquidBlocks(state, liquidWater)); got != 0 {
		t.Fatalf("water blocks = %d, want 0 from the explicit empty provider", got)
	}
}

// Liquids supplied through the explicit field are detected normally.
func TestExplicitLiquidsProviderDetectsWaterlogged(t *testing.T) {
	layer := map[cube.Pos]world.Liquid{}
	for y := range 4 {
		layer[cube.Pos{0, y, 0}] = waterSource
	}
	sim := newLiquidSim(newLiquidWorld())
	sim.Liquids = explicitLiquids{layer: layer}

	state := submergedState()
	if got := len(sim.touchingLiquidBlocks(state, liquidWater)); got == 0 {
		t.Fatal("expected waterlogged blocks from the explicit provider")
	}
	sim.SimulateState(state)
	assertVec(t, state.Vel, mgl32.Vec3{0, -0.005, 0})
}

// With RequireLiquidLayer set, a simulator that cannot see layer 1 refuses to
// simulate rather than silently mis-simulating waterlogged blocks.
func TestRequireLiquidLayerFailsClosed(t *testing.T) {
	sim := newLiquidSim(newLiquidWorld())
	sim.Options.RequireLiquidLayer = true
	state := submergedState()
	state.Vel = mgl32.Vec3{0.5, 0.5, 0.5}

	result := sim.SimulateState(state)
	if result.Outcome != SimulationOutcomeUnreliable {
		t.Fatalf("outcome = %v, want unreliable when the liquid layer is unavailable", result.Outcome)
	}
}

// The same simulator proceeds normally once layer-1 support is supplied.
func TestRequireLiquidLayerPassesWithProvider(t *testing.T) {
	sim := newLiquidSim(newLayeredLiquidWorld())
	sim.Options.RequireLiquidLayer = true
	state := submergedState()

	result := sim.SimulateState(state)
	if result.Outcome == SimulationOutcomeUnreliable {
		t.Fatal("a simulator with liquid layer support must not fail closed")
	}
}

// Without the option, the legacy fallback still simulates, preserving
// compatibility for callers that do not track layer 1.
func TestLiquidLayerFallbackStillSimulatesByDefault(t *testing.T) {
	sim := newLiquidSim(filledColumn(waterSource))
	state := submergedState()

	result := sim.SimulateState(state)
	if result.Outcome != SimulationOutcomeNormal {
		t.Fatalf("outcome = %v, want normal", result.Outcome)
	}
}

// By default bedsim keeps its sneak and consumable impulse clamps; the opt-in
// selects upstream's behavior, which applies neither.
func TestUpstreamImpulseClampingOptIn(t *testing.T) {
	cases := []struct {
		name     string
		upstream bool
		input    InputState
		want     float32
	}{
		{"sneak default", false, InputState{SneakDown: true, MoveVector: mgl32.Vec2{0, 1}}, MaxSneakImpulse * 0.98},
		{"sneak upstream", true, InputState{SneakDown: true, MoveVector: mgl32.Vec2{0, 1}}, 0.98},
		{"consumable default", false, InputState{UsingConsumable: true, MoveVector: mgl32.Vec2{0, 1}}, MaxConsumingImpulse * 0.98},
		{"consumable upstream", true, InputState{UsingConsumable: true, MoveVector: mgl32.Vec2{0, 1}}, 0.98},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sim := newLiquidSim(newLiquidWorld())
			sim.Options.UpstreamImpulseClamping = tc.upstream
			state := newBaseState()

			sim.applyInput(state, tc.input)
			if !approxEqual(state.Impulse.Y(), tc.want) {
				t.Fatalf("impulse Y = %v, want %v", state.Impulse.Y(), tc.want)
			}
		})
	}
}

// Upstream still clamps the raw move vector to the unit range.
func TestUpstreamImpulseClampingStillBoundsMoveVector(t *testing.T) {
	sim := newLiquidSim(newLiquidWorld())
	sim.Options.UpstreamImpulseClamping = true
	state := newBaseState()

	sim.applyInput(state, InputState{MoveVector: mgl32.Vec2{5, -5}})
	if !approxEqual(state.Impulse.X(), 0.98) || !approxEqual(state.Impulse.Y(), -0.98) {
		t.Fatalf("impulse = %v, want the move vector clamped to [-1, 1] then scaled", state.Impulse)
	}
}

// A flying player is treated as an unsupported scenario before physics run at
// all, so no liquid physics are applied and the state is reset to the client.
func TestFlyingIsUnreliableBeforePhysics(t *testing.T) {
	sim := newLiquidSim(filledColumn(waterSource))
	state := submergedState()
	state.Flying = true
	state.Vel = mgl32.Vec3{0.25, 0.25, 0.25}
	state.Client.Vel = mgl32.Vec3{1, 2, 3}

	result := sim.SimulateState(state)
	if result.Outcome != SimulationOutcomeUnreliable {
		t.Fatalf("outcome = %v, want unreliable", result.Outcome)
	}
	assertVec(t, state.Vel, mgl32.Vec3{1, 2, 3})
}

// The liquid gate itself also excludes flying, independently of the reliability
// check above. Exercised directly because the public path cannot reach it.
func TestLiquidGateExcludesFlying(t *testing.T) {
	sim := newLiquidSim(filledColumn(waterSource))
	state := submergedState()
	state.Flying = true
	state.Gravity = NormalGravity

	sim.simulateMovement(state)
	if approxEqual(state.Vel.Y(), -0.005) {
		t.Fatal("flying must not take the liquid path")
	}
	if !approxEqual(state.Vel.Y(), -NormalGravity*NormalGravityMultiplier) {
		t.Fatalf("vertical velocity = %v, want normal gravity", state.Vel.Y())
	}
}

// The drop contribution weights an open neighbour with liquid below by
// (decay(below) - decay(current) + 8). Competing horizontal terms make the
// weight observable through the normalized result.
func TestFlowDropWeightIsEight(t *testing.T) {
	w := newLiquidWorld().
		set(cube.Pos{0, 0, 0}, block.Water{Depth: 8}).
		set(cube.Pos{-1, 0, 0}, block.Water{Depth: 7}).
		set(cube.Pos{0, 0, 1}, block.Water{Depth: 4}).
		set(cube.Pos{0, 0, -1}, block.Water{Depth: 8}).
		set(cube.Pos{1, -1, 0}, block.Water{Depth: 8})
	sim := newLiquidSim(w)

	flow := sim.liquidFlow(cube.Pos{0, 0, 0}, block.Water{Depth: 8})
	// +X: open with liquid below -> (0 - 0 + 8) = +8
	// -X: same-type neighbour     -> (1 - 0)     = -1
	// +Z: same-type neighbour     -> (4 - 0)     = +4
	want := mgl32.Vec3{7, 0, 4}.Normalize()
	assertVec(t, flow, want)
}

// Falling liquid blocked by a solid neighbour adds a downward term of exactly
// 6 against the unit-normalized horizontal flow.
func TestFallingFlowDownwardWeightIsSix(t *testing.T) {
	w := newLiquidWorld().
		set(cube.Pos{0, 0, 0}, block.Water{Depth: 8, Falling: true}).
		set(cube.Pos{-1, 0, 0}, block.Water{Depth: 4}).
		set(cube.Pos{1, 0, 0}, block.Stone{})
	sim := newLiquidSim(w)

	flow := sim.liquidFlow(cube.Pos{0, 0, 0}, block.Water{Depth: 8, Falling: true})
	// Horizontal flow normalizes to (-1, 0, 0), then Y -= 6, then normalizes.
	want := mgl32.Vec3{-1, -6, 0}.Normalize()
	assertVec(t, flow, want)
}

// A waterlogged stairs block whose solid face points at the neighbour blocks
// flow through that face.
func TestStairsSolidFaceBlocksFlow(t *testing.T) {
	build := func(facing cube.Direction) mgl32.Vec3 {
		w := newLayeredLiquidWorld()
		w.waterlog(cube.Pos{0, 0, 0}, block.Stairs{Facing: facing}, block.Water{Depth: 8})
		w.set(cube.Pos{1, 0, 0}, block.Water{Depth: 4})
		sim := newLiquidSim(w)
		return sim.liquidFlow(cube.Pos{0, 0, 0}, block.Water{Depth: 8})
	}

	// Facing east: the stairs' full side faces the +X neighbour and closes it.
	if flow := build(cube.East); !approxEqual(flow.X(), 0) {
		t.Fatalf("east-facing stairs: flow X = %v, want 0", flow.X())
	}
	// Facing west: the +X side is open, so flow proceeds toward the shallower
	// neighbour.
	if flow := build(cube.West); !(flow.X() > 0) {
		t.Fatalf("west-facing stairs: flow X = %v, want positive", flow.X())
	}
}

// A simulator with no world must not panic on any liquid path.
func TestNilWorldIsSafe(t *testing.T) {
	sim := &Simulator{Options: SimulationOptions{PositionCorrectionThreshold: 0.3}}
	state := submergedState()
	state.Swimming = true
	state.SwimWaterGraceTicks = DefaultSwimWaterGraceTicks

	sim.SimulateState(state)

	if got := len(sim.touchingLiquidBlocks(state, liquidWater)); got != 0 {
		t.Fatalf("water blocks = %d, want 0 with no world", got)
	}
	if sim.containsAnyLiquid(state.BoundingBox(false)) {
		t.Fatal("no world must contain no liquid")
	}
	if flow := sim.liquidFlow(cube.Pos{0, 0, 0}, block.Water{Depth: 8}); flow.Len() != 0 {
		t.Fatalf("flow = %v, want zero with no world", flow)
	}
	if sim.HasLiquidLayer() {
		t.Fatal("no world must not report liquid layer support")
	}
}

// The collapsed swim hitbox fits under a ceiling that a standing hitbox would
// hit, so the swim pose genuinely changes collision results.
func TestSwimHitboxChangesCeilingCollision(t *testing.T) {
	newCeilingSim := func() (*Simulator, *MovementState) {
		w := newLiquidWorld().
			fill(cube.Pos{-1, 0, -1}, cube.Pos{1, 1, 1}, waterSource).
			set(cube.Pos{0, 2, 0}, block.Stone{})
		state := submergedState()
		// Starts clear of the ceiling in both poses; only the standing hitbox
		// reaches it after the upward move.
		state.Pos = mgl32.Vec3{0.5, 0, 0.5}
		state.Client.Pos = state.Pos
		state.Vel = mgl32.Vec3{0, 0.5, 0}
		return newLiquidSim(w), state
	}

	sim, standing := newCeilingSim()
	sim.SimulateState(standing)

	sim2, swimming := newCeilingSim()
	swimming.Swimming = true
	// The swim pose requires server-observed water contact, which this state
	// has: the player is standing in the water column below.
	swimming.SwimWaterGraceTicks = DefaultSwimWaterGraceTicks
	sim2.SimulateState(swimming)

	if !standing.CollideY {
		t.Fatal("a standing hitbox must hit the ceiling")
	}
	if swimming.CollideY {
		t.Fatal("a swimming hitbox must fit under the ceiling")
	}
}

// Repeated identical runs must agree exactly. A single pinned golden detects
// map-iteration nondeterminism only probabilistically, so this repeats the same
// scenario and compares runs against each other.
func TestLiquidSimulationIsRepeatable(t *testing.T) {
	run := func() (mgl32.Vec3, mgl32.Vec3) {
		w := newLiquidWorld().
			fill(cube.Pos{-8, 0, -8}, cube.Pos{8, 8, 8}, block.Water{Depth: 8}).
			set(cube.Pos{1, 0, 0}, block.Water{Depth: 6}).
			set(cube.Pos{0, 0, 1}, block.Water{Depth: 4}).
			set(cube.Pos{-1, 1, 0}, block.Water{Depth: 8, Falling: true}).
			set(cube.Pos{2, 0, 2}, block.Stone{})
		sim := newLiquidSim(w)
		sim.Inventory = depthStriderInventory{level: 2}
		state := submergedState()
		state.Swimming = true
		state.SwimAmount = 0.5
		state.SwimWaterGraceTicks = DefaultSwimWaterGraceTicks
		input := InputState{
			Jumping:    true,
			MoveVector: mgl32.Vec2{0.5, 1},
			Pitch:      25,
			Yaw:        40,
			HeadYaw:    40,
		}
		for range 20 {
			sim.Simulate(state, input)
		}
		return state.Pos, state.Vel
	}

	wantPos, wantVel := run()
	for i := range 5 {
		gotPos, gotVel := run()
		if gotPos != wantPos || gotVel != wantVel {
			t.Fatalf("run %d diverged: pos %v vs %v, vel %v vs %v",
				i, gotPos, wantPos, gotVel, wantVel)
		}
	}
}

// Pinned change detector for the mixed-liquid path; update deliberately.
func TestLiquidGoldenScenario(t *testing.T) {
	// Deep enough that the player stays submerged for the whole run, so the
	// golden measures liquid physics rather than a surface transition.
	w := newLiquidWorld().
		fill(cube.Pos{-8, 0, -8}, cube.Pos{8, 8, 8}, block.Water{Depth: 8}).
		set(cube.Pos{1, 0, 0}, block.Water{Depth: 6}).
		set(cube.Pos{0, 0, 1}, block.Water{Depth: 4}).
		set(cube.Pos{-1, 1, 0}, block.Water{Depth: 8, Falling: true}).
		set(cube.Pos{2, 0, 2}, block.Stone{})
	sim := newLiquidSim(w)
	sim.Inventory = depthStriderInventory{level: 2}

	state := submergedState()
	state.Swimming = true
	state.SwimAmount = 0.5
	state.SwimWaterGraceTicks = DefaultSwimWaterGraceTicks

	// Rotation and impulse must arrive through InputState: Simulate overwrites
	// both from the input every tick, so seeding them on the state would leave
	// the pitch steering and moveRelative terms untested. The swim speed
	// multiplier is left at its default so the Depth Strider drag term, which
	// is gated on multiplier <= 1, is actually reached.
	input := InputState{
		Jumping:    true,
		MoveVector: mgl32.Vec2{0.5, 1},
		Pitch:      25,
		Yaw:        40,
		HeadYaw:    40,
	}
	for range 20 {
		sim.Simulate(state, input)
	}

	wantPos := mgl32.Vec3{-0.012654960155487061, 2.922518253326416, 3.5954680442810059}
	wantVel := mgl32.Vec3{-0.02702143903177032, 0.15549643337726593, 0.1142856627702713}

	const tolerance = 1e-6
	for axis, name := range []string{"X", "Y", "Z"} {
		if math32.Abs(state.Pos[axis]-wantPos[axis]) > tolerance {
			t.Errorf("Pos.%s = %.17g, want %.17g", name, state.Pos[axis], wantPos[axis])
		}
		if math32.Abs(state.Vel[axis]-wantVel[axis]) > tolerance {
			t.Errorf("Vel.%s = %.17g, want %.17g", name, state.Vel[axis], wantVel[axis])
		}
	}
}
