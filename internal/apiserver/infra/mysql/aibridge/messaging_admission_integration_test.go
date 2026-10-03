//go:build integration

package aibridge

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/google/uuid"
)

func TestMQAdmissionClosedRejectsEveryNewFamilyWithoutRows(t *testing.T) {
	f := newMQFixture(t)
	calls := 0
	s := f.commandStore(&calls)
	f.request.Actor.OrgID = "1"
	f.scope.OrganizationID = "1"
	mustMQ(t, s.StageStart(t.Context(), f.request))
	_, err := f.db.Exec("UPDATE ai_bridge_requests SET session_id=? WHERE request_id=?", f.session, f.request.RequestID)
	mustMQ(t, err)
	_, err = f.db.Exec("UPDATE ai_messaging_admission SET closed=TRUE WHERE singleton=1")
	mustMQ(t, err)
	t.Cleanup(func() {
		_, e := f.db.Exec("UPDATE ai_messaging_admission SET closed=FALSE WHERE singleton=1")
		if e != nil {
			t.Error(e)
		}
	})
	start := f.request
	start.RequestID = uuid.NewString()
	discard := false
	for _, scenario := range []struct {
		name, id string
		submit   func() error
	}{
		{"start", start.RequestID, func() error { return s.StageStart(t.Context(), start) }},
		{"change", uuid.NewString(), nil},
		{"retry", uuid.NewString(), nil},
		{"evaluation_start", uuid.NewString(), nil},
		{"evaluation_cancel", uuid.NewString(), nil},
	} {
		switch scenario.name {
		case "change":
			scenario.submit = func() error {
				return s.StageChange(t.Context(), f.request.RequestID, app.Change{CommandID: scenario.id, SessionID: f.session, Actor: f.request.Actor, Action: "cancel", ExpectedVersion: 2})
			}
		case "retry":
			scenario.submit = func() error {
				return s.SubmitParticipantRetry(t.Context(), app.DraftScope{OrganizationID: 1, OperatorUserID: 42}, f.session, app.ParticipantRetry{CommandID: scenario.id, ExpectedRunID: f.run, ExpectedVersion: 4, Reason: "explicit", Confirm: true, ExpectedProviderInvocations: 1})
			}
		case "evaluation_start":
			scenario.submit = func() error {
				return s.SubmitEvaluationStart(t.Context(), app.EvaluationScope{OrganizationID: 1, OperatorUserID: 42, RunID: f.run}, scenario.id, app.EvaluationStart{CommandID: scenario.id, ExpectedVersion: 1, Reason: "explicit", Confirm: true})
			}
		case "evaluation_cancel":
			scenario.submit = func() error {
				return s.SubmitEvaluationCancel(t.Context(), app.EvaluationScope{OrganizationID: 1, OperatorUserID: 42, RunID: f.run}, scenario.id, app.EvaluationCancel{CommandID: scenario.id, ExpectedVersion: 1, Reason: "explicit", Confirm: true, Discard: &discard})
			}
		}
		t.Run(scenario.name, func(t *testing.T) {
			if err := scenario.submit(); !errors.Is(err, app.ErrRuntimeAdmissionClosed) {
				t.Fatal("closed runtime admission did not definitively refuse new operation", err)
			}
			for _, table := range []string{"ai_messaging_outbox", "ai_messaging_operations"} {
				var n int
				mustMQ(t, f.db.QueryRow("SELECT COUNT(*) FROM "+table+" WHERE "+map[string]string{"ai_messaging_outbox": "message_id", "ai_messaging_operations": "command_id"}[table]+"=?", scenario.id).Scan(&n))
				if n != 0 {
					t.Fatal("closed gate wrote", table)
				}
			}
		})
	}
	var n int
	mustMQ(t, f.db.QueryRow("SELECT COUNT(*) FROM ai_bridge_requests WHERE request_id=?", start.RequestID).Scan(&n))
	if n != 0 || calls != 1 {
		t.Fatal("closed gate persisted original business or sealed", n, calls)
	}
	mustMQ(t, s.StageStart(t.Context(), f.request))
	changed := f.request
	changed.Goal = "different body"
	if err := s.StageStart(t.Context(), changed); !errors.Is(err, app.ErrConflict) {
		t.Fatal("closed gate hid identity conflict", err)
	}
	_, err = s.ReadOperation(t.Context(), f.scope, f.request.RequestID)
	mustMQ(t, err)
}

