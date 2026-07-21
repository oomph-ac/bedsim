# bedsim

Server-side Minecraft Bedrock movement simulation library for Go.

`bedsim` replicates the Bedrock client's movement physics on the server, producing authoritative position and velocity values that can be compared against client-reported state. It covers collisions, stepping, edge avoidance, liquids and currents, swimming, bubble columns, Riptide, crawling, gliding, movement enchantments, movement-sensitive blocks, and teleportation.

Original code was written by [ethaniccc](https://github.com/ethaniccc) in [oomph](https://github.com/oomph-ac/oomph) and has been ported over into this library.
The liquid movement physics were ported from [oomph#145](https://github.com/oomph-ac/oomph/pull/145) by [NopeNotDark](https://github.com/NopeNotDark).

## Installation

```
go get github.com/oomph-ac/bedsim
```

## Setup

Before calling any bedsim function (`BlockName`, `BlockClimbable`, `BlockFriction`, or running a simulation tick), you **must** finalize the Dragonfly block registry used by your world. Without this, block runtime/hash lookups may be incomplete and `BlockName` can cache incorrect mappings permanently.

```go
import "github.com/df-mc/dragonfly/server/world"

var blocks = world.NewBlockRegistry()

func init() {
    // Register custom blocks/states before finalizing.
    // blocks.RegisterBlock(...)
    // blocks.RegisterBlockState(...)

    blocks.Finalize()
}
```

```go
conf := server.DefaultConfig()
conf.Blocks = blocks

sessionConf := session.Config{BlockRegistry: blocks}

ch := chunk.New(blocks, world.Overworld.Range())
decoded, err := chunk.NetworkDecode(blocks, payload, subChunkCount, world.Overworld.Range())
```

If you use only vanilla blocks, `world.DefaultBlockRegistry` is still valid after it has been finalized by Dragonfly configuration setup or by an explicit `world.DefaultBlockRegistry.Finalize()` call. If you register custom blocks, do so **before** calling `Finalize`.

## Usage

Implement provider adapters to bridge your world and player systems:

```go
sim := bedsim.Simulator{
    World:          myWorldProvider,      // block lookups, collisions, chunk-loaded checks
    BlockSemantics: myBlockSemantics,     // optional: per-world names, friction, climbability
    Liquids:        myLiquidProvider,     // second block layer (waterlogged blocks)
    Effects:        myEffectsProvider,    // jump boost, levitation, slow falling
    Inventory:      myInventoryProvider,  // elytra equipped check
    Equipment:      myEquipmentProvider,  // movement enchantments and leather boots
    Options: bedsim.SimulationOptions{
        Mode:                        bedsim.SimulationModeAuthoritative,
        PositionCorrectionThreshold: 0.5,
        VelocityCorrectionThreshold: 0.5,
        RequireLiquidLayer:          true,
    },
}

result := sim.Simulate(&state, input)
if result.NeedsCorrection {
    // server and client have diverged
}
```

Set `BlockSemantics` when movement behavior must come from a per-world block
registry or custom block data instead of bedsim's Dragonfly-backed defaults.
Custom friction values must be finite and positive; invalid values fall back to
Dragonfly defaults.

Implement `DepthStriderProvider` on the inventory adapter when Depth Strider
should affect water movement.

> **Set `Liquids` if you simulate authoritatively.** Bedrock stores waterlogged
> blocks as a liquid in the *second* block layer. Without a `LiquidProvider`,
> bedsim only sees liquids returned by `WorldProvider.Block`, so waterlogged
> blocks look dry and the player is simulated with air physics inside them.
> Check `sim.HasLiquidLayer()` at startup, or set
> `SimulationOptions.RequireLiquidLayer` to make the simulator return
> `SimulationOutcomeUnreliable` rather than silently mis-simulate.
>
> If `Liquids` is nil and `World` itself implements `LiquidProvider`, that is
> used instead. This keeps pre-existing integrations working, but it is
> discovered by type assertion, so a signature typo degrades silently — prefer
> the explicit field.

### Optional movement capabilities

`WorldProvider` is the only required world interface. A world may additionally
implement `BubbleColumnProvider` for upward/downward columns and
`MovementCollisionProvider` for player-dependent collision shapes such as
scaffolding and powder snow. Dynamic collision resolution receives sneak and
descend intent plus leather-boots state.

`MovementEquipmentProvider` supplies Depth Strider, Soul Speed, Swift Sneak,
Riptide, and leather-boots checks. The legacy `DepthStriderProvider` inventory
extension remains a fallback when the equipment provider reports no Depth
Strider level. `EffectsProvider` also controls Weaving-aware cobweb movement.

Pose changes update `MovementState.Size`. Set `StandingHeight`,
`SneakingHeight`, or `CrawlingHeight` when using non-vanilla dimensions; zero
values preserve the current standing height and use vanilla crouch/crawl
heights.

Movement-sensitive block behavior includes honey blocks, sweet berry bushes,
powder snow, scaffolding, cobwebs (including Weaving), soul sand with Soul
Speed, slime blocks, beds, climbables, fences/walls, and per-block friction.
Dynamic collision behavior still depends on the world adapter returning the
correct shapes for the current block state.

### Liquid movement

When the player's hitbox touches water or lava (and the player is not flying),
bedsim runs liquid travel instead of the normal ground/air step. Water takes
priority when both are touched, and a player whose client reports the swimming
pose keeps water travel even after leaving the water blocks.

Liquid travel covers: per-liquid acceleration and drag, liquid gravity,
levitation, Depth Strider, the dolphin-boost swim multiplier, pitch-steered
swim travel with a surface clamp, flow from depth gradients including falling
liquid and solid faces, and an exit probe that boosts the player over a ledge.

Callers feed these `InputState` fields from the client's input flags:

| Field | Client input flag |
| --- | --- |
| `StartSwimming` / `StopSwimming` | `StartSwimming` / `StopSwimming` |
| `WantDown` / `WantDownSlow` | `WantDown` / `WantDownSlow` |
| `AutoJumpingInWater` | `AutoJumpingInWater` |
| `AscendBlock` | `AscendBlock` |

`Jumping` remains edge-triggered from `StartJumping` and arms a ground jump.
`EffectiveJumping` is derived from the held jump key, `AutoJumpingInWater`, and
`AscendBlock`, and drives liquid ascent and ladder climbing.

These `MovementState` fields tune liquid physics; each is optional and falls
back to a documented default when left at its zero value:

- `UnderwaterMovementSpeed` (default `0.02`)
- `LavaMovementSpeed` (default `0.02`)
- `SwimSpeedMultiplier` (default `1`; a dolphin boost sets it to `2`)

Set `DolphinBoostTicks` when the client receives a dolphin boost. `Simulate`
counts it down and restores `SwimSpeedMultiplier` to its default on expiry;
callers using `SimulateState` must manage that lifecycle themselves.

While the swim pose is active the bounding box collapses to a width-sized cube,
matching the client's swim pose. This affects collisions and liquid detection,
not just liquid travel. The pose requires both `Swimming` and recent
server-observed water contact — see `MovementState.SwimPose` and the divergence
note below.

#### Divergences from the upstream source

Two behaviors intentionally differ from oomph PR #145.

**Swim water-travel grace (security hardening).** Upstream keys water travel on
`touchingWater || Swimming`, trusting the client's swimming flag because the
surrounding anticheat validates it. A standalone simulator cannot. Left as-is,
a client that latches the flag gets two things it should not: *zero gravity*
forever in open air with no correction raised, and a server-side hitbox
shrunk from 1.8 to 0.6, letting it fit through gaps a standing player cannot.

bedsim gates **both** the water-travel branch and the swim pose on recent
server-observed water contact, bounded by
`SimulationOptions.SwimWaterGraceTicks` (default `DefaultSwimWaterGraceTicks`,
10). The budget refills on every tick the hitbox actually touches water, is
clamped to the configured bound before anything reads it, and is cleared on any
frame that was not simulated — unreliable, unloaded chunk, immobile, or
teleport. A negative value requires real water contact on every tick. Lava the
player is actually standing in takes priority over a retained water grace.

Because the budget is applied at the start of a tick and decremented at the end,
it is constant for the whole tick, so collision, liquid detection and exit
probing always agree on one hitbox. The cost is that entering water adopts the
swim pose one tick later than upstream, erring toward the larger box.

Players genuinely in water are unaffected. Two residual limits are worth
knowing: a client that reaches real water once every `SwimWaterGraceTicks` ticks
sustains water travel at up to a 10:1 duty cycle, so the guard bounds hovering
to the neighbourhood of actual water rather than eliminating it; and the pose
lag above is a deliberate one-tick divergence from upstream. Lower
`SwimWaterGraceTicks` to tighten both.

**Impulse clamps.** Upstream removed `MaxSneakImpulse` and `MaxConsumingImpulse`
in this PR, clamping the move vector to `[-1, 1]` instead. bedsim keeps both by
default, because they are public API affecting all movement and removing them
would be a breaking change outside liquid scope. Set
`SimulationOptions.UpstreamImpulseClamping` to opt into upstream's behavior.

### Simulation modes

- `Simulate` — applies client input, runs physics, advances tick counters, and returns the result. Use this when bedsim owns the full tick lifecycle.
- `SimulateState` — runs physics on the current state without applying input or ticking counters. Use this when your caller handles input parsing and tick management externally.

### Correction modes

- `SimulationModeAuthoritative` — `NeedsCorrection` becomes true if position or velocity drift exceeds thresholds.
- `SimulationModePermissive` — only position drift can trigger `NeedsCorrection`.
- `SimulationModePassive` — never triggers `NeedsCorrection` (deltas are still reported).

## Simulation result

Each tick returns a `SimulationResult` containing:

- Authoritative `Position`, `Velocity`, and `Movement` vectors
- Collision flags (`CollideX`, `CollideY`, `CollideZ`, `OnGround`)
- `PositionDelta` / `VelocityDelta` — difference from client-reported values
- `NeedsCorrection` — whether deltas exceed configured thresholds
- `Outcome` — which simulation path was taken (normal, teleport, unreliable, unloaded chunk, immobile)
