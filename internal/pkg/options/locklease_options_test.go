package options

import "testing"

func TestLockLeaseOptionsDefaultToAutomaticRenewal(t *testing.T) {
	opts := NewLockLeaseOptions()
	if !opts.RenewalEnabled {
		t.Fatal("lock lease renewal must be enabled by default")
	}
}
