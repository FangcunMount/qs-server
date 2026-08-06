package retryobservability

import "testing"

func TestAttemptClassification(t *testing.T) {
	if got := AttemptClassForOrigin("initial"); got != AttemptInitial {
		t.Fatalf("initial origin = %q", got)
	}
	for _, origin := range []string{"automatic", "manual", "force", "lease_recovery"} {
		if got := AttemptClassForOrigin(origin); got != AttemptRetry {
			t.Fatalf("origin %q = %q", origin, got)
		}
	}
	if AttemptClassForAttempt(1) != AttemptInitial || AttemptClassForAttempt(2) != AttemptRetry {
		t.Fatal("transport attempt classification drifted")
	}
}

func TestComponentLabelsStayBounded(t *testing.T) {
	tests := []struct {
		layer string
		input string
		want  string
	}{
		{LayerBusiness, "interpretation", "interpretation"},
		{LayerOutbox, "assessment-mysql-relay", "mysql"},
		{LayerOutbox, "report-mongo-relay", "mongo"},
		{LayerHold, "org-123-event-456", "retry_hold"},
		{LayerTransport, "qs-worker", "worker"},
		{LayerTransport, "projection-consumer-1", "apiserver_consumer"},
	}
	for _, test := range tests {
		if got := NormalizeComponent(test.layer, test.input); got != test.want {
			t.Fatalf("NormalizeComponent(%q, %q) = %q, want %q", test.layer, test.input, got, test.want)
		}
	}
}

func TestOriginAndOutcomeLabelsStayBounded(t *testing.T) {
	if got := normalizeOrigin(LayerBusiness, "org-123", AttemptRetry); got != "automatic" {
		t.Fatalf("unknown business retry origin = %q", got)
	}
	if got := normalizeOrigin(LayerOutbox, "manual", AttemptRetry); got != "na" {
		t.Fatalf("outbox origin = %q", got)
	}
	if got := normalizeOutcome("published"); got != OutcomeSuccess {
		t.Fatalf("published outcome = %q", got)
	}
	if got := normalizeOutcome("event-123-error"); got != OutcomeFailure {
		t.Fatalf("unknown outcome = %q", got)
	}
}
