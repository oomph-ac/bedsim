package bedsim

import (
	"github.com/chewxy/math32"

	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/go-gl/mathgl/mgl32"
)

// BubbleColumnDirection is the direction a bubble column accelerates entities.
type BubbleColumnDirection uint8

const (
	BubbleColumnUp BubbleColumnDirection = iota
	BubbleColumnDown
)

// BubbleColumnProvider exposes bubble-column direction separately from liquid
// state so worlds without a concrete bubble-column block type can participate.
type BubbleColumnProvider interface {
	BubbleColumn(pos cube.Pos) (BubbleColumnDirection, bool)
}

func applyBubbleColumn(state *MovementState, direction BubbleColumnDirection, surface bool) {
	velocity := state.Vel
	switch direction {
	case BubbleColumnDown:
		cap := float32(-0.3)
		if surface {
			cap = -0.9
		}
		velocity[1] = math32.Max(cap, velocity[1]-0.03)
	default:
		change, cap := float32(0.06), float32(0.7)
		if surface {
			change, cap = 0.1, 1.8
		}
		velocity[1] = math32.Min(cap, velocity[1]+change)
	}
	state.SetVel(velocity)
}

// applyBubbleColumns applies at most one column impulse per tick. An entity
// carries a single resolved column contact regardless of how many column cells
// its hitbox overlaps, and the topmost overlapped cell decides the contact:
// only that cell can have the open air above it that selects the surface form.
func (s *Simulator) applyBubbleColumns(state *MovementState) {
	provider, ok := s.World.(BubbleColumnProvider)
	if !ok {
		return
	}
	bb := state.BoundingBox(s.Options.UseSlideOffset)
	min, max := bb.Min(), bb.Max()
	contact, found := cube.Pos{}, false
	var direction BubbleColumnDirection
	for x := int(math32.Floor(min.X())); x < int(math32.Ceil(max.X())); x++ {
		for y := int(math32.Floor(min.Y())); y < int(math32.Ceil(max.Y())); y++ {
			for z := int(math32.Floor(min.Z())); z < int(math32.Ceil(max.Z())); z++ {
				pos := cube.Pos{x, y, z}
				cellDirection, ok := provider.BubbleColumn(pos)
				if !ok || (found && pos.Y() <= contact.Y()) {
					continue
				}
				contact, direction, found = pos, cellDirection, true
			}
		}
	}
	if !found {
		return
	}
	above := contact.Side(cube.FaceUp)
	_, liquidAbove := s.liquidAt(above)
	applyBubbleColumn(state, direction, !liquidAbove && s.blockAir(s.blockAtPos(above)))
	state.FallDistance = 0
}

func (s *Simulator) attemptRiptide(state *MovementState, touchingWater bool) bool {
	if s.Equipment == nil || state.InVehicle || state.RiptideTicks > 0 || !state.RiptideReady || (!touchingWater && !state.RiptideInRain) {
		return false
	}
	level := s.Equipment.EnchantmentLevel(EnchantmentRiptide)
	if level <= 0 || !state.StartingSpinAttack {
		return false
	}
	force := 1.5 + 0.75*float32(level-1)
	pitch := state.Rotation.X() * math32.Pi / 180
	yaw := state.Rotation.Z() * math32.Pi / 180
	direction := mgl32.Vec3{-MCSin(yaw) * MCCos(pitch), -MCSin(pitch), MCCos(yaw) * MCCos(pitch)}
	if length := direction.Len(); length > 0 {
		direction = direction.Mul(force / length)
	}
	state.SetVel(state.Vel.Add(direction))
	state.RiptideTicks = 20
	state.RiptideCollision = false
	state.StartingSpinAttack = false
	return true
}

func stopRiptideOnBlockCollision(state *MovementState) {
	if state.RiptideTicks > 0 && (state.CollideX || state.CollideZ) {
		state.RiptideTicks = 0
		state.RiptideCollision = false
	}
}
