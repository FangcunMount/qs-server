package messaging

import (
	"bytes"
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	basemessaging "github.com/FangcunMount/component-base/pkg/messaging"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	rmtransport "github.com/FangcunMount/reliable-messaging/transport"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
)

func TestRetryEventHoldDuplicateIsStatePreservingNoop(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	store := &mysqlRetryEventHoldStore{db: db, provider: "nsq", policy: retrygovernance.DefaultOutboxPolicy}
	message := basemessaging.NewMessage("message-1", []byte(`{"id":"event-1","data":{"org_id":7}}`))
	message.Topic = "evaluation"
	message.Channel = "qs-worker"
	message.Attempts = 3

	mock.ExpectExec(regexp.QuoteMeta("ON DUPLICATE KEY UPDATE id=LAST_INSERT_ID(id)")).
		WithArgs("event-1", "message-1", int64(7), "nsq", "evaluation", "qs-worker", string(message.Payload), 3, "automatic retry paused", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(42, 0))
	if err := store.Hold(t.Context(), message, "evaluation.retry.requested", nil); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRetryEventHoldDeliveryKeepsApplicationIdentityAndFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	store := &mysqlRetryEventHoldStore{db: db, provider: "nsq", policy: retrygovernance.DefaultOutboxPolicy}
	delivery := rmtransport.Received{
		ID: "app-1", TransportID: "physical-1", Topic: "evaluation", Channel: "qs-worker",
		Payload: []byte(`{"id":"event-1","data":{"org_id":7}}`), Attempts: 3,
	}
	mock.ExpectExec(regexp.QuoteMeta("ON DUPLICATE KEY UPDATE id=LAST_INSERT_ID(id)")).
		WithArgs("event-1", "app-1", int64(7), "nsq", "evaluation", "qs-worker", string(delivery.Payload), 3, "automatic retry paused", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(42, 0))
	if err := store.HoldDelivery(t.Context(), delivery, "evaluation.retry.requested", nil); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	wantErr := errors.New("mysql unavailable")
	mock.ExpectExec(regexp.QuoteMeta("ON DUPLICATE KEY UPDATE id=LAST_INSERT_ID(id)")).
		WithArgs("event-1", "app-1", int64(7), "nsq", "evaluation", "qs-worker", string(delivery.Payload), 3, "automatic retry paused", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnError(wantErr)
	if err := store.HoldDelivery(t.Context(), delivery, "evaluation.retry.requested", nil); !errors.Is(err, wantErr) {
		t.Fatalf("hold error = %v, want %v", err, wantErr)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRetryEventHoldClaimUsesDispositionScheduleAndLeaseCAS(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	store := &mysqlRetryEventHoldStore{db: db, provider: "nsq", policy: retrygovernance.DefaultOutboxPolicy}
	now := time.Date(2026, 7, 19, 2, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id, event_id, message_id, topic_name, channel_name, payload_json, replay_attempt_count").
		WithArgs(now, now, now).
		WillReturnRows(sqlmock.NewRows([]string{"id", "event_id", "message_id", "topic_name", "channel_name", "payload_json", "replay_attempt_count"}).
			AddRow(uint64(1), "event-1", "message-1", "topic", "channel", `{}`, 4))
	mock.ExpectExec("UPDATE retry_event_hold.*retry_disposition='automatic'.*next_attempt_at.*claim_expires_at").
		WithArgs(sqlmock.AnyArg(), now.Add(time.Minute), now, uint64(1), now, now, now).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	item, err := store.claim(t.Context(), now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if item == nil || item.ID != 1 || item.ClaimToken == "" || item.ReplayAttemptCount != 4 {
		t.Fatalf("claim = %#v", item)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

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

type wireOnlyPublisher struct {
	topic  string
	bodies [][]byte
	err    error
}

func (p *wireOnlyPublisher) PublishWire(_ context.Context, topic string, body []byte) error {
	p.topic = topic
	p.bodies = append(p.bodies, append([]byte(nil), body...))
	return p.err
}

func TestRetryEventHoldNSQReplayUsesOriginalSDKWireIdentity(t *testing.T) {
	item := &heldEvent{ID: 1, EventID: "event-1", MessageID: "message-1", Topic: "evaluation", Payload: []byte(`{"id":"event-1","eventType":"evaluation.retry.requested"}`), ClaimToken: "claim-1"}
	store := &holdStoreStub{items: []*heldEvent{item}}
	publisher := &wireOnlyPublisher{}
	replayer, err := NewSDKRetryEventHoldReplayer(store, publisher)
	if err != nil {
		t.Fatal(err)
	}
	if err := replayer.RunOnce(t.Context(), time.Date(2026, 7, 19, 1, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	want, err := legacy.Encode(legacy.Envelope{UUID: item.MessageID, Metadata: basemessaging.NewMessage(item.MessageID, item.Payload).Metadata, Payload: item.Payload}, legacy.Revision2)
	if err != nil {
		t.Fatal(err)
	}
	if publisher.topic != item.Topic || len(publisher.bodies) != 1 || !bytes.Equal(publisher.bodies[0], want) || store.replayed != 1 {
		t.Fatalf("wire replay changed original identity or payload: topic=%q wire=%q replayed=%d", publisher.topic, publisher.bodies, store.replayed)
	}
}

func TestRetryEventHoldNSQRequiresWirePortAndRetainsUnknown(t *testing.T) {
	if _, err := NewSDKRetryEventHoldReplayer(&holdStoreStub{}, nil); err == nil {
		t.Fatal("missing NSQ wire publisher was accepted")
	}
	item := &heldEvent{ID: 1, EventID: "event-1", MessageID: "message-1", Topic: "evaluation", Payload: []byte(`{"id":"event-1"}`), ClaimToken: "claim-1"}
	store := &holdStoreStub{items: []*heldEvent{item}}
	publisher := &wireOnlyPublisher{err: errors.New("publish outcome unknown")}
	replayer, err := NewSDKRetryEventHoldReplayer(store, publisher)
	if err != nil {
		t.Fatal(err)
	}
	if err := replayer.RunOnce(t.Context(), time.Now()); err != nil {
		t.Fatal(err)
	}
	if store.replayed != 0 || store.replayFailures != 1 || len(publisher.bodies) != 1 {
		t.Fatalf("unknown publish was not retained: store=%#v publisher=%#v", store, publisher)
	}
}
