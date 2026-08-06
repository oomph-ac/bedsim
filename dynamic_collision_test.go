package bedsim

import (
	"github.com/chewxy/math32"
	"testing"

	"github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/go-gl/mathgl/mgl32"
)

type dynamicCollisionWorld struct {
	environmentWorld
	lastContext MovementCollisionContext
}

func (w *dynamicCollisionWorld) GetMovementBBoxes(_ cube.BBox32, context MovementCollisionContext) []cube.BBox32 {
	w.lastContext = context
	if context.LeatherBoots && !context.Descending && !context.WantDown {
		return []cube.BBox32{cube.Box32(0, 0, 0, 1, 1, 1)}
	}
	return nil
}

func TestMovementCollisionProviderReceivesPlayerDependentContext(t *testing.T) {
	w := &dynamicCollisionWorld{}
	sim := &Simulator{World: w, Equipment: leatherEquipment{}}
	state := newBaseState()
	state.Sneaking = true
	state.PressingDescend = false

	boxes := sim.nearbyBBoxes(state, state.BoundingBox(false))

	if len(boxes) != 1 {
		t.Fatalf("expected dynamic powder-snow collision, got %d boxes", len(boxes))
	}
	if !w.lastContext.LeatherBoots || !w.lastContext.Sneaking {
		t.Fatalf("expected equipment and sneak context, got %+v", w.lastContext)
	}
}

type leatherEquipment struct{}

func (leatherEquipment) EnchantmentLevel(MovementEnchantment) int { return 0 }
func (leatherEquipment) WearingLeatherBoots() bool                { return true }

func TestCannotUnsneakUnderLowCeiling(t *testing.T) {
	sim := &Simulator{World: staticWorld{chunkLoaded: true, boxes: []cube.BBox32{
		cube.Box32(-1, 1.5, -1, 1, 2, 1),
	}}}
	state := newBaseState()
	state.Sneaking = true
	state.Size[1] = 1.49

	sim.applyInput(state, InputState{StopSneaking: true})

	if !state.Sneaking || state.Size.Y() != 1.49 {
		t.Fatalf("expected forced sneak pose under ceiling, got sneaking=%v size=%v", state.Sneaking, state.Size)
	}
}

func TestCannotStopCrawlingUnderLowCeiling(t *testing.T) {
	sim := &Simulator{World: staticWorld{chunkLoaded: true, boxes: []cube.BBox32{
		cube.Box32(-1, 0.7, -1, 1, 2, 1),
	}}}
	state := newBaseState()
	state.Crawling = true
	state.Size[1] = 0.6

	sim.applyInput(state, InputState{StopCrawling: true})

	if !state.Crawling || state.Size.Y() != 0.6 {
		t.Fatalf("expected forced crawl pose under ceiling, got crawling=%v size=%v", state.Crawling, state.Size)
	}
}

func TestPoseRestoresCustomStandingHeight(t *testing.T) {
	state := newBaseState()
	state.Size[1] = 2
	sim := &Simulator{}

	sim.applyInput(state, InputState{StartSneaking: true})
	sim.applyInput(state, InputState{StopSneaking: true})

	if state.Size.Y() != 2 {
		t.Fatalf("expected custom standing height 2 to be restored, got %v", state.Size.Y())
	}
}

func TestSneakingInWaterDescends(t *testing.T) {
	w := environmentWorld{blocks: map[cube.Pos]world.Block{{0, 0, 0}: block.Water{Still: true, Depth: 8}}}
	sim := &Simulator{World: w}
	state := newBaseState()
	state.Pos = mgl32.Vec3{0.5, 0, 0.5}
	state.Gravity = NormalGravity

	sim.Simulate(state, InputState{SneakDown: true, Sneaking: true})

	if want := float32(-0.037); math32.Abs(state.Vel.Y()-want) > 1e-6 {
		t.Fatalf("expected water descent velocity %v, got %v", want, state.Vel.Y())
	}
}
