package cacheutil

import (
	"testing"
	"time"
)

func TestJitterTTLZeroRatioUnchanged(t *testing.T) {
	base := 180 * time.Second
	if got := JitterTTL(base, 0); got != base {
		t.Fatalf("JitterTTL() = %v, want %v", got, base)
	}
}

func TestJitterTTLSpreadWithinBounds(t *testing.T) {
	base := 100 * time.Millisecond
	ratio := 0.2
	max := base + time.Duration(float64(base)*ratio)
	for i := 0; i < 200; i++ {
		got := JitterTTL(base, ratio)
		if got < base || got > max {
			t.Fatalf("JitterTTL() = %v, want in [%v,%v]", got, base, max)
		}
	}
}

func TestJitterTTLProducesSpread(t *testing.T) {
	base := time.Second
	seen := make(map[time.Duration]struct{})
	for i := 0; i < 100; i++ {
		seen[JitterTTL(base, 0.5)] = struct{}{}
	}
	if len(seen) < 2 {
		t.Fatal("expected jitter to produce multiple distinct TTL values")
	}
}
