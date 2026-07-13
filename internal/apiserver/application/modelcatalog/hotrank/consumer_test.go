package hotrank

import (
	"context"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/hotrank"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcodec"
	"github.com/FangcunMount/qs-server/pkg/event"
)

type projectionCapture struct{ facts []hotrank.SubmissionFact }

func (p *projectionCapture) ProjectSubmission(_ context.Context, fact hotrank.SubmissionFact) error {
	p.facts = append(p.facts, fact)
	return nil
}

func TestEventConsumerProjectsAnswerSheetSubmitted(t *testing.T) {
	projection := &projectionCapture{}
	occurredAt := time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC)
	evt := event.Event[map[string]any]{
		BaseEvent: event.BaseEvent{ID: "evt-1", EventTypeValue: eventcatalog.AnswerSheetSubmitted, OccurredAtValue: occurredAt, AggregateTypeValue: "AnswerSheet", AggregateIDValue: "42"},
		Data:      map[string]any{"questionnaire_code": "Q-1", "submitted_at": occurredAt},
	}
	payload, err := eventcodec.EncodeDomainEvent(evt)
	if err != nil {
		t.Fatal(err)
	}
	if err := NewEventConsumer(projection)(context.Background(), eventcatalog.AnswerSheetSubmitted, payload); err != nil {
		t.Fatal(err)
	}
	if len(projection.facts) != 1 || projection.facts[0].EventID != "evt-1" || projection.facts[0].QuestionnaireCode != "Q-1" {
		t.Fatalf("facts = %#v", projection.facts)
	}
}

func TestEventConsumerFailsWhenProjectionIsUnavailable(t *testing.T) {
	err := NewEventConsumer(nil)(context.Background(), eventcatalog.AnswerSheetSubmitted, nil)
	if err == nil {
		t.Fatal("consumer error = nil, want unavailable projection error")
	}
}
