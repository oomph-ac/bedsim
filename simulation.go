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
	if state == nil || !finiteMovementState(state) {
		return s.invalidSimulationResult(nil)
	}
	if !finiteInput(input) {
		return s.invalidSimulationResult(state)
	}

	pose := movementPoseSnapshot{
		size:     state.Size,
		sneaking: state.Sneaking,
		crawling: state.Crawling,
		swimming: state.Swimming,
		swimAmt:  state.SwimAmount,
	}
	s.applyInput(state, input)
	reason := s.simulateCore(state, true)
	if reason == SimulationOutcomeUnloadedChunk {
		pose.restore(state)
	}
	if s.Options.SprintTiming == SprintTimingLegacy {
		s.applyLegacySprint(state, input)
	}
	state.AirSpeed = effectiveAirSpeed(state)
	advanceTeleport := reason != SimulationOutcomeTeleport || state.HasTeleport()
	s.tickState(state, advanceTeleport)
	return s.resultFromState(state, reason)
}

// movementPoseSnapshot preserves pose fields across an unloaded simulation.
type movementPoseSnapshot struct {
	size     mgl32.Vec3
	sneaking bool
	crawling bool
	swimming bool
	swimAmt  float32
}

// restore replaces the state's pose fields with the snapshot.
func (p movementPoseSnapshot) restore(state *MovementState) {
	state.Size = p.size
	state.Sneaking = p.sneaking
	state.Crawling = p.crawling
	state.Swimming = p.swimming
	state.SwimAmount = p.swimAmt
}

// SimulateState runs movement simulation using the current state values, without applying input updates
// or advancing tick counters. Callers that use it must advance tick counters
// and clear transient fields such as KnockbackPending, RiptideReady, and
// StoppedSwimmingThisTick themselves.
func (s *Simulator) SimulateState(state *MovementState) SimulationResult {
	if state == nil || !finiteMovementState(state) {
		return s.invalidSimulationResult(nil)
	}
	reason := s.simulateCore(state, false)
	return s.resultFromState(state, reason)
}

// invalidSimulationResult returns the mode-aware result for invalid data and
// preserves state when its numeric fields are safe to expose.
func (s *Simulator) invalidSimulationResult(state *MovementState) SimulationResult {
	result := SimulationResult{
		Outcome:         SimulationOutcomeInvalidInput,
		NeedsCorrection: s == nil || s.Options.Mode != SimulationModePassive,
	}
	if state == nil || !finiteMovementState(state) {
		return result
	}
	result.Position = state.Pos
	result.Velocity = state.Vel
	result.Movement = state.Mov
	result.OnGround = state.OnGround
	result.CollideX = state.CollideX
	result.CollideY = state.CollideY
	result.CollideZ = state.CollideZ
	result.PositionDelta = state.Pos.Sub(state.Client.Pos)
	result.VelocityDelta = state.Vel.Sub(state.Client.Vel)
	return result
}

