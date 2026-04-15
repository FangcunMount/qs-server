package eventing

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/pkg/event"
)

type fakeCollector struct {
	events  []event.DomainEvent
	cleared bool
}

func (f *fakeCollector) Events() []event.DomainEvent {
	return f.events
}

func (f *fakeCollector) ClearEvents() {
	f.cleared = true
	f.events = nil
}

type fakePublisher struct {
	published []string
	failAt    map[string]error
}

func (f *fakePublisher) Publish(_ context.Context, evt event.DomainEvent) error {
	f.published = append(f.published, evt.EventType())
	if err, ok := f.failAt[evt.EventType()]; ok {
		return err
	}
	return nil
}

func (f *fakePublisher) PublishAll(ctx context.Context, events []event.DomainEvent) error {
	for _, evt := range events {
		if err := f.Publish(ctx, evt); err != nil {
			return err
		}
	}
	return nil
}

func TestPublishCollectedEvents_NilPublisherDoesNotClear(t *testing.T) {
	collector := &fakeCollector{
		events: []event.DomainEvent{
			event.New("assessment.submitted", "Assessment", "1", struct{}{}),
		},
	}
	missingCalled := false

	PublishCollectedEvents(context.Background(), nil, collector, func() {
		missingCalled = true
	}, nil)

	if !missingCalled {
		t.Fatal("expected missing publisher callback")
	}
	if collector.cleared {
		t.Fatal("expected collector to remain uncleared when publisher is nil")
	}
}

func TestPublishCollectedEvents_FailureDoesNotStopAndClearsAtEnd(t *testing.T) {
	firstErr := errors.New("boom")
	publisher := &fakePublisher{
		failAt: map[string]error{
			"assessment.submitted": firstErr,
		},
	}
	collector := &fakeCollector{
		events: []event.DomainEvent{
			event.New("assessment.submitted", "Assessment", "1", struct{}{}),
			event.New("report.generated", "Report", "2", struct{}{}),
		},
	}

	var failures []string
	PublishCollectedEvents(context.Background(), publisher, collector, nil, func(evt event.DomainEvent, err error) {
		failures = append(failures, evt.EventType()+":"+err.Error())
	})

	if len(publisher.published) != 2 {
		t.Fatalf("expected 2 publish attempts, got %d", len(publisher.published))
	}
	if len(failures) != 1 || failures[0] != "assessment.submitted:boom" {
		t.Fatalf("unexpected failures: %#v", failures)
	}
	if !collector.cleared {
		t.Fatal("expected collector to be cleared after publish loop")
	}
}

func TestCollectWrapsEvents(t *testing.T) {
	source := Collect(
		event.New("task.opened", "AssessmentTask", "1", struct {
			OpenAt time.Time `json:"open_at"`
		}{OpenAt: time.Now()}),
	)

	if got := len(source.Events()); got != 1 {
		t.Fatalf("expected 1 wrapped event, got %d", got)
	}
	source.ClearEvents()
	if got := len(source.Events()); got != 0 {
		t.Fatalf("expected wrapped events to clear, got %d", got)
	}
}
