package bedsim

import (
	"github.com/chewxy/math32"

	"github.com/df-mc/dragonfly/server/block"
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

func (s *Simulator) applyBubbleColumns(state *MovementState) {
	provider, ok := s.World.(BubbleColumnProvider)
	if !ok {
		return
	}
	bb := state.BoundingBox(s.Options.UseSlideOffset)
	min, max := bb.Min(), bb.Max()
	for x := int(math32.Floor(min.X())); x < int(math32.Ceil(max.X())); x++ {
		for y := int(math32.Floor(min.Y())); y < int(math32.Ceil(max.Y())); y++ {
			for z := int(math32.Floor(min.Z())); z < int(math32.Ceil(max.Z())); z++ {
				pos := cube.Pos{x, y, z}
				direction, found := provider.BubbleColumn(pos)
				if !found {
					continue
				}
				above := pos.Side(cube.FaceUp)
				_, liquidAbove := s.liquidAt(above)
				applyBubbleColumn(state, direction, !liquidAbove && s.blockAir(s.blockAtPos(above)))
			}
		}
	}
}

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

func (s *Simulator) riptideImpulse(state *MovementState, level int, wasInWater, headInWater bool) mgl32.Vec3 {
	force := 0.75 * float32(level+1)
	pitch := state.Rotation.X() * math32.Pi / 180
	yaw := state.Rotation.Z() * math32.Pi / 180
	direction := mgl32.Vec3{-MCSin(yaw) * MCCos(pitch), -MCSin(pitch), MCCos(yaw) * MCCos(pitch)}
	if length := direction.Len(); length > 0 {
		direction = direction.Mul(force / length)
	}
	if wasInWater {
		if headInWater {
			direction[1] = direction[1] / 0.8 * NormalGravityMultiplier
		} else {
			direction[1] += 0.08
		}
	}
	return direction
}

func (s *Simulator) riptideHeadInWater(state *MovementState) bool {
	position := state.Pos.Add(mgl32.Vec3{0, DefaultPlayerHeightOffset, 0})
	pos := posFromVec3(position)
	liquid, ok := s.liquidAt(pos)
	return ok && liquidWater.matches(liquid) && position.Y() < float32(pos.Y())+liquidHeight(liquid)
}

func (s *Simulator) simulateRiptide(state *MovementState, wasInWater, headInWater bool) {
	if s.Equipment != nil {
		if level := s.Equipment.EnchantmentLevel(EnchantmentRiptide); level > 0 {
			state.SetVel(state.Vel.Add(s.riptideImpulse(state, level, wasInWater, headInWater)))
		}
	}
	oldVel := state.Vel
	oldOnGround := state.OnGround
	oldY := state.Pos.Y()
	state.OnGround = false
	s.tryCollisions(state, false)
	stopRiptideOnBlockCollision(state)
	updateFallDistance(state, oldY)
	state.SetMov(state.Vel)
	s.setPostCollisionMotion(state, oldVel, oldOnGround, block.Air{})
	s.applyInsideBlockEffects(state)
	s.applyBubbleColumns(state)
}

func stopRiptideOnBlockCollision(state *MovementState) {
	if state.RiptideTicks > 0 && (state.CollideX || state.CollideZ) {
		state.RiptideTicks = 0
		state.RiptideCollision = false
	}
}