func (s *Simulator) simulateCore(state *MovementState, consumeTransient bool) SimulationOutcome {
	state.ensurePoseHeights()
	if consumeTransient {
		defer func() {
			state.RiptideReady = false
		}()
	}
	teleported := s.attemptTeleport(state)
	if teleported {
		// A teleport relocates the player without observing the destination,
		// so any retained water contact from the origin is void.
		state.SwimWaterGraceTicks = 0
		state.StuckSpeedMultiplier = mgl32.Vec3{}
		return SimulationOutcomeTeleport
	}
	if state.InVehicle {
		s.resetToClient(state)
		return SimulationOutcomeMounted
	}

	reliable := s.simulationIsReliable(state)
	if !reliable {
		s.resetToClient(state)
		return SimulationOutcomeUnreliable
	}
	if s.Options.RequireLiquidLayer && !s.HasLiquidLayer() {
		if debugf := s.Options.Debugf; debugf != nil {
			debugf("no liquid layer available and RequireLiquidLayer is set")
		}
		s.resetToClient(state)
		return SimulationOutcomeUnreliable
	}
	sweepVelocity := state.Vel
	if state.HasKnockback() {
		sweepVelocity = state.Knockback
	}
	if s.World != nil && !s.movementAreaLoaded(state.BoundingBox(s.Options.UseSlideOffset).Extend(sweepVelocity)) {
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

	if !s.simulateMovement(state) {
		state.SetVel(mgl32.Vec3{})
		state.SwimWaterGraceTicks = 0
		state.StuckSpeedMultiplier = mgl32.Vec3{}
		return SimulationOutcomeUnloadedChunk
	}
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
	} else if !startFlag && !stopFlag && !state.ServerSprintApplied && state.ServerSprint != state.Sprinting {
		if state.ServerSprint {
			state.Sprinting = true
		} else {
			state.Sprinting = false
		}
	} else if startFlag {
		state.Sprinting = true
		needsSpeedAdjusted = isModernSprint
	} else if stopFlag {
		state.Sprinting = false
		needsSpeedAdjusted = isModernSprint && !state.ServerUpdatedSpeed
	}
	state.ServerSprintApplied = true

	if needsSpeedAdjusted {
		state.ServerUpdatedSpeed = false
		state.MovementSpeed = state.DefaultMovementSpeed
		if state.Sprinting {
			state.MovementSpeed *= 1.3
		}
	}
	state.AirSpeed = effectiveAirSpeed(state)

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
	state.StoppedSwimmingThisTick = wasSwimming && input.StopSwimming
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
	state.JumpHeight = state.JumpStrength
	if state.JumpHeight <= 0 {
		state.JumpHeight = DefaultJumpHeight
	}
	if s.Effects != nil {
		if amp, ok := s.Effects.GetEffect(packet.EffectJumpBoost); ok {
			state.JumpHeight += float32(amp+1) * 0.1
		}
	}

	if !state.PressingJump {
		state.JumpDelay = 0
	}
	if state.Gravity == 0 {
		state.Gravity = NormalGravity
	}
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
	state.AirSpeed = effectiveAirSpeed(state)
}

// effectiveAirSpeed returns the air acceleration for the current sprint state.
// Vanilla picks from a fixed pair here, so movement effects must not reach it
// the way they reach the ground and liquid speeds.
func effectiveAirSpeed(state *MovementState) float32 {
	if state.Sprinting {
		return SprintAirSpeed
	}
	return WalkAirSpeed
}

