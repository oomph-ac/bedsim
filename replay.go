package bedsim

import (
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/go-gl/mathgl/mgl32"
)

// HeadLiquidObservation describes liquid at the player's continuous vanilla
// eye attachment position after a simulation tick. Known is false when the
// required world or second-layer liquid data is unavailable.
type HeadLiquidObservation struct {
	Position mgl32.Vec3
	Water    bool
	Lava     bool
	Known    bool
}

// ObserveHeadLiquid reports whether the player's eye attachment is below the
// local water or lava surface. This uses continuous position, the current
// player pose, entity scale, partial liquid height, and the configured liquid
// layer rather than a block-grid feet/head heuristic.
func (s *Simulator) ObserveHeadLiquid(state *MovementState) HeadLiquidObservation {
	observation := HeadLiquidObservation{}
	if s == nil || state == nil || !finiteMovementState(state) {
		return observation
	}
	position := state.EyePosition()
	observation.Position = position
	pos := posFromVec3(position)
	probe := cube.Box32(
		float32(pos.X()), float32(pos.Y()), float32(pos.Z()),
		float32(pos.X()+1), float32(pos.Y()+1), float32(pos.Z()+1),
	)
	if !s.movementAreaLoaded(probe) || s.Options.RequireLiquidLayer && !s.HasLiquidLayer() {
		return observation
	}
	observation.Known = true
	liquid, ok := s.liquidAt(pos)
	if !ok || position.Y() >= float32(pos.Y())+liquidHeight(liquid) {
		return observation
	}
	observation.Water = liquidWater.matches(liquid)
	observation.Lava = liquidLava.matches(liquid)
	return observation
}

// ReplayFrame captures one uninterrupted simulation tick and its post-tick
// liquid observation. State is a value snapshot and does not alias later
// frames through SupportingBlockPos.
type ReplayFrame struct {
	State      MovementState
	Result     SimulationResult
	HeadLiquid HeadLiquidObservation
}

// ReplayResult contains the final continuous movement state and every replayed
// tick. Callers decide whether a non-normal outcome invalidates their use case;
// Replay itself adds no pathfinding or correction policy.
type ReplayResult struct {
	State  MovementState
	Frames []ReplayFrame
}

// Replay advances initial through the supplied per-tick inputs with the same
// state continuity as repeated Simulate calls. The input state is copied, so
// callers may evaluate speculative trajectories without mutating live state.
func (s *Simulator) Replay(initial MovementState, inputs []InputState) ReplayResult {
	state := cloneMovementState(initial)
	frames := make([]ReplayFrame, 0, len(inputs))
	for _, input := range inputs {
		result := s.Simulate(&state, input)
		frames = append(frames, ReplayFrame{
			State:      cloneMovementState(state),
			Result:     result,
			HeadLiquid: s.ObserveHeadLiquid(&state),
		})
	}
	return ReplayResult{State: cloneMovementState(state), Frames: frames}
}

func cloneMovementState(state MovementState) MovementState {
	if state.SupportingBlockPos != nil {
		pos := *state.SupportingBlockPos
		state.SupportingBlockPos = &pos
	}
	return state
}
