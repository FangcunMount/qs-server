package evaluation

import (
	"os"
	"testing"
)

func TestEvaluationAcceptanceFailureProbeRequiresExplicitNarrowConfig(t *testing.T) {
	t.Setenv(evaluationAcceptanceFailureEnv, "")
	if err := os.Unsetenv(evaluationAcceptanceFailureEnv); err != nil {
		t.Fatal(err)
	}
	if gate, err := evaluationFailureGateFromEnv(nil); err != nil || gate != nil {
		t.Fatalf("absent probe=%v err=%v", gate, err)
	}
	for _, raw := range []string{
		"",
		`{"token":"probe-unique-0001","org_id":7,"testee_id":8,"model_code":"model","starts_at":"2026-09-30T12:00:00Z","expires_at":"2026-09-30T12:05:00Z","wildcard":true}`,
		`{"token":"probe-unique-0001"}{"token":"probe-unique-0002"}`,
	} {
		if _, err := parseEvaluationAcceptanceFailureScope(raw); err == nil {
			t.Fatalf("malformed probe accepted: %q", raw)
		}
	}
}
