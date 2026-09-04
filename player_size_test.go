package bedsim

import "testing"

func TestDefaultPlayerSizeMatchesPoseFallback(t *testing.T) {
	size := DefaultPlayerSize()
	if size.X() != DefaultPlayerWidth || size.Y() != DefaultPlayerHeight || size.Z() != 1 {
		t.Fatalf("DefaultPlayerSize() = %v, want (%v, %v, 1)", size, DefaultPlayerWidth, DefaultPlayerHeight)
	}

	var state MovementState
	state.ensurePoseHeights()
	if state.StandingHeight != DefaultPlayerHeight {
		t.Fatalf("zero-size standing height = %v, want DefaultPlayerHeight %v", state.StandingHeight, DefaultPlayerHeight)
	}
	if state.StandingHeight <= state.SneakingHeight || state.SneakingHeight <= state.CrawlingHeight {
		t.Fatalf("pose heights must strictly decrease: standing %v sneaking %v crawling %v", state.StandingHeight, state.SneakingHeight, state.CrawlingHeight)
	}
}
