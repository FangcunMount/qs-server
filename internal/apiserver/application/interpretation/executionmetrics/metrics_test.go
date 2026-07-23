package executionmetrics

import (
	"testing"
	"time"
)

func TestMetricLabelsAreBounded(t *testing.T) {
	if got := normalizeBuilderIdentity(""); got != "unresolved" {
		t.Fatalf("empty builder identity = %q, want unresolved", got)
	}
	if got := normalizeResult("arbitrary-error"); got != ResultError {
		t.Fatalf("arbitrary result = %q, want error", got)
	}
	if got := normalizeResult(ResultSuccess); got != ResultSuccess {
		t.Fatalf("success result = %q, want success", got)
	}
	if got := nonNegativeSeconds(-time.Second); got != 0 {
		t.Fatalf("negative duration = %v, want 0", got)
	}
}
