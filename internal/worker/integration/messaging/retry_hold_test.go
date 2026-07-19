package messaging

import (
	"context"
	"errors"
	"testing"
	"time"

	basemessaging "github.com/FangcunMount/component-base/pkg/messaging"
)

type holdStoreStub struct {
	items          []*heldEvent
	claimed        int
	replayed       int
	replayFailures int
}

func (s *holdStoreStub) claim(context.Context, time.Time, time.Duration) (*heldEvent, error) {
	if s.claimed >= len(s.items) {
		return nil, nil
	}
	item := s.items[s.claimed]
	s.claimed++
	return item, nil
}
func (s *holdStoreStub) markReplayed(context.Context, *heldEvent, time.Time) error {
	s.replayed++
	return nil
}
func (s *holdStoreStub) markReplayFailed(context.Context, *heldEvent, error, time.Time) error {
	s.replayFailures++
	return nil
}

type publisherStub struct {
	topic   string
	message *basemessaging.Message
	err     error
}

func (p *publisherStub) Publish(context.Context, string, []byte) error { return p.err }
func (p *publisherStub) PublishMessage(_ context.Context, topic string, message *basemessaging.Message) error {
	p.topic, p.message = topic, message
	return p.err
}
func (*publisherStub) Close() error { return nil }

func TestRetryEventHoldReplayerPreservesMessageIdentity(t *testing.T) {
	item := &heldEvent{ID: 1, EventID: "event-1", MessageID: "message-1", Topic: "evaluation", Payload: []byte(`{"id":"event-1","eventType":"evaluation.retry.requested"}`), ClaimToken: "claim-1"}
	store := &holdStoreStub{items: []*heldEvent{item}}
	publisher := &publisherStub{}
	replayer := NewRetryEventHoldReplayer(store, publisher)
	if err := replayer.RunOnce(t.Context(), time.Date(2026, 7, 19, 1, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	if store.claimed != 1 || store.replayed != 1 || store.replayFailures != 0 {
		t.Fatalf("store state=%#v", store)
	}
	if publisher.topic != item.Topic || publisher.message == nil || publisher.message.UUID != item.MessageID || string(publisher.message.Payload) != string(item.Payload) {
		t.Fatalf("published topic/message=%q %#v", publisher.topic, publisher.message)
	}
}

func TestRetryEventHoldReplayerPersistsPublishFailure(t *testing.T) {
	store := &holdStoreStub{items: []*heldEvent{{ID: 1, EventID: "event-1", MessageID: "message-1", Topic: "evaluation", Payload: []byte(`{}`)}}}
	publisher := &publisherStub{err: errors.New("mq unavailable")}
	replayer := NewRetryEventHoldReplayer(store, publisher)
	if err := replayer.RunOnce(t.Context(), time.Now()); err != nil {
		t.Fatal(err)
	}
	if store.replayed != 0 || store.replayFailures != 1 {
		t.Fatalf("store state=%#v", store)
	}
}
