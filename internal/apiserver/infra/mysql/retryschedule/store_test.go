package retryschedule

import (
	"context"
	"database/sql"
	shared "github.com/FangcunMount/qs-server/internal/apiserver/eventing/transactionplan"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	model "github.com/FangcunMount/qs-server/internal/apiserver/eventing/retryschedule"
	standard "github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/event"
	catalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	payload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type testRoutes struct{}

func (testRoutes) GetTopicForEvent(string) (string, bool) { return model.LifecycleTopic, true }

func plan(t *testing.T) model.Frozen {
	t.Helper()
	at := time.Date(2026, 10, 20, 15, 40, 0, 999999999, time.UTC)
	a := model.Authority{Owner: "qs.mysql", OrgID: 7, Kind: model.Evaluation, SubjectID: "41", RunID: "41:1", AssessmentID: 41, TesteeID: 9, ExpectedAttempt: 1, Origin: retrygovernance.AttemptOriginAutomatic, DecisionHash: [32]byte{1}}
	evt := event.Event[payload.EvaluationRequestedData]{BaseEvent: event.BaseEvent{ID: "retry-first", EventTypeValue: catalog.EvaluationRetryRequested, OccurredAtValue: at, AggregateTypeValue: "Evaluation", AggregateIDValue: "41"}, Data: payload.EvaluationRequestedData{OrgID: 7, AssessmentID: 41, TesteeID: 9, ExpectedAttempt: 1, AttemptOrigin: "automatic", Mode: "next_attempt", RequestedAt: at}}
	prepared, err := standard.PrepareIntents([]event.DomainEvent{evt}, testRoutes{}, "api-server")
	require.NoError(t, err)
	f, err := model.New(a, prepared[0].Message.Input(), at)
	require.NoError(t, err)
	return f
}

func database(t *testing.T) (*gorm.DB, *sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	pool, mock, err := sqlmock.New()
	require.NoError(t, err)
	db, err := gorm.Open(gormmysql.New(gormmysql.Config{Conn: pool, SkipInitializeWithVersion: true}), &gorm.Config{DisableAutomaticPing: true})
	require.NoError(t, err)
	t.Cleanup(func() {
		mock.ExpectClose()
		require.NoError(t, pool.Close())
		require.NoError(t, mock.ExpectationsWereMet())
	})
	return db, pool, mock
}

func TestScheduleConstructorIsInertAndRejectsOrdinaryOrForeignRootHandles(t *testing.T) {
	db, _, mock := database(t)
	binding := transaction.Binding{Owner: "qs.mysql", StoreID: "primary", WriterEpoch: "m3-1"}
	s, err := New(db, binding)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet(), "construction cannot query or install schema")
	f := plan(t)
	_, err = s.SaveFrozen(t.Context(), f)
	require.ErrorIs(t, err, hostmysql.ErrActiveTransactionRequired)
	_, err = s.SaveFrozen(hostmysql.WithTx(t.Context(), db), f)
	require.ErrorIs(t, err, transaction.ErrTransactionRequired)
	_, err = s.Find(hostmysql.WithTx(t.Context(), db), f)
	require.ErrorIs(t, err, transaction.ErrTransactionRequired)
	_, err = s.Due(t.Context(), f.DueAt(), 101)
	require.ErrorIs(t, err, model.ErrInvalid)
	other, _, otherMock := database(t)
	otherMock.ExpectBegin()
	root := other.Begin()
	require.NoError(t, root.Error)
	_, err = s.SaveFrozen(hostmysql.WithTx(t.Context(), root), f)
	require.ErrorIs(t, err, transaction.ErrBinding)
	_, err = New(root, binding)
	require.ErrorIs(t, err, transaction.ErrTransactionRequired)
	otherMock.ExpectRollback()
	require.NoError(t, root.Rollback().Error)
}

