//go:build integration

package aibridge

import (
	"bytes"
	"database/sql"
	"errors"
	"sync"
	"testing"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	durable "github.com/FangcunMount/reliable-messaging/delivery/mysql"
	"github.com/google/uuid"
)

func TestMQConcurrentSubmissionRetainsFirstWire(t *testing.T) {
	f := newMQFixture(t)
	calls := 0
	s := f.commandStore(&calls)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- s.StageStart(t.Context(), f.request) }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		mustMQ(t, err)
	}
	var count int
	mustMQ(t, f.db.QueryRow("SELECT COUNT(*) FROM ai_messaging_outbox WHERE message_id=?", f.request.RequestID).Scan(&count))
	if count != 1 || calls != 1 {
		t.Fatal("concurrent replay sealed more than once", count, calls)
	}
	var before, after []byte
	mustMQ(t, f.db.QueryRow("SELECT wire FROM ai_messaging_outbox WHERE message_id=?", f.request.RequestID).Scan(&before))
	changed := f.request
	changed.Goal = "different original intent"
	if err := s.StageStart(t.Context(), changed); !errors.Is(err, app.ErrConflict) {
		t.Fatal("changed original identity accepted", err)
	}
	mustMQ(t, s.StageStart(t.Context(), f.request))
	mustMQ(t, f.db.QueryRow("SELECT wire FROM ai_messaging_outbox WHERE message_id=?", f.request.RequestID).Scan(&after))
	if !bytes.Equal(before, after) || calls != 1 {
		t.Fatal("replay changed first wire")
	}
}

func TestMQCommandsUseUTCWithNonUTCSessions(t *testing.T) {
	for _, zone := range []string{"+00:00", "+08:00", "-05:00"} {
		t.Run(zone, func(t *testing.T) {
			f := newMQFixture(t)
			f.db.SetMaxOpenConns(1)
			_, err := f.db.Exec("SET SESSION time_zone=?", zone)
			mustMQ(t, err)
			calls := 0
			s := f.commandStore(&calls)
			mustMQ(t, s.StageStart(t.Context(), f.request))
			var record durable.Record
			mustMQ(t, f.tx(func(tx *sql.Tx) error {
				rows, err := f.store.Outbox.Pending(t.Context(), tx, 20)
				if err != nil {
					return err
				}
				if len(rows) != 1 || rows[0].MessageID != f.request.RequestID {
					t.Fatal("new command not due under session timezone", zone)
				}
				record = rows[0]
				return nil
			}))
			mustMQ(t, f.tx(func(tx *sql.Tx) error {
				return f.store.Outbox.Retry(t.Context(), tx, record.Identity, record.BodySHA256, 5, "test_unknown")
			}))
			var remaining int64
			mustMQ(t, f.db.QueryRow("SELECT TIMESTAMPDIFF(MICROSECOND,UTC_TIMESTAMP(6),available_at) FROM ai_messaging_outbox WHERE message_id=?", record.MessageID).Scan(&remaining))
			if remaining <= 0 || remaining > 5_000_000 {
				t.Fatal("retry no longer uses UTC", zone, remaining)
			}
			var before, after string
			const schedule = "SELECT DATE_FORMAT(available_at,'%Y-%m-%d %H:%i:%s.%f') FROM ai_messaging_outbox WHERE message_id=?"
			mustMQ(t, f.db.QueryRow(schedule, record.MessageID).Scan(&before))
			mustMQ(t, s.StageStart(t.Context(), f.request))
			mustMQ(t, f.db.QueryRow(schedule, record.MessageID).Scan(&after))
			if before != after || calls != 1 {
				t.Fatal("duplicate changed retry schedule", zone)
			}
			// Simulate an already authenticated decision to isolate UTC scheduling.
			// The real receiver separately verifies identity/hash and atomic settlement.
			mustMQ(t, f.tx(func(tx *sql.Tx) error {
				return f.store.Outbox.Confirm(t.Context(), tx, record.Identity, record.BodySHA256)
			}))
			_, err = f.db.Exec("UPDATE ai_bridge_requests SET session_id=? WHERE request_id=?", f.session, f.request.RequestID)
			mustMQ(t, err)
			change := app.Change{CommandID: uuid.NewString(), SessionID: f.session, Actor: f.request.Actor, Action: "cancel", ExpectedVersion: 1}
			mustMQ(t, s.StageChange(t.Context(), f.request.RequestID, change))
			mustMQ(t, f.tx(func(tx *sql.Tx) error {
				rows, err := f.store.Outbox.Pending(t.Context(), tx, 20)
				if err == nil && (len(rows) != 1 || rows[0].MessageID != change.CommandID) {
					t.Fatal("new Change not due under session timezone", zone)
				}
				return err
			}))
		})
	}
}
