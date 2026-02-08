# Repository Guidelines

## Project Overview
`bedsim` is a server-side Minecraft Bedrock movement simulation library for Go.
It replicates the Bedrock client's movement physics (collisions, stepping, edge-avoidance, gliding, teleport smoothing) so callers can produce authoritative position/velocity values and/or detect divergence from client-reported state.

## What belongs in this repo
- **Movement physics**: collision resolution, stepping, impulses, gravity/jump, gliding, teleport handling.
- **Core types**: `Simulator`, `MovementState`, `InputState`, `SimulationOptions`, `SimulationResult`.
- **Provider interfaces**: small adapters in `interfaces.go` (`WorldProvider`, `EffectsProvider`, `InventoryProvider`) that let you plug in your world/effects/inventory systems.

## What does NOT belong in this repo
- **Proxy/session logic**: packet ordering, connection lifecycle, or per-feature behavior.
- **Pathfinding / waypoint planning**: “where to go next” lives elsewhere (oomph-ac/baritone)
- **Bot “humanization”**: turn-rate limiting, speed jitter, or per-module steering heuristics should live in the caller.

## Key APIs
- **Run a tick**: `(*Simulator).Simulate(state, input)` (applies input + ticks counters).
- **Physics only**: `(*Simulator).SimulateState(state)` (no input/counter updates).
- **Correction behavior**: `SimulationOptions.Mode` (`SimulationModeAuthoritative`, `SimulationModePermissive`, `SimulationModePassive`).
- **Outcomes**: `SimulationResult.Outcome` (`SimulationOutcomeNormal`, `SimulationOutcomeTeleport`, `SimulationOutcomeUnreliable`, `SimulationOutcomeUnloadedChunk`, `SimulationOutcomeImmobileOrNotReady`).

## Integration notes
- **Determinism**: keep simulation deterministic for a given `(state, input, world snapshot)`; avoid time-based logic inside simulation.
- **WorldProvider**: `IsChunkLoaded()` is used to bail out early. Treat unloaded chunks as “do not simulate forward”.
- **Collision consistency**: `Block()`, `BlockCollisions()`, and `GetNearbyBBoxes()` must agree or you’ll get phantom penetrations/unsticks.
- **No policy**: thresholds and “what to do when `NeedsCorrection` is true” are caller decisions.

## Development commands
- `go test ./...`
- `go test ./... -run TestName`
