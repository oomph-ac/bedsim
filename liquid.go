package bedsim

import (
	"github.com/chewxy/math32"

	"github.com/df-mc/dragonfly/server/block"
	dfcube "github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/ethaniccc/float32-cube/cube"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

// Liquid movement follows oomph PR #145 at 0bcbb8b, with provider-based liquid
// lookup and legacy impulse clamps. It also requires
// recent server-observed water contact before trusting the client swim flag.
// See README.md for complete compatibility and security notes.

// liquidKind identifies the liquid family being simulated.
type liquidKind uint8

const (
	liquidWater liquidKind = iota
	liquidLava
)

func (k liquidKind) typeName() string {
	if k == liquidLava {
		return "lava"
	}
	return "water"
}

func (k liquidKind) matches(liquid world.Liquid) bool {
	return liquid.LiquidType() == k.typeName()
}

var liquidFaces = [...]struct {
	delta dfcube.Pos
	vec   mgl32.Vec3
}{
	{dfcube.Pos{-1, 0, 0}, mgl32.Vec3{-1, 0, 0}},
	{dfcube.Pos{1, 0, 0}, mgl32.Vec3{1, 0, 0}},
	{dfcube.Pos{0, 0, -1}, mgl32.Vec3{0, 0, -1}},
	{dfcube.Pos{0, 0, 1}, mgl32.Vec3{0, 0, 1}},
}

func (s *Simulator) simulateLiquidTravel(state *MovementState, kind liquidKind, touchingLiquid bool) {
	initialY := state.Pos.Y()
	water := kind == liquidWater
	// Captured before updateSwimTravel, matching the upstream ordering.
	jumping := state.EffectiveJumping
	if water {
		if state.WantDown || state.WantDownSlow || state.PressingDescend {
			vel := state.Vel
			vel[1] -= 0.04
			state.SetVel(vel)
		}
		s.updateSwimTravel(state)
	}

	if jumping {
		vel := state.Vel
		if state.SwimAmount > 0 && state.SwimAmount < 1 || water && state.Swimming && !touchingLiquid {
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
	depthStriderLevel := float32(0)
	swimSpeedMultiplier := float32(DefaultSwimSpeedMultiplier)
	if water {
		moveRelativeSpeed = state.UnderwaterMovementSpeed
		if moveRelativeSpeed == 0 {
			moveRelativeSpeed = DefaultUnderwaterMovementSpeed
		}
		if state.Swimming && state.SwimSpeedMultiplier != 0 {
			swimSpeedMultiplier = state.SwimSpeedMultiplier
		}
		if s.Equipment != nil {
			depthStriderLevel = math32.Min(math32.Max(float32(s.Equipment.EnchantmentLevel(EnchantmentDepthStrider)), 0), 3)
		}
		if depthStriderLevel == 0 {
			if inventory, ok := s.Inventory.(DepthStriderProvider); ok {
				depthStriderLevel = math32.Min(math32.Max(float32(inventory.DepthStriderLevel()), 0), 3)
			}
		}
		if !state.OnGround {
			depthStriderLevel *= 0.5
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
		drag := float32(0.8)
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
			target := LevitationGravityMultiplier * float32(amplifier+1)
			vel[1] += (target - vel[1]) * 0.2
		} else if state.HasGravity {
			vel[1] -= liquidGravity(state.Swimming, water)
		}
	} else if state.HasGravity {
		vel[1] -= liquidGravity(state.Swimming, water)
	}

	if state.CollideX || state.CollideZ {
		raised := mgl32.Vec3{vel.X(), vel.Y() + 0.6 + initialY - state.Pos.Y(), vel.Z()}
		raisedBox := state.BoundingBox(s.Options.UseSlideOffset).Translate(raised)
		hasCollision := len(s.nearbyBBoxes(state, raisedBox)) > 0
		hasLiquid := s.containsAnyLiquid(raisedBox)
		s.debugf("liquid exit probe collision=%t liquid=%t box=%v", hasCollision, hasLiquid, raisedBox)
		if !hasCollision && !hasLiquid {
			vel[1] = 0.3
		}
		state.RiptideTicks = 0
	}
	state.SetVel(vel)
	s.applyBubbleColumns(state)
	s.applyInsideBlockEffects(state)
	state.FallDistance = 0
}

func liquidGravity(swimming, water bool) float32 {
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
	targetY := -MCSin(state.Rotation.X() * math32.Pi / 180)
	rate := float32(0.06)
	if targetY < -0.2 {
		rate = 0.085
	}

	if targetY > 0 && !state.WantDownSlow {
		belowPos := posFromVec3(state.Pos.Add(mgl32.Vec3{0, DefaultPlayerHeightOffset - 1.1}))
		if _, belowAir := s.liquidMovementBlock(belowPos).(block.Air); belowAir {
			liquidPos := posFromVec3(state.Pos.Add(mgl32.Vec3{0, DefaultPlayerHeightOffset - 1.2}))
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

func (s *Simulator) touchingLiquidBlocks(state *MovementState, kind liquidKind) []dfcube.Pos {
	box := state.BoundingBox(s.Options.UseSlideOffset).GrowVec3(mgl32.Vec3{1e-4, 0, 1e-4})
	offset := mgl32.Vec3{0.001, 0.401, 0.001}
	if kind == liquidLava {
		offset = mgl32.Vec3{0.1, 0.4, 0.1}
	}
	box = shrinkLiquidBox(box, offset)

	min, max := box.Min(), box.Max()
	minX, minY, minZ := int(math32.Floor(min.X())), int(math32.Floor(min.Y())), int(math32.Floor(min.Z()))
	maxX, maxY, maxZ := int(math32.Floor(max.X()+1)), int(math32.Floor(max.Y()+1)), int(math32.Floor(max.Z()+1))
	positions := make([]dfcube.Pos, 0, 4)
	for x := minX; x < maxX; x++ {
		for y := minY; y < maxY; y++ {
			for z := minZ; z < maxZ; z++ {
				pos := dfcube.Pos{x, y, z}
				liquid, ok := s.liquidAt(pos)
				if !ok || !kind.matches(liquid) {
					continue
				}
				if s.Options.Debugf != nil {
					height := liquidHeight(liquid)
					surface := float32(pos[1]) + height
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

func shrinkLiquidBox(box cube.BBox, offset mgl32.Vec3) cube.BBox {
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

func (s *Simulator) liquidMovementBlock(pos dfcube.Pos) world.Block {
	if liquid, ok := s.liquidAt(pos); ok {
		return liquid
	}
	return s.blockAtPos(pos)
}

// blockCollisions returns the collision boxes at pos, treating an absent world
// as empty space so liquid flow never dereferences a nil provider.
func (s *Simulator) blockCollisions(pos dfcube.Pos) []cube.BBox {
	if s.World == nil {
		return nil
	}
	return s.World.BlockCollisions(pos)
}

// liquidLayer resolves the configured second-layer liquid source. The explicit
// Simulator.Liquids field wins; a World that itself implements LiquidProvider is
// accepted for compatibility with integrations written before the field existed.
func (s *Simulator) liquidLayer() (LiquidProvider, bool) {
	if s.Liquids != nil {
		return s.Liquids, true
	}
	if provider, ok := s.World.(LiquidProvider); ok {
		return provider, true
	}
	return nil, false
}

// HasLiquidLayer reports whether this simulator can see liquids in the second
// block layer, which is what makes waterlogged blocks visible to movement.
// Callers running authoritatively should assert this at startup, or set
// SimulationOptions.RequireLiquidLayer to fail closed instead.
func (s *Simulator) HasLiquidLayer() bool {
	_, ok := s.liquidLayer()
	return ok
}

func (s *Simulator) liquidAt(pos dfcube.Pos) (world.Liquid, bool) {
	if provider, ok := s.liquidLayer(); ok {
		if liquid, found := provider.Liquid(pos); found {
			return liquid, true
		}
	}
	liquid, ok := s.blockAtPos(pos).(world.Liquid)
	return liquid, ok
}

func liquidHeight(liquid world.Liquid) float32 {
	if liquid.LiquidFalling() {
		return 1
	}
	return float32(liquid.LiquidDepth()+1) / 9
}

func (s *Simulator) containsAnyLiquid(box cube.BBox) bool {
	min, max := box.Min(), box.Max()
	minX, minY, minZ := int(math32.Floor(min.X())), int(math32.Floor(min.Y())), int(math32.Floor(min.Z()))
	maxX, maxY, maxZ := int(math32.Ceil(max.X())), int(math32.Ceil(max.Y())), int(math32.Ceil(max.Z()))
	for x := minX; x < maxX; x++ {
		for z := minZ; z < maxZ; z++ {
			for y := minY; y < maxY; y++ {
				if _, ok := s.liquidAt(dfcube.Pos{x, y, z}); ok {
					return true
				}
			}
		}
	}
	return false
}

func (s *Simulator) applyLiquidFlow(state *MovementState, positions []dfcube.Pos, kind liquidKind) {
	flow := mgl32.Vec3{}
	for _, pos := range positions {
		liquid, ok := s.liquidAt(pos)
		if !ok || !kind.matches(liquid) {
			continue
		}
		flow = flow.Add(s.liquidFlow(pos, liquid))
	}
	if length := flow.Len(); length >= 1e-4 {
		strength := float32(0.014)
		if kind == liquidLava {
			strength = 0.0035
		}
		state.SetVel(state.Vel.Add(flow.Mul(strength / length)))
		s.debugf("%s flow applied strength=%.6f flow=%v vel=%v", kind.typeName(), strength, flow, state.Vel)
	}
}

func (s *Simulator) liquidFlow(pos dfcube.Pos, liquid world.Liquid) mgl32.Vec3 {
	currentDecay := liquidDecay(liquid)
	flow := mgl32.Vec3{}
	for _, face := range liquidFaces {
		neighbourPos := pos.Add(face.delta)
		if neighbour, ok := s.liquidAt(neighbourPos); ok {
			if neighbour.LiquidType() == liquid.LiquidType() {
				if !s.liquidFlowSideClosed(pos, neighbourPos) && !s.liquidFlowSideClosed(neighbourPos, pos) {
					flow = flow.Add(face.vec.Mul(float32(liquidDecay(neighbour) - currentDecay)))
				}
				continue
			}
		}
		if len(s.blockCollisions(neighbourPos)) != 0 {
			continue
		}
		below := neighbourPos.Side(dfcube.FaceDown)
		if lower, ok := s.liquidAt(below); ok && lower.LiquidType() == liquid.LiquidType() {
			flow = flow.Add(face.vec.Mul(float32(liquidDecay(lower) - currentDecay + 8)))
		}
	}
	if liquid.LiquidFalling() {
		for _, face := range liquidFaces {
			neighbourPos := pos.Add(face.delta)
			aboveNeighbour := neighbourPos.Side(dfcube.FaceUp)
			if len(s.blockCollisions(neighbourPos)) != 0 || len(s.blockCollisions(aboveNeighbour)) != 0 {
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
	return mgl32.Vec3{}
}

func (s *Simulator) liquidFlowSideClosed(pos, side dfcube.Pos) bool {
	stairs, ok := s.blockAtPos(pos).(block.Stairs)
	return ok && stairs.Model().FaceSolid(pos, pos.Face(side), s.World)
}

func liquidDecay(liquid world.Liquid) int {
	if liquid.LiquidFalling() {
		return 0
	}
	return 8 - liquid.LiquidDepth()
}