func (s *Simulator) tickState(state *MovementState, advanceTeleport bool) {
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
	state.KnockbackPending = false
	if advanceTeleport && state.TicksSinceTeleport < math32.MaxUint64 {
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
	state.StoppedSwimmingThisTick = false
}

func (s *Simulator) simulateMovement(state *MovementState) bool {
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
	defer func() {
		if inWater {
			state.SwimWaterGraceTicks = grace
		} else if state.SwimWaterGraceTicks > 0 {
			state.SwimWaterGraceTicks--
		}
	}()
	// The launch is a one-shot impulse; the remaining Riptide ticks decay
	// through ordinary travel rather than a dedicated movement mode.
	if !state.Flying && s.attemptRiptide(state, inWater, s.riptideHeadInWater(state)) {
		if debugf := s.Options.Debugf; debugf != nil {
			debugf("riptide launch applied: %v", state.Vel)
		}
	}

	// Observed lava takes precedence over retained water evidence.
	waterTravel := inWater ||
		(state.Swimming && state.SwimWaterGraceTicks > 0 && len(lavaBlocks) == 0)

	if !state.Flying && (waterTravel || len(lavaBlocks) != 0) {
		if applied := attemptKnockback(state); applied && s.Options.Debugf != nil {
			s.Options.Debugf("knockback applied in liquid: %v", state.Vel)
		}
		if waterTravel {
			if state.Gliding {
				state.Gliding = false
				state.GlideBoostTicks = 0
			}
			s.applyLiquidFlow(state, waterBlocks, liquidWater)
			if !s.simulateLiquidTravel(state, liquidWater, inWater) {
				return false
			}
		} else {
			s.applyLiquidFlow(state, lavaBlocks, liquidLava)
			if !s.simulateLiquidTravel(state, liquidLava, true) {
				return false
			}
		}
		return true
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
			if !s.movementSweepLoaded(state) {
				return false
			}
			oldVel := state.Vel
			oldY := state.Pos.Y()
			s.tryCollisions(state, false)
			stopRiptideOnBlockCollision(state)
			updateFallDistance(state, oldY)
			if debugf := s.Options.Debugf; debugf != nil {
				debugf("(glide) oldVel=%v, collisions=%v diff=%v", oldVel, state.Vel, state.Vel.Sub(state.Client.Vel))
			}
			state.SetMov(state.Vel)
			if stuckMovement {
				state.SetVel(mgl32.Vec3{})
			}
			s.applyInsideBlockEffects(state)
			s.applyBubbleColumns(state)
			return true
		}

		state.Gliding = false
		if debugf := s.Options.Debugf; debugf != nil {
			debugf("cannot allow glide (onGround=%v hasElytra=%v)", state.OnGround, hasElytra)
		}
	}

	var clientJumpPrevented bool
	if applied := attemptKnockback(state); applied && s.Options.Debugf != nil {
		s.Options.Debugf("knockback applied: %v", state.Vel)
	}
	if debugf := s.Options.Debugf; debugf != nil {
		debugf("blockUnder=%s, blockFriction=%v, speed=%v", BlockName(blockUnder), blockFriction, moveRelativeSpeed)
	}
	moveRelative(state, moveRelativeSpeed)
	if debugf := s.Options.Debugf; debugf != nil {
		debugf("moveRelative force applied (vel=%v)", state.Vel)
	}
	if jumped := s.attemptJump(state, &clientJumpPrevented); jumped && s.Options.Debugf != nil {
		s.Options.Debugf("jump force applied (sprint=%v): %v", state.Sprinting, state.Vel)
	}
	insideSemantics := s.blockMovementSemantics(s.blockAtPos(posFromVec3(state.Pos)))
	if insideSemantics.Traversal == movementblock.TraversalNone && state.SupportingBlockPos != nil {
		supportingSemantics := s.blockMovementSemantics(s.blockAtPos(*state.SupportingBlockPos))
		if supportingSemantics.Traversal == movementblock.TraversalScaffolding {
			insideSemantics.Traversal = supportingSemantics.Traversal
		}
	}
	leatherBoots := s.Equipment != nil && s.Equipment.WearingLeatherBoots()
	scaffoldDescend := applyAscendableMovement(state, insideSemantics.Traversal, leatherBoots)

	nearClimbable := s.climbableContact(state, insideSemantics.Climbable)
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
		if debugf := s.Options.Debugf; debugf != nil {
			debugf("added climb velocity: %v (collided=%v effectiveJumping=%v)", newVel, state.CollideX || state.CollideZ, state.EffectiveJumping)
		}
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
		if debugf := s.Options.Debugf; debugf != nil {
			debugf("web force applied (vel=%v)", newVel)
		}
	}

	stuckMovement := applyStuckSpeedMultiplier(state)
	if !s.movementSweepLoaded(state) {
		return false
	}
	s.avoidEdge(state)

	oldVel := state.Vel
	oldOnGround := state.OnGround
	oldY := state.Pos.Y()
	s.tryCollisions(state, clientJumpPrevented)
	stopRiptideOnBlockCollision(state)
	updateFallDistance(state, oldY)
	if scaffoldDescend || nearClimbable {
		state.FallDistance = 0
	}

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
		if debugf := s.Options.Debugf; debugf != nil {
			debugf("walkOnBlock: y changed, skipping block walk effects")
		}
	}

	state.SetMov(state.Vel)
	if stuckMovement {
		state.SetVel(mgl32.Vec3{})
		oldVel = mgl32.Vec3{}
	}
	s.setPostCollisionMotion(state, oldVel, oldOnGround, blockUnder)

	if inCobweb {
		if debugf := s.Options.Debugf; debugf != nil {
			debugf("post-move cobweb force applied (0 vel)")
		}
		state.SetVel(mgl32.Vec3{})
	}

	newVel := state.Vel
	if !scaffoldDescend {
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
	}
	newVel[0] *= blockFriction
	newVel[2] *= blockFriction
	state.SetVel(newVel)
	s.applyInsideBlockEffects(state)
	s.applyBubbleColumns(state)
	return true
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
	if state.PendingTeleports > 0 {
		// QueueTeleport marks TeleportPending before this path, which keeps an
		// explicitly queued origin distinct from legacy callers that only set
		// PendingTeleports and TeleportPos.
		if state.TeleportPending || state.PendingTeleportPos != (mgl32.Vec3{}) {
			state.TeleportPos = state.PendingTeleportPos
		}
		state.TeleportPending = true
	}
	if !state.HasTeleport() {
		return false
	}

	if !state.TeleportIsSmoothed {
		state.SetPos(state.TeleportPos)
		state.SetVel(mgl32.Vec3{})
		state.JumpDelay = 0
		state.TeleportPending = false
		if state.PendingTeleports > 0 {
			state.PendingTeleports--
		}
		if state.PendingTeleports == 0 {
			state.PendingTeleportPos = mgl32.Vec3{}
		}
		state.TicksSinceTeleport = teleportCompleteTick(state.TeleportCompletionTicks)
		return true
	}

	posDelta := state.TeleportPos.Sub(state.Pos)
	remaining := state.RemainingTeleportTicks()
	if remaining < int(^uint(0)>>1) {
		remaining++
	}
	newPos := state.Pos.Add(posDelta.Mul(1.0 / float32(remaining)))
	state.SetPos(newPos)
	state.JumpDelay = 0
	if remaining == 1 {
		state.TeleportPending = false
		if state.PendingTeleports > 0 {
			state.PendingTeleports--
		}
		if state.PendingTeleports == 0 {
			state.PendingTeleportPos = mgl32.Vec3{}
		}
		state.TicksSinceTeleport = teleportCompleteTick(state.TeleportCompletionTicks)
	}
	return true
}

