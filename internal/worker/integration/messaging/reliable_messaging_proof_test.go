//go:build reliable_messaging

package messaging

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	basemessaging "github.com/FangcunMount/component-base/pkg/messaging"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	"github.com/FangcunMount/reliable-messaging/message"
	"github.com/stretchr/testify/require"
)

// Actual settlement + hold persistence, with dispatcher/ACK failures injected
// at their boundaries. This is not Assessment domain idempotency acceptance.
func TestReliableMessagingDurableHold(t *testing.T) {
	dsn := os.Getenv("RM_QS_HOLD_DSN")
	if dsn == "" {
		t.Fatal("isolated MySQL DSN required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	db, err := sql.Open("mysql", dsn)
	require.NoError(t, err)
	defer db.Close()
	schema, err := os.ReadFile("/tmp/qs-retry-event-hold.sql")
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, string(schema))
	require.NoError(t, err)
	hold := &mysqlRetryEventHoldStore{db: db, provider: "nsq", policy: retrygovernance.DefaultOutboxPolicy}
	dispatcher := &fakeDispatcher{err: eventruntime.ErrAutomaticRetryPaused}
	handler := createDispatchHandlerWithObserverAndHold(testLogger(), dispatcher, "evaluation", "proof-worker", nil, hold)
	intent, err := message.New(message.Input{Producer: "qs-server", ID: "stable-event", Destination: "evaluation", EventType: "evaluation.retry.requested", SchemaVersion: "v1", Scope: "7", ContentType: "application/json", OccurredAt: "2026-09-22T00:00:00Z", Payload: []byte(`{"id":"stable-event","eventType":"evaluation.retry.requested","occurredAt":"2026-09-22T00:00:00Z","aggregateType":"Assessment","aggregateID":"7","data":{"org_id":7},"extension":{"preserve":true}}`)})
	require.NoError(t, err)
	fresh := func(id string) *basemessaging.Message {
		m := basemessaging.NewMessage(id, intent.Input().Payload)
		m.Topic = "evaluation"
		m.Channel = "proof-worker"
		m.Attempts = 3
		return m
	}
	ackLost := errors.New("injected lost ACK")
	first := fresh("original-nsq-delivery")
	first.SetAckFunc(func() error {
		var n int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM retry_event_hold WHERE message_id=?", first.UUID).Scan(&n); err != nil {
			return err
		}
		if n != 1 {
			return errors.New("ACK before durable hold")
		}
		return ackLost
	})
	require.ErrorIs(t, handler(ctx, first), ackLost)
	// Governance changes after the first hold must survive broker redelivery.
	_, err = db.ExecContext(ctx, "UPDATE retry_event_hold SET retry_disposition='manual_required',replay_attempt_count=7,manual_replay_request_id='frozen-request' WHERE message_id=?", first.UUID)
	require.NoError(t, err)
	acked := 0
	duplicate := fresh(first.UUID)
	duplicate.SetAckFunc(func() error { acked++; return nil })
	require.NoError(t, handler(ctx, duplicate))
	require.Equal(t, 1, acked)
	var count, attempts int
	var payload, disposition, request string
	require.NoError(t, db.QueryRowContext(ctx, "SELECT COUNT(*) FROM retry_event_hold").Scan(&count))
	require.Equal(t, 1, count)
	require.NoError(t, db.QueryRowContext(ctx, "SELECT payload_json,retry_disposition,replay_attempt_count,manual_replay_request_id FROM retry_event_hold WHERE message_id=?", first.UUID).Scan(&payload, &disposition, &attempts, &request))
	require.Equal(t, string(intent.Input().Payload), payload)
	require.Equal(t, "manual_required", disposition)
	require.Equal(t, 7, attempts)
	require.Equal(t, "frozen-request", request)
	_, err = db.ExecContext(ctx, `CREATE TRIGGER reject_hold BEFORE INSERT ON retry_event_hold FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='isolated hold unavailable'`)
	require.NoError(t, err)
	nacked := 0
	failed := fresh("new-nsq-delivery")
	failed.SetAckFunc(func() error { acked++; return nil })
	failed.SetNackFunc(func() error { nacked++; return nil })
	require.Error(t, handler(ctx, failed))
	require.Equal(t, 1, nacked)
	require.Equal(t, 1, acked)
	require.NoError(t, db.QueryRowContext(ctx, "SELECT COUNT(*) FROM retry_event_hold").Scan(&count))
	require.Equal(t, 1, count)
}