func TestScheduleValidationFailsClosedOnSchemaAndOriginalPrimaryIdentity(t *testing.T) {
	for _, mode := range []string{"schema", "binding"} {
		t.Run(mode, func(t *testing.T) {
			db, _, mock := database(t)
			s, err := New(db, transaction.Binding{Owner: "qs.mysql", StoreID: "primary", WriterEpoch: "m3-1"})
			require.NoError(t, err)
			version := 1
			if mode == "schema" {
				version = 2
			}
			mock.ExpectQuery(regexp.QuoteMeta("SELECT version FROM qs_retry_schedule_schema WHERE id=1")).WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow(version))
			if mode == "binding" {
				mock.ExpectQuery(regexp.QuoteMeta("SELECT store_id,writer_epoch,schema_version FROM rm_tx_binding WHERE owner=?")).WithArgs([]byte("qs.mysql")).WillReturnRows(sqlmock.NewRows([]string{"store_id", "writer_epoch", "schema_version"}).AddRow("foreign-primary", "m3-1", 1))
			}
			err = s.Validate(context.Background())
			if mode == "schema" {
				require.ErrorIs(t, err, transaction.ErrSchema)
			} else {
				require.ErrorIs(t, err, transaction.ErrBinding)
			}
		})
	}
}

func TestScheduleReaderRejectsCorruptIndexAndDispatchWithoutWitnessReference(t *testing.T) {
	for _, mode := range []string{"ready", "early_index", "committed_without_proof", "wrong_owner", "wrong_hash"} {
		t.Run(mode, func(t *testing.T) {
			db, pool, mock := database(t)
			s, err := New(db, transaction.Binding{Owner: "qs.mysql", StoreID: "primary", WriterEpoch: "m3-1"})
			require.NoError(t, err)
			f := plan(t)
			a, in := f.Authority(), f.Message().Input()
			input, digest := f.InputHash(), f.Digest()
			due, err := f.IndexDue(time.Microsecond)
			require.NoError(t, err)
			state, owner := model.Ready, a.Owner
			switch mode {
			case "early_index":
				due = due.Add(-time.Microsecond)
			case "committed_without_proof":
				state = model.DispatchCommitted
			case "wrong_owner":
				owner = "qs.mongo"
			case "wrong_hash":
				digest[0] ^= 1
			}
			mock.ExpectQuery("SELECT fixture").WillReturnRows(sqlmock.NewRows([]string{"identity", "input", "digest", "owner", "scope", "kind", "subject", "run", "message", "due", "plan", "state", "revision", "proof"}).AddRow(f.KeyIdentity(), input[:], digest[:], owner, in.Scope, a.Kind, a.SubjectID, a.RunID, in.ID, due, f.Bytes(), string(state), 1, nil))
			r, err := s.readRow(pool.QueryRow("SELECT fixture"))
			if mode == "ready" {
				require.NoError(t, err)
				require.Equal(t, f.Bytes(), r.Plan.Bytes())
			} else {
				require.ErrorIs(t, err, model.ErrInvalid)
			}
		})
	}
}

func TestDispatchCannotForgeOriginalRootFromProofShape(t *testing.T) {
	db, _, _ := database(t)
	s, err := New(db, transaction.Binding{Owner: "qs.mysql", StoreID: "primary", WriterEpoch: "m3-1"})
	require.NoError(t, err)
	f := plan(t)
	r := model.Record{Plan: f, State: model.Ready, Version: 1}
	q, err := model.FreezeDispatch(r, f.DueAt())
	require.NoError(t, err)
	p, err := shared.Freeze(q, testRoutes{}, f.Authority().Owner, "m3-1")
	require.NoError(t, err)
	e := transaction.Execution{Plan: p, Fence: 1, WriterEpoch: "m3-1"}
	proof, err := model.DispatchProof(r, e)
	require.NoError(t, err)
	_, err = s.GuardDispatch(t.Context(), r, proof)
	require.ErrorIs(t, err, transaction.ErrTransactionRequired)
	require.ErrorIs(t, s.MarkDispatched(t.Context(), r, proof), transaction.ErrTransactionRequired)
	var missing *evidence.EventEvidenceV2
	_, err = s.GuardDispatch(t.Context(), r, missing)
	require.ErrorIs(t, err, model.ErrInvalid)
}
