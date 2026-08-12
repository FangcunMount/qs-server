package options

import (
	"testing"
	"time"
)

func TestInterpretationLeaseDefaultsNormalizeMissingDuration(t *testing.T) {
	t.Parallel()
	opts := (&InterpretationLeaseGovernanceOptions{}).normalized()
	if opts.RunDuration != 5*time.Minute {
		t.Fatalf("run duration = %s, want 5m", opts.RunDuration)
	}
}