func TestMQAdmissionClosedReplaysEveryOriginalFamilyWithoutResealing(t *testing.T) {
	f := newMQFixture(t)
	f.request.Actor.OrgID = "1"
	f.scope.OrganizationID = "1"
	calls := 0
	s := f.commandStore(&calls)
	mustMQ(t, s.StageStart(t.Context(), f.request))
	_, e := f.db.Exec("UPDATE ai_bridge_requests SET session_id=? WHERE request_id=?", f.session, f.request.RequestID)
	mustMQ(t, e)
	change := app.Change{CommandID: uuid.NewString(), SessionID: f.session, Actor: f.request.Actor, Action: "cancel", ExpectedVersion: 2}
	retry := app.ParticipantRetry{CommandID: uuid.NewString(), ExpectedRunID: f.run, ExpectedVersion: 4, Reason: "explicit retry", Confirm: true, ExpectedProviderInvocations: 1}
	start := app.EvaluationStart{CommandID: uuid.NewString(), ExpectedVersion: 2, Reason: "frozen", Confirm: true}
	discard := false
	cancel := app.EvaluationCancel{CommandID: uuid.NewString(), ExpectedVersion: 3, Reason: "cancel", Confirm: true, Discard: &discard}
	for _, scenario := range []struct {
		name, id, resource string
		submit             func() error
	}{
		{"start", f.request.RequestID, f.request.RequestID, func() error { return s.StageStart(t.Context(), f.request) }},
		{"change", change.CommandID, f.session, func() error { return s.StageChange(t.Context(), f.request.RequestID, change) }},
		{"retry", retry.CommandID, f.session, func() error {
			return s.SubmitParticipantRetry(t.Context(), app.DraftScope{OrganizationID: 1, OperatorUserID: 42}, f.session, retry)
		}},
		{"evaluation_start", start.CommandID, f.run, func() error {
			return s.SubmitEvaluationStart(t.Context(), app.EvaluationScope{OrganizationID: 1, OperatorUserID: 42, RunID: f.run}, start.CommandID, start)
		}},
		{"evaluation_cancel", cancel.CommandID, f.run, func() error {
			return s.SubmitEvaluationCancel(t.Context(), app.EvaluationScope{OrganizationID: 1, OperatorUserID: 42, RunID: f.run}, cancel.CommandID, cancel)
		}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			_, e := f.db.Exec("UPDATE ai_messaging_admission SET closed=FALSE WHERE singleton=1")
			mustMQ(t, e)
			mustMQ(t, scenario.submit())
			var before, after []byte
			var originalTime, timeAfter time.Time
			mustMQ(t, f.db.QueryRow("SELECT wire,created_at FROM ai_messaging_outbox WHERE message_id=?", scenario.id).Scan(&before, &originalTime))
			_, e = f.db.Exec("UPDATE ai_messaging_admission SET closed=TRUE WHERE singleton=1")
			mustMQ(t, e)
			beforeCalls := calls
			mustMQ(t, scenario.submit())
			mustMQ(t, f.db.QueryRow("SELECT wire,created_at FROM ai_messaging_outbox WHERE message_id=?", scenario.id).Scan(&after, &timeAfter))
			if !bytes.Equal(before, after) || !originalTime.Equal(timeAfter) || beforeCalls != calls {
				t.Fatal("closed gate resealed or rebound original operation")
			}
			op, e := s.ReadOperation(t.Context(), f.scope, scenario.id)
			mustMQ(t, e)
			if op.CommandID != scenario.id || op.ResourceID != scenario.resource || op.Status != "submitted" {
				t.Fatal("original operation/query lost", op)
			}
		})
	}
	// Replaying an old Change must not depend on today's projection/version.
	_, e = f.db.Exec("UPDATE ai_bridge_requests SET session_id=? WHERE request_id=?", uuid.NewString(), f.request.RequestID)
	mustMQ(t, e)
	mustMQ(t, s.StageChange(t.Context(), f.request.RequestID, change))
	change.ExpectedVersion++
	if e = s.StageChange(t.Context(), f.request.RequestID, change); !errors.Is(e, app.ErrConflict) {
		t.Fatal("changed intent bypassed conflict while closed", e)
	}
	change.ExpectedVersion--
	change.Actor.SubjectID = "different subject"
	if e = s.StageChange(t.Context(), f.request.RequestID, change); !errors.Is(e, app.ErrConflict) {
		t.Fatal("actor rebound original closed operation", e)
	}
}

