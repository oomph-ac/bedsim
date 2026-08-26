package bedsim

import (
	"testing"

	"github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/go-gl/mathgl/mgl32"
)

func TestMovementState_EyePositionUsesVanillaPoseOffset(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		state MovementState
		wantY float32
	}{
		{name: "standing", state: MovementState{}, wantY: 11.62},
		{name: "sneaking", state: MovementState{Sneaking: true}, wantY: 11.27},
		{name: "swimming", state: MovementState{Swimming: true, SwimWaterGraceTicks: 1}, wantY: 10.4},
		{name: "crawling", state: MovementState{Crawling: true}, wantY: 10.4},
		{name: "gliding", state: MovementState{Gliding: true}, wantY: 10.4},
		{name: "scaled", state: MovementState{Size: mgl32.Vec3{0.6, 1.8, 2}}, wantY: 13.24},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := test.state
			state.Pos = mgl32.Vec3{2, 10, 3}
			if got := state.EyePosition(); got != (mgl32.Vec3{2, test.wantY, 3}) {
				t.Fatalf("eye position = %v, want %v", got, (mgl32.Vec3{2, test.wantY, 3}))
			}
		})
	}
}

func TestSimulator_ObserveHeadLiquidUsesPoseAndSurfaceHeight(t *testing.T) {
	t.Parallel()

	w := newLiquidWorld().set(cube.Pos{0, 0, 0}, block.Water{Depth: 4, Still: true})
	sim := newLiquidSim(w)
	state := submergedState()
	state.Pos = mgl32.Vec3{0.5, 0.1, 0.5}

	standing := sim.ObserveHeadLiquid(state)
	if !standing.Known || standing.Water {
		t.Fatalf("standing observation = %+v, want known and breathable", standing)
	}

	state.Swimming = true
	state.SwimWaterGraceTicks = 1
	swimming := sim.ObserveHeadLiquid(state)
	if !swimming.Known || !swimming.Water {
		t.Fatalf("swimming observation = %+v, want head below partial water surface", swimming)
	}
}

func TestSimulator_ObserveHeadLiquidReadsSecondLayerAndFailsClosed(t *testing.T) {
	t.Parallel()

	pos := cube.Pos{0, 0, 0}
	w := newLayeredLiquidWorld().waterlog(pos, block.Stairs{}, waterSource)
	sim := newLiquidSim(w)
	sim.Liquids = w
	state := submergedState()
	state.Swimming = true
	state.SwimWaterGraceTicks = 1
	state.Pos = mgl32.Vec3{0.5, 0.1, 0.5}
	if got := sim.ObserveHeadLiquid(state); !got.Known || !got.Water {
		t.Fatalf("waterlogged observation = %+v, want known water", got)
	}

	missing := newLiquidSim(newLiquidWorld())
	missing.Options.RequireLiquidLayer = true
	if got := missing.ObserveHeadLiquid(state); got.Known {
		t.Fatalf("missing required liquid layer observation = %+v, want unknown", got)
	}

	unloadedWorld := newLiquidWorld()
	unloadedWorld.chunkLoaded = false
	if got := newLiquidSim(unloadedWorld).ObserveHeadLiquid(state); got.Known {
		t.Fatalf("unloaded observation = %+v, want unknown", got)
	}
}

func TestSimulator_ReplayPreservesTickContinuityAndSnapshots(t *testing.T) {
	t.Parallel()

	sim := newLiquidSim(filledColumn(waterSource))
	initial := *submergedState()
	initial.HasGravity = true
	initial.MovementSpeed = 0.1
	initial.DefaultMovementSpeed = 0.1
	inputs := []InputState{
		{MoveVector: mgl32.Vec2{0, 1}, ClientPos: initial.Pos},
		{MoveVector: mgl32.Vec2{0, 1}, ClientPos: initial.Pos},
	}

	replay := sim.Replay(initial, inputs)
	if len(replay.Frames) != len(inputs) {
		t.Fatalf("frames = %d, want %d", len(replay.Frames), len(inputs))
	}
	if replay.Frames[1].State.Pos == replay.Frames[0].State.Pos {
		t.Fatalf("second tick did not advance continuous state: frames=%+v", replay.Frames)
	}
	if replay.State.Pos != replay.Frames[1].State.Pos {
		t.Fatalf("final state position = %v, want last frame %v", replay.State.Pos, replay.Frames[1].State.Pos)
	}
	if initial.Pos != (mgl32.Vec3{0.5, 0.5, 0.5}) {
		t.Fatalf("Replay mutated input state position: %v", initial.Pos)
	}
	for i, frame := range replay.Frames {
		if frame.Result.Outcome != SimulationOutcomeNormal {
			t.Fatalf("frame %d outcome = %v, want normal", i, frame.Result.Outcome)
		}
		if !frame.HeadLiquid.Known || !frame.HeadLiquid.Water {
			t.Fatalf("frame %d liquid = %+v, want known submerged water", i, frame.HeadLiquid)
		}
	}

	support := cube.Pos{1, 2, 3}
	initial.SupportingBlockPos = &support
	replay = sim.Replay(initial, []InputState{{ClientPos: initial.Pos}, {ClientPos: initial.Pos}})
	if replay.Frames[0].State.SupportingBlockPos != nil && replay.Frames[1].State.SupportingBlockPos != nil &&
		replay.Frames[0].State.SupportingBlockPos == replay.Frames[1].State.SupportingBlockPos {
		t.Fatal("replay frame snapshots alias SupportingBlockPos")
	}
}
