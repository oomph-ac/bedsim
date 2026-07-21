package bedsim

import (
	"math"

	"github.com/df-mc/dragonfly/server/block/cube"
)

func applyInsideBlockMovement(state *MovementState, blockName string, weaving bool) {
	velocity := state.Vel
	switch blockName {
	case "minecraft:honey_block":
		velocity[0] *= 0.4
		velocity[1] = max(-0.12, velocity[1])
		velocity[2] *= 0.4
	case "minecraft:sweet_berry_bush":
		velocity[0] *= 0.8
		velocity[1] *= 0.75
		velocity[2] *= 0.8
	case "minecraft:powder_snow":
		velocity[0] *= 0.9
		velocity[1] *= 1.5
		velocity[2] *= 0.9
	case "minecraft:web", "minecraft:cobweb":
		xz, y := 0.25, 0.05
		if weaving {
			xz, y = 0.5, 0.25
		}
		velocity[0] *= xz
		velocity[1] *= y
		velocity[2] *= xz
	}
	state.SetVel(velocity)
}

func applyAscendableMovement(state *MovementState, blockName string, leatherBoots bool) {
	velocity := state.Vel
	switch blockName {
	case "minecraft:scaffolding":
		if state.PressingDescend {
			velocity[1] = -0.15
		} else if state.PressingAscend {
			velocity[1] = 0.15
		}
	case "minecraft:powder_snow":
		if state.PressingDescend {
			velocity[1] = -0.15
		} else if state.PressingAscend && leatherBoots {
			velocity[1] = 0.2
		}
	}
	state.SetVel(velocity)
}

func (s *Simulator) applyInsideBlockEffects(state *MovementState) {
	if s.World == nil {
		return
	}
	bb := state.BoundingBox(s.Options.UseSlideOffset)
	min, maxPoint := bb.Min(), bb.Max()
	for x := int(math.Floor(min.X())); x < int(math.Ceil(maxPoint.X())); x++ {
		for y := int(math.Floor(min.Y())); y < int(math.Ceil(maxPoint.Y())); y++ {
			for z := int(math.Floor(min.Z())); z < int(math.Ceil(maxPoint.Z())); z++ {
				pos := cube.Pos{x, y, z}
				if !bb.IntersectsWith(cube.Box(0, 0, 0, 1, 1, 1).Translate(pos.Vec3())) {
					continue
				}
				b := s.World.Block(pos)
				if s.blockAir(b) {
					continue
				}
				name := s.blockName(b)
				if name == "minecraft:web" || name == "minecraft:cobweb" {
					continue
				}
				applyInsideBlockMovement(state, name, false)
			}
		}
	}
}
