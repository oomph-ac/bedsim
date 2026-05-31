# bedsim

Server-side Minecraft Bedrock movement simulation library for Go.

`bedsim` replicates the Bedrock client's movement physics (collisions, stepping, edge-avoidance, gliding, teleportation) on the server, producing authoritative position and velocity values that can be compared against client-reported state.

Original code was written by [ethaniccc](https://github.com/ethaniccc) in [oomph](https://github.com/oomph-ac/oomph) and has been ported over into this library.

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
    Effects:        myEffectsProvider,    // jump boost, levitation, slow falling
    Inventory:      myInventoryProvider,  // elytra equipped check
    Options: bedsim.SimulationOptions{
        Mode:                        bedsim.SimulationModeAuthoritative,
        PositionCorrectionThreshold: 0.5,
        VelocityCorrectionThreshold: 0.5,
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