func TestMQAdmissionCloseWaitsForOriginalCommitOrRollback(t *testing.T) {
	for _, rollback := range []bool{false, true} {
		t.Run(map[bool]string{false: "commit", true: "rollback"}[rollback], func(t *testing.T) {
			f := newMQFixture(t)
			calls := 0
			s := f.commandStore(&calls)
			seal := s.Seal
			entered, release := make(chan struct{}), make(chan struct{})
			defer func() {
				select {
				case <-release:
				default:
					close(release)
				}
			}()
			sentinel := errors.New("submission aborted before commit")
			s.Seal = func(k pb.MessagingKind, id, aggregate, org, stamp string, b *pb.MessagingBody) (*app.PreparedMessaging, error) {
				close(entered)
				<-release
				if rollback {
					return nil, sentinel
				}
				return seal(k, id, aggregate, org, stamp, b)
			}
			submitted := make(chan error, 1)
			go func() { submitted <- s.StageStart(t.Context(), f.request) }()
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("submit did not enter original transaction")
			}
			closeTx, e := f.db.BeginTx(t.Context(), nil)
			mustMQ(t, e)
			defer func() { _ = closeTx.Rollback() }()
			var connection uint64
			mustMQ(t, closeTx.QueryRow("SELECT CONNECTION_ID()").Scan(&connection))
			state, e := (MessagingAdmission{}).Inspect(t.Context(), closeTx)
			mustMQ(t, e)
			closed := make(chan error, 1)
			go func() {
				_, e := (MessagingAdmission{}).Set(t.Context(), closeTx, true, state.Revision)
				if e == nil {
					e = closeTx.Commit()
				}
				closed <- e
			}()
			// Verify MySQL itself is waiting for this submission's shared gate lock.
			deadline := time.Now().Add(5 * time.Second)
			waiting := false
			for time.Now().Before(deadline) {
				var n int
				mustMQ(t, f.db.QueryRow("SELECT COUNT(*) FROM performance_schema.data_lock_waits w JOIN performance_schema.threads t ON t.THREAD_ID=w.REQUESTING_THREAD_ID WHERE t.PROCESSLIST_ID=?", connection).Scan(&n))
				if n == 1 {
					waiting = true
					break
				}
				time.Sleep(25 * time.Millisecond)
			}
			if !waiting {
				close(release)
				t.Fatal("closure was not blocked by the original submission transaction")
			}
			select {
			case e := <-closed:
				close(release)
				t.Fatal("closure reported before in-flight commit/rollback", e)
			default:
			}
			close(release)
			if e = <-submitted; rollback {
				if !errors.Is(e, sentinel) {
					t.Fatal(e)
				}
			} else {
				mustMQ(t, e)
			}
			mustMQ(t, <-closed)
			for _, table := range []string{"ai_bridge_requests", "ai_messaging_operations", "ai_messaging_outbox"} {
				column := "request_id"
				if table == "ai_messaging_operations" {
					column = "command_id"
				}
				if table == "ai_messaging_outbox" {
					column = "message_id"
				}
				var n int
				mustMQ(t, f.db.QueryRow("SELECT COUNT(*) FROM "+table+" WHERE "+column+"=?", f.request.RequestID).Scan(&n))
				if (n == 1) == rollback {
					t.Fatal("closure did not wait for atomic submission", table, n)
				}
			}
			newRequest := f.request
			newRequest.RequestID = uuid.NewString()
			if e = s.StageStart(t.Context(), newRequest); !errors.Is(e, app.ErrRuntimeAdmissionClosed) {
				t.Fatal("new submission crossed completed closure", e)
			}
			if !rollback {
				mustMQ(t, s.StageStart(t.Context(), f.request))
			}
		})
	}
}

