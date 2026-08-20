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

// BubbleColumnSurfaceProvider optionally supplies the exact client-side
// surface variant for a bubble-column cell. The bool reports whether the
// adapter knows the variant; false falls back to the block-above heuristic.
type BubbleColumnSurfaceProvider interface {
	BubbleColumnSurface(pos cube.Pos) (surface, known bool)
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

func (s *Simulator) applyBubbleColumns(state *MovementState) {
	provider, ok := s.World.(BubbleColumnProvider)
	if !ok {
		return
	}
	bb := state.BoundingBox(s.Options.UseSlideOffset)
	min, max := bb.Min(), bb.Max()
	found := false
	for x := int(math32.Floor(min.X())); x < int(math32.Ceil(max.X())); x++ {
		for y := int(math32.Floor(min.Y())); y < int(math32.Ceil(max.Y())); y++ {
			for z := int(math32.Floor(min.Z())); z < int(math32.Ceil(max.Z())); z++ {
				pos := cube.Pos{x, y, z}
				direction, ok := provider.BubbleColumn(pos)
				if !ok {
					continue
				}
				found = true
				surface, known := false, false
				if surfaceProvider, ok := s.World.(BubbleColumnSurfaceProvider); ok {
					surface, known = surfaceProvider.BubbleColumnSurface(pos)
				}
				if !known {
					above := pos.Side(cube.FaceUp)
					_, liquidAbove := s.liquidAt(above)
					surface = !liquidAbove && s.blockAir(s.blockAtPos(above))
				}
				applyBubbleColumn(state, direction, surface)
			}
		}
	}
	if !found {
		return
	}
	state.FallDistance = 0
}

// attemptRiptide applies a validated one-shot Riptide launch.
func (s *Simulator) attemptRiptide(state *MovementState, touchingWater, headInWater bool) bool {
	if s.Equipment == nil || state.InVehicle || state.RiptideTicks > 0 || !state.RiptideReady || (!touchingWater && !state.RiptideInRain) {
		return false
	}
	level := s.Equipment.EnchantmentLevel(EnchantmentRiptide)
	if level <= 0 || !state.StartingSpinAttack {
		return false
	}
	state.SetVel(state.Vel.Add(s.riptideImpulse(state, level, touchingWater, headInWater)))
	state.RiptideTicks = 20
	state.RiptideCollision = false
	state.StartingSpinAttack = false
	return true
}

// riptideImpulse returns the one-shot launch velocity for a spin attack. The
// grounded adjustment compensates for the drag and gravity the same tick will
// apply afterwards, so it is skipped entirely while airborne.
func (s *Simulator) riptideImpulse(state *MovementState, level int, wasInWater, headInWater bool) mgl32.Vec3 {
	force := 0.75 * float32(level+1)
	pitch := state.Rotation.X() * math32.Pi / 180
	yaw := state.Rotation.Z() * math32.Pi / 180
	direction := mgl32.Vec3{-MCSin(yaw) * MCCos(pitch), -MCSin(pitch), MCCos(yaw) * MCCos(pitch)}
	if length := direction.Len(); length > 0 {
		direction = direction.Mul(force / length)
	}
	if state.OnGround && state.HasGravity {
		if wasInWater && !headInWater {
			direction[1] = direction[1] / WaterDrag * NormalGravityMultiplier
		} else {
			direction[1] += NormalGravity
		}
	}
	return direction
}

// riptideHeadInWater reports whether the player's head is below the local
// water surface.
func (s *Simulator) riptideHeadInWater(state *MovementState) bool {
	heightOffset := DefaultPlayerHeightOffset
	if state.Sneaking {
		heightOffset = SneakingPlayerHeightOffset
	}
	position := state.Pos.Add(mgl32.Vec3{0, heightOffset, 0})
	pos := posFromVec3(position)
	liquid, ok := s.liquidAt(pos)
	return ok && liquidWater.matches(liquid) && position.Y() < float32(pos.Y())+liquidHeight(liquid)
}

func stopRiptideOnBlockCollision(state *MovementState) {
	if state.RiptideTicks > 0 && (state.CollideX || state.CollideZ) {
		state.RiptideTicks = 0
		state.RiptideCollision = false
	}
}
