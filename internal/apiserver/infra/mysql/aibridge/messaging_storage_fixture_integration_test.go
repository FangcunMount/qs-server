//go:build integration

package aibridge

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"database/sql"
	"os"
	"testing"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	jose "github.com/go-jose/go-jose/v4"
	"github.com/google/uuid"
)

type mqFixture struct {
	t                                *testing.T
	db                               *sql.DB
	store                            *MessagingStore
	request                          app.Start
	session, run                     string
	scope                            app.OperationScope
	qsSign, qsCrypt, aiSign, aiCrypt jose.JSONWebKey
}

func newMQFixture(t *testing.T) *mqFixture {
	t.Helper()
	dsn := os.Getenv("QS_AI_MQ_TEST_DSN")
	if dsn == "" {
		t.Fatal("required disposable QS MQ MySQL DSN is missing")
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Ping(); err != nil {
		t.Fatal(err)
	}
	// Disposable test databases open admission explicitly; production migration
	// starts closed and must be opened only by the reviewed host operator tool.
	if _, err = db.Exec("UPDATE ai_messaging_admission SET closed=FALSE WHERE singleton=1"); err != nil {
		t.Fatal(err)
	}
	f := &mqFixture{t: t, db: db, store: NewMessagingStore(), session: uuid.NewString(), run: uuid.NewString()}
	f.request = app.Start{RequestID: uuid.NewString(), Actor: app.Actor{OrgID: "18446744073709551615", SubjectID: "42"}, TesteeID: "7", AssessmentIDs: []string{"9"}, Goal: "真实原事务：中文"}
	f.scope = app.OperationScope{OrganizationID: f.request.Actor.OrgID, SubjectID: "42", ResourceID: f.request.RequestID}
	key := func(id string) jose.JSONWebKey {
		k, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if e != nil {
			t.Fatal(e)
		}
		return jose.JSONWebKey{Key: k, KeyID: id}
	}
	f.qsSign, f.qsCrypt, f.aiSign, f.aiCrypt = key("qs-sign"), key("qs-crypt"), key("ai-sign"), key("ai-crypt")
	t.Cleanup(func() {
		if _, err := db.Exec("DELETE FROM ai_messaging_legacy_commands WHERE request_id=?", f.request.RequestID); err != nil {
			t.Error(err)
		}
		for _, aggregate := range []string{f.request.RequestID, f.run} {
			for _, table := range []string{"ai_messaging_inbox", "ai_messaging_operations", "ai_messaging_outbox", "ai_messaging_aggregates", "ai_messaging_failures"} {
				if _, err := db.Exec("DELETE FROM "+table+" WHERE aggregate_key=?", aggregate); err != nil {
					t.Error(err)
				}
			}
		}
		if _, err := db.Exec("DELETE FROM ai_messaging_evaluation_states WHERE run_id=?", f.run); err != nil {
			t.Error(err)
		}
		for _, table := range []string{"ai_bridge_events", "ai_bridge_commands", "ai_bridge_requests"} {
			if _, err := db.Exec("DELETE FROM "+table+" WHERE request_id=?", f.request.RequestID); err != nil {
				t.Error(err)
			}
		}
		_ = db.Close()
	})
	return f
}

func (f *mqFixture) tx(fn func(*sql.Tx) error) error {
	tx, err := f.db.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err = fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}
func (f *mqFixture) startBody() *pb.MessagingBody {
	return &pb.MessagingBody{Value: &pb.MessagingBody_Start{Start: &pb.StartCommand{RequestId: f.request.RequestID, Actor: &pb.Actor{OrgId: f.scope.OrganizationID, SubjectId: f.scope.SubjectID}, TesteeId: f.request.TesteeID, AssessmentIds: f.request.AssessmentIDs, Goal: f.request.Goal}}}
}
func (f *mqFixture) protect(kind pb.MessagingKind, id, aggregate, correlation string, body *pb.MessagingBody, fromAI bool) *app.PreparedMessaging {
	f.t.Helper()
	signing, recipient := f.qsSign, f.aiCrypt.Public()
	if fromAI {
		signing, recipient = f.aiSign, f.qsCrypt.Public()
	}
	m, err := app.ProtectMessaging(kind, id, aggregate, correlation, f.scope.OrganizationID, "2026-10-03T16:00:00.123456+08:00", body, signing, recipient)
	if err != nil {
		f.t.Fatal(err)
	}
	return m
}
func mustMQ(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// Inline body adapter for the current Broker transaction acceptance.
type localEventBody struct{}

func (*localEventBody) Read(_ context.Context, e *pb.MessagingEnvelope) ([]byte, error) {
	return e.GetInlineBody(), nil
}

// Fixture-only retained operation: direct facts, no retired publisher or executor.
func seedRetainedOperation(t *testing.T, db *sql.DB, kind pb.MessagingKind, id, aggregate string, scope app.OperationScope, body *pb.MessagingBody) string {
	t.Helper()
	e, _, err := app.PrepareMessaging(kind, id, aggregate, "", body)
	mustMQ(t, err)
	_, err = db.ExecContext(t.Context(), `INSERT INTO ai_messaging_operations(command_id,kind,body_sha256,organization_id,subject_id,resource_id,aggregate_key,aggregate_sequence,created_at) VALUES(?,?,?,?,?,?,?,1,UTC_TIMESTAMP(6))`, id, kind, e.BodySha256, scope.OrganizationID, scope.SubjectID, scope.ResourceID, aggregate)
	mustMQ(t, err)
	return e.BodySha256
}
