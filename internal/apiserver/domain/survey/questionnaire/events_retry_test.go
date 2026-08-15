package questionnaire

import (
	"context"
	"testing"
	"time"
)

func TestLifecycleEventMetadataIsStableAcrossTransactionCallbackRetries(t *testing.T) {
	occurredAt := time.Date(2026, 8, 15, 13, 0, 0, 123000000, time.UTC)
	ctx := WithLifecycleEventMetadata(context.Background(), occurredAt)
	lifecycle := NewLifecycle()

	first := createValidQuestionnaire("RETRY-STABLE", "Retry stable questionnaire")
	second := createValidQuestionnaire("RETRY-STABLE", "Retry stable questionnaire")
	if err := lifecycle.Publish(ctx, first); err != nil {
		t.Fatal(err)
	}
	if err := lifecycle.Publish(ctx, second); err != nil {
		t.Fatal(err)
	}

	firstEvents, secondEvents := first.Events(), second.Events()
	if len(firstEvents) != 1 || len(secondEvents) != 1 {
		t.Fatalf("event counts = %d/%d, want 1/1", len(firstEvents), len(secondEvents))
	}
	if firstEvents[0].EventID() != secondEvents[0].EventID() || firstEvents[0].EventID() == "" {
		t.Fatalf("event IDs = %q/%q, want one fixed non-empty ID", firstEvents[0].EventID(), secondEvents[0].EventID())
	}
	if !firstEvents[0].OccurredAt().Equal(occurredAt) || !secondEvents[0].OccurredAt().Equal(occurredAt) {
		t.Fatalf("event times = %s/%s, want %s", firstEvents[0].OccurredAt(), secondEvents[0].OccurredAt(), occurredAt)
	}
	firstChanged := firstEvents[0].(QuestionnaireChangedEvent)
	secondChanged := secondEvents[0].(QuestionnaireChangedEvent)
	if !firstChanged.Data.ChangedAt.Equal(occurredAt) || !secondChanged.Data.ChangedAt.Equal(occurredAt) {
		t.Fatal("payload changed_at is not retry-stable")
	}
}
