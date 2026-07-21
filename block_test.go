package bedsim

import (
	"math"
	"testing"

	"github.com/df-mc/dragonfly/server/world"
)

type namedBlock struct {
	name        string
	base, state uint64
	encodeCalls *int
}

func (b namedBlock) EncodeBlock() (string, map[string]any) {
	*b.encodeCalls++
	return b.name, nil
}

func (b namedBlock) Hash() (uint64, uint64) {
	return b.base, b.state
}

func (namedBlock) Model() world.BlockModel {
	return nil
}

func TestBlockNameCachesRawHashPair(t *testing.T) {
	var calls int
	b := namedBlock{name: "test:cached", base: 0xf32ca, state: 7, encodeCalls: &calls}

	if got := BlockName(b); got != b.name {
		t.Fatalf("first BlockName() = %q, want %q", got, b.name)
	}
	if got := BlockName(b); got != b.name {
		t.Fatalf("second BlockName() = %q, want %q", got, b.name)
	}
	if calls != 1 {
		t.Fatalf("EncodeBlock() called %d times, want 1", calls)
	}
}

func TestBlockNameDoesNotCacheUnknownHash(t *testing.T) {
	var calls int
	b := namedBlock{name: "test:unknown", state: math.MaxUint64, encodeCalls: &calls}

	BlockName(b)
	BlockName(b)
	if calls != 2 {
		t.Fatalf("EncodeBlock() called %d times, want 2", calls)
	}
}
