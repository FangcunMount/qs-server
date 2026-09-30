package interpretation

import (
	"os"
	"testing"
)

func TestAcceptanceFailureProbeIsOffUnlessExplicitlyConfigured(t *testing.T) {
	t.Setenv(acceptanceFailureProbeEnv, "")
	if err := os.Unsetenv(acceptanceFailureProbeEnv); err != nil {
		t.Fatal(err)
	}
	if gate, err := acceptanceFailureGateFromEnv(nil); err != nil || gate != nil {
		t.Fatalf("absent probe = %v, %v", gate, err)
	}
	t.Setenv(acceptanceFailureProbeEnv, "")
	if _, err := parseAcceptanceFailureScope(""); err == nil {
		t.Fatal("present but empty acceptance probe was accepted")
	}
	for _, raw := range []string{
		`{"token":"probe-unique-0001","org_id":7,"testee_id":8,"model_code":"model","starts_at":"2026-09-30T12:00:00Z","expires_at":"2026-09-30T12:05:00Z","wildcard":true}`,
		`{"token":"probe-unique-0001"}{"token":"probe-unique-0002"}`,
	} {
		if _, err := parseAcceptanceFailureScope(raw); err == nil {
			t.Fatalf("malformed probe accepted: %q", raw)
		}
	}
}
