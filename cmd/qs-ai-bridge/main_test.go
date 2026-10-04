package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestMQRetiredBridgeModesRejectBeforeStorageOrTransport(t *testing.T) {
	// Invalid storage configuration would produce a different error if borrowed.
	t.Setenv("QS_AI_BRIDGE_DSN", "invalid database configuration")
	for _, mode := range []string{"stage-start", "stage-change", "relay", "receive"} {
		t.Run(mode, func(t *testing.T) {
			var output bytes.Buffer
			err := runArgs(&output, []string{"-mode", mode})
			if err == nil || !strings.Contains(err.Error(), "legacy AI transport retired") || output.Len() != 0 {
				t.Fatalf("retired mode reached resources or wrote a result: %v", err)
			}
		})
	}
}
