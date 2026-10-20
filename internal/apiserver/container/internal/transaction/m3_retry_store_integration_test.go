//go:build integration && m3_retry_store

package transaction_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	modtx "github.com/FangcunMount/qs-server/internal/apiserver/container/internal/transaction"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	interpretation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/generation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	interpretationrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/run"
	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/eventevidencebinding"
	model "github.com/FangcunMount/qs-server/internal/apiserver/eventing/retryschedule"
	standard "github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	mongorepo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/interpretation"
	mongostore "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/retryschedule"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/checkpoint"
	mysqlstore "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/retryschedule"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationrun"
	"github.com/FangcunMount/qs-server/internal/pkg/event"
	catalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/backpressure"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	"github.com/FangcunMount/reliable-messaging/transaction"
	driver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func retryMigration(t *testing.T, backend, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "internal", "pkg", "migration", "migrations", backend, name))
	require.NoError(t, err)
	return raw
}

func retryRoutes(t *testing.T) *catalog.Catalog {
	t.Helper()
	source, err := catalog.Load("../../../../../configs/events.yaml")
	require.NoError(t, err)
	return catalog.NewCatalog(source)
}

func retryFrozen(t *testing.T, a model.Authority, evt event.DomainEvent, due time.Time) model.Frozen {
	t.Helper()
	prepared, err := standard.PrepareIntents([]event.DomainEvent{evt}, retryRoutes(t), "api-server")
	require.NoError(t, err)
	f, err := model.New(a, prepared[0].Message.Input(), due)
	require.NoError(t, err)
	return f
}

func retryStoreReceipt(t *testing.T, backend string, cases map[string]bool) {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"backend": backend, "passed": len(cases), "failed": 0, "skipped": 0, "cases": cases, "scope": "original_run_and_future_plan_storage_only", "broker_dispatch_not_claimed": true, "system_login_not_claimed": true})
	require.NoError(t, err)
	t.Log("M3_QS_RETRY_STORE_RECEIPT " + string(raw))
}

