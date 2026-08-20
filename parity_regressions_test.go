package bedsim

import (
	"testing"

	"github.com/chewxy/math32"
	"github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

func TestZeroValueStateHasNoSyntheticEvents(t *testing.T) {
	var state MovementState
	if state.HasKnockback() {
		t.Fatal("zero state must not report knockback")
	}
	if state.HasTeleport() {
		t.Fatal("zero state must not report teleport")
	}
}

func TestSimulationRejectsNonFiniteInputAndState(t *testing.T) {
	state := newBaseState()
	state.Pos = mgl32.Vec3{1, 2, 3}
	result := (&Simulator{World: mockWorld{}}).Simulate(state, InputState{Pitch: math32.NaN()})
	if result.Outcome != SimulationOutcomeInvalidInput || !result.NeedsCorrection {
		t.Fatalf("invalid input result = %+v", result)
	}
	if state.Pos != (mgl32.Vec3{1, 2, 3}) {
		t.Fatalf("invalid input mutated state position to %v", state.Pos)
	}

	state = newBaseState()
	state.Vel[0] = math32.Inf(1)
	result = (&Simulator{World: mockWorld{}}).SimulateState(state)
	if result.Outcome != SimulationOutcomeInvalidInput || !result.NeedsCorrection {
		t.Fatalf("invalid state result = %+v", result)
	}
}

func TestInvalidInputResultPreservesAuthoritativeState(t *testing.T) {
	state := newBaseState()
	state.Pos = mgl32.Vec3{12, 64, 9}
	state.Vel = mgl32.Vec3{0.1, 0.2, 0.3}
	state.Mov = mgl32.Vec3{0.2, 0, 0}

	result := (&Simulator{}).Simulate(state, InputState{Pitch: math32.NaN()})

	if result.Outcome != SimulationOutcomeInvalidInput {
		t.Fatalf("outcome = %v, want invalid input", result.Outcome)
	}
	if result.Position != state.Pos || result.Velocity != state.Vel || result.Movement != state.Mov {
		t.Fatalf("invalid input dropped authoritative state: result=%+v state=%+v", result, state)
	}
}

func TestPassiveModeDoesNotRequestCorrectionForInvalidInput(t *testing.T) {
	result := (&Simulator{Options: SimulationOptions{Mode: SimulationModePassive}}).SimulateState(&MovementState{
		Vel: mgl32.Vec3{math32.NaN(), 0, 0},
	})
	if result.Outcome != SimulationOutcomeInvalidInput || result.NeedsCorrection {
		t.Fatalf("passive invalid-input result = %+v", result)
	}
}

func TestMountedStateSkipsMovement(t *testing.T) {
	state := newBaseState()
	state.InVehicle = true
	state.Pos = mgl32.Vec3{10, 70, 10}
	state.Vel = mgl32.Vec3{1, 2, 3}
	state.Client.Pos = mgl32.Vec3{4, 5, 6}
	state.Client.Vel = mgl32.Vec3{0.1, 0.2, 0.3}

	result := (&Simulator{World: mockWorld{}}).SimulateState(state)
	if result.Outcome != SimulationOutcomeMounted {
		t.Fatalf("outcome = %v, want mounted", result.Outcome)
	}
	if state.Pos != state.Client.Pos || state.Vel != state.Client.Vel {
		t.Fatalf("mounted state was simulated: pos=%v vel=%v", state.Pos, state.Vel)
	}
}

func TestSimulateStateLeavesTransientInputForCaller(t *testing.T) {
	state := newBaseState()
	state.RiptideReady = true
	(&Simulator{World: mockWorld{}}).SimulateState(state)
	if !state.RiptideReady {
		t.Fatal("SimulateState must not consume caller-managed transient input")
	}
}

func TestActiveRiptideRunsOrdinaryPhysics(t *testing.T) {
	state := newBaseState()
	state.RiptideTicks = 5
	state.Vel = mgl32.Vec3{0, 0.8, 0}
	state.Gravity = NormalGravity
	state.HasGravity = true

	(&Simulator{World: mockWorld{}, Equipment: fixedEquipment{EnchantmentRiptide: 2}}).SimulateState(state)
	if math32.Abs(state.Vel.Y()-0.8) <= 1e-6 {
		t.Fatalf("riptide tick skipped gravity: %v", state.Vel)
	}
	if state.Vel.Z() != 0 {
		t.Fatalf("riptide tick re-applied its launch impulse: %v", state.Vel)
	}
}

func TestRiptideLaunchAppliesImpulseOnce(t *testing.T) {
	sim := &Simulator{World: mockWorld{}, Equipment: fixedEquipment{EnchantmentRiptide: 2}}
	state := newBaseState()
	state.RiptideInRain = true
	state.RiptideReady = true
	state.StartingSpinAttack = true

	sim.SimulateState(state)
	// The 2.25 impulse for level 2 decays through ordinary air friction the
	// same tick, so the launch is observable but never the raw impulse.
	launched := state.Vel.Z()
	if math32.Abs(launched-2.25*DefaultAirFriction) > 1e-6 {
		t.Fatalf("riptide launch velocity = %v, want %v", launched, 2.25*DefaultAirFriction)
	}

	state.RiptideReady = false
	state.StartingSpinAttack = false
	sim.SimulateState(state)
	if state.Vel.Z() > launched {
		t.Fatalf("riptide gained speed after its launch tick: %v", state.Vel.Z())
	}
}

func TestActiveRiptideConsumesRetainedWaterGrace(t *testing.T) {
	state := newBaseState()
	state.RiptideTicks = 5
	state.SwimWaterGraceTicks = 2

	(&Simulator{World: mockWorld{}, Equipment: fixedEquipment{}}).SimulateState(state)
	if state.SwimWaterGraceTicks != 1 {
		t.Fatalf("active Riptide retained water grace = %d, want 1", state.SwimWaterGraceTicks)
	}
}

func TestMovementSpeedUsesEffectiveAttribute(t *testing.T) {
	withoutEffect := newBaseState()
	withoutEffect.MovementSpeed = 0.12
	withoutEffect.DefaultMovementSpeed = 0.12
	withoutEffect.Impulse = mgl32.Vec2{0, 1}

	withEffect := *withoutEffect

	base := (&Simulator{World: mockWorld{}}).SimulateState(withoutEffect)
	withSpeedEffect := (&Simulator{World: mockWorld{}, Effects: fixedEffects{packet.EffectSpeed: 0}}).SimulateState(&withEffect)
	if base.Velocity != withSpeedEffect.Velocity {
		t.Fatalf("effective movement speed was modified by a second effect pass: base=%v with_effect=%v", base.Velocity, withSpeedEffect.Velocity)
	}
}

func TestAirSpeedIgnoresTheMovementAttribute(t *testing.T) {
	state := newBaseState()
	state.MovementSpeed = 0.2
	state.DefaultMovementSpeed = 0.2

	(&Simulator{World: mockWorld{}}).Simulate(state, InputState{StartSprinting: true})
	if math32.Abs(state.MovementSpeed-0.26) > 1e-6 {
		t.Fatalf("sprinting movement speed = %v, want 0.26", state.MovementSpeed)
	}
	if state.AirSpeed != SprintAirSpeed {
		t.Fatalf("sprinting air speed = %v, want %v", state.AirSpeed, SprintAirSpeed)
	}
}

func TestTeleportDoesNotApplyJumpImpulse(t *testing.T) {
	state := newBaseState()
	state.OnGround = true
	state.Jumping = true
	support := cube.Pos{7, 8, 9}
	state.SupportingBlockPos = &support
	state.QueueTeleport(mgl32.Vec3{10, 20, 30}, false, 0)

	result := (&Simulator{World: mockWorld{}}).SimulateState(state)
	if result.Outcome != SimulationOutcomeTeleport {
		t.Fatalf("outcome = %v, want teleport", result.Outcome)
	}
	if state.Vel != (mgl32.Vec3{}) {
		t.Fatalf("teleport applied jump/other velocity: %v", state.Vel)
	}
	if state.HasTeleport() {
		t.Fatal("completed hard teleport remained active")
	}
	if state.SupportingBlockPos != nil {
		t.Fatalf("teleport retained stale support block: %v", *state.SupportingBlockPos)
	}
}

func TestQueuedTeleportEscapesUnloadedOrigin(t *testing.T) {
	state := newBaseState()
	state.Pos = mgl32.Vec3{16.5, 0, 0.5}
	state.Client.Pos = state.Pos
	state.QueueTeleport(mgl32.Vec3{0.5, 0, 0.5}, false, 0)

	result := (&Simulator{World: selectiveChunkWorld{}}).Simulate(state, InputState{ClientPos: state.Client.Pos})

	if result.Outcome != SimulationOutcomeTeleport {
		t.Fatalf("outcome = %v, want teleport from unloaded origin", result.Outcome)
	}
	if state.Pos != state.TeleportPos || state.HasTeleport() {
		t.Fatalf("queued teleport was not completed: pos=%v target=%v pending=%v", state.Pos, state.TeleportPos, state.HasTeleport())
	}
}

func TestQueueTeleportCanTargetOrigin(t *testing.T) {
	state := newBaseState()
	state.Pos = mgl32.Vec3{10, 20, 30}
	state.QueueTeleport(mgl32.Vec3{}, false, 0)

	result := (&Simulator{World: mockWorld{}}).SimulateState(state)
	if result.Outcome != SimulationOutcomeTeleport || state.Pos != (mgl32.Vec3{}) {
		t.Fatalf("origin teleport result=%+v pos=%v", result, state.Pos)
	}
}

func TestHardTeleportAtMaximumCompletionTickFinishes(t *testing.T) {
	state := newBaseState()
	state.QueueTeleport(mgl32.Vec3{10, 20, 30}, false, math32.MaxUint64)
	sim := &Simulator{World: mockWorld{}}

	first := sim.SimulateState(state)
	second := sim.SimulateState(state)

	if first.Outcome != SimulationOutcomeTeleport {
		t.Fatalf("first outcome = %v, want teleport", first.Outcome)
	}
	if state.HasTeleport() || second.Outcome == SimulationOutcomeTeleport {
		t.Fatalf("completed maximum-window teleport remained active: active=%v second=%v", state.HasTeleport(), second.Outcome)
	}
}

func TestLegacyTeleportFieldsCanBeRearmed(t *testing.T) {
	state := newBaseState()
	sim := &Simulator{World: mockWorld{}}
	state.TeleportPos = mgl32.Vec3{1, 2, 3}
	state.TicksSinceTeleport = 0
	state.TeleportCompletionTicks = 0

	if result := sim.SimulateState(state); result.Outcome != SimulationOutcomeTeleport {
		t.Fatalf("first outcome = %v, want teleport", result.Outcome)
	}

	state.TeleportPos = mgl32.Vec3{4, 5, 6}
	state.TicksSinceTeleport = 0
	state.TeleportCompletionTicks = 0
	result := sim.SimulateState(state)
	if result.Outcome != SimulationOutcomeTeleport || state.Pos != state.TeleportPos {
		t.Fatalf("rearmed teleport result=%+v pos=%v target=%v", result, state.Pos, state.TeleportPos)
	}
}

func TestLegacyPendingTeleportKeepsExplicitTarget(t *testing.T) {
	state := newBaseState()
	state.PendingTeleports = 1
	state.TeleportPos = mgl32.Vec3{10, 20, 30}

	result := (&Simulator{World: mockWorld{}}).SimulateState(state)
	if result.Outcome != SimulationOutcomeTeleport || state.Pos != state.TeleportPos {
		t.Fatalf("legacy teleport result=%+v pos=%v target=%v", result, state.Pos, state.TeleportPos)
	}
}

func TestGlideAtVerticalPitchRemainsFinite(t *testing.T) {
	state := newBaseState()
	state.Gliding = true
	state.OnGround = false
	state.Rotation = mgl32.Vec3{-90, 0, 0}
	state.Vel = mgl32.Vec3{1, 0, 0}

	(&Simulator{World: mockWorld{}, Inventory: mockInventory{hasElytra: true}}).SimulateState(state)
	for axis, value := range state.Vel {
		if !finiteFloat(value) {
			t.Fatalf("glide velocity axis %d is not finite: %v", axis, state.Vel)
		}
	}
}

func TestGlideNearVerticalPitchDoesNotExplode(t *testing.T) {
	state := newBaseState()
	state.Gliding = true
	state.OnGround = false
	state.Rotation = mgl32.Vec3{-89.999, 0, 0}
	state.Vel = mgl32.Vec3{1, 0, 0}

	(&Simulator{World: mockWorld{}, Inventory: mockInventory{hasElytra: true}}).SimulateState(state)
	for axis, value := range state.Vel {
		if !finiteFloat(value) || math32.Abs(value) > 10 {
			t.Fatalf("near-vertical glide velocity axis %d = %v", axis, value)
		}
	}
}

func TestShallowLiquidBelowPlayerIsNotContact(t *testing.T) {
	w := newLiquidWorld().set(cube.Pos{0, 0, 0}, block.Water{Depth: 0, Still: true})
	sim := newLiquidSim(w)
	state := submergedState()

	if got := len(sim.touchingLiquidBlocks(state, liquidWater)); got != 0 {
		t.Fatalf("shallow liquid blocks = %d, want no contact above its surface", got)
	}
}

func TestMovementChecksSweptChunks(t *testing.T) {
	state := newBaseState()
	state.Pos = mgl32.Vec3{15.5, 0, 0.5}
	state.Vel = mgl32.Vec3{1, 0, 0}

	result := (&Simulator{World: selectiveChunkWorld{}}).SimulateState(state)
	if result.Outcome != SimulationOutcomeUnloadedChunk {
		t.Fatalf("outcome = %v, want unloaded chunk for swept movement", result.Outcome)
	}
}

func TestMovementPreflightsAuxiliaryWorldProbes(t *testing.T) {
	w := &auxiliaryProbeWorld{}
	state := newBaseState()
	state.HasGravity = false

	result := (&Simulator{World: w}).SimulateState(state)

	if result.Outcome != SimulationOutcomeUnloadedChunk {
		t.Fatalf("outcome = %v, want unloaded chunk for unknown auxiliary probes", result.Outcome)
	}
	if w.blockReads != 0 {
		t.Fatalf("unknown auxiliary area was read %d times", w.blockReads)
	}
}

func TestMovementChecksStepProbeArea(t *testing.T) {
	w := stepProbeWorld{staticWorld: staticWorld{
		chunkLoaded: true,
		boxes:       []cube.BBox32{cube.Box32(1, 0, 0, 2, 0.5, 1)},
	}}
	state := newBaseState()
	state.Pos = mgl32.Vec3{0.5, 0, 0.5}
	state.Client.Pos = state.Pos
	state.Vel = mgl32.Vec3{1, 0, 0}
	state.OnGround = true
	state.HasGravity = false

	result := (&Simulator{
		World:   w,
		Options: SimulationOptions{IgnoreClientStepTiebreaker: true},
	}).SimulateState(state)

	if result.Outcome != SimulationOutcomeUnloadedChunk {
		t.Fatalf("outcome = %v, want unloaded chunk for unknown step probe", result.Outcome)
	}
	if state.Pos != (mgl32.Vec3{0.5, 0, 0.5}) {
		t.Fatalf("unknown step probe moved state to %v", state.Pos)
	}
}

func TestMovementChecksSneakEdgeProbeArea(t *testing.T) {
	state := newBaseState()
	state.Sneaking = true
	state.OnGround = true
	state.HasGravity = false
	state.Vel = mgl32.Vec3{0.2, 0, 0}

	result := (&Simulator{World: edgeProbeWorld{}}).SimulateState(state)

	if result.Outcome != SimulationOutcomeUnloadedChunk {
		t.Fatalf("outcome = %v, want unloaded chunk for unknown sneak-edge probe", result.Outcome)
	}
}

func TestMovementChecksLiquidExitProbeArea(t *testing.T) {
	base := newLiquidWorld().fill(cube.Pos{-1, 0, -1}, cube.Pos{0, 2, 1}, waterSource)
	for y := range 3 {
		base.set(cube.Pos{1, y, 0}, block.Stone{})
	}
	w := liquidExitProbeWorld{liquidWorld: base}
	state := submergedState()
	state.HasGravity = false
	state.Vel = mgl32.Vec3{1, 0, 0}

	result := newLiquidSim(w).SimulateState(state)

	if result.Outcome != SimulationOutcomeUnloadedChunk {
		t.Fatalf("outcome = %v, want unloaded chunk for unknown liquid-exit probe", result.Outcome)
	}
}

func TestMovementChecksLiquidFlowProbeArea(t *testing.T) {
	base := newLiquidWorld().set(cube.Pos{15, 0, 0}, waterSource)
	w := liquidFlowProbeWorld{liquidWorld: base}
	state := submergedState()
	state.Pos = mgl32.Vec3{15.5, 0.5, 0.5}
	state.Client.Pos = state.Pos
	state.HasGravity = false

	result := newLiquidSim(w).SimulateState(state)

	if result.Outcome != SimulationOutcomeUnloadedChunk {
		t.Fatalf("outcome = %v, want unloaded chunk for unknown liquid-flow probe", result.Outcome)
	}
}

func TestMovementChecksTargetPoseArea(t *testing.T) {
	w := poseProbeWorld{staticWorld: staticWorld{
		chunkLoaded: true,
		boxes:       []cube.BBox32{cube.Box32(-1, 0.7, -1, 1, 1.8, 1)},
	}}
	state := newBaseState()
	state.CrawlingHeight = 0.6
	state.Crawling = true
	state.Size[1] = state.CrawlingHeight

	result := (&Simulator{World: w}).Simulate(state, InputState{StopCrawling: true})

	if result.Outcome != SimulationOutcomeUnloadedChunk {
		t.Fatalf("outcome = %v, want unloaded chunk for unknown target pose", result.Outcome)
	}
	if !state.Crawling || state.Size[1] != state.CrawlingHeight {
		t.Fatalf("unknown target pose was committed: crawling=%v size=%v", state.Crawling, state.Size)
	}
}

func TestImmobileMovementDoesNotCheckUnappliedSweep(t *testing.T) {
	state := newBaseState()
	state.Pos = mgl32.Vec3{15.5, 0, 0.5}
	state.Client.Pos = state.Pos
	state.Vel = mgl32.Vec3{1, 0, 0}
	state.Immobile = true
	state.RiptideReady = true

	result := (&Simulator{World: selectiveChunkWorld{}}).Simulate(state, InputState{ClientPos: state.Pos})

	if result.Outcome != SimulationOutcomeImmobileOrNotReady {
		t.Fatalf("outcome = %v, want immobile/not ready", result.Outcome)
	}
	if state.Vel != (mgl32.Vec3{}) {
		t.Fatalf("immobile state retained stale velocity: %v", state.Vel)
	}
	if state.RiptideReady || state.TicksSinceKnockback != 2 {
		t.Fatalf("immobile tick did not advance transient state: ready=%v knockback=%d", state.RiptideReady, state.TicksSinceKnockback)
	}
}

func TestLegacySprintTransitionUpdatesSpeedOnUnloadedTick(t *testing.T) {
	state := newBaseState()
	state.Pos = mgl32.Vec3{16.5, 0, 0.5}
	state.Client.Pos = state.Pos
	sim := &Simulator{
		World: selectiveChunkWorld{},
		Options: SimulationOptions{
			SprintTiming: SprintTimingLegacy,
		},
	}

	result := sim.Simulate(state, InputState{StartSprinting: true, ClientPos: state.Pos})

	if result.Outcome != SimulationOutcomeUnloadedChunk {
		t.Fatalf("outcome = %v, want unloaded chunk", result.Outcome)
	}
	if !state.Sprinting || math32.Abs(state.MovementSpeed-0.13) > 1e-6 {
		t.Fatalf("legacy sprint transition desynchronized state: sprinting=%v speed=%v", state.Sprinting, state.MovementSpeed)
	}
}

func TestQueuedKnockbackChecksSweptChunks(t *testing.T) {
	state := newBaseState()
	state.Pos = mgl32.Vec3{15.5, 0, 0.5}
	state.Client.Pos = state.Pos
	state.QueueKnockback(mgl32.Vec3{1, 0, 0})

	result := (&Simulator{World: selectiveChunkWorld{}}).SimulateState(state)

	if result.Outcome != SimulationOutcomeUnloadedChunk {
		t.Fatalf("outcome = %v, want unloaded chunk for queued knockback", result.Outcome)
	}
	if state.Pos != state.Client.Pos {
		t.Fatalf("queued knockback moved into unloaded area: %v", state.Pos)
	}
}

func TestInputAccelerationChecksSweptChunks(t *testing.T) {
	state := newBaseState()
	state.Pos = mgl32.Vec3{15.69, 0, 0.5}
	state.Client.Pos = state.Pos
	state.HasGravity = false

	result := (&Simulator{World: selectiveChunkWorld{}}).Simulate(state, InputState{MoveVector: mgl32.Vec2{1, 0}})

	if result.Outcome != SimulationOutcomeUnloadedChunk {
		t.Fatalf("outcome = %v, want unloaded chunk for same-tick acceleration", result.Outcome)
	}
}

func TestRiptideLaunchChecksSweptChunks(t *testing.T) {
	state := newBaseState()
	state.Pos = mgl32.Vec3{0.5, 0, 15.5}
	state.Client.Pos = state.Pos
	state.RiptideInRain = true
	state.RiptideReady = true

	result := (&Simulator{
		World:     selectiveChunkWorld{},
		Equipment: fixedEquipment{EnchantmentRiptide: 2},
	}).Simulate(state, InputState{StartSpinAttack: true})

	if result.Outcome != SimulationOutcomeUnloadedChunk {
		t.Fatalf("outcome = %v, want unloaded chunk for Riptide launch", result.Outcome)
	}
	if state.RiptideTicks != 0 || !state.RiptideReady || !state.StartingSpinAttack {
		t.Fatalf("unloaded Riptide consumed launch state: ticks=%d ready=%v starting=%v", state.RiptideTicks, state.RiptideReady, state.StartingSpinAttack)
	}
	if state.TicksSinceKnockback != 1 {
		t.Fatalf("unloaded Riptide advanced tick counters: knockback=%d", state.TicksSinceKnockback)
	}
}

func TestRiptideLaunchRetriesAfterUnloadedTick(t *testing.T) {
	state := newBaseState()
	state.Pos = mgl32.Vec3{0.5, 0, 15.5}
	state.Client.Pos = state.Pos
	state.RiptideInRain = true
	state.RiptideReady = true
	equipment := fixedEquipment{EnchantmentRiptide: 2}

	first := (&Simulator{
		World:     selectiveChunkWorld{},
		Equipment: equipment,
	}).Simulate(state, InputState{ClientPos: state.Client.Pos, StartSpinAttack: true})
	if first.Outcome != SimulationOutcomeUnloadedChunk {
		t.Fatalf("first outcome = %v, want unloaded chunk", first.Outcome)
	}

	second := (&Simulator{
		World:     mockWorld{},
		Equipment: equipment,
	}).Simulate(state, InputState{ClientPos: state.Client.Pos})
	if second.Outcome != SimulationOutcomeNormal || state.RiptideTicks == 0 {
		t.Fatalf("retried launch outcome=%v ticks=%d", second.Outcome, state.RiptideTicks)
	}
	if state.RiptideReady || state.StartingSpinAttack {
		t.Fatalf("successful retry left launch pending: ready=%v starting=%v", state.RiptideReady, state.StartingSpinAttack)
	}
}

func TestRiptideHeadProbeRequiresLoadedArea(t *testing.T) {
	base := newLiquidWorld().set(cube.Pos{0, 0, 0}, waterSource)
	w := &riptideHeadProbeWorld{liquidWorld: base}
	state := newBaseState()
	state.Pos = mgl32.Vec3{0.5, 0, 0.5}
	state.Client.Pos = state.Pos
	state.CrawlingHeight = 0.6
	state.Crawling = true
	state.Size[1] = state.CrawlingHeight
	state.OnGround = true
	state.HasGravity = true
	state.RiptideReady = true
	state.StartingSpinAttack = true

	result := (&Simulator{
		World:     w,
		Equipment: fixedEquipment{EnchantmentRiptide: 2},
	}).SimulateState(state)

	if result.Outcome != SimulationOutcomeUnloadedChunk {
		t.Fatalf("outcome = %v, want unloaded chunk for unknown Riptide head probe", result.Outcome)
	}
	if w.headProbes != 1 {
		t.Fatalf("Riptide head probe checks = %d, want 1", w.headProbes)
	}
	if state.RiptideTicks != 0 || !state.RiptideReady || !state.StartingSpinAttack {
		t.Fatalf("unknown head probe consumed launch state: ticks=%d ready=%v starting=%v", state.RiptideTicks, state.RiptideReady, state.StartingSpinAttack)
	}
}

func TestIneligibleRiptideSkipsHeadProbe(t *testing.T) {
	w := &riptideHeadProbeWorld{liquidWorld: newLiquidWorld()}
	state := newBaseState()
	state.CrawlingHeight = 0.6
	state.Crawling = true
	state.Size[1] = state.CrawlingHeight
	state.HasGravity = false

	result := (&Simulator{World: w}).SimulateState(state)

	if result.Outcome != SimulationOutcomeNormal {
		t.Fatalf("outcome = %v, want normal movement without Riptide", result.Outcome)
	}
	if w.headProbes != 0 {
		t.Fatalf("ineligible Riptide performed %d head probes", w.headProbes)
	}
}

func TestCompletedTeleportCounterAdvancesOnce(t *testing.T) {
	state := newBaseState()
	state.QueueTeleport(mgl32.Vec3{10, 20, 30}, false, 0)

	result := (&Simulator{World: mockWorld{}}).Simulate(state, InputState{})

	if result.Outcome != SimulationOutcomeTeleport {
		t.Fatalf("outcome = %v, want teleport", result.Outcome)
	}
	if state.TicksSinceTeleport != 1 {
		t.Fatalf("completed teleport tick counter = %d, want 1", state.TicksSinceTeleport)
	}
}

func TestMovementRejectsOutOfRangeSweep(t *testing.T) {
	state := newBaseState()
	state.Pos = mgl32.Vec3{math32.MaxFloat32, 0, 0}

	result := (&Simulator{World: selectiveChunkWorld{}}).SimulateState(state)
	if result.Outcome != SimulationOutcomeUnloadedChunk {
		t.Fatalf("out-of-range sweep outcome = %v, want unloaded chunk", result.Outcome)
	}
}

func TestMovementAreaProviderCannotApproveUnsafeVolume(t *testing.T) {
	sim := &Simulator{World: approvingAreaWorld{}}
	for name, aabb := range map[string]cube.BBox32{
		"coordinate": cube.Box32(0, 0, 0, math32.MaxFloat32, 1, 1),
		"height":     cube.Box32(0, 0, 0, 1, math32.MaxFloat32, 1),
	} {
		t.Run(name, func(t *testing.T) {
			if sim.movementAreaLoaded(aabb) {
				t.Fatalf("provider approved unsafe %s volume: %v", name, aabb)
			}
		})
	}
}

func TestUnloadedTickDoesNotCommitPoseChanges(t *testing.T) {
	state := newBaseState()
	state.Pos = mgl32.Vec3{16.5, 0, 0.5}
	state.Swimming = true
	state.Size[1] = state.StandingHeight
	originalSize := state.Size

	result := (&Simulator{World: selectiveChunkWorld{}}).Simulate(state, InputState{StopSwimming: true})
	if result.Outcome != SimulationOutcomeUnloadedChunk {
		t.Fatalf("outcome = %v, want unloaded chunk", result.Outcome)
	}
	if !state.Swimming || state.Size != originalSize {
		t.Fatalf("unloaded tick committed pose change: swimming=%v size=%v", state.Swimming, state.Size)
	}
}

func TestAdjacentClimbableIsNotContact(t *testing.T) {
	w := environmentWorld{blocks: map[cube.Pos]world.Block{
		{1, 0, 0}: block.Ladder{Facing: cube.West},
	}}
	state := newBaseState()
	state.Pos = mgl32.Vec3{0.8, 0, 0.5}
	state.Client.Pos = state.Pos
	state.EffectiveJumping = true
	state.Gravity = NormalGravity

	(&Simulator{World: w}).SimulateState(state)
	if math32.Abs(state.Vel.Y()-ClimbSpeed) < 1e-6 {
		t.Fatalf("a ladder the player only overlaps was treated as climbable contact: %v", state.Vel)
	}
}

func TestClimbableContactResetsFallDistance(t *testing.T) {
	w := environmentWorld{blocks: map[cube.Pos]world.Block{
		{0, 0, 0}: block.Ladder{Facing: cube.West},
	}}
	state := newBaseState()
	state.Pos = mgl32.Vec3{0.5, 0, 0.5}
	state.Client.Pos = state.Pos
	state.Vel = mgl32.Vec3{0, -0.1, 0}
	state.FallDistance = 4
	state.HasGravity = false

	(&Simulator{World: w}).SimulateState(state)

	if state.FallDistance != 0 {
		t.Fatalf("climbable contact left fall distance = %v", state.FallDistance)
	}
}

func TestStandingOnClimbableBlockDoesNotEnableClimbing(t *testing.T) {
	pos := cube.Pos{0, -1, 0}
	w := environmentWorld{
		solids: map[cube.Pos]bool{pos: true},
		blocks: map[cube.Pos]world.Block{pos: semanticsNamedBlock{name: "minecraft:ladder"}},
	}
	state := newBaseState()
	state.Pos = mgl32.Vec3{0.5, 0, 0.5}
	state.EffectiveJumping = true

	(&Simulator{World: w}).SimulateState(state)
	if math32.Abs(state.Vel.Y()-ClimbSpeed) < 1e-6 {
		t.Fatalf("standing on a climbable block enabled climbing: %v", state.Vel)
	}
}

func TestClimbableBlockBelowIsNotContact(t *testing.T) {
	w := environmentWorld{blocks: map[cube.Pos]world.Block{
		{0, -1, 0}: block.Ladder{Facing: cube.West},
	}}
	state := newBaseState()
	state.Pos = mgl32.Vec3{0.5, 0, 0.5}
	state.Client.Pos = state.Pos
	state.EffectiveJumping = true
	state.Gravity = NormalGravity

	(&Simulator{World: w}).SimulateState(state)
	if math32.Abs(state.Vel.Y()-ClimbSpeed) < 1e-6 {
		t.Fatalf("ladder below the player was treated as climbable contact: %v", state.Vel)
	}
}

func TestPowderSnowSupportDoesNotEnableTraversal(t *testing.T) {
	pos := cube.Pos{0, 0, 0}
	w := &dynamicCollisionWorld{environmentWorld: environmentWorld{
		solids: map[cube.Pos]bool{pos: true},
		blocks: map[cube.Pos]world.Block{pos: semanticsNamedBlock{name: "minecraft:powder_snow"}},
	}}
	state := newBaseState()
	state.Pos = mgl32.Vec3{0.5, 1, 0.5}
	state.OnGround = true
	state.PressingAscend = true

	(&Simulator{World: w, Equipment: leatherEquipment{}}).SimulateState(state)
	if math32.Abs(state.Vel.Y()-0.2) < 1e-6 {
		t.Fatalf("powder snow below the player enabled traversal: %v", state.Vel)
	}
}

func TestDynamicCollisionProviderKeepsStaticSupportFallback(t *testing.T) {
	pos := cube.Pos{0, 0, 0}
	w := &dynamicCollisionWorld{environmentWorld: environmentWorld{solids: map[cube.Pos]bool{pos: true}}}
	state := newBaseState()
	state.Pos = mgl32.Vec3{0.5, 1, 0.5}
	state.OnGround = true

	(&Simulator{World: w}).checkSupportingBlockPos(state, false, mgl32.Vec3{})
	if state.SupportingBlockPos == nil || *state.SupportingBlockPos != pos {
		t.Fatalf("dynamic collision provider lost support block: %v", state.SupportingBlockPos)
	}
}

func TestFilteredCollisionBoxesPreserveProviderOrder(t *testing.T) {
	first := cube.Box32(2, 0, 0, 3, 1, 1)
	second := cube.Box32(1, 0, 0, 2, 1, 1)
	got := filteredCollisionBoxes([]cube.BBox32{first, cube.Box32(0, 0, 0, 0, 1, 1), second})
	if len(got) != 2 || got[0] != first || got[1] != second {
		t.Fatalf("collision order changed while filtering: %v", got)
	}
}

func TestCollisionPresenceFiltersInvalidBoxes(t *testing.T) {
	state := newBaseState()
	sim := &Simulator{World: invalidCollisionWorld{}}

	if sim.hasNearbyBBoxes(state, state.BoundingBox(false)) {
		t.Fatal("zero-volume collision box was reported as present")
	}
}

type invalidCollisionWorld struct{ mockWorld }

func (invalidCollisionWorld) GetNearbyBBoxes(cube.BBox32) []cube.BBox32 {
	return []cube.BBox32{cube.Box32(0, 0, 0, 0, 1, 1)}
}

// HasNearbyBBoxes reports the invalid box to exercise the unfilterable fast path.
func (invalidCollisionWorld) HasNearbyBBoxes(cube.BBox32) bool {
	return true
}

type selectiveChunkWorld struct{}

func (selectiveChunkWorld) Block(cube.Pos) world.Block { return block.Air{} }

func (selectiveChunkWorld) BlockCollisions(cube.Pos) []cube.BBox32 { return nil }

func (selectiveChunkWorld) GetNearbyBBoxes(cube.BBox32) []cube.BBox32 { return nil }

func (selectiveChunkWorld) IsChunkLoaded(chunkX, chunkZ int32) bool {
	return chunkX == 0 && chunkZ == 0
}

type auxiliaryProbeWorld struct {
	blockReads int
}

func (w *auxiliaryProbeWorld) Block(cube.Pos) world.Block {
	w.blockReads++
	return block.Air{}
}

func (*auxiliaryProbeWorld) BlockCollisions(cube.Pos) []cube.BBox32 { return nil }

func (*auxiliaryProbeWorld) GetNearbyBBoxes(cube.BBox32) []cube.BBox32 { return nil }

func (*auxiliaryProbeWorld) IsChunkLoaded(int32, int32) bool { return true }

// IsMovementAreaLoaded accepts the actor box but rejects surrounding probes.
func (*auxiliaryProbeWorld) IsMovementAreaLoaded(aabb cube.BBox32) bool {
	return aabb.Min().X() >= -0.3 && aabb.Min().Y() >= 0 && aabb.Min().Z() >= -0.3 &&
		aabb.Max().X() <= 0.3 && aabb.Max().Y() <= 1.8 && aabb.Max().Z() <= 0.3
}

type stepProbeWorld struct {
	staticWorld
}

// IsMovementAreaLoaded rejects collision probes above the ordinary movement sweep.
func (stepProbeWorld) IsMovementAreaLoaded(aabb cube.BBox32) bool {
	return aabb.Max().Y() <= 1.81
}

type edgeProbeWorld struct {
	mockWorld
}

// IsMovementAreaLoaded rejects the downward sneak-edge support probe.
func (edgeProbeWorld) IsMovementAreaLoaded(aabb cube.BBox32) bool {
	return aabb.Min().Y() >= -0.1
}

type liquidExitProbeWorld struct {
	*liquidWorld
}

// IsMovementAreaLoaded rejects the raised liquid-exit probe.
func (liquidExitProbeWorld) IsMovementAreaLoaded(aabb cube.BBox32) bool {
	return aabb.Max().Y() <= 2.5
}

type liquidFlowProbeWorld struct {
	*liquidWorld
}

// IsMovementAreaLoaded rejects liquid-flow reads across the chunk boundary.
func (liquidFlowProbeWorld) IsMovementAreaLoaded(aabb cube.BBox32) bool {
	return aabb.Max().X() < 16
}

type poseProbeWorld struct {
	staticWorld
}

type riptideHeadProbeWorld struct {
	*liquidWorld
	headProbes int
}

// IsMovementAreaLoaded rejects the block containing a crawling player's head.
func (w *riptideHeadProbeWorld) IsMovementAreaLoaded(aabb cube.BBox32) bool {
	if aabb.Min().Y() == 1 && aabb.Max().Y() == 2 && aabb.Min().X() == 0 && aabb.Max().X() == 1 && aabb.Min().Z() == 0 && aabb.Max().Z() == 1 {
		w.headProbes++
		return false
	}
	return true
}

// IsMovementAreaLoaded accepts the current crawl pose but rejects standing.
func (poseProbeWorld) IsMovementAreaLoaded(aabb cube.BBox32) bool {
	return aabb.Max().Y() <= 0.7
}

type approvingAreaWorld struct {
	mockWorld
}

// IsMovementAreaLoaded approves every volume so BedSim's own bounds are tested.
func (approvingAreaWorld) IsMovementAreaLoaded(cube.BBox32) bool {
	return true
}
