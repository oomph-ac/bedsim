# bedsim

Server-side Minecraft Bedrock movement simulation library for Go.

`bedsim` replicates the Bedrock client's movement physics (collisions, stepping, edge-avoidance, gliding, teleportation) on the server, producing authoritative position and velocity values that can be compared against client-reported state.

## Installation

```
go get github.com/oomph-ac/bedsim
```

## Usage

Implement the three provider interfaces to bridge your world and player systems:

```go
sim := bedsim.Simulator{
    World:     myWorldProvider,     // block lookups, collisions, chunk-loaded checks
    Effects:   myEffectsProvider,   // jump boost, levitation, slow falling
    Inventory: myInventoryProvider, // elytra equipped check
    Options: bedsim.SimulationOptions{
        PositionCorrectionThreshold: 0.5,
        VelocityCorrectionThreshold: 0.5,
    },
}

result := sim.Simulate(&state, input)
if result.NeedsCorrection {
    // server and client have diverged
}
```

### Simulation modes

- `Simulate` — applies client input, runs physics, advances tick counters, and returns the result. Use this when bedsim owns the full tick lifecycle.
- `SimulateState` — runs physics on the current state without applying input or ticking counters. Use this when your caller handles input parsing and tick management externally.

## Simulation result

Each tick returns a `SimulationResult` containing:

- Authoritative `Position`, `Velocity`, and `Movement` vectors
- Collision flags (`CollideX`, `CollideY`, `CollideZ`, `OnGround`)
- `PositionDelta` / `VelocityDelta` — difference from client-reported values
- `NeedsCorrection` — whether deltas exceed configured thresholds
- `Outcome` — which simulation path was taken (normal, teleport, unreliable, unloaded chunk, immobile)
