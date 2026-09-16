package bedsim

import (
	"testing"

	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/go-gl/mathgl/mgl32"
)

func TestHorizontalFlightPosePassesUnderCeiling(t *testing.T) {
	for _, spin := range []bool{false, true} {
		state := newBaseState()
		state.Gliding = !spin
		if spin {
			state.RiptideTicks = 10
		}
		state.Vel = mgl32.Vec3{0, 0, 1}
		sim := Simulator{World: staticWorld{chunkLoaded: true, boxes: []cube.BBox32{
			cube.Box32(-1, .75, .8, 1, 2, 3),
		}}}
		if !sim.tryCollisions(state) || state.CollideZ || state.Pos.Z() != 1 {
			t.Fatalf("spin=%v: compact flight blocked by overhead block: pos=%v collision=%v", spin, state.Pos, state.CollideZ)
		}
		for _, bb := range []cube.BBox32{state.BoundingBox(false), state.ClientBoundingBox(false)} {
			if !approxEqual(bb.Max().Y()-bb.Min().Y(), .6) {
				t.Fatalf("spin=%v: box=%v", spin, bb)
			}
		}
		state.Size[2] = 2
		if !approxEqual(state.BoundingBox(false).Max().Y()-state.BoundingBox(false).Min().Y(), 1.2) {
			t.Fatal("compact flight height ignored scale")
		}
	}
}

func TestHorizontalFlightExitFitsCeiling(t *testing.T) {
	for _, exit := range []string{"stop glide", "invalid glide", "stop spin", "spin expires", "spin collision"} {
		for _, ceiling := range []struct {
			name         string
			height, want float32
		}{
			{"standing", 3, 1.8}, {"sneaking", 1.6, 1.49}, {"crawling", .75, .6},
		} {
			t.Run(exit+"/"+ceiling.name, func(t *testing.T) {
				state := newBaseState()
				state.HasGravity = false
				sim := Simulator{World: staticWorld{chunkLoaded: true, boxes: []cube.BBox32{
					cube.Box32(-2, ceiling.height, -2, 2, 4, 2),
				}}}
				var result SimulationResult
				switch exit {
				case "stop glide":
					state.Gliding = true
					result = sim.Simulate(state, InputState{StopGliding: true})
				case "invalid glide":
					state.Gliding = true
					result = sim.SimulateState(state)
				case "stop spin":
					state.RiptideTicks, state.RiptideCollision = 10, true
					result = sim.Simulate(state, InputState{StopSpinAttack: true})
				case "spin expires":
					state.RiptideTicks = 1
					result = sim.Simulate(state, InputState{})
				case "spin collision":
					state.ensurePoseHeights()
					state.RiptideTicks, state.CollideX = 10, true
					if !sim.stopRiptideOnBlockCollision(state) {
						t.Fatal("unknown collision")
					}
					result.Outcome = SimulationOutcomeNormal
				}
				if result.Outcome != SimulationOutcomeNormal {
					t.Fatalf("outcome=%v", result.Outcome)
				}
				bb := state.BoundingBox(false)
				if state.Gliding || state.RiptideTicks != 0 || !approxEqual(bb.Max().Y()-bb.Min().Y(), ceiling.want) {
					t.Fatalf("exit pose: glide=%v spin=%d box=%v, want height %g", state.Gliding, state.RiptideTicks, bb, ceiling.want)
				}
				if state.Crawling != (ceiling.name == "crawling") || state.Sneaking != (ceiling.name == "sneaking") {
					t.Fatalf("wrong pose flags: crawling=%v sneaking=%v", state.Crawling, state.Sneaking)
				}
			})
		}
	}
}

func TestHorizontalFlightExitWaitsForLoadedWorld(t *testing.T) {
	for _, spin := range []bool{false, true} {
		state := newBaseState()
		state.Gliding = !spin
		input := InputState{StopGliding: !spin, StopSpinAttack: spin}
		if spin {
			state.RiptideTicks, state.RiptideCollision = 10, true
		}
		sim := Simulator{World: staticWorld{chunkLoaded: false}}
		result := sim.Simulate(state, input)
		if result.Outcome != SimulationOutcomeUnloadedChunk || state.Gliding != !spin || spin && state.RiptideTicks != 10 {
			t.Fatalf("spin=%v: unknown world changed flight pose: outcome=%v glide=%v ticks=%d", spin, result.Outcome, state.Gliding, state.RiptideTicks)
		}
	}
}