// teleportCompleteTick returns the first tick after a teleport's completion
// window without overflowing the counter.
func teleportCompleteTick(completionTicks uint64) uint64 {
	if completionTicks == math32.MaxUint64 {
		return completionTicks
	}
	return completionTicks + 1
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
	if vel[1] < 0 && lookHz > GlideHorizontalLookEpsilon {
		yAccel := vel[1] * -0.1 * sqrPitchCos
		vel[1] += yAccel
		vel[0] += lookX * yAccel / lookHz
		vel[2] += lookZ * yAccel / lookHz
	}
	if pitch < 0 && lookHz > GlideHorizontalLookEpsilon {
		yAccel := velHz * -pitchSin * 0.04
		vel[1] += yAccel * 3.2
		vel[0] -= lookX * yAccel / lookHz
		vel[2] -= lookZ * yAccel / lookHz
	}
	if lookHz > GlideHorizontalLookEpsilon {
		vel[0] += (lookX/lookHz*velHz - vel[0]) * 0.1
		vel[2] += (lookZ/lookHz*velHz - vel[2]) * 0.1
	}

	if state.GlideBoostTicks > 0 {
		oldVel := vel
		vel[0] += (lookX * 0.1) + (((lookX * 1.5) - vel[0]) * 0.5)
		vel[1] += (lookY * 0.1) + (((lookY * 1.5) - vel[1]) * 0.5)
		vel[2] += (lookZ * 0.1) + (((lookZ * 1.5) - vel[2]) * 0.5)
		if debugf := s.Options.Debugf; debugf != nil {
			debugf("applied glide boost (old=%v new=%v)", oldVel, vel)
		}
		if debugf := s.Options.Debugf; debugf != nil {
			debugf("glide boost dirVec=[%f %f %f]", lookX, lookY, lookZ)
		}
	}

	vel[0] *= 0.99
	vel[1] *= 0.98
	vel[2] *= 0.99

	state.SetVel(vel)
}

