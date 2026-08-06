package eventing

import (
	"testing"

	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
	"github.com/FangcunMount/qs-server/internal/pkg/retryobservability"
)

func TestRelayAttemptClassificationConsumesImmediateAttempt(t *testing.T) {
	tracker := NewOutboxAttemptTracker()
	pending := outboxport.PendingEvent{EventID: "event-1", AttemptCount: 0}
	tracker.Mark(pending.EventID)

	if got := relayAttemptClass(pending, tracker); got != retryobservability.AttemptRetry {
		t.Fatalf("first relay after immediate attempt = %q, want retry", got)
	}
	if got := relayAttemptClass(pending, tracker); got != retryobservability.AttemptInitial {
		t.Fatalf("consumed tracker entry = %q, want initial fallback", got)
	}
}

func TestRelayAttemptClassificationUsesDurableAttemptCount(t *testing.T) {
	pending := outboxport.PendingEvent{EventID: "event-2", AttemptCount: 1}
	if got := relayAttemptClass(pending, nil); got != retryobservability.AttemptRetry {
		t.Fatalf("durable retry = %q, want retry", got)
	}
}