func TestMQAdmissionStorageAndCancellationRemainUncertain(t *testing.T) {
	f := newMQFixture(t)
	calls := 0
	s := f.commandStore(&calls)
	gate := MessagingAdmission{}
	tx, e := f.db.BeginTx(t.Context(), nil)
	mustMQ(t, e)
	state, e := gate.Inspect(t.Context(), tx)
	mustMQ(t, e)
	_, e = gate.Set(t.Context(), tx, true, state.Revision)
	mustMQ(t, e)
	ctx, cancel := context.WithTimeout(t.Context(), 150*time.Millisecond)
	e = s.StageStart(ctx, f.request)
	cancel()
	if !errors.Is(e, context.DeadlineExceeded) || errors.Is(e, app.ErrRuntimeAdmissionClosed) {
		t.Fatal("uncommitted/cancelled gate mistaken for definite refusal", e)
	}
	mustMQ(t, tx.Rollback())
	// Missing row is not a known closed decision. Do not fabricate a default row.
	_, e = f.db.Exec("DELETE FROM ai_messaging_admission WHERE singleton=1")
	mustMQ(t, e)
	t.Cleanup(func() {
		_, e := f.db.Exec("INSERT INTO ai_messaging_admission(singleton,closed,revision,updated_at) VALUES(1,FALSE,?,UTC_TIMESTAMP(6)) ON DUPLICATE KEY UPDATE singleton=singleton", state.Revision)
		if e != nil {
			t.Error(e)
		}
	})
	e = s.StageStart(t.Context(), f.request)
	if !errors.Is(e, sql.ErrNoRows) || errors.Is(e, app.ErrRuntimeAdmissionClosed) {
		t.Fatal("storage unavailable reported as definitive maintenance refusal", e)
	}
	for _, table := range []string{"ai_bridge_requests", "ai_messaging_operations", "ai_messaging_outbox"} {
		column := "request_id"
		if table == "ai_messaging_operations" {
			column = "command_id"
		}
		if table == "ai_messaging_outbox" {
			column = "message_id"
		}
		var n int
		mustMQ(t, f.db.QueryRow("SELECT COUNT(*) FROM "+table+" WHERE "+column+"=?", f.request.RequestID).Scan(&n))
		if n != 0 {
			t.Fatal("uncertain admission wrote", table)
		}
	}
	if calls != 0 {
		t.Fatal("uncertain admission sealed a message")
	}
	mustMQ(t, f.db.PingContext(t.Context())) // borrowed host pool is still usable
}

func TestMQAdmissionOperatorRevisionAndCancelledClose(t *testing.T) {
	f := newMQFixture(t)
	gate := MessagingAdmission{}
	reader, e := f.db.BeginTx(t.Context(), nil)
	mustMQ(t, e)
	state, e := gate.Read(t.Context(), reader)
	mustMQ(t, e)
	ctx, cancel := context.WithTimeout(t.Context(), 150*time.Millisecond)
	writer, e := f.db.BeginTx(ctx, nil)
	mustMQ(t, e)
	_, e = gate.Set(ctx, writer, true, state.Revision)
	if !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal("close did not honour cancellation", e)
	}
	cancel()
	_ = writer.Rollback()
	mustMQ(t, reader.Rollback())
	mustMQ(t, f.tx(func(tx *sql.Tx) error {
		unchanged, e := gate.Inspect(t.Context(), tx)
		if e != nil {
			return e
		}
		if unchanged.Closed || unchanged.Revision != state.Revision {
			t.Fatal("cancelled close mutated gate", unchanged)
		}
		closed, e := gate.Set(t.Context(), tx, true, state.Revision)
		if e != nil {
			return e
		}
		if !closed.Closed || closed.Revision != state.Revision+1 {
			t.Fatal("closure revision lost", closed)
		}
		_, e = gate.Set(t.Context(), tx, false, state.Revision)
		if !errors.Is(e, app.ErrConflict) {
			t.Fatal("stale operator reopened gate", e)
		}
		return nil
	}))
	mustMQ(t, f.tx(func(tx *sql.Tx) error { _, e := gate.Set(t.Context(), tx, false, state.Revision+1); return e }))
	mustMQ(t, f.db.PingContext(t.Context()))
}