func (s *Simulator) walkOnBlock(state *MovementState, blockUnder world.Block) {
	if !state.OnGround || state.Sneaking {
		if debugf := s.Options.Debugf; debugf != nil {
			debugf("walkOnBlock: conditions not met (onGround=%v sneaking=%v)", state.OnGround, state.Sneaking)
		}
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
	if debugf := s.Options.Debugf; debugf != nil {
		debugf("walkOnBlock: oldVel=%v newVel=%v", oldVel, newVel)
	}
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
		newVel[1] = BedBounceMultiplier * old.Y()
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
		if state.Jumping && s.Options.Debugf != nil {
			s.Options.Debugf("rejected jump from client (onGround=%v jumpDelay=%d)", state.OnGround, state.JumpDelay)
		}
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
			if debugf := s.Options.Debugf; debugf != nil {
				debugf("jump determined to be blocked")
			}
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
	if !s.movementAreaLoaded(collisionBB.Extend(jumpVel)) {
		return false
	}
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
	if debugf := s.Options.Debugf; debugf != nil {
		debugf("Y-collision non-step=%v /w penetration=%v (oneWay=%v)", yVel, penetration, useOneWayCollisions)
	}

	for i := len(bbList) - 1; i >= 0; i-- {
		xVel = BBClipCollide(bbList[i], collisionBB, xVel, useOneWayCollisions, &penetration)
	}
	collisionBB = collisionBB.Translate(xVel)
	if debugf := s.Options.Debugf; debugf != nil {
		debugf("(X) hz-collision non-step=%v /w penetration=%v (oneWay=%v)", xVel, penetration, useOneWayCollisions)
	}

	for i := len(bbList) - 1; i >= 0; i-- {
		zVel = BBClipCollide(bbList[i], collisionBB, zVel, useOneWayCollisions, &penetration)
	}
	collisionBB = collisionBB.Translate(zVel)
	if debugf := s.Options.Debugf; debugf != nil {
		debugf("(Z) hz-collision non-step=%v /w penetration=%v (oneWay=%v)", zVel, penetration, useOneWayCollisions)
	}

	collisionVel := yVel.Add(xVel).Add(zVel)
	collisionPos := mgl32.Vec3{
		(collisionBB.Min().X() + collisionBB.Max().X()) * 0.5,
		collisionBB.Min().Y(),
		(collisionBB.Min().Z() + collisionBB.Max().Z()) * 0.5,
	}
	if debugf := s.Options.Debugf; debugf != nil {
		debugf("endCollisionVel=%v endCollisionPos=%v", collisionVel, collisionPos)
	}

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
		if debugf := s.Options.Debugf; debugf != nil {
			debugf("stepYVel=%v", stepYVel)
		}

		for _, blockBox := range bbList {
			stepXVel = BBClipCollide(blockBox, stepBB, stepXVel, useOneWayCollisions, nil)
		}
		stepBB = stepBB.Translate(stepXVel)
		if debugf := s.Options.Debugf; debugf != nil {
			debugf("stepXVel=%v", stepXVel)
		}

		for _, blockBox := range bbList {
			stepZVel = BBClipCollide(blockBox, stepBB, stepZVel, useOneWayCollisions, nil)
		}
		stepBB = stepBB.Translate(stepZVel)
		if debugf := s.Options.Debugf; debugf != nil {
			debugf("stepZVel=%v", stepZVel)
		}

		inverseYStepVel := stepYVel.Mul(-1)
		for _, blockBox := range bbList {
			inverseYStepVel = BBClipCollide(blockBox, stepBB, inverseYStepVel, useOneWayCollisions, nil)
		}
		stepBB = stepBB.Translate(inverseYStepVel)
		stepYVel = stepYVel.Add(inverseYStepVel)
		if debugf := s.Options.Debugf; debugf != nil {
			debugf("inverseYStepVel=%v", inverseYStepVel)
		}

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
		if debugf := s.Options.Debugf; debugf != nil {
			debugf("endStepVel=%v endStepPos=%v", stepVel, stepPos)
		}
		if debugf := s.Options.Debugf; debugf != nil {
			debugf("newBBList count: %d", newBBListCount)
		}

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
				completedStep = true
				if useSlideOffset {
					slideOffset := state.SlideOffset.Mul(SlideOffsetMultiplier)
					slideOffset[1] += stepVel.Y()
					state.SlideOffset = slideOffset
				}
				if debugf := s.Options.Debugf; debugf != nil {
					debugf("step successful")
				}
			} else {
				if debugf := s.Options.Debugf; debugf != nil {
					debugf("step failed (client rejection) [clientPos=%v collisionPos=%v stepPos=%v]", state.Client.Pos, collisionPos, stepPos)
				}
			}
		} else {
			if debugf := s.Options.Debugf; debugf != nil {
				debugf("step failed")
			}
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
			if debugf := s.Options.Debugf; debugf != nil {
				debugf("applying slideOffset, able to subtract endPos.y this frame by %f", state.SlideOffset.Y())
			}
		} else {
			if debugf := s.Options.Debugf; debugf != nil {
				debugf("using slide offset, RESETTING slide offset vector")
			}
			state.SlideOffset = mgl32.Vec2{}
		}
	}
	state.SetPos(endPos)

	yCollision = math32.Abs(currVel.Y()-collisionVel.Y()) >= 1e-5
	state.CollideX = math32.Abs(currVel.X()-collisionVel.X()) >= 1e-5
	state.CollideY = yCollision
	state.CollideZ = math32.Abs(currVel.Z()-collisionVel.Z()) >= 1e-5

	state.OnGround = (yCollision && currVel.Y() < 0) ||
		(onGround && !yCollision && math32.Abs(currVel.Y()) <= 1e-5) ||
		(clientJumpPrevented && onGround) || completedStep
	s.checkSupportingBlockPos(state, useSlideOffset, currVel)
	state.SetVel(collisionVel)
	if debugf := s.Options.Debugf; debugf != nil {
		debugf("clientVel=%v clientPos=%v", state.Client.Mov, state.Client.Pos)
	}
	if debugf := s.Options.Debugf; debugf != nil {
		debugf("finalVel=%v finalPos=%v", collisionVel, state.Pos)
	}
	if debugf := s.Options.Debugf; debugf != nil {
		debugf("(client) hzCollision=%v yCollision=%v", state.Client.HorizontalCollision, state.Client.VerticalCollision)
	}
	if debugf := s.Options.Debugf; debugf != nil {
		debugf("(server) xCollision=%v yCollision=%v zCollision=%v", state.CollideX, state.CollideY, state.CollideZ)
	}
}