func TestM3RetryMySQLStore(t *testing.T) {
	require.True(t, strings.HasPrefix(os.Getenv("QS_M3_FIXTURE_PROJECT"), "rocketmq-m2-"))
	cfg, err := driver.ParseDSN(os.Getenv("QS_M3_RETRY_DSN"))
	require.NoError(t, err)
	require.Equal(t, "tcp", cfg.Net)
	require.Empty(t, cfg.DBName)
	host, _, err := net.SplitHostPort(cfg.Addr)
	require.NoError(t, err)
	require.True(t, net.ParseIP(host).IsLoopback())
	cfg.ParseTime, cfg.MultiStatements = true, true
	admin, err := sql.Open("mysql", cfg.FormatDSN())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, admin.Close()) })
	name := fmt.Sprintf("qs_m3_retry_store_%d", time.Now().UnixNano())
	_, err = admin.ExecContext(t.Context(), "CREATE DATABASE `"+name+"`")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := admin.ExecContext(context.Background(), "DROP DATABASE `"+name+"`")
		require.NoError(t, err)
	})
	cfg.DBName = name
	db, err := gorm.Open(gormmysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pool.Close()) })
	_, err = pool.ExecContext(t.Context(), string(retryMigration(t, "mysql", "000106_transaction_messaging.up.sql")))
	require.NoError(t, err)
	binding := transaction.Binding{Owner: "qs.mysql", StoreID: "qs-native-retry-mysql", WriterEpoch: "m3-1"}
	_, err = pool.ExecContext(t.Context(), "INSERT INTO rm_tx_binding VALUES(?,?,?,1)", []byte(binding.Owner), []byte(binding.StoreID), []byte(binding.WriterEpoch))
	require.NoError(t, err)
	var before, after int
	require.NoError(t, pool.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=?", name).Scan(&before))
	store, err := mysqlstore.New(db, binding)
	require.NoError(t, err)
	require.ErrorIs(t, store.Validate(t.Context()), transaction.ErrSchema)
	require.NoError(t, pool.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=?", name).Scan(&after))
	require.Equal(t, before, after)
	_, err = pool.ExecContext(t.Context(), string(retryMigration(t, "mysql", "000107_business_retry_schedule.up.sql")))
	require.NoError(t, err)
	require.NoError(t, store.Validate(t.Context()))
	require.NoError(t, db.AutoMigrate(&checkpoint.RuntimeCheckpointPO{}))
	runs := checkpoint.NewRunRepository(db)
	at := time.Now().UTC().Truncate(time.Millisecond)
	claim, err := runs.Claim(t.Context(), evaluationrun.ClaimRequest{AssessmentID: 5001, Token: "original-claim", ClaimedAt: at.Add(-time.Second), LeaseUntil: at.Add(time.Minute), Origin: retrygovernance.AttemptOriginInitial})
	require.NoError(t, err)
	require.True(t, claim.Claimed)
	failed := claim.Run
	require.NoError(t, failed.Fail(at, evalrun.Failure{Kind: evalrun.FailureKindCalculation, Message: "controlled original run failure", Retryable: true}))
	a, err := assessment.NewAssessment(1, testee.NewID(9), assessment.NewQuestionnaireRefByCode(meta.NewCode("Q-RETRY"), "v1"), assessment.NewAnswerSheetRef(meta.FromUint64(6001)), assessment.NewAdhocOrigin(), assessment.WithID(meta.FromUint64(5001)))
	require.NoError(t, err)
	decision := failed.RetryDecision()
	require.NotNil(t, decision)
	require.NotNil(t, decision.NextAttemptAt)
	evt := assessment.NewEvaluationRetryRequestedEvent(a, failed.Attempt().Number, retrygovernance.AttemptOriginAutomatic, "", *decision.NextAttemptAt)
	require.NoError(t, failed.AttachRetryEvent(evt.EventID()))
	digest, err := json.Marshal(failed.RetryDecision())
	require.NoError(t, err)
	authority := model.Authority{Owner: binding.Owner, OrgID: 1, Kind: model.Evaluation, SubjectID: a.ID().String(), RunID: failed.ID().String(), AssessmentID: a.ID().Uint64(), TesteeID: 9, ExpectedAttempt: failed.Attempt().Number, Origin: retrygovernance.AttemptOriginAutomatic, DecisionHash: sha256.Sum256(digest)}
	frozen := retryFrozen(t, authority, evt, *decision.NextAttemptAt)
	root := modtx.NewMySQLRunner(db)
	abort := errors.New("fixture abort after both original writes")
	write := func(ctx context.Context) error {
		winner, err := store.SaveFrozen(ctx, frozen)
		if err != nil {
			return err
		}
		if winner.Digest() != frozen.Digest() {
			return model.ErrConflict
		}
		return runs.SaveClaimed(ctx, failed)
	}
	err = root.WithinTransaction(t.Context(), func(ctx context.Context) error {
		if err := write(ctx); err != nil {
			return err
		}
		return abort
	})
	require.ErrorIs(t, err, abort)
	_, err = store.Find(t.Context(), frozen)
	require.ErrorIs(t, err, transaction.ErrNotFound)
	unchanged, err := runs.FindLatestByAssessmentID(t.Context(), 5001)
	require.NoError(t, err)
	require.Equal(t, evalrun.StatusRunning, unchanged.Attempt().Status)
	require.NoError(t, root.WithinTransaction(t.Context(), write))
	saved, err := store.Find(t.Context(), frozen)
	require.NoError(t, err)
	require.Equal(t, frozen.Bytes(), saved.Plan.Bytes())
	require.Equal(t, model.Ready, saved.State)
	committed, err := runs.FindLatestByAssessmentID(t.Context(), 5001)
	require.NoError(t, err)
	require.Equal(t, evalrun.StatusFailed, committed.Attempt().Status)
	require.Equal(t, evt.EventID(), committed.RetryDecision().RetryEventID)
	otherAt := frozen.DueAt().Add(time.Second)
	other := retryFrozen(t, authority, assessment.NewEvaluationRetryRequestedEvent(a, failed.Attempt().Number, retrygovernance.AttemptOriginAutomatic, "", otherAt), otherAt)
	require.NoError(t, root.WithinTransaction(t.Context(), func(ctx context.Context) error {
		winner, err := store.SaveFrozen(ctx, other)
		if err != nil {
			return err
		}
		if winner.Digest() != frozen.Digest() {
			return model.ErrConflict
		}
		return nil
	}))
	due, err := store.Due(t.Context(), frozen.DueAt().Add(-time.Nanosecond), 10)
	require.NoError(t, err)
	require.Empty(t, due)
	index, err := frozen.IndexDue(time.Microsecond)
	require.NoError(t, err)
	due, err = store.Due(t.Context(), index, 10)
	require.NoError(t, err)
	require.Len(t, due, 1)
	require.Equal(t, frozen.Message().Input().Payload, due[0].Plan.Message().Input().Payload)
	var requests int
	require.NoError(t, pool.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM rm_tx_request").Scan(&requests))
	require.Zero(t, requests)
	require.False(t, db.Migrator().HasTable("rm_outbox"))
	retryStoreReceipt(t, "mysql", map[string]bool{"constructor_no_ddl": true, "schema_missing_rejected": true, "original_root_rollback_atomic": true, "original_root_commit_atomic": true, "duplicate_first_wire_deadline": true, "due_index_not_early": true, "future_authorization_no_half_or_outbox": true})
}

func TestM3RetryMongoStore(t *testing.T) {
	require.True(t, strings.HasPrefix(os.Getenv("QS_M3_FIXTURE_PROJECT"), "rocketmq-m2-"))
	uri := os.Getenv("QS_M3_MONGO_URI")
	parsed, err := url.Parse(uri)
	require.NoError(t, err)
	require.True(t, net.ParseIP(parsed.Hostname()).IsLoopback())
	require.True(t, strings.Contains(uri, "replicaSet=sdk-proof"))
	client, err := mongo.Connect(t.Context(), options.Client().ApplyURI(uri).SetAppName("qs-m3-retry-store"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Disconnect(context.Background())) })
	db := client.Database(fmt.Sprintf("qs_m3_retry_store_%d", time.Now().UnixNano()))
	t.Cleanup(func() { require.NoError(t, db.Drop(context.Background())) })
	install := func(name string) {
		var commands []json.RawMessage
		require.NoError(t, json.Unmarshal(retryMigration(t, "mongodb", name), &commands))
		for _, raw := range commands {
			var command bson.D
			require.NoError(t, bson.UnmarshalExtJSON(raw, true, &command))
			require.NoError(t, db.RunCommand(t.Context(), command).Err())
		}
	}
	install("000041_transaction_messaging.up.json")
	binding := transaction.Binding{Owner: "qs.mongo", StoreID: "qs-native-retry-mongo", WriterEpoch: "m3-1"}
	_, err = db.Collection("rm_tx_binding").InsertOne(t.Context(), bson.M{"_id": binding.Owner, "store_id": binding.StoreID, "writer_epoch": binding.WriterEpoch, "schema_version": 1})
	require.NoError(t, err)
	before, err := db.ListCollectionNames(t.Context(), bson.D{})
	require.NoError(t, err)
	store, err := mongostore.New(db, binding)
	require.NoError(t, err)
	require.ErrorIs(t, store.Validate(t.Context()), transaction.ErrSchema)
	after, err := db.ListCollectionNames(t.Context(), bson.D{})
	require.NoError(t, err)
	require.ElementsMatch(t, before, after)
	install("000042_business_retry_schedule.up.json")
	require.NoError(t, store.Validate(t.Context()))
	require.NoError(t, mongorepo.InstallLifecycleSchema(t.Context(), db))
	root := modtx.NewMongoRunner(db, modtx.MongoRunnerOptions{Boundary: "interpretation_commit", Limiter: backpressure.NewLimiter(2, time.Second)})
	runs := mongorepo.NewRunRepositoryWithoutSchema(db)
	generations := mongorepo.NewGenerationRepositoryWithoutSchema(db)
	at := time.Now().UTC().Truncate(time.Millisecond)
	g, err := generation.New(meta.FromUint64(51), generation.Key{OutcomeID: meta.FromUint64(71), ReportType: policy.ReportTypeStandard, TemplateVersion: "v1"}, at.Add(-time.Minute))
	require.NoError(t, err)
	run, err := interpretationrun.NewPending(meta.FromUint64(61), g.ID(), 1)
	require.NoError(t, err)
	require.NoError(t, run.StartWithLease(at.Add(-time.Second), "original-worker", at.Add(time.Minute)))
	require.NoError(t, g.Begin(run.ID(), *run.StartedAt()))
	require.NoError(t, root.WithinTransaction(t.Context(), func(ctx context.Context) error {
		if err := generations.Create(ctx, g); err != nil {
			return err
		}
		return runs.Create(ctx, run)
	}))
	require.NoError(t, run.Fail(at, interpretationrun.Failure{Kind: interpretationrun.FailureKindBuild, Code: "controlled", SafeMessage: "fixture", Retryable: true}))
	decision := run.RetryDecision()
	require.NotNil(t, decision)
	require.NotNil(t, decision.NextAttemptAt)
	evt := interpretation.NewInterpretationRetryRequestedEvent(interpretation.RetryRequestedEventInput{OrgID: 7, GenerationID: g.ID().String(), RunID: run.ID().String(), AssessmentID: "41", OutcomeID: "71", TesteeID: 9, ExpectedAttempt: run.Attempt(), AttemptOrigin: "automatic", Mode: "next_attempt", RequestedAt: *decision.NextAttemptAt})
	prepared, err := standard.PrepareIntents([]event.DomainEvent{evt}, retryRoutes(t), "api-server")
	require.NoError(t, err)
	business, err := eventevidencebinding.Retry(evt.Data)
	require.NoError(t, err)
	reference, err := evidence.NewStandard(standard.ReferenceFromMessage(prepared[0].Message), business)
	require.NoError(t, err)
	require.NoError(t, run.AttachRetryEventEvidence(reference))
	digest, err := json.Marshal(run.RetryDecision())
	require.NoError(t, err)
	authority := model.Authority{Owner: binding.Owner, OrgID: 7, Kind: model.Interpretation, SubjectID: g.ID().String(), RunID: run.ID().String(), AssessmentID: 41, OutcomeID: 71, TesteeID: 9, ExpectedAttempt: run.Attempt(), Origin: retrygovernance.AttemptOriginAutomatic, DecisionHash: sha256.Sum256(digest)}
	frozen := retryFrozen(t, authority, evt, *decision.NextAttemptAt)
	abort := errors.New("fixture abort after both original writes")
	write := func(ctx context.Context) error {
		winner, err := store.SaveFrozen(ctx, frozen)
		if err != nil {
			return err
		}
		if winner.Digest() != frozen.Digest() {
			return model.ErrConflict
		}
		return runs.Save(ctx, run)
	}
	err = root.WithinTransaction(t.Context(), func(ctx context.Context) error {
		if err := write(ctx); err != nil {
			return err
		}
		return abort
	})
	require.ErrorIs(t, err, abort)
	_, err = store.Find(t.Context(), frozen)
	require.ErrorIs(t, err, transaction.ErrNotFound)
	unchanged, err := runs.FindByID(t.Context(), run.ID())
	require.NoError(t, err)
	require.Equal(t, interpretationrun.StatusRunning, unchanged.Status())
	require.NoError(t, root.WithinTransaction(t.Context(), write))
	saved, err := store.Find(t.Context(), frozen)
	require.NoError(t, err)
	require.Equal(t, frozen.Bytes(), saved.Plan.Bytes())
	committed, err := runs.FindByID(t.Context(), run.ID())
	require.NoError(t, err)
	require.Equal(t, interpretationrun.StatusFailed, committed.Status())
	require.Equal(t, evt.EventID(), committed.RetryDecision().RetryEventID)
	due, err := store.Due(t.Context(), frozen.DueAt().Add(-time.Nanosecond), 10)
	require.NoError(t, err)
	require.Empty(t, due)
	index, err := frozen.IndexDue(time.Millisecond)
	require.NoError(t, err)
	due, err = store.Due(t.Context(), index, 10)
	require.NoError(t, err)
	require.Len(t, due, 1)
	require.Equal(t, frozen.Message().Input().Payload, due[0].Plan.Message().Input().Payload)
	requests, err := db.Collection("rm_tx_request").CountDocuments(t.Context(), bson.D{})
	require.NoError(t, err)
	require.Zero(t, requests)
	names, err := db.ListCollectionNames(t.Context(), bson.D{})
	require.NoError(t, err)
	require.NotContains(t, names, "rm_outbox")
	var expired context.Context
	require.NoError(t, root.WithinTransaction(t.Context(), func(ctx context.Context) error { expired = ctx; return nil }))
	_, err = store.SaveFrozen(expired, frozen)
	require.ErrorIs(t, err, transaction.ErrTransactionRequired)
	retryStoreReceipt(t, "mongo", map[string]bool{"constructor_no_ddl": true, "schema_missing_rejected": true, "original_root_rollback_atomic": true, "original_root_commit_atomic": true, "due_index_not_early": true, "future_authorization_no_half_or_outbox": true, "expired_session_rejected": true})
}
