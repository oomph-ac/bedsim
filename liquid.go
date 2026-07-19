package bedsim

import (
	"math"

	"github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/go-gl/mathgl/mgl64"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

var liquidFaces = [...]struct {
	delta cube.Pos
	vec   mgl64.Vec3
}{
	{cube.Pos{-1, 0, 0}, mgl64.Vec3{-1, 0, 0}},
	{cube.Pos{1, 0, 0}, mgl64.Vec3{1, 0, 0}},
	{cube.Pos{0, 0, -1}, mgl64.Vec3{0, 0, -1}},
	{cube.Pos{0, 0, 1}, mgl64.Vec3{0, 0, 1}},
}

func (s *Simulator) simulateLiquidTravel(state *MovementState, water bool) {
	initialY := state.Pos.Y()
	if water {
		if state.WantDown || state.WantDownSlow {
			vel := state.Vel
			vel[1] -= 0.04
			state.SetVel(vel)
		}
		s.updateSwimTravel(state)
	}

	if state.EffectiveJumping {
		vel := state.Vel
		if state.SwimAmount > 0 && state.SwimAmount < 1 {
			vel[1] = 0
		} else {
			vel[1] += 0.04
		}
		state.SetVel(vel)
	}

	moveRelativeSpeed := state.LavaMovementSpeed
	if moveRelativeSpeed == 0 {
		moveRelativeSpeed = DefaultLavaMovementSpeed
	}
	depthStriderLevel := 0.0
	swimSpeedMultiplier := DefaultSwimSpeedMultiplier
	if water {
		moveRelativeSpeed = state.UnderwaterMovementSpeed
		if moveRelativeSpeed == 0 {
			moveRelativeSpeed = DefaultUnderwaterMovementSpeed
		}
		if state.Swimming && state.SwimSpeedMultiplier != 0 {
			swimSpeedMultiplier = state.SwimSpeedMultiplier
		}
		if inventory, ok := s.Inventory.(DepthStriderProvider); ok {
			depthStriderLevel = math.Min(math.Max(float64(inventory.DepthStriderLevel()), 0), 3)
			if !state.OnGround {
				depthStriderLevel *= 0.5
			}
		}
		depthStriderFraction := depthStriderLevel / 3
		if swimSpeedMultiplier > 1 {
			moveRelativeSpeed *= (0.7 + depthStriderFraction*0.3) * swimSpeedMultiplier
		} else {
			moveRelativeSpeed += (state.MovementSpeed - moveRelativeSpeed) * depthStriderFraction
		}
	}

	moveRelative(state, moveRelativeSpeed)
	oldVel := state.Vel
	oldOnGround := state.OnGround
	s.tryCollisions(state, false)
	s.setPostCollisionMotion(state, oldVel, oldOnGround, block.Air{})
	state.SetMov(state.Vel)

	vel := state.Vel
	if water {
		drag := 0.8
		if state.Sprinting {
			drag = 0.9
		}
		if depthStriderLevel > 0 && swimSpeedMultiplier <= 1 {
			drag += (0.54600006 - drag) * (depthStriderLevel / 3)
		}
		vel[0] *= drag
		vel[1] *= 0.8
		vel[2] *= drag
	} else {
		vel = vel.Mul(0.5)
	}

	if s.Effects != nil {
		if amplifier, ok := s.Effects.GetEffect(packet.EffectLevitation); ok {
			target := LevitationGravityMultiplier * float64(amplifier+1)
			vel[1] += (target - vel[1]) * 0.2
		} else if state.HasGravity {
			vel[1] -= liquidGravity(state.Swimming, water)
		}
	} else if state.HasGravity {
		vel[1] -= liquidGravity(state.Swimming, water)
	}

	if state.CollideX || state.CollideZ {
		raised := mgl64.Vec3{vel.X(), vel.Y() + 0.6 + initialY - state.Pos.Y(), vel.Z()}
		raisedBox := state.BoundingBox(s.Options.UseSlideOffset).Translate(raised)
		hasCollision := hasNearbyBBoxes(s.World, raisedBox)
		hasLiquid := s.containsAnyLiquid(raisedBox)
		s.debugf("liquid exit probe collision=%t liquid=%t box=%v", hasCollision, hasLiquid, raisedBox)
		if !hasCollision && !hasLiquid {
			vel[1] = 0.3
		}
	}
	state.SetVel(vel)
	state.FallDistance = 0
}

func liquidGravity(swimming, water bool) float64 {
	if !water {
		return 0.02
	}
	if swimming {
		return 0
	}
	return 0.005
}

func (s *Simulator) updateSwimTravel(state *MovementState) {
	if !state.Swimming || state.EffectiveJumping {
		return
	}
	targetY := -MCSin(state.Rotation.X() * math.Pi / 180)
	rate := 0.06
	if targetY < -0.2 {
		rate = 0.085
	}

	if targetY > 0 && !state.WantDownSlow {
		belowPos := cube.PosFromVec3(state.Pos.Add(mgl64.Vec3{0, DefaultPlayerHeightOffset - 1.1}))
		if _, belowAir := s.liquidMovementBlock(belowPos).(block.Air); belowAir {
			liquidPos := cube.PosFromVec3(state.Pos.Add(mgl64.Vec3{0, DefaultPlayerHeightOffset - 1.2}))
			if _, liquid := s.liquidAt(liquidPos); !liquid {
				vel := state.Vel
				vel[1] = 0
				state.SetVel(vel)
				return
			}
		}
	}
	vel := state.Vel
	vel[1] += (targetY - vel[1]) * rate
	state.SetVel(vel)
}

func (s *Simulator) touchingLiquidBlocks(state *MovementState, liquidType string) []cube.Pos {
	box := state.BoundingBox(s.Options.UseSlideOffset).GrowVec3(mgl64.Vec3{1e-4, 0, 1e-4})
	offset := mgl64.Vec3{0.001, 0.401, 0.001}
	if liquidType == "lava" {
		offset = mgl64.Vec3{0.1, 0.4, 0.1}
	}
	box = shrinkLiquidBox(box, offset)

	min, max := box.Min(), box.Max()
	minX, minY, minZ := int(math.Floor(min.X())), int(math.Floor(min.Y())), int(math.Floor(min.Z()))
	maxX, maxY, maxZ := int(math.Floor(max.X()+1)), int(math.Floor(max.Y()+1)), int(math.Floor(max.Z()+1))
	positions := make([]cube.Pos, 0, 4)
	for x := minX; x < maxX; x++ {
		for y := minY; y < maxY; y++ {
			for z := minZ; z < maxZ; z++ {
				pos := cube.Pos{x, y, z}
				liquid, ok := s.liquidAt(pos)
				if !ok || liquid.LiquidType() != liquidType {
					continue
				}
				if s.Options.Debugf != nil {
					height := liquidHeight(liquid)
					surface := float64(pos[1]) + height
					s.debugf(
						"liquid block type=%s pos=%v depth=%d falling=%t height=%.6f surface=%.6f boxY=[%.6f %.6f] immersion=%.6f",
						liquid.LiquidType(), pos, liquid.LiquidDepth(), liquid.LiquidFalling(), height, surface,
						box.Min().Y(), box.Max().Y(), surface-box.Min().Y(),
					)
				}
				positions = append(positions, pos)
			}
		}
	}
	return positions
}

func shrinkLiquidBox(box cube.BBox, offset mgl64.Vec3) cube.BBox {
	min, max := box.Min().Add(offset), box.Max().Sub(offset)
	originalMin, originalMax := box.Min(), box.Max()
	for axis := range 3 {
		if min[axis] > max[axis] {
			mid := (originalMin[axis] + originalMax[axis]) * 0.5
			min[axis], max[axis] = mid, mid
		}
	}
	return cube.Box(min.X(), min.Y(), min.Z(), max.X(), max.Y(), max.Z())
}

func (s *Simulator) liquidMovementBlock(pos cube.Pos) world.Block {
	if liquid, ok := s.liquidAt(pos); ok {
		return liquid
	}
	return s.blockAtPos(pos)
}

func (s *Simulator) liquidAt(pos cube.Pos) (world.Liquid, bool) {
	if provider, ok := s.World.(LiquidProvider); ok {
		if liquid, found := provider.Liquid(pos); found {
			return liquid, true
		}
	}
	liquid, ok := s.blockAtPos(pos).(world.Liquid)
	return liquid, ok
}

func liquidHeight(liquid world.Liquid) float64 {
	if liquid.LiquidFalling() {
		return 1
	}
	return float64(liquid.LiquidDepth()+1) / 9
}

func (s *Simulator) containsAnyLiquid(box cube.BBox) bool {
	min, max := box.Min(), box.Max()
	minX, minY, minZ := int(math.Floor(min.X())), int(math.Floor(min.Y())), int(math.Floor(min.Z()))
	maxX, maxY, maxZ := int(math.Ceil(max.X())), int(math.Ceil(max.Y())), int(math.Ceil(max.Z()))
	for x := minX; x < maxX; x++ {
		for z := minZ; z < maxZ; z++ {
			for y := minY; y < maxY; y++ {
				if _, ok := s.liquidAt(cube.Pos{x, y, z}); ok {
					return true
				}
			}
		}
	}
	return false
}

func (s *Simulator) applyLiquidFlow(state *MovementState, positions []cube.Pos, liquidType string) {
	flow := mgl64.Vec3{}
	for _, pos := range positions {
		liquid, ok := s.liquidAt(pos)
		if !ok || liquid.LiquidType() != liquidType {
			continue
		}
		flow = flow.Add(s.liquidFlow(pos, liquid))
	}
	if length := flow.Len(); length >= 1e-4 {
		strength := 0.014
		if liquidType == "lava" {
			strength = 0.007
		}
		state.SetVel(state.Vel.Add(flow.Mul(strength / length)))
		s.debugf("%s flow applied strength=%.6f flow=%v vel=%v", liquidType, strength, flow, state.Vel)
	}
}

func (s *Simulator) liquidFlow(pos cube.Pos, liquid world.Liquid) mgl64.Vec3 {
	currentDecay := liquidDecay(liquid)
	flow := mgl64.Vec3{}
	for _, face := range liquidFaces {
		neighbourPos := pos.Add(face.delta)
		if neighbour, ok := s.liquidAt(neighbourPos); ok {
			if neighbour.LiquidType() == liquid.LiquidType() {
				if !s.liquidFlowSideClosed(pos, neighbourPos) && !s.liquidFlowSideClosed(neighbourPos, pos) {
					flow = flow.Add(face.vec.Mul(float64(liquidDecay(neighbour) - currentDecay)))
				}
				continue
			}
		}
		if len(s.World.BlockCollisions(neighbourPos)) != 0 {
			continue
		}
		below := neighbourPos.Side(cube.FaceDown)
		if lower, ok := s.liquidAt(below); ok && lower.LiquidType() == liquid.LiquidType() {
			flow = flow.Add(face.vec.Mul(float64(liquidDecay(lower) - currentDecay + 8)))
		}
	}
	if liquid.LiquidFalling() {
		for _, face := range liquidFaces {
			neighbourPos := pos.Add(face.delta)
			aboveNeighbour := neighbourPos.Side(cube.FaceUp)
			if len(s.World.BlockCollisions(neighbourPos)) != 0 || len(s.World.BlockCollisions(aboveNeighbour)) != 0 {
				if length := flow.Len(); length > 1e-4 {
					flow = flow.Mul(1 / length)
				}
				flow[1] -= 6
				break
			}
		}
	}
	if length := flow.Len(); length > 1e-4 {
		return flow.Mul(1 / length)
	}
	return mgl64.Vec3{}
}

func (s *Simulator) liquidFlowSideClosed(pos, side cube.Pos) bool {
	stairs, ok := s.blockAtPos(pos).(block.Stairs)
	return ok && stairs.Model().FaceSolid(pos, pos.Face(side), s.World)
}

func liquidDecay(liquid world.Liquid) int {
	if liquid.LiquidFalling() {
		return 0
	}
	return 8 - liquid.LiquidDepth()
}