func (s *Simulator) avoidEdge(state *MovementState) {
	w := s.World
	if w == nil {
		return
	}
	if !state.Sneaking || !state.OnGround || state.Vel.Y() > 0 {
		if debugf := s.Options.Debugf; debugf != nil {
			debugf(
				"avoidEdge: conditions not met (sneaking=%v onGround=%v yVel=%v)",
				state.Sneaking,
				state.OnGround,
				state.Vel.Y(),
			)
		}
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
	if debugf := s.Options.Debugf; debugf != nil {
		debugf("(avoidEdge): oldVel=%v newVel=%v", oldVel, newVel)
	}
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

// climbableContact reports ladder/vine contact. Vanilla tests the single block
// cell the player stands in, which insideClimbable already resolves; an adapter
// overrides only when orientation lives outside the block registry.
func (s *Simulator) climbableContact(state *MovementState, insideClimbable bool) bool {
	if s.World == nil {
		return insideClimbable
	}
	if provider, ok := s.World.(ClimbableContactProvider); ok {
		return provider.HasClimbableContact(state.BoundingBox(s.Options.UseSlideOffset))
	}
	return insideClimbable
}

// movementAreaLoaded reports whether the complete movement volume is known.
func (s *Simulator) movementAreaLoaded(aabb cube.BBox32) bool {
	if s.World == nil {
		return true
	}
	if provider, ok := s.World.(MovementAreaProvider); ok {
		return provider.IsMovementAreaLoaded(aabb)
	}
	minX, minZ, maxX, maxZ, ok := movementChunkRange(aabb)
	if !ok {
		return false
	}
	for chunkX := int64(minX); chunkX <= int64(maxX); chunkX++ {
		for chunkZ := int64(minZ); chunkZ <= int64(maxZ); chunkZ++ {
			if !s.World.IsChunkLoaded(int32(chunkX), int32(chunkZ)) {
				return false
			}
		}
	}
	return true
}

// movementSweepLoaded reports whether the world contains the displacement
// produced after all same-tick acceleration has been applied.
func (s *Simulator) movementSweepLoaded(state *MovementState) bool {
	return s.World == nil || s.movementAreaLoaded(state.BoundingBox(s.Options.UseSlideOffset).Extend(state.Vel))
}

const (
	maxMovementChunkSpan  int64   = 256
	minMovementBlockCoord float32 = -2147483648
	maxMovementBlockCoord float32 = 2147483520
)

// movementChunkRange returns a bounded chunk range for a movement volume.
func movementChunkRange(aabb cube.BBox32) (minX, minZ, maxX, maxZ int32, ok bool) {
	min, max := aabb.Min(), aabb.Max()
	minBlockX, minBlockZ := math32.Floor(min.X()), math32.Floor(min.Z())
	maxBlockX, maxBlockZ := math32.Ceil(max.X())-1, math32.Ceil(max.Z())-1
	for _, value := range []float32{minBlockX, minBlockZ, maxBlockX, maxBlockZ} {
		if !finiteFloat(value) || value < minMovementBlockCoord || value > maxMovementBlockCoord {
			return 0, 0, 0, 0, false
		}
	}

	minX, minZ = int32(minBlockX)>>4, int32(minBlockZ)>>4
	maxX, maxZ = int32(maxBlockX)>>4, int32(maxBlockZ)>>4
	spanX := int64(maxX) - int64(minX) + 1
	spanZ := int64(maxZ) - int64(minZ) + 1
	if spanX <= 0 || spanZ <= 0 || spanX > maxMovementChunkSpan || spanZ > maxMovementChunkSpan {
		return 0, 0, 0, 0, false
	}
	return minX, minZ, maxX, maxZ, true
}

func (s *Simulator) checkSupportingBlockPos(state *MovementState, useSlideOffset bool, vel mgl32.Vec3) {
	if !state.OnGround {
		state.SupportingBlockPos = nil
		return
	}
	decBB := state.BoundingBox(useSlideOffset).ExtendTowards(cube.FaceDown, 1e-3)
	s.findSupportingBlock(state, decBB)
	if state.SupportingBlockPos == nil {
		decBB = decBB.Translate(mgl32.Vec3{-vel[0], 0, -vel[2]})
		s.findSupportingBlock(state, decBB)
	}
}

func (s *Simulator) findSupportingBlock(state *MovementState, bb cube.BBox32) {
	w := s.World
	if w == nil {
		state.SupportingBlockPos = nil
		return
	}
	if provider, ok := w.(MovementSupportProvider); ok {
		if pos, found := provider.SupportingBlock(bb, s.movementCollisionContext(state)); found {
			state.SupportingBlockPos = &pos
		} else {
			state.SupportingBlockPos = nil
		}
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
			if BBHasZeroVolume(box) {
				continue
			}
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
		return filteredCollisionBoxes(provider.GetMovementBBoxes(aabb, s.movementCollisionContext(state)))
	}
	return filteredCollisionBoxes(s.World.GetNearbyBBoxes(aabb))
}

// movementCollisionContext builds the dynamic collision context for state.
func (s *Simulator) movementCollisionContext(state *MovementState) MovementCollisionContext {
	return MovementCollisionContext{
		Position:     [3]float32(state.Pos),
		Sneaking:     state.Sneaking,
		Descending:   state.PressingDescend,
		WantDown:     state.WantDown,
		LeatherBoots: s.Equipment != nil && s.Equipment.WearingLeatherBoots(),
	}
}

// filteredCollisionBoxes removes invalid boxes without reordering valid ones.
func filteredCollisionBoxes(boxes []cube.BBox32) []cube.BBox32 {
	for i, box := range boxes {
		if !BBHasZeroVolume(box) {
			continue
		}
		filtered := make([]cube.BBox32, 0, len(boxes)-1)
		filtered = append(filtered, boxes[:i]...)
		for _, remaining := range boxes[i+1:] {
			if !BBHasZeroVolume(remaining) {
				filtered = append(filtered, remaining)
			}
		}
		return filtered
	}
	return boxes
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
	return len(filteredCollisionBoxes(s.World.GetNearbyBBoxes(aabb))) > 0
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
	return s.movementAreaLoaded(state.BoundingBox(s.Options.UseSlideOffset))
}

func setSwimmingPoseFlags(state *MovementState) {
	state.Sneaking = false
	state.Crawling = false
	state.Size[1] = state.StandingHeight
}

func (s *Simulator) restorePoseAfterSwimming(state *MovementState, collisionsAvailable bool) {
	if !collisionsAvailable {
		return
	}
	if s.canFitHeight(state, state.StandingHeight) {
		state.Sneaking = false
		state.Crawling = false
		state.Size[1] = state.StandingHeight
		return
	}
	if s.canFitHeight(state, state.SneakingHeight) {
		state.Sneaking = true
		state.Crawling = false
		state.Size[1] = state.SneakingHeight
		return
	}
	state.Sneaking = false
	state.Crawling = true
	state.Size[1] = state.CrawlingHeight
}
