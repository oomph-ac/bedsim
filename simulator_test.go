package bedsim

import (
	"fmt"
	"strings"
	"testing"

	"github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/go-gl/mathgl/mgl64"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

type mockWorld struct{}

func (mockWorld) Block(pos cube.Pos) world.Block {
	return block.Air{}
}

func (mockWorld) BlockCollisions(pos cube.Pos) []cube.BBox {
	return nil
}

func (mockWorld) GetNearbyBBoxes(aabb cube.BBox) []cube.BBox {
	return nil
}

func (mockWorld) IsChunkLoaded(chunkX, chunkZ int32) bool {
	return true
}

type staticWorld struct {
	chunkLoaded bool
	boxes       []cube.BBox
}

func (w staticWorld) Block(pos cube.Pos) world.Block {
	return block.Air{}
}

func (w staticWorld) BlockCollisions(pos cube.Pos) []cube.BBox {
	return nil
}

func (w staticWorld) GetNearbyBBoxes(aabb cube.BBox) []cube.BBox {
	if len(w.boxes) == 0 {
		return nil
	}

	out := make([]cube.BBox, 0, len(w.boxes))
	for _, bb := range w.boxes {
		if aabb.IntersectsWith(bb) {
			out = append(out, bb)
		}
	}
	return out
}

func (w staticWorld) IsChunkLoaded(chunkX, chunkZ int32) bool {
	if !w.chunkLoaded {
		return false
	}
	return true
}

type mockEffects struct{}

func (mockEffects) GetEffect(effectID int32) (int32, bool) {
	return 0, false
}

type mockInventory struct {
	hasElytra bool
}

func (m mockInventory) HasElytra() bool {
	return m.hasElytra
}

func newBaseState() *MovementState {
	return &MovementState{
		Client: ClientState{
			Pos: mgl64.Vec3{},
			Vel: mgl64.Vec3{},
			Mov: mgl64.Vec3{},
		},
		Pos:                  mgl64.Vec3{},
		Vel:                  mgl64.Vec3{},
		Mov:                  mgl64.Vec3{},
		Size:                 mgl64.Vec3{0.6, 1.8, 1},
		MovementSpeed:        0.1,
		DefaultMovementSpeed: 0.1,
		AirSpeed:             0.02,
		OnGround:             false,
		HasGravity:           true,
		Ready:                true,
		Alive:                true,
		GameMode:             packet.GameTypeSurvival,
		TicksSinceKnockback:  1,
		TicksSinceTeleport:   1,
	}
}

func containsLog(logs []string, needle string) bool {
	for _, line := range logs {
		if strings.Contains(line, needle) {
			return true
		}
	}
	return false
}

func TestSimulateMoveRelative(t *testing.T) {
	sim := &Simulator{
		World:   mockWorld{},
		Effects: mockEffects{},
		Options: SimulationOptions{
			UseSlideOffset:              false,
			PositionCorrectionThreshold: 0.3,
		},
	}

	state := newBaseState()

	input := InputState{
		MoveVector: mgl64.Vec2{0, 1},
		ClientPos:  mgl64.Vec3{},
		ClientVel:  mgl64.Vec3{},
		Yaw:        0,
		Pitch:      0,
		HeadYaw:    0,
	}

	result := sim.Simulate(state, input)
	if result.Velocity.Z() <= 0 {
		t.Fatalf("expected forward velocity, got %v", result.Velocity)
	}
	if result.Velocity.Y() >= 0 {
		t.Fatalf("expected gravity to apply, got %v", result.Velocity)
	}
}

func TestSimulateStateOutcomeTeleport(t *testing.T) {
	sim := &Simulator{
		World:   mockWorld{},
		Effects: mockEffects{},
	}

	state := newBaseState()
	state.TeleportPos = mgl64.Vec3{12, 63, -4}
	state.TicksSinceTeleport = 0
	state.TeleportCompletionTicks = 0
	state.TeleportIsSmoothed = false

	result := sim.SimulateState(state)
	if result.Outcome != SimulationOutcomeTeleport {
		t.Fatalf("expected teleport outcome, got %v", result.Outcome)
	}
	if result.Position != state.TeleportPos {
		t.Fatalf("expected teleported position %v, got %v", state.TeleportPos, result.Position)
	}
}

func TestSimulateStateOutcomeUnreliable(t *testing.T) {
	sim := &Simulator{
		World:   mockWorld{},
		Effects: mockEffects{},
	}

	state := newBaseState()
	state.GameMode = packet.GameTypeCreative
	state.Pos = mgl64.Vec3{10, 70, 10}
	state.Client.Pos = mgl64.Vec3{3, 64, -1}
	state.Vel = mgl64.Vec3{0.3, 0.9, -0.2}
	state.Client.Vel = mgl64.Vec3{-0.1, 0, 0.2}

	result := sim.SimulateState(state)
	if result.Outcome != SimulationOutcomeUnreliable {
		t.Fatalf("expected unreliable outcome, got %v", result.Outcome)
	}
	if state.Pos != state.Client.Pos {
		t.Fatalf("expected state reset to client position, got %v vs %v", state.Pos, state.Client.Pos)
	}
	if state.Vel != state.Client.Vel {
		t.Fatalf("expected state reset to client velocity, got %v vs %v", state.Vel, state.Client.Vel)
	}
}

func TestSimulateStateOutcomeUnloadedChunk(t *testing.T) {
	sim := &Simulator{
		World:   staticWorld{chunkLoaded: false},
		Effects: mockEffects{},
	}

	state := newBaseState()
	state.Vel = mgl64.Vec3{0.2, 0.1, -0.1}

	result := sim.SimulateState(state)
	if result.Outcome != SimulationOutcomeUnloadedChunk {
		t.Fatalf("expected unloaded chunk outcome, got %v", result.Outcome)
	}
	if state.Vel != (mgl64.Vec3{}) {
		t.Fatalf("expected velocity to be cleared, got %v", state.Vel)
	}
}

func TestSimulateStateOutcomeImmobileOrNotReady(t *testing.T) {
	sim := &Simulator{
		World:   mockWorld{},
		Effects: mockEffects{},
	}

	state := newBaseState()
	state.Immobile = true
	state.Vel = mgl64.Vec3{0.5, -0.3, 0.5}

	result := sim.SimulateState(state)
	if result.Outcome != SimulationOutcomeImmobileOrNotReady {
		t.Fatalf("expected immobile/not-ready outcome, got %v", result.Outcome)
	}
	if state.Vel != (mgl64.Vec3{}) {
		t.Fatalf("expected velocity to be cleared, got %v", state.Vel)
	}
}

func TestSimulateStateSkipsGravityWhenDisabled(t *testing.T) {
	sim := &Simulator{
		World:   mockWorld{},
		Effects: mockEffects{},
	}

	state := newBaseState()
	state.HasGravity = false
	state.Impulse = mgl64.Vec2{0, 0.98}

	result := sim.SimulateState(state)
	if result.Outcome != SimulationOutcomeNormal {
		t.Fatalf("expected normal outcome, got %v", result.Outcome)
	}
	if result.Velocity.Y() != 0 {
		t.Fatalf("expected no gravity change on Y velocity, got %v", result.Velocity)
	}
}

func TestSimulateStateInvalidGlideContinuesNormalMovement(t *testing.T) {
	sim := &Simulator{
		World:     mockWorld{},
		Effects:   mockEffects{},
		Inventory: mockInventory{hasElytra: false},
	}

	state := newBaseState()
	state.Gliding = true
	state.OnGround = true
	state.Impulse = mgl64.Vec2{0, 0.98}

	result := sim.SimulateState(state)
	if result.Outcome != SimulationOutcomeNormal {
		t.Fatalf("expected normal outcome, got %v", result.Outcome)
	}
	if state.Gliding {
		t.Fatalf("expected gliding to be cleared")
	}
	if result.Velocity.Z() <= 0 {
		t.Fatalf("expected movement to continue after invalid glide, got %v", result.Velocity)
	}
}

func TestSimulateStateDebugTraceIncludesCollisionStream(t *testing.T) {
	var logs []string
	sim := &Simulator{
		World:   mockWorld{},
		Effects: mockEffects{},
		Options: SimulationOptions{
			Debugf: func(format string, args ...any) {
				logs = append(logs, fmt.Sprintf(format, args...))
			},
		},
	}

	state := newBaseState()
	state.Impulse = mgl64.Vec2{0, 0.98}

	result := sim.SimulateState(state)
	if result.Outcome != SimulationOutcomeNormal {
		t.Fatalf("expected normal outcome, got %v", result.Outcome)
	}

	expected := []string{
		"blockUnder=",
		"moveRelative force applied",
		"Y-collision non-step=",
		"(X) hz-collision non-step=",
		"(Z) hz-collision non-step=",
		"finalVel=",
		"(server) xCollision=",
	}
	for _, needle := range expected {
		if !containsLog(logs, needle) {
			t.Fatalf("expected debug logs to contain %q, logs=%v", needle, logs)
		}
	}
}

func TestSimulateStateDebugTraceJumpBlocked(t *testing.T) {
	var logs []string
	sim := &Simulator{
		World: staticWorld{
			chunkLoaded: true,
			boxes: []cube.BBox{
				cube.Box(0, 2, 1, 1, 3, 2),
			},
		},
		Effects: mockEffects{},
		Options: SimulationOptions{
			Debugf: func(format string, args ...any) {
				logs = append(logs, fmt.Sprintf(format, args...))
			},
		},
	}

	state := newBaseState()
	state.Pos = mgl64.Vec3{0, 0, 0.69}
	state.Client.Pos = state.Pos
	state.OnGround = true
	state.Jumping = true
	state.Sprinting = true
	state.Rotation = mgl64.Vec3{0, 0, 0}
	state.JumpHeight = DefaultJumpHeight

	result := sim.SimulateState(state)
	if result.Outcome != SimulationOutcomeNormal {
		t.Fatalf("expected normal outcome, got %v", result.Outcome)
	}
	if !containsLog(logs, "jump determined to be blocked") {
		t.Fatalf("expected jump-block debug log, logs=%v", logs)
	}
}

// TestStepUpTiebreaker verifies the client-alignment tie-breaker in tryCollisions:
//   - Without IgnoreClientStepTiebreaker, a slab/stair step-up is rejected when
//     the client position matches the pre-step position.
//   - With IgnoreClientStepTiebreaker, the same step-up is accepted.
//   - A genuinely blocked step (collision above) is still rejected even with the flag.
func TestStepUpTiebreaker(t *testing.T) {
	// Geometry: ground at Y=0, a 0.5-high slab at X=1 (X=1..2, Y=0..0.5).
	// The player stands on the ground at X≈0.5, walks in +X toward the slab.
	// The step-up (0.5 blocks) is within StepHeight (0.6).
	slabBox := cube.Box(1, 0, -1, 2, 0.5, 2)
	groundBox := cube.Box(-1, -1, -1, 1, 0, 2)

	startPos := mgl64.Vec3{0.5, 0, 0.5}

	runSim := func(ignoreStepTiebreaker bool) (mgl64.Vec3, bool) {
		w := staticWorld{chunkLoaded: true, boxes: []cube.BBox{slabBox, groundBox}}
		sim := &Simulator{
			World:   w,
			Effects: mockEffects{},
			Options: SimulationOptions{
				PositionCorrectionThreshold:    0.3,
				IgnoreClientStepTiebreaker: ignoreStepTiebreaker,
			},
		}
		state := newBaseState()
		state.Pos = startPos
		state.Client.Pos = startPos
		state.OnGround = true
		state.JumpHeight = DefaultJumpHeight

		input := InputState{
			MoveVector: mgl64.Vec2{0, 1},
			ClientPos:  startPos,
			ClientVel:  mgl64.Vec3{},
			Yaw:        -90, // face +X
			HeadYaw:    -90,
		}

		// Run enough ticks for the player to reach the slab edge and attempt step-up.
		for range 10 {
			sim.Simulate(state, input)
			input.ClientPos = state.Pos
			input.ClientVel = state.Vel
		}
		stepped := state.Pos.Y() >= 0.45
		return state.Pos, stepped
	}

	t.Run("rejected without flag", func(t *testing.T) {
		pos, stepped := runSim(false)
		if stepped {
			t.Fatalf("expected step-up to be rejected by tie-breaker, but player stepped up to Y=%.4f", pos.Y())
		}
	})

	t.Run("accepted with flag", func(t *testing.T) {
		pos, stepped := runSim(true)
		if !stepped {
			t.Fatalf("expected step-up to be accepted with IgnoreClientStepTiebreaker, but player at Y=%.4f", pos.Y())
		}
	})

	t.Run("blocked step still rejected with flag", func(t *testing.T) {
		// Place a ceiling directly above the slab so stepping up would cause collision.
		ceilingBox := cube.Box(1, 1.3, -1, 2, 2.3, 2) // leaves only 0.8 gap, player is 1.8 tall
		w := staticWorld{chunkLoaded: true, boxes: []cube.BBox{slabBox, groundBox, ceilingBox}}
		sim := &Simulator{
			World:   w,
			Effects: mockEffects{},
			Options: SimulationOptions{
				PositionCorrectionThreshold:    0.3,
				IgnoreClientStepTiebreaker: true,
			},
		}
		state := newBaseState()
		state.Pos = startPos
		state.Client.Pos = startPos
		state.OnGround = true
		state.JumpHeight = DefaultJumpHeight

		input := InputState{
			MoveVector: mgl64.Vec2{0, 1},
			ClientPos:  startPos,
			ClientVel:  mgl64.Vec3{},
			Yaw:        -90,
			HeadYaw:    -90,
		}

		for range 10 {
			sim.Simulate(state, input)
			input.ClientPos = state.Pos
			input.ClientVel = state.Vel
		}
		if state.Pos.Y() >= 0.45 {
			t.Fatalf("expected blocked step to be rejected even with flag, but player at Y=%.4f", state.Pos.Y())
		}
	})
}

func TestResultFromStateCorrectionModes(t *testing.T) {
	tests := []struct {
		name    string
		mode    SimulationMode
		mutate  func(*MovementState)
		wantSet bool
	}{
		{
			name: "authoritative velocity-only drift",
			mode: SimulationModeAuthoritative,
			mutate: func(state *MovementState) {
				state.Vel = mgl64.Vec3{0.5, 0, 0}
				state.Client.Vel = mgl64.Vec3{}
			},
			wantSet: true,
		},
		{
			name: "permissive velocity-only drift",
			mode: SimulationModePermissive,
			mutate: func(state *MovementState) {
				state.Vel = mgl64.Vec3{0.5, 0, 0}
				state.Client.Vel = mgl64.Vec3{}
			},
			wantSet: false,
		},
		{
			name: "permissive position drift",
			mode: SimulationModePermissive,
			mutate: func(state *MovementState) {
				state.Pos = mgl64.Vec3{0.5, 0, 0}
				state.Client.Pos = mgl64.Vec3{}
			},
			wantSet: true,
		},
		{
			name: "passive position drift",
			mode: SimulationModePassive,
			mutate: func(state *MovementState) {
				state.Pos = mgl64.Vec3{0.5, 0, 0}
				state.Client.Pos = mgl64.Vec3{}
			},
			wantSet: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			state := newBaseState()
			tc.mutate(state)

			sim := &Simulator{
				Options: SimulationOptions{
					Mode:                        tc.mode,
					PositionCorrectionThreshold: 0.1,
					VelocityCorrectionThreshold: 0.1,
				},
			}
			result := sim.resultFromState(state, SimulationOutcomeNormal)
			if result.NeedsCorrection != tc.wantSet {
				t.Fatalf("mode=%v needsCorrection=%v want=%v", tc.mode, result.NeedsCorrection, tc.wantSet)
			}
		})
	}
}
