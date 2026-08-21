package bedsim

import (
	"github.com/chewxy/math32"

	"github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
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
	delta cube.Pos
	vec   mgl32.Vec3
}{
	{cube.Pos{-1, 0, 0}, mgl32.Vec3{-1, 0, 0}},
	{cube.Pos{1, 0, 0}, mgl32.Vec3{1, 0, 0}},
	{cube.Pos{0, 0, -1}, mgl32.Vec3{0, 0, -1}},
	{cube.Pos{0, 0, 1}, mgl32.Vec3{0, 0, 1}},
}

func (s *Simulator) simulateLiquidTravel(state *MovementState, kind liquidKind, touchingLiquid bool) bool {
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
	stuckMovement := applyStuckSpeedMultiplier(state)
	if !s.movementSweepLoaded(state) {
		return false
	}
	oldVel := state.Vel
	oldOnGround := state.OnGround
	if !s.tryCollisions(state) {
		return false
	}
	stopRiptideOnBlockCollision(state)
	if stuckMovement {
		state.SetMov(state.Vel)
		state.SetVel(mgl32.Vec3{})
		oldVel = mgl32.Vec3{}
	}
	s.setPostCollisionMotion(state, oldVel, oldOnGround, block.Air{})
	if !stuckMovement {
		state.SetMov(state.Vel)
	}

	vel := state.Vel
	if water {
		drag := float32(0.8)
		if state.Sprinting || state.StoppedSwimmingThisTick {
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
		if !s.movementAreaLoaded(raisedBox) {
			return false
		}
		hasCollision := s.hasNearbyBBoxes(state, raisedBox)
		hasLiquid := s.containsAnyLiquid(raisedBox)
		if debugf := s.Options.Debugf; debugf != nil {
			debugf("liquid exit probe collision=%t liquid=%t box=%v", hasCollision, hasLiquid, raisedBox)
		}
		if !hasCollision && !hasLiquid {
			vel[1] = 0.3
		}
	}
	state.SetVel(vel)
	s.applyBubbleColumns(state)
	s.applyInsideBlockEffects(state)
	state.FallDistance = 0
	return true
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

	if targetY > 0 && !state.WantDownSlow && !state.PressingDescend {
		belowPos := posFromVec3(state.Pos.Add(mgl32.Vec3{0, DefaultPlayerHeightOffset - 1.1}))
		if s.blockAir(s.liquidMovementBlock(belowPos)) {
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

func (s *Simulator) touchingLiquidBlocks(state *MovementState, kind liquidKind) []cube.Pos {
	box := state.BoundingBox(s.Options.UseSlideOffset).GrowVec3(mgl32.Vec3{1e-4, 0, 1e-4})
	offset := mgl32.Vec3{0.001, 0.401, 0.001}
	if kind == liquidLava {
		offset = mgl32.Vec3{0.1, 0.4, 0.1}
	}
	box = shrinkLiquidBox(box, offset)

	min, max := box.Min(), box.Max()
	minX, minY, minZ := int(math32.Floor(min.X())), int(math32.Floor(min.Y())), int(math32.Floor(min.Z()))
	maxX, maxY, maxZ := int(math32.Floor(max.X()+1)), int(math32.Floor(max.Y()+1)), int(math32.Floor(max.Z()+1))
	positions := make([]cube.Pos, 0, 4)
	for x := minX; x < maxX; x++ {
		for y := minY; y < maxY; y++ {
			for z := minZ; z < maxZ; z++ {
				pos := cube.Pos{x, y, z}
				liquid, ok := s.liquidAt(pos)
				if !ok || !kind.matches(liquid) {
					continue
				}
				if !liquidIntersects(box, pos, liquid) {
					continue
				}
				if debugf := s.Options.Debugf; debugf != nil {
					height := liquidHeight(liquid)
					surface := float32(pos[1]) + height
					debugf(
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

func shrinkLiquidBox(box cube.BBox32, offset mgl32.Vec3) cube.BBox32 {
	min, max := box.Min().Add(offset), box.Max().Sub(offset)
	originalMin, originalMax := box.Min(), box.Max()
	for axis := range 3 {
		if min[axis] > max[axis] {
			mid := (originalMin[axis] + originalMax[axis]) * 0.5
			min[axis], max[axis] = mid, mid
		}
	}
	return cube.Box32(min.X(), min.Y(), min.Z(), max.X(), max.Y(), max.Z())
}

func (s *Simulator) liquidMovementBlock(pos cube.Pos) world.Block {
	if liquid, ok := s.liquidAt(pos); ok {
		return liquid
	}
	return s.blockAtPos(pos)
}

// blockCollisions returns the collision boxes at pos, treating an absent world
// as empty space so liquid flow never dereferences a nil provider.
func (s *Simulator) blockCollisions(pos cube.Pos) []cube.BBox32 {
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

func (s *Simulator) liquidAt(pos cube.Pos) (world.Liquid, bool) {
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

// liquidIntersects reports whether box reaches the liquid surface in pos.
func liquidIntersects(box cube.BBox32, pos cube.Pos, liquid world.Liquid) bool {
	surface := float32(pos[1]) + liquidHeight(liquid)
	return box.Max().Y() > float32(pos[1]) && box.Min().Y() < surface
}

func (s *Simulator) containsAnyLiquid(box cube.BBox32) bool {
	min, max := box.Min(), box.Max()
	minX, minY, minZ := int(math32.Floor(min.X())), int(math32.Floor(min.Y())), int(math32.Floor(min.Z()))
	maxX, maxY, maxZ := int(math32.Ceil(max.X())), int(math32.Ceil(max.Y())), int(math32.Ceil(max.Z()))
	for x := minX; x < maxX; x++ {
		for z := minZ; z < maxZ; z++ {
			for y := minY; y < maxY; y++ {
				pos := cube.Pos{x, y, z}
				if liquid, ok := s.liquidAt(pos); ok && liquidIntersects(box, pos, liquid) {
					return true
				}
			}
		}
	}
	return false
}

func (s *Simulator) applyLiquidFlow(state *MovementState, positions []cube.Pos, kind liquidKind) bool {
	if len(positions) == 0 {
		return true
	}
	if !s.movementAreaLoaded(state.BoundingBox(s.Options.UseSlideOffset).Grow(1)) {
		return false
	}
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
		if debugf := s.Options.Debugf; debugf != nil {
			debugf("%s flow applied strength=%.6f flow=%v vel=%v", kind.typeName(), strength, flow, state.Vel)
		}
	}
	return true
}

func (s *Simulator) liquidFlow(pos cube.Pos, liquid world.Liquid) mgl32.Vec3 {
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
		below := neighbourPos.Side(cube.FaceDown)
		if lower, ok := s.liquidAt(below); ok && lower.LiquidType() == liquid.LiquidType() {
			flow = flow.Add(face.vec.Mul(float32(liquidDecay(lower) - currentDecay + 8)))
		}
	}
	if liquid.LiquidFalling() {
		for _, face := range liquidFaces {
			neighbourPos := pos.Add(face.delta)
			aboveNeighbour := neighbourPos.Side(cube.FaceUp)
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

func (s *Simulator) liquidFlowSideClosed(pos, side cube.Pos) bool {
	return s.blockAtPos(pos).Model().FaceSolid(pos, pos.Face(side), s.World)
}

func liquidDecay(liquid world.Liquid) int {
	if liquid.LiquidFalling() {
		return 0
	}
	return 8 - liquid.LiquidDepth()
}
