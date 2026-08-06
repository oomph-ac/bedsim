package bedsim

import (
	"iter"

	"github.com/chewxy/math32"

	"github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/go-gl/mathgl/mgl32"
	movementblock "github.com/oomph-ac/bedsim/block"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

// Simulate runs a movement simulation tick and returns the resulting state.
func (s *Simulator) Simulate(state *MovementState, input InputState) SimulationResult {
	if state == nil {
		return SimulationResult{}
	}

	s.applyInput(state, input)
	reason := s.simulateCore(state)
	if s.Options.SprintTiming == SprintTimingLegacy {
		s.applyLegacySprint(state, input)
	}
	s.tickState(state)
	return s.resultFromState(state, reason)
}

// SimulateState runs movement simulation using the current state values, without applying input updates
// or advancing tick counters. This is useful when the caller handles input parsing and ticking externally.
func (s *Simulator) SimulateState(state *MovementState) SimulationResult {
	if state == nil {
		return SimulationResult{}
	}
	reason := s.simulateCore(state)
	return s.resultFromState(state, reason)
}

func (s *Simulator) debugf(format string, args ...any) {
	if s != nil && s.Options.Debugf != nil {
		s.Options.Debugf(format, args...)
	}
}

func (s *Simulator) debugfIf(cond bool, format string, args ...any) {
	if cond {
		s.debugf(format, args...)
	}
}

func (s *Simulator) simulateCore(state *MovementState) SimulationOutcome {
	state.ensurePoseHeights()
	defer func() {
		state.RiptideReady = false
	}()
	teleported := s.attemptTeleport(state)
	if teleported {
		// A teleport relocates the player without observing the destination,
		// so any retained water contact from the origin is void.
		state.SwimWaterGraceTicks = 0
		state.StuckSpeedMultiplier = mgl32.Vec3{}
		return SimulationOutcomeTeleport
	}

	reliable := s.simulationIsReliable(state)
	if !reliable {
		s.resetToClient(state)
		return SimulationOutcomeUnreliable
	}
	if s.Options.RequireLiquidLayer && !s.HasLiquidLayer() {
		s.debugf("no liquid layer available and RequireLiquidLayer is set")
		s.resetToClient(state)
		return SimulationOutcomeUnreliable
	}
	if s.World != nil && !s.World.IsChunkLoaded(int32(math32.Floor(state.Pos.X()))>>4, int32(math32.Floor(state.Pos.Z()))>>4) {
		state.SetVel(mgl32.Vec3{})
		state.SwimWaterGraceTicks = 0
		state.StuckSpeedMultiplier = mgl32.Vec3{}
		return SimulationOutcomeUnloadedChunk
	}
	if state.Immobile || !state.Ready {
		state.SetVel(mgl32.Vec3{})
		// Frozen ticks observe nothing, so the budget must not simply pause
		// and resume later.
		state.SwimWaterGraceTicks = 0
		state.StuckSpeedMultiplier = mgl32.Vec3{}
		return SimulationOutcomeImmobileOrNotReady
	}

	s.simulateMovement(state)
	return SimulationOutcomeNormal
}

func (s *Simulator) resultFromState(state *MovementState, outcome SimulationOutcome) SimulationResult {
	result := SimulationResult{
		Position: state.Pos,
		Velocity: state.Vel,
		Movement: state.Mov,
		OnGround: state.OnGround,
		CollideX: state.CollideX,
		CollideY: state.CollideY,
		CollideZ: state.CollideZ,
		Outcome:  outcome,
	}

	result.PositionDelta = state.Pos.Sub(state.Client.Pos)
	result.VelocityDelta = state.Vel.Sub(state.Client.Vel)

	needsPos := s.Options.PositionCorrectionThreshold > 0 && result.PositionDelta.Len() > s.Options.PositionCorrectionThreshold
	needsVel := s.Options.VelocityCorrectionThreshold > 0 && result.VelocityDelta.Len() > s.Options.VelocityCorrectionThreshold
	switch s.Options.Mode {
	case SimulationModePassive:
		result.NeedsCorrection = false
	case SimulationModePermissive:
		// Permissive mode allows velocity drift and only corrects positional divergence.
		result.NeedsCorrection = needsPos
	default:
		result.NeedsCorrection = needsPos || needsVel
	}

	return result
}

func (s *Simulator) applyInput(state *MovementState, input InputState) {
	state.ensurePoseHeights()
	poseCollisionsAvailable := s.poseCollisionsAvailable(state)
	state.Client.HorizontalCollision = input.HorizontalCollision
	state.Client.VerticalCollision = input.VerticalCollision

	state.Client.LastPos = state.Client.Pos
	state.Client.Pos = input.ClientPos
	state.Client.LastVel = state.Client.Vel
	state.Client.Vel = input.ClientVel
	state.Client.LastMov = state.Client.Mov
	state.Client.Mov = state.Client.Pos.Sub(state.Client.LastPos)

	if input.StartFlying {
		state.Client.ToggledFly = true
		if state.TrustFlyStatus {
			state.Flying = true
		}
	} else if input.StopFlying {
		if state.Flying {
			state.JustDisabledFlight = true
		}
		state.Flying = false
		state.Client.ToggledFly = false
	}

	state.SetRotation(mgl32.Vec3{input.Pitch, input.HeadYaw, input.Yaw})

	state.PressingSneak = input.Sneaking
	state.PressingSprint = input.SprintDown
	state.PressingAscend = input.AscendBlock || input.Jumping
	state.PressingDescend = input.DescendBlock || input.Sneaking

	startFlag, stopFlag := input.StartSprinting, input.StopSprinting
	needsSpeedAdjusted := false
	isModernSprint := s.Options.SprintTiming == SprintTimingModern
	if startFlag && stopFlag {
		needsSpeedAdjusted = isModernSprint
		state.Sprinting = false
		state.AirSpeed = 0.02
	} else if !startFlag && !stopFlag && !state.ServerSprintApplied && state.ServerSprint != state.Sprinting {
		if state.ServerSprint {
			state.Sprinting = true
			state.AirSpeed = 0.026
		} else {
			state.Sprinting = false
			state.AirSpeed = 0.02
		}
	} else if startFlag {
		state.Sprinting = true
		needsSpeedAdjusted = isModernSprint
		state.AirSpeed = 0.026
	} else if stopFlag {
		state.Sprinting = false
		needsSpeedAdjusted = isModernSprint && !state.ServerUpdatedSpeed
		state.AirSpeed = 0.02
	}
	state.ServerSprintApplied = true

	if needsSpeedAdjusted {
		state.ServerUpdatedSpeed = false
		state.MovementSpeed = state.DefaultMovementSpeed
		if state.Sprinting {
			state.MovementSpeed *= 1.3
		}
	}

	wantSneak := input.SneakDown || input.StartSneaking
	if input.StopSneaking {
		wantSneak = false
	}
	if input.StartSneaking {
		state.Sneaking = true
		if !state.Crawling {
			state.Size[1] = state.SneakingHeight
		}
	} else if input.StopSneaking {
		if state.Crawling {
			state.Sneaking = false
		} else if poseCollisionsAvailable && s.canFitHeight(state, state.StandingHeight) {
			state.Sneaking = false
			state.Size[1] = state.StandingHeight
		} else {
			state.Sneaking = true
			state.Size[1] = state.SneakingHeight
		}
	} else {
		if state.Crawling {
			state.Sneaking = false
		} else if input.SneakDown {
			state.Sneaking = true
			state.Size[1] = state.SneakingHeight
		} else if state.Sneaking && (!poseCollisionsAvailable || !s.canFitHeight(state, state.StandingHeight)) {
			state.Size[1] = state.SneakingHeight
		} else {
			state.Sneaking = false
			state.Size[1] = state.StandingHeight
		}
	}
	if input.StartCrawling {
		if poseCollisionsAvailable && !s.canFitHeight(state, state.StandingHeight) {
			state.Crawling = true
			state.Sneaking = false
			state.Size[1] = state.CrawlingHeight
		}
	} else if input.StopCrawling {
		targetHeight := state.StandingHeight
		if wantSneak {
			targetHeight = state.SneakingHeight
		}
		if poseCollisionsAvailable && s.canFitHeight(state, targetHeight) {
			state.Crawling = false
			state.Sneaking = wantSneak
			state.Size[1] = targetHeight
		} else {
			state.Crawling = true
			state.Size[1] = state.CrawlingHeight
		}
	}

	wasSwimming := state.Swimming
	if input.StopSwimming {
		state.Swimming = false
		s.restorePoseAfterSwimming(state, poseCollisionsAvailable)
	} else if input.StartSwimming {
		state.Swimming = true
		if state.SwimPose() || poseCollisionsAvailable && s.canFitHeight(state, state.StandingHeight) {
			setSwimmingPoseFlags(state)
		}
	}
	if wasSwimming {
		state.SwimAmount = ClampFloat(state.SwimAmount+0.1, 0, 1)
	} else {
		state.SwimAmount = ClampFloat(state.SwimAmount-0.1, 0, 1)
	}
	state.AutoJumpingInWater = input.AutoJumpingInWater
	state.WantDown = input.WantDown
	state.WantDownSlow = input.WantDownSlow

	// Preserve bedsim's public impulse clamps unless upstream behavior is opted in.
	maxImpulse := float32(1)
	if !s.Options.UpstreamImpulseClamping {
		if input.UsingConsumable || (input.UsingItem && !input.UsingSpear) {
			maxImpulse *= MaxConsumingImpulse
		}
		if state.Sneaking || state.Crawling || state.Gliding {
			state.TicksSinceCanSlowdown++
			sneakMultiplier := MaxSneakImpulse
			if state.TicksSinceCanSlowdown > 2 && s.Equipment != nil {
				sneakMultiplier += 0.15 * float32(s.Equipment.EnchantmentLevel(EnchantmentSwiftSneak))
			}
			maxImpulse *= ClampFloat(sneakMultiplier, 0, 1)
		} else {
			state.TicksSinceCanSlowdown = 0
		}
	}
	moveVector := mgl32.Vec2{
		ClampFloat(input.MoveVector[0], -maxImpulse, maxImpulse),
		ClampFloat(input.MoveVector[1], -maxImpulse, maxImpulse),
	}
	if input.InventoryAction {
		moveVector = mgl32.Vec2{}
	}

	// Ground jumps are edge-triggered; liquid and ladder ascent may be held.
	state.Jumping = input.StartJumping
	state.PressingJump = input.Jumping
	state.EffectiveJumping = input.Jumping || input.AutoJumpingInWater || input.AscendBlock
	state.JumpHeight = DefaultJumpHeight
	if s.Effects != nil {
		if amp, ok := s.Effects.GetEffect(packet.EffectJumpBoost); ok {
			state.JumpHeight += float32(amp+1) * 0.1
		}
	}

	if !state.PressingJump {
		state.JumpDelay = 0
	}
	state.Gravity = NormalGravity
	state.SlowFalling = false
	if s.Effects != nil {
		if _, ok := s.Effects.GetEffect(packet.EffectSlowFalling); ok {
			state.SlowFalling = true
		}
	}

	if input.StopGliding {
		state.Gliding = false
	} else if input.StartGliding {
		state.Gliding = true
	}

	state.StartingSpinAttack = input.StartSpinAttack
	if input.StopSpinAttack && state.RiptideTicks > 0 && state.RiptideCollision {
		state.RiptideTicks = 0
		state.RiptideCollision = false
		state.SetVel(state.Vel.Mul(-0.2))
	}

	state.Impulse = moveVector.Mul(0.98)
}

func (s *Simulator) applyLegacySprint(state *MovementState, input InputState) {
	needsSpeedAdjusted := false
	if input.StartSprinting && input.StopSprinting {
		state.Sprinting = false
		needsSpeedAdjusted = true
	} else if input.StartSprinting {
		state.Sprinting = true
		needsSpeedAdjusted = true
	} else if input.StopSprinting {
		state.Sprinting = false
		needsSpeedAdjusted = !state.ServerUpdatedSpeed
	}
	if needsSpeedAdjusted {
		state.ServerUpdatedSpeed = false
		state.MovementSpeed = state.DefaultMovementSpeed
		if state.Sprinting {
			state.MovementSpeed *= 1.3
		}
	}
}

func (s *Simulator) tickState(state *MovementState) {
	if state.GlideBoostTicks > 0 {
		state.GlideBoostTicks--
	}
	if state.DolphinBoostTicks > 0 {
		state.DolphinBoostTicks--
		if state.DolphinBoostTicks <= 0 {
			state.DolphinBoostTicks = 0
			state.SwimSpeedMultiplier = DefaultSwimSpeedMultiplier
		}
	}
	state.TicksSinceKnockback++
	if state.TicksSinceTeleport < math32.MaxUint64 {
		state.TicksSinceTeleport++
	}
	if state.JumpDelay > 0 {
		state.JumpDelay--
	}
	if state.RiptideTicks > 0 {
		state.RiptideTicks--
		if state.RiptideTicks == 0 {
			state.RiptideCollision = false
		}
	}
	state.JustDisabledFlight = false
}

func (s *Simulator) simulateMovement(state *MovementState) {
	vel := state.Vel
	for axis := range 3 {
		if math32.Abs(vel[axis]) < 1e-8 {
			vel[axis] = 0
		}
	}
	state.SetVel(vel)

	// Bound retained water evidence before collision and travel inspect it.
	grace := s.swimWaterGraceTicks()
	if state.SwimWaterGraceTicks > grace {
		state.SwimWaterGraceTicks = grace
	}

	waterBlocks := s.touchingLiquidBlocks(state, liquidWater)
	lavaBlocks := s.touchingLiquidBlocks(state, liquidLava)
	inWater := len(waterBlocks) != 0
	if inWater && state.Swimming {
		state.SwimWaterGraceTicks = grace
		setSwimmingPoseFlags(state)
	}
	if !state.Flying && s.attemptRiptide(state, inWater) {
		s.debugf("riptide launch applied: %v", state.Vel)
	}

	defer func() {
		if inWater {
			state.SwimWaterGraceTicks = grace
		} else if state.SwimWaterGraceTicks > 0 {
			state.SwimWaterGraceTicks--
		}
	}()

	// Observed lava takes precedence over retained water evidence.
	waterTravel := inWater ||
		(state.Swimming && state.SwimWaterGraceTicks > 0 && len(lavaBlocks) == 0)

	if !state.Flying && (waterTravel || len(lavaBlocks) != 0) {
		s.debugfIf(attemptKnockback(state), "knockback applied in liquid: %v", state.Vel)
		if waterTravel {
			if state.Gliding {
				state.Gliding = false
				state.GlideBoostTicks = 0
			}
			s.applyLiquidFlow(state, waterBlocks, liquidWater)
			s.simulateLiquidTravel(state, liquidWater, inWater)
		} else {
			s.applyLiquidFlow(state, lavaBlocks, liquidLava)
			s.simulateLiquidTravel(state, liquidLava, true)
		}
		return
	}

	blockUnder := s.blockAtPos(posFromVec3(state.Pos.Sub(mgl32.Vec3{0, 0.5})))
	blockFriction := DefaultAirFriction
	moveRelativeSpeed := state.AirSpeed
	if state.OnGround {
		mSpeed := state.MovementSpeed
		blockSemantics := s.blockMovementSemantics(blockUnder)
		blockFriction *= blockSemantics.GroundFriction
		accelerationMultiplier := blockSemantics.GroundAccelerationFrictionMultiplier
		if s.Equipment != nil && s.Equipment.EnchantmentLevel(EnchantmentSoulSpeed) > 0 && blockSemantics.SoulSpeedNeutralizesAccelerationFriction {
			accelerationMultiplier = 1
		}
		accelerationFriction := blockFriction * accelerationMultiplier
		moveRelativeSpeed = mSpeed * (0.16277136 / (accelerationFriction * accelerationFriction * accelerationFriction))
	}

	if state.Gliding && s.Effects != nil {
		if _, levitating := s.Effects.GetEffect(packet.EffectLevitation); levitating {
			state.Gliding = false
		}
	}
	if state.Gliding {
		hasElytra := s.Inventory != nil && s.Inventory.HasElytra()
		if hasElytra && !state.OnGround {
			state.OnGround = false
			s.simulateGlide(state)
			stuckMovement := applyStuckSpeedMultiplier(state)
			oldVel := state.Vel
			oldY := state.Pos.Y()
			s.tryCollisions(state, false)
			stopRiptideOnBlockCollision(state)
			updateFallDistance(state, oldY)
			s.debugf("(glide) oldVel=%v, collisions=%v diff=%v", oldVel, state.Vel, state.Vel.Sub(state.Client.Vel))
			state.SetMov(state.Vel)
			if stuckMovement {
				state.SetVel(mgl32.Vec3{})
			}
			s.applyInsideBlockEffects(state)
			return
		}

		state.Gliding = false
		s.debugf("cannot allow glide (onGround=%v hasElytra=%v)", state.OnGround, hasElytra)
	}

	var clientJumpPrevented bool
	s.debugfIf(attemptKnockback(state), "knockback applied: %v", state.Vel)
	s.debugf("blockUnder=%s, blockFriction=%v, speed=%v", BlockName(blockUnder), blockFriction, moveRelativeSpeed)
	moveRelative(state, moveRelativeSpeed)
	s.debugf("moveRelative force applied (vel=%v)", state.Vel)
	s.debugfIf(s.attemptJump(state, &clientJumpPrevented), "jump force applied (sprint=%v): %v", state.Sprinting, state.Vel)
	insideSemantics := s.blockMovementSemantics(s.blockAtPos(posFromVec3(state.Pos)))
	leatherBoots := s.Equipment != nil && s.Equipment.WearingLeatherBoots()
	applyAscendableMovement(state, insideSemantics.Traversal, leatherBoots)

	nearClimbable := insideSemantics.Climbable
	if nearClimbable {
		newVel := state.Vel
		negClimbSpeed := -ClimbSpeed
		if newVel[1] < negClimbSpeed {
			newVel[1] = negClimbSpeed
		}
		if state.EffectiveJumping || state.CollideX || state.CollideZ {
			newVel[1] = ClimbSpeed
		}
		if state.Sneaking && newVel[1] < 0 {
			newVel[1] = 0
		}
		state.SetVel(newVel)
		s.debugf("added climb velocity: %v (collided=%v effectiveJumping=%v)", newVel, state.CollideX || state.CollideZ, state.EffectiveJumping)
	}

	inCobweb := s.isInsideCobweb(state)

	if inCobweb {
		newVel := state.Vel
		xz, y := float32(0.25), float32(0.05)
		if s.Effects != nil {
			if _, weaving := s.Effects.GetEffect(EffectWeaving); weaving {
				xz, y = 0.5, 0.25
			}
		}
		newVel[0] *= xz
		newVel[1] *= y
		newVel[2] *= xz
		state.SetVel(newVel)
		s.debugf("web force applied (vel=%v)", newVel)
	}

	stuckMovement := applyStuckSpeedMultiplier(state)
	s.avoidEdge(state)

	oldVel := state.Vel
	oldOnGround := state.OnGround
	oldY := state.Pos.Y()
	s.tryCollisions(state, clientJumpPrevented)
	stopRiptideOnBlockCollision(state)
	updateFallDistance(state, oldY)

	if state.SupportingBlockPos != nil {
		blockUnder = s.blockAtPos(*state.SupportingBlockPos)
	} else {
		blockUnder = s.blockAtPos(posFromVec3(state.Pos.Sub(mgl32.Vec3{0, 0.2})))
		if s.blockAir(blockUnder) {
			below := s.blockAtPos(posFromVec3(state.Pos).Side(cube.FaceDown))
			if IsWall(below) || IsFence(below) {
				blockUnder = below
			}
		}
	}

	if oldY == state.Pos.Y() {
		s.walkOnBlock(state, blockUnder)
	} else {
		s.debugf("walkOnBlock: y changed, skipping block walk effects")
	}

	state.SetMov(state.Vel)
	if stuckMovement {
		state.SetVel(mgl32.Vec3{})
		oldVel = mgl32.Vec3{}
	}
	s.setPostCollisionMotion(state, oldVel, oldOnGround, blockUnder)

	if inCobweb {
		s.debugf("post-move cobweb force applied (0 vel)")
		state.SetVel(mgl32.Vec3{})
	}

	newVel := state.Vel
	if s.Effects != nil {
		if amp, ok := s.Effects.GetEffect(packet.EffectLevitation); ok {
			levSpeed := LevitationGravityMultiplier * float32(amp+1)
			newVel[1] += (levSpeed - newVel[1]) * 0.2
		} else if state.HasGravity {
			newVel[1] -= effectiveGravity(state, newVel)
			newVel[1] *= NormalGravityMultiplier
		}
	} else if state.HasGravity {
		newVel[1] -= effectiveGravity(state, newVel)
		newVel[1] *= NormalGravityMultiplier
	}
	newVel[0] *= blockFriction
	newVel[2] *= blockFriction
	state.SetVel(newVel)
	s.applyInsideBlockEffects(state)
}

func (s *Simulator) simulationIsReliable(state *MovementState) bool {
	if state.RemainingTeleportTicks() > 0 {
		return true
	}

	if state.GameMode != packet.GameTypeSurvival && state.GameMode != packet.GameTypeAdventure {
		return false
	}
	if state.Flying || state.JustDisabledFlight || state.NoClip || !state.Alive {
		return false
	}
	return true
}

func (s *Simulator) resetToClient(state *MovementState) {
	// A frame we did not simulate proves nothing about water contact, so the
	// retained evidence is dropped rather than carried across the gap.
	state.SwimWaterGraceTicks = 0
	state.StuckSpeedMultiplier = mgl32.Vec3{}
	state.LastPos = state.Client.LastPos
	state.Pos = state.Client.Pos
	state.LastVel = state.Client.LastVel
	state.Vel = state.Client.Vel
	state.LastMov = state.Client.LastMov
	state.Mov = state.Client.Mov
	if state.Flying || state.NoClip {
		state.OnGround = false
	}

	if s.Options.LimitAllVelocity {
		limit := s.Options.LimitAllVelocityThreshold
		if limit < 0 {
			limit = -limit
		}
		state.Vel[0] = ClampFloat(state.Vel[0], -limit, limit)
		state.Vel[1] = ClampFloat(state.Vel[1], -limit, limit)
		state.Vel[2] = ClampFloat(state.Vel[2], -limit, limit)
	}
}

func (s *Simulator) attemptTeleport(state *MovementState) bool {
	if !state.HasTeleport() {
		return false
	}

	if !state.TeleportIsSmoothed {
		state.SetPos(state.TeleportPos)
		state.SetVel(mgl32.Vec3{})
		state.JumpDelay = 0
		s.attemptJump(state, nil)
		return true
	}

	posDelta := state.TeleportPos.Sub(state.Pos)
	if remaining := state.RemainingTeleportTicks() + 1; remaining > 0 {
		newPos := state.Pos.Add(posDelta.Mul(1.0 / float32(remaining)))
		state.SetPos(newPos)
		state.JumpDelay = 0
		return remaining > 1
	}
	return false
}

func (s *Simulator) simulateGlide(state *MovementState) {
	radians := math32.Pi / 180.0
	yaw, pitch := state.Rotation.Z()*radians, state.Rotation.X()*radians
	yawCos := MCCos(-yaw - math32.Pi)
	yawSin := MCSin(-yaw - math32.Pi)
	pitchCos := MCCos(pitch)
	pitchSin := MCSin(pitch)

	lookX := yawSin * -pitchCos
	lookY := -pitchSin
	lookZ := yawCos * -pitchCos

	vel := state.Vel
	velHz := math32.Sqrt(vel[0]*vel[0] + vel[2]*vel[2])
	lookHz := pitchCos
	sqrPitchCos := pitchCos * pitchCos

	gravity := effectiveGravity(state, vel)
	vel[1] += -gravity + sqrPitchCos*(gravity*0.75)
	if vel[1] < 0 && lookHz > 0 {
		yAccel := vel[1] * -0.1 * sqrPitchCos
		vel[1] += yAccel
		vel[0] += lookX * yAccel / lookHz
		vel[2] += lookZ * yAccel / lookHz
	}
	if pitch < 0 {
		yAccel := velHz * -pitchSin * 0.04
		vel[1] += yAccel * 3.2
		vel[0] -= lookX * yAccel / lookHz
		vel[2] -= lookZ * yAccel / lookHz
	}
	if lookHz > 0 {
		vel[0] += (lookX/lookHz*velHz - vel[0]) * 0.1
		vel[2] += (lookZ/lookHz*velHz - vel[2]) * 0.1
	}

	if state.GlideBoostTicks > 0 {
		oldVel := vel
		vel[0] += (lookX * 0.1) + (((lookX * 1.5) - vel[0]) * 0.5)
		vel[1] += (lookY * 0.1) + (((lookY * 1.5) - vel[1]) * 0.5)
		vel[2] += (lookZ * 0.1) + (((lookZ * 1.5) - vel[2]) * 0.5)
		s.debugf("applied glide boost (old=%v new=%v)", oldVel, vel)
		s.debugf("glide boost dirVec=[%f %f %f]", lookX, lookY, lookZ)
	}

	vel[0] *= 0.99
	vel[1] *= 0.98
	vel[2] *= 0.99

	state.SetVel(vel)
}

func (s *Simulator) walkOnBlock(state *MovementState, blockUnder world.Block) {
	if !state.OnGround || state.Sneaking {
		s.debugf("walkOnBlock: conditions not met (onGround=%v sneaking=%v)", state.OnGround, state.Sneaking)
		return
	}

	oldVel := state.Vel
	newVel := state.Vel
	semantics := s.blockMovementSemantics(blockUnder)
	if semantics.Bounce == movementblock.BounceSlime || semantics.Honey {
		yMov := math32.Abs(newVel.Y())
		if yMov < 0.1 && !state.PressingSneak {
			d1 := 0.4 + yMov*0.2
			newVel[0] *= d1
			newVel[2] *= d1
		}
	}
	state.SetVel(newVel)
	s.debugf("walkOnBlock: oldVel=%v newVel=%v", oldVel, newVel)
}

func (s *Simulator) landOnBlock(state *MovementState, old mgl32.Vec3, blockUnder world.Block) {
	newVel := state.Vel
	if old.Y() >= 0 || state.PressingSneak {
		newVel[1] = 0
		state.SetVel(newVel)
		return
	}

	switch s.blockMovementSemantics(blockUnder).Bounce {
	case movementblock.BounceSlime:
		newVel[1] = SlimeBounceMultiplier * old.Y()
		if math32.Abs(newVel[1]) < 1e-4 {
			newVel[1] = 0.0
		}
	case movementblock.BounceBed:
		newVel[1] = math32.Min(0.75, BedBounceMultiplier*old.Y())
	default:
		newVel[1] = 0
	}
	state.SetVel(newVel)
}

func effectiveGravity(state *MovementState, velocity mgl32.Vec3) float32 {
	if state.SlowFalling && velocity.Y() < 0 {
		return SlowFallingGravity
	}
	return state.Gravity
}

func (s *Simulator) setPostCollisionMotion(state *MovementState, oldVel mgl32.Vec3, oldOnGround bool, blockUnder world.Block) {
	if !oldOnGround && state.CollideY {
		s.landOnBlock(state, oldVel, blockUnder)
	} else if state.CollideY {
		newVel := state.Vel
		newVel[1] = 0
		state.SetVel(newVel)
	}

	newVel := state.Vel
	if state.CollideX {
		newVel[0] = 0
	}
	if state.CollideZ {
		newVel[2] = 0
	}
	state.SetVel(newVel)
}

func updateFallDistance(state *MovementState, oldY float32) {
	yDelta := state.Pos.Y() - oldY
	if yDelta < 0 && !state.OnGround {
		state.FallDistance -= yDelta
	} else if yDelta > 0 {
		state.FallDistance = 0
	}
	if state.OnGround && state.FallDistance > 0 {
		state.FallDistance = 0
	}
}

func moveRelative(state *MovementState, moveRelativeSpeed float32) {
	impulse := state.Impulse
	force := impulse.Y()*impulse.Y() + impulse.X()*impulse.X()

	if force >= 1e-4 {
		force = moveRelativeSpeed / math32.Max(math32.Sqrt(force), 1.0)
		mf, ms := impulse.Y()*force, impulse.X()*force

		yaw := state.Rotation.Z() * math32.Pi / 180.0
		v2, v3 := MCSin(yaw), MCCos(yaw)

		newVel := state.Vel
		newVel[0] += ms*v3 - mf*v2
		newVel[2] += mf*v3 + ms*v2
		state.SetVel(newVel)
	}
}

func attemptKnockback(state *MovementState) bool {
	if state.HasKnockback() {
		state.SetVel(state.Knockback)
		return true
	}
	return false
}

func (s *Simulator) attemptJump(state *MovementState, clientJumpPrevented *bool) bool {
	if !state.Jumping || !state.OnGround || state.JumpDelay > 0 {
		s.debugfIf(state.Jumping, "rejected jump from client (onGround=%v jumpDelay=%d)", state.OnGround, state.JumpDelay)
		return false
	}

	newVel := state.Vel
	jumpHeight := state.JumpHeight
	inBlock := s.blockAtPos(posFromVec3(state.Pos))
	below := s.blockAtPos(posFromVec3(state.Pos.Sub(mgl32.Vec3{0, 0.1})))
	if s.blockMovementSemantics(inBlock).Honey || s.blockMovementSemantics(below).Honey {
		jumpHeight *= 0.6
	}
	newVel[1] = math32.Max(jumpHeight, newVel[1])
	state.JumpDelay = JumpDelayTicks

	if state.Sprinting {
		force := state.Rotation.Z() * 0.017453292
		newVel[0] -= MCSin(force) * 0.2
		newVel[2] += MCCos(force) * 0.2
	}

	if clientJumpPrevented != nil && !state.HasKnockback() && !state.HasTeleport() {
		if s.isJumpBlocked(state, newVel) {
			*clientJumpPrevented = true
			s.debugf("jump determined to be blocked")
		}
	}

	state.SetVel(newVel)
	return true
}

func (s *Simulator) isJumpBlocked(state *MovementState, jumpVel mgl32.Vec3) bool {
	w := s.World
	if w == nil {
		return false
	}
	useSlideOffset := s.Options.UseSlideOffset
	collisionBB := state.BoundingBox(useSlideOffset)
	bbList := s.nearbyBBoxes(state, collisionBB.Extend(jumpVel))

	yVel := mgl32.Vec3{0, jumpVel.Y()}
	xVel := mgl32.Vec3{jumpVel.X()}
	zVel := mgl32.Vec3{0, 0, jumpVel.Z()}

	for i := len(bbList) - 1; i >= 0; i-- {
		yVel = BBClipCollide(bbList[i], collisionBB, yVel, false, nil)
	}
	collisionBB = collisionBB.Translate(yVel)

	for i := len(bbList) - 1; i >= 0; i-- {
		xVel = BBClipCollide(bbList[i], collisionBB, xVel, false, nil)
	}
	collisionBB = collisionBB.Translate(xVel)

	for i := len(bbList) - 1; i >= 0; i-- {
		zVel = BBClipCollide(bbList[i], collisionBB, zVel, false, nil)
	}
	initialBlockCond := ((xVel[0] != jumpVel[0]) || (zVel[2] != jumpVel[2])) && yVel[1] == jumpVel[1]
	if !initialBlockCond {
		return false
	}

	xVel = mgl32.Vec3{jumpVel.X()}
	yVel = mgl32.Vec3{0, jumpVel.Y()}
	zVel = mgl32.Vec3{0, 0, jumpVel.Z()}
	collisionBB = state.BoundingBox(useSlideOffset)

	for i := len(bbList) - 1; i >= 0; i-- {
		xVel = BBClipCollide(bbList[i], collisionBB, xVel, false, nil)
	}
	collisionBB = collisionBB.Translate(xVel)

	for i := len(bbList) - 1; i >= 0; i-- {
		zVel = BBClipCollide(bbList[i], collisionBB, zVel, false, nil)
	}
	collisionBB = collisionBB.Translate(zVel)

	for i := len(bbList) - 1; i >= 0; i-- {
		yVel = BBClipCollide(bbList[i], collisionBB, yVel, false, nil)
	}
	return yVel[1] != jumpVel[1] && xVel[0] == jumpVel[0] && zVel[2] == jumpVel[2]
}

func (s *Simulator) tryCollisions(state *MovementState, clientJumpPrevented bool) {
	w := s.World
	if w == nil {
		return
	}
	useSlideOffset := s.Options.UseSlideOffset
	correctionThreshold := s.Options.PositionCorrectionThreshold

	var completedStep bool
	collisionBB := state.BoundingBox(useSlideOffset)
	currVel := state.Vel
	bbList := s.nearbyBBoxes(state, collisionBB.Extend(currVel))

	useOneWayCollisions := state.StuckInCollider
	penetration := mgl32.Vec3{}

	yVel := mgl32.Vec3{0, currVel.Y()}
	if clientJumpPrevented {
		yVel[1] = 0
	}
	xVel := mgl32.Vec3{currVel.X()}
	zVel := mgl32.Vec3{0, 0, currVel.Z()}

	for i := len(bbList) - 1; i >= 0; i-- {
		yVel = BBClipCollide(bbList[i], collisionBB, yVel, useOneWayCollisions, &penetration)
	}
	collisionBB = collisionBB.Translate(yVel)
	s.debugf("Y-collision non-step=%v /w penetration=%v (oneWay=%v)", yVel, penetration, useOneWayCollisions)

	for i := len(bbList) - 1; i >= 0; i-- {
		xVel = BBClipCollide(bbList[i], collisionBB, xVel, useOneWayCollisions, &penetration)
	}
	collisionBB = collisionBB.Translate(xVel)
	s.debugf("(X) hz-collision non-step=%v /w penetration=%v (oneWay=%v)", xVel, penetration, useOneWayCollisions)

	for i := len(bbList) - 1; i >= 0; i-- {
		zVel = BBClipCollide(bbList[i], collisionBB, zVel, useOneWayCollisions, &penetration)
	}
	collisionBB = collisionBB.Translate(zVel)
	s.debugf("(Z) hz-collision non-step=%v /w penetration=%v (oneWay=%v)", zVel, penetration, useOneWayCollisions)

	collisionVel := yVel.Add(xVel).Add(zVel)
	collisionPos := mgl32.Vec3{
		(collisionBB.Min().X() + collisionBB.Max().X()) * 0.5,
		collisionBB.Min().Y(),
		(collisionBB.Min().Z() + collisionBB.Max().Z()) * 0.5,
	}
	s.debugf("endCollisionVel=%v endCollisionPos=%v", collisionVel, collisionPos)

	hasPenetration := penetration.LenSqr() >= 9.999999999999999e-12
	state.StuckInCollider = state.PenetratedLastFrame && hasPenetration
	state.PenetratedLastFrame = hasPenetration

	xCollision := currVel.X() != collisionVel.X()
	yCollision := (currVel.Y() != collisionVel.Y()) || clientJumpPrevented
	zCollision := currVel.Z() != collisionVel.Z()
	onGround := state.OnGround || (yCollision && currVel.Y() < 0.0)

	if onGround && (xCollision || zCollision) {
		stepYVel := mgl32.Vec3{0, StepHeight}
		stepXVel := mgl32.Vec3{currVel.X()}
		stepZVel := mgl32.Vec3{0, 0, currVel.Z()}

		stepBB := state.BoundingBox(useSlideOffset)
		for _, blockBox := range bbList {
			stepYVel = BBClipCollide(blockBox, stepBB, stepYVel, useOneWayCollisions, nil)
		}
		stepBB = stepBB.Translate(stepYVel)
		s.debugf("stepYVel=%v", stepYVel)

		for _, blockBox := range bbList {
			stepXVel = BBClipCollide(blockBox, stepBB, stepXVel, useOneWayCollisions, nil)
		}
		stepBB = stepBB.Translate(stepXVel)
		s.debugf("stepXVel=%v", stepXVel)

		for _, blockBox := range bbList {
			stepZVel = BBClipCollide(blockBox, stepBB, stepZVel, useOneWayCollisions, nil)
		}
		stepBB = stepBB.Translate(stepZVel)
		s.debugf("stepZVel=%v", stepZVel)

		inverseYStepVel := stepYVel.Mul(-1)
		for _, blockBox := range bbList {
			inverseYStepVel = BBClipCollide(blockBox, stepBB, inverseYStepVel, useOneWayCollisions, nil)
		}
		stepBB = stepBB.Translate(inverseYStepVel)
		stepYVel = stepYVel.Add(inverseYStepVel)
		s.debugf("inverseYStepVel=%v", inverseYStepVel)

		stepVel := stepYVel.Add(stepXVel).Add(stepZVel)
		newBBListCount := 0
		hasStepCollisions := false
		if s.Options.Debugf != nil {
			newBBListCount = len(s.nearbyBBoxes(state, stepBB))
			hasStepCollisions = newBBListCount > 0
		} else {
			hasStepCollisions = s.hasNearbyBBoxes(state, stepBB)
		}
		stepPos := mgl32.Vec3{
			(stepBB.Min().X() + stepBB.Max().X()) * 0.5,
			stepBB.Min().Y(),
			(stepBB.Min().Z() + stepBB.Max().Z()) * 0.5,
		}
		s.debugf("endStepVel=%v endStepPos=%v", stepVel, stepPos)
		s.debugf("newBBList count: %d", newBBListCount)

		if !hasStepCollisions && Vec3HzDistSqr(collisionVel) < Vec3HzDistSqr(stepVel) {
			// Match vanilla's step-vs-collision tie-breaker using client alignment to avoid false
			// positives where the server predicts a step that the client rejects.
			// When IgnoreClientStepTiebreaker is set (pathfinder mode), skip the
			// tie-breaker since the caller drives its own movement and always
			// wants step-ups accepted.
			stepPosDist := stepPos.Sub(state.Client.Pos).Len()
			collisionPosDist := collisionPos.Sub(state.Client.Pos).Len()
			if s.Options.IgnoreClientStepTiebreaker || collisionPosDist > correctionThreshold || stepPosDist <= collisionPosDist {
				collisionVel = stepVel
				collisionBB = stepBB
				if useSlideOffset {
					completedStep = true
					slideOffset := state.SlideOffset.Mul(SlideOffsetMultiplier)
					slideOffset[1] += stepVel.Y()
					state.SlideOffset = slideOffset
				}
				s.debugf("step successful")
			} else {
				s.debugf("step failed (client rejection) [clientPos=%v collisionPos=%v stepPos=%v]", state.Client.Pos, collisionPos, stepPos)
			}
		} else {
			s.debugf("step failed")
		}
	}

	endPos := mgl32.Vec3{
		(collisionBB.Min().X() + collisionBB.Max().X()) * 0.5,
		collisionBB.Min().Y(),
		(collisionBB.Min().Z() + collisionBB.Max().Z()) * 0.5,
	}

	if useSlideOffset {
		if completedStep {
			// Older clients keep a slide offset accumulator that gets applied to the final Y.
			endPos[1] -= state.SlideOffset.Y()
			s.debugf("applying slideOffset, able to subtract endPos.y this frame by %f", state.SlideOffset.Y())
		} else {
			s.debugf("using slide offset, RESETTING slide offset vector")
			state.SlideOffset = mgl32.Vec2{}
		}
	}
	state.SetPos(endPos)

	yCollision = math32.Abs(currVel.Y()-collisionVel.Y()) >= 1e-5
	state.CollideX = math32.Abs(currVel.X()-collisionVel.X()) >= 1e-5
	state.CollideY = yCollision
	state.CollideZ = math32.Abs(currVel.Z()-collisionVel.Z()) >= 1e-5

	state.OnGround = (yCollision && currVel.Y() < 0) || (state.OnGround && !yCollision && math32.Abs(currVel.Y()) <= 1e-5)
	checkSupportingBlockPos(state, w, useSlideOffset, currVel)
	state.SetVel(collisionVel)
	s.debugf("clientVel=%v clientPos=%v", state.Client.Mov, state.Client.Pos)
	s.debugf("finalVel=%v finalPos=%v", collisionVel, state.Pos)
	s.debugf("(client) hzCollision=%v yCollision=%v", state.Client.HorizontalCollision, state.Client.VerticalCollision)
	s.debugf("(server) xCollision=%v yCollision=%v zCollision=%v", state.CollideX, state.CollideY, state.CollideZ)
}

func (s *Simulator) avoidEdge(state *MovementState) {
	w := s.World
	if w == nil {
		return
	}
	if !state.Sneaking || !s.isAboveGround(state) || state.Vel.Y() > 0 {
		s.debugf(
			"avoidEdge: conditions not met (sneaking=%v onGround=%v yVel=%v)",
			state.Sneaking,
			state.OnGround,
			state.Vel.Y(),
		)
		return
	}

	edgeBoundry := float32(0.025)
	offset := float32(0.05)
	// Cap iterations to avoid excessive work with very large velocities.
	// should never happen, defensive.
	const maxIter = 1000

	oldVel := state.Vel
	newVel := state.Vel
	useSlideOffset := s.Options.UseSlideOffset
	bb := state.BoundingBox(useSlideOffset).GrowVec3(mgl32.Vec3{-edgeBoundry, 0, -edgeBoundry})
	xMov, zMov := newVel.X(), newVel.Z()

	i := 0
	for i = 0; i < maxIter && xMov != 0.0 && !s.hasNearbyBBoxes(state, bb.Translate(mgl32.Vec3{xMov, -StepHeight * 1.01, 0})); i++ {
		if xMov < offset && xMov >= -offset {
			xMov = 0
		} else if xMov > 0 {
			xMov -= offset
		} else {
			xMov += offset
		}
	}
	if i == maxIter {
		xMov = 0
	}

	for i = 0; i < maxIter && zMov != 0.0 && !s.hasNearbyBBoxes(state, bb.Translate(mgl32.Vec3{0, -StepHeight * 1.01, zMov})); i++ {
		if zMov < offset && zMov >= -offset {
			zMov = 0
		} else if zMov > 0 {
			zMov -= offset
		} else {
			zMov += offset
		}
	}
	if i == maxIter {
		zMov = 0
	}

	for i = 0; i < maxIter && xMov != 0.0 && zMov != 0.0 && !s.hasNearbyBBoxes(state, bb.Translate(mgl32.Vec3{xMov, -StepHeight * 1.01, zMov})); i++ {
		if xMov < offset && xMov >= -offset {
			xMov = 0
		} else if xMov > 0 {
			xMov -= offset
		} else {
			xMov += offset
		}

		if zMov < offset && zMov >= -offset {
			zMov = 0
		} else if zMov > 0 {
			zMov -= offset
		} else {
			zMov += offset
		}
	}
	if i == maxIter {
		xMov = 0
		zMov = 0
	}

	newVel[0] = xMov
	newVel[2] = zMov
	state.SetVel(newVel)
	s.debugf("(avoidEdge): oldVel=%v newVel=%v", oldVel, newVel)
}

func (s *Simulator) isAboveGround(state *MovementState) bool {
	if state.OnGround {
		return true
	}
	if state.FallDistance >= 0.6 || s.World == nil {
		return false
	}
	distance := 0.6 - state.FallDistance
	bb := state.BoundingBox(s.Options.UseSlideOffset).GrowVec3(mgl32.Vec3{-0.025, 0, -0.025})
	return s.hasNearbyBBoxes(state, bb.Translate(mgl32.Vec3{0, -distance}))
}

func (s *Simulator) isInsideCobweb(state *MovementState) bool {
	if s.World == nil {
		return false
	}

	bb := state.BoundingBox(s.Options.UseSlideOffset)
	insideCobweb := false
	for pos, b := range nearbyBlocks(bb.Grow(1), s.World) {
		if s.blockAir(b) {
			continue
		}
		if !bb.IntersectsWith(cube.Box32(0, 0, 0, 1, 1, 1).Translate(posVec3(pos))) {
			continue
		}
		if s.blockMovementSemantics(b).Cobweb {
			insideCobweb = true
		}
		if insideCobweb {
			break
		}
	}
	return insideCobweb
}

func nearbyBlocks(aabb cube.BBox32, w WorldProvider) iter.Seq2[cube.Pos, world.Block] {
	return func(yield func(cube.Pos, world.Block) bool) {
		if w == nil {
			return
		}
		min, max := aabb.Min(), aabb.Max()
		minX, minY, minZ := int(math32.Floor(min[0])), int(math32.Floor(min[1])), int(math32.Floor(min[2]))
		maxX, maxY, maxZ := int(math32.Ceil(max[0])), int(math32.Ceil(max[1])), int(math32.Ceil(max[2]))

		for y := minY; y <= maxY; y++ {
			for x := minX; x <= maxX; x++ {
				for z := minZ; z <= maxZ; z++ {
					pos := cube.Pos{x, y, z}
					if !yield(pos, w.Block(pos)) {
						return
					}
				}
			}
		}
	}
}

func checkSupportingBlockPos(state *MovementState, w WorldProvider, useSlideOffset bool, vel mgl32.Vec3) {
	if !state.OnGround {
		state.SupportingBlockPos = nil
		return
	}
	decBB := state.BoundingBox(useSlideOffset).ExtendTowards(cube.FaceDown, 1e-3)
	findSupportingBlock(state, w, decBB)
	if state.SupportingBlockPos == nil {
		decBB = decBB.Translate(mgl32.Vec3{-vel[0], 0, -vel[2]})
		findSupportingBlock(state, w, decBB)
	}
}

func findSupportingBlock(state *MovementState, w WorldProvider, bb cube.BBox32) {
	if w == nil {
		return
	}
	var blockPos *cube.Pos
	minDist := float32(math32.MaxFloat32 - 1)
	centerPos := posVec3(posFromVec3(state.Pos)).Add(mgl32.Vec3{0.5, 0.5, 0.5})

	for pos := range nearbyBlocks(bb, w) {
		boxes := w.BlockCollisions(pos)
		if len(boxes) == 0 {
			continue
		}

		for _, box := range boxes {
			if !bb.IntersectsWith(box.Translate(posVec3(pos))) {
				continue
			}
			dist := posVec3(pos).Sub(centerPos).LenSqr()
			if dist < minDist {
				minDist = dist
				supportPos := pos
				blockPos = &supportPos
			}
			break
		}
	}

	state.SupportingBlockPos = blockPos
}

func (s *Simulator) blockAtPos(pos cube.Pos) world.Block {
	if s.World == nil {
		return block.Air{}
	}
	return s.World.Block(pos)
}

func (s *Simulator) nearbyBBoxes(state *MovementState, aabb cube.BBox32) []cube.BBox32 {
	if s.World == nil {
		return nil
	}
	if provider, ok := s.World.(MovementCollisionProvider); ok {
		leatherBoots := s.Equipment != nil && s.Equipment.WearingLeatherBoots()
		return provider.GetMovementBBoxes(aabb, MovementCollisionContext{
			Position:     [3]float32(state.Pos),
			Sneaking:     state.Sneaking,
			Descending:   state.PressingDescend,
			WantDown:     state.WantDown,
			LeatherBoots: leatherBoots,
		})
	}
	return s.World.GetNearbyBBoxes(aabb)
}

type nearbyBBoxProbe interface {
	HasNearbyBBoxes(aabb cube.BBox32) bool
}

func (s *Simulator) hasNearbyBBoxes(state *MovementState, aabb cube.BBox32) bool {
	if s.World == nil {
		return false
	}
	if _, dynamic := s.World.(MovementCollisionProvider); dynamic {
		return len(s.nearbyBBoxes(state, aabb)) > 0
	}
	if probe, ok := s.World.(nearbyBBoxProbe); ok {
		return probe.HasNearbyBBoxes(aabb)
	}
	return len(s.World.GetNearbyBBoxes(aabb)) > 0
}

func (s *Simulator) canFitHeight(state *MovementState, height float32) bool {
	if s.World == nil {
		return true
	}
	standing := *state
	standing.Size[1] = height
	standing.Sneaking = false
	standing.Swimming = false
	standing.SwimWaterGraceTicks = 0
	standing.PressingDescend = false
	standing.WantDown = false
	return len(s.nearbyBBoxes(&standing, standing.BoundingBox(s.Options.UseSlideOffset))) == 0
}

func (s *Simulator) poseCollisionsAvailable(state *MovementState) bool {
	if s.World == nil {
		return true
	}
	chunkX := int32(math32.Floor(state.Pos.X())) >> 4
	chunkZ := int32(math32.Floor(state.Pos.Z())) >> 4
	return s.World.IsChunkLoaded(chunkX, chunkZ)
}

func setSwimmingPoseFlags(state *MovementState) {
	state.Sneaking = false
	state.Crawling = false
	state.Size[1] = state.StandingHeight
}

func (s *Simulator) restorePoseAfterSwimming(state *MovementState, collisionsAvailable bool) {
	if collisionsAvailable && s.canFitHeight(state, state.StandingHeight) {
		state.Sneaking = false
		state.Crawling = false
		state.Size[1] = state.StandingHeight
		return
	}
	if collisionsAvailable && s.canFitHeight(state, state.SneakingHeight) {
		state.Sneaking = true
		state.Crawling = false
		state.Size[1] = state.SneakingHeight
		return
	}
	state.Sneaking = false
	state.Crawling = true
	state.Size[1] = state.CrawlingHeight
}
