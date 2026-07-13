package cache

import (
	"bytes"
	"testing"
	"time"
)

func TestPolicyMergeAndPayloadContract(t *testing.T) {
	parent := Policy{Compress: PolicySwitchEnabled, Singleflight: PolicySwitchEnabled}
	merged := (Policy{Compress: PolicySwitchDisabled, Negative: PolicySwitchEnabled}).MergeWith(parent)
	if merged.Compress.Enabled(true) || !merged.Singleflight.Enabled(false) || !merged.Negative.Enabled(false) {
		t.Fatalf("merged policy = %+v", merged)
	}

	raw := []byte("payload large enough to exercise compression")
	compressed := (Policy{Compress: PolicySwitchEnabled}).CompressValue(raw)
	if bytes.Equal(raw, compressed) {
		t.Fatal("compressed payload equals raw payload")
	}
	if got := (Policy{}).DecompressValue(compressed); !bytes.Equal(got, raw) {
		t.Fatal("compressed payload did not round trip")
	}
}

func TestJitterTTLBounds(t *testing.T) {
	const ttl = time.Minute
	for i := 0; i < 100; i++ {
		got := JitterTTL(ttl, 0.2)
		if got < ttl || got > 72*time.Second {
			t.Fatalf("JitterTTL() = %s, want within [60s,72s]", got)
		}
	}
}
