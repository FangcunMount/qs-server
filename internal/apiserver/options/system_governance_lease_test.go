package options

import (
	"testing"
	"time"
)

func TestInterpretationLeaseWorstCaseRecoveryWindow(t *testing.T) {
	t.Parallel()

	opts := &InterpretationLeaseGovernanceOptions{
		RunDuration:             5 * time.Minute,
		ReconcileInterval:       10 * time.Second,
		ReconcileJitterFraction: 0.2,
	}
	if got := opts.WorstCaseRecoveryWindowAfterExpiry(); got != 12*time.Second {
		t.Fatalf("after expiry = %s, want 12s", got)
	}
	if got := opts.WorstCaseRecoveryWindowAfterCrash(); got != 5*time.Minute+12*time.Second {
		t.Fatalf("after crash = %s, want 5m12s", got)
	}
}

func TestInterpretationLeaseDefaultsNormalizeMissingValues(t *testing.T) {
	t.Parallel()

	opts := (&InterpretationLeaseGovernanceOptions{}).normalized()
	if opts.RunDuration != 5*time.Minute || opts.ReconcileInterval != 10*time.Second {
		t.Fatalf("defaults = duration:%s interval:%s", opts.RunDuration, opts.ReconcileInterval)
	}
}
