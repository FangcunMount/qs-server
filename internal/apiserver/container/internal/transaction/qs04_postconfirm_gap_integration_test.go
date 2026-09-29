//go:build integration && reliable_messaging && reliable_messaging_m4 && reliable_messaging_m4_integration

package transaction

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	appintake "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/intake"
	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/checkpoint"
	assessmentmysql "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	mysqlstandard "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/standardoutbox"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationrun"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventpayload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	"github.com/FangcunMount/reliable-messaging/relay"
	sdkmysql "github.com/FangcunMount/reliable-messaging/storage/mysql"
	sdknsq "github.com/FangcunMount/reliable-messaging/transport/nsq"
	domainwire "github.com/FangcunMount/reliable-messaging/wire/domain"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/nsqio/go-nsq"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// The companion script creates a disposable MySQL and a memory-backed NSQ
// channel. No Worker starts before the broker is killed after PUB OK.
func TestQS04PostConfirmSeed(t *testing.T) {
	dsn, nsqTCP := os.Getenv("RM_QS04_MYSQL_DSN"), os.Getenv("RM_QS04_NSQ_TCP")
	parsed, err := mysqldriver.ParseDSN(dsn)
	if err != nil || parsed.Net != "tcp" || parsed.DBName != "m6_qs04_postconfirm" ||
		!strings.HasPrefix(parsed.Addr, "127.0.0.1:") || !strings.HasPrefix(nsqTCP, "127.0.0.1:") {
		t.Fatal("QS-04 proof requires disposable localhost MySQL and NSQ endpoints")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 35*time.Second)
	defer cancel()
	db, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	defer sqlDB.Close()
	require.NoError(t, db.AutoMigrate(&assessmentmysql.AssessmentPO{}, &checkpoint.RuntimeCheckpointPO{}))
	_, err = sqlDB.ExecContext(ctx, sdkmysql.Schema)
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(ctx, `ALTER TABLE rm_outbox ADD KEY ix_rm_outbox_message_id (message_id,id)`)
	require.NoError(t, err)
	require.NoError(t, createEvaluationRequestRefTable(ctx, sqlDB))
	require.NoError(t, createGapRecoveryRequestTable(ctx, sqlDB))
	parsedCatalog, err := eventcatalog.Parse([]byte(`version: "1"
topics:
  evaluation:
    name: qs.evaluation.lifecycle
events:
  evaluation.requested:
    topic: evaluation
    delivery: durable_outbox
    aggregate: Assessment
    domain: evaluation
    handler: evaluation_requested_handler
`))
	require.NoError(t, err)
	stager, err := mysqlstandard.NewStager(eventcatalog.NewCatalog(parsedCatalog), eventruntime.SourceAPIServer)
	require.NoError(t, err)
	repo := assessmentmysql.NewAssessmentRepository(db)
	intake := appintake.NewService(repo, proofModelValidator{}, NewMySQLRunner(db), stager)
	kind, code, version := "scale", "MODEL-1", "1.0.0"
	created, err := intake.CreateForAnswerSheet(ctx, appintake.CreateCommand{
		OrgID: 1, TesteeID: 2, AnswerSheetID: 90020077,
		QuestionnaireCode: "Q-001", QuestionnaireVersion: "v1", OriginType: "adhoc",
		ModelKind: &kind, ModelCode: &code, ModelVersion: &version,
	})
	require.NoError(t, err)
	_, err = intake.SubmitForEvaluation(ctx, created.ID)
	require.NoError(t, err)
	var assessmentCount, outboxCount, runCount int64
	require.NoError(t, db.Table("assessment").Where("id = ? AND status = ?", created.ID, "submitted").Count(&assessmentCount).Error)
	require.EqualValues(t, 1, assessmentCount)
	require.NoError(t, db.Table("rm_outbox").Where("event_type = ? AND state = ?", "evaluation.requested", "pending").Count(&outboxCount).Error)
	require.EqualValues(t, 1, outboxCount)
	var messageID string
	var originalWire []byte
	require.NoError(t, sqlDB.QueryRowContext(ctx, "SELECT message_id, payload FROM rm_outbox WHERE event_type = 'evaluation.requested'").Scan(&messageID, &originalWire))
	wire, recognized, err := legacy.Decode(originalWire)
	require.NoError(t, err)
	require.True(t, recognized)
	require.Equal(t, messageID, wire.UUID)
	envelope, err := domainwire.DecodeEnvelope(wire.Payload)
	require.NoError(t, err)
	require.Equal(t, messageID, envelope.ID)
	require.Equal(t, strconv.FormatUint(created.ID, 10), envelope.AggregateID)
	var data eventpayload.EvaluationRequestedData
	require.NoError(t, json.Unmarshal(envelope.Data, &data))
	require.EqualValues(t, created.ID, data.AssessmentID)
	require.Equal(t, kind, data.ModelKind)
	require.Equal(t, code, data.ModelCode)
	require.Equal(t, version, data.ModelVersion)
	require.NoError(t, db.Table("runtime_checkpoint").Where("scope = ? AND assessment_id = ?", "evaluation_run", created.ID).Count(&runCount).Error)
	require.Zero(t, runCount)

	config := nsq.NewConfig()
	config.HeartbeatInterval = time.Second
	config.ReadTimeout, config.WriteTimeout = 3*time.Second, time.Second
	producer, err := nsq.NewProducer(nsqTCP, config)
	require.NoError(t, err)
	producer.SetLogger(nil, nsq.LogLevelError)
	defer producer.Stop()
	publisher, err := sdknsq.New(producer, map[string]string{"qs.evaluation.lifecycle": "qs.evaluation.lifecycle"}, 1)
	require.NoError(t, err)
	store, err := sdkmysql.New(sqlDB)
	require.NoError(t, err)
	forwarder, err := relay.New(store, publisher, relay.Config{
		Concurrency: 1, PollInterval: 50 * time.Millisecond, Lease: 5 * time.Second,
		PublishTimeout: 2 * time.Second, WriteTimeout: time.Second,
		Retry: standardoutbox.SDKRetryPolicy(), Observe: func(relay.Event) {},
	})
	require.NoError(t, err)
	relayCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- forwarder.Run(relayCtx) }()
	require.Eventually(t, func() bool {
		var count int64
		return db.Table("rm_outbox").Where("event_type = ? AND state = ?", "evaluation.requested", "published").Count(&count).Error == nil && count == 1
	}, 20*time.Second, 50*time.Millisecond)
	stop()
	require.NoError(t, <-done)
	require.NoError(t, publisher.Drain(ctx))
	require.NoError(t, db.Table("runtime_checkpoint").Where("scope = ? AND assessment_id = ?", "evaluation_run", created.ID).Count(&runCount).Error)
	require.Zero(t, runCount)
	inspection, err := mysqlstandard.InspectOriginalRequest(ctx, sqlDB, 1, created.ID, time.Now().Add(time.Hour))
	require.NoError(t, err)
	require.Equal(t, "candidate_never_claimed", inspection.State)
	require.Equal(t, messageID, inspection.EventID)
	t.Logf("assessment_id=%d outbox=published run_count=0", created.ID)
}

type observedClaim struct {
	id      string
	claimed bool
	err     error
}

// The companion script first kills the broker after PUB OK and verifies that
// its in-memory message disappeared. This second stage reschedules the same
// Outbox row, observes the original wire through the real NSQ channel, and
// exercises the durable first-Run claim and duplicate guard. It does not run
// the model or claim a completed business effect.
func TestQS04PostConfirmRecoverAndClaim(t *testing.T) {
	dsn, nsqTCP := os.Getenv("RM_QS04_MYSQL_DSN"), os.Getenv("RM_QS04_NSQ_TCP")
	parsed, err := mysqldriver.ParseDSN(dsn)
	if err != nil || parsed.Net != "tcp" || parsed.DBName != "m6_qs04_postconfirm" ||
		!strings.HasPrefix(parsed.Addr, "127.0.0.1:") || !strings.HasPrefix(nsqTCP, "127.0.0.1:") {
		t.Fatal("QS-04 recovery proof requires disposable localhost MySQL and NSQ endpoints")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 35*time.Second)
	defer cancel()
	db, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	defer sqlDB.Close()
	var assessmentID uint64
	require.NoError(t, sqlDB.QueryRowContext(ctx, `SELECT id FROM assessment WHERE answer_sheet_id=90020077 AND status='submitted'`).Scan(&assessmentID))
	var eventID, originalState string
	var originalWire []byte
	var originalVersion uint64
	require.NoError(t, sqlDB.QueryRowContext(ctx, `SELECT message_id,payload,state,version FROM rm_outbox WHERE event_type='evaluation.requested'`).
		Scan(&eventID, &originalWire, &originalState, &originalVersion))
	require.Equal(t, "published", originalState)
	inspection, err := mysqlstandard.InspectOriginalRequest(ctx, sqlDB, 1, assessmentID, time.Now().Add(-10*time.Minute))
	require.NoError(t, err)
	require.Equal(t, "candidate_never_claimed", inspection.State)
	require.Equal(t, eventID, inspection.EventID)

	ledger, err := mysqlstandard.NewGapRecoveryLedger(sqlDB)
	require.NoError(t, err)
	review := mysqlstandard.GapRecoveryRequest{OrgID: 1, RequestID: "isolated-recover-original", ActorID: 99,
		AssessmentID: assessmentID, EventID: eventID, ExpectedVersion: originalVersion,
		Reason: "confirmed NSQ message disappeared before first Worker claim", SubmittedBefore: time.Now().Add(-10 * time.Minute)}
	decision, err := ledger.Authorize(ctx, review)
	require.NoError(t, err)
	require.True(t, decision.Authorized)
	require.Equal(t, "authorized", decision.Code)
	var state, persistedEventID string
	var persistedWire []byte
	require.NoError(t, sqlDB.QueryRowContext(ctx, `SELECT message_id,payload,state FROM rm_outbox WHERE event_type='evaluation.requested'`).
		Scan(&persistedEventID, &persistedWire, &state))
	require.Equal(t, eventID, persistedEventID)
	require.Equal(t, originalWire, persistedWire)
	require.Equal(t, "retry_wait", state)

	config := nsq.NewConfig()
	config.HeartbeatInterval = time.Second
	config.ReadTimeout, config.WriteTimeout = 3*time.Second, time.Second
	observed := make(chan observedClaim, 2)
	consumer, err := nsq.NewConsumer("qs.evaluation.lifecycle", "rm-qs04-postconfirm", config)
	require.NoError(t, err)
	consumer.SetLogger(nil, nsq.LogLevelError)
	consumer.AddHandler(nsq.HandlerFunc(func(message *nsq.Message) error {
		wire, recognized, decodeErr := legacy.Decode(message.Body)
		if decodeErr != nil || !recognized {
			observed <- observedClaim{err: decodeErr}
			return nil
		}
		envelope, decodeErr := domainwire.DecodeEnvelope(wire.Payload)
		if decodeErr != nil {
			observed <- observedClaim{err: decodeErr}
			return nil
		}
		var data eventpayload.EvaluationRequestedData
		if decodeErr = json.Unmarshal(envelope.Data, &data); decodeErr != nil {
			observed <- observedClaim{err: decodeErr}
			return nil
		}
		if wire.UUID != eventID || envelope.ID != eventID || data.AssessmentID != int64(assessmentID) ||
			string(message.Body) != string(originalWire) {
			observed <- observedClaim{id: wire.UUID, err: fmt.Errorf("recovered message identity or payload changed")}
			return nil
		}
		now := time.Now()
		claim, claimErr := checkpoint.NewRepository(db).Claim(ctx, evaluationrun.ClaimRequest{
			AssessmentID: assessmentID, Token: uuid.NewString(), ClaimedAt: now, LeaseUntil: now.Add(5 * time.Minute),
		})
		observed <- observedClaim{id: wire.UUID, claimed: claim.Claimed, err: claimErr}
		return nil
	}))
	require.NoError(t, consumer.ConnectToNSQD(nsqTCP))
	defer consumer.Stop()
	producer, err := nsq.NewProducer(nsqTCP, config)
	require.NoError(t, err)
	producer.SetLogger(nil, nsq.LogLevelError)
	defer producer.Stop()
	publisher, err := sdknsq.New(producer, map[string]string{"qs.evaluation.lifecycle": "qs.evaluation.lifecycle"}, 1)
	require.NoError(t, err)
	store, err := sdkmysql.New(sqlDB)
	require.NoError(t, err)
	forwarder, err := relay.New(store, publisher, relay.Config{
		Concurrency: 1, PollInterval: 50 * time.Millisecond, Lease: 5 * time.Second,
		PublishTimeout: 2 * time.Second, WriteTimeout: time.Second,
		Retry: standardoutbox.SDKRetryPolicy(), Observe: func(relay.Event) {},
	})
	require.NoError(t, err)
	relayCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- forwarder.Run(relayCtx) }()
	first := waitQS04Claim(t, ctx, observed)
	require.NoError(t, first.err)
	require.Equal(t, eventID, first.id)
	require.True(t, first.claimed)
	require.Eventually(t, func() bool {
		var count int64
		return db.Table("rm_outbox").Where("message_id=? AND state='published' AND attempt_count=2", eventID).Count(&count).Error == nil && count == 1
	}, 15*time.Second, 50*time.Millisecond)
	stop()
	require.NoError(t, <-done)
	require.NoError(t, publisher.Drain(ctx))
	require.NoError(t, producer.Publish("qs.evaluation.lifecycle", originalWire))
	second := waitQS04Claim(t, ctx, observed)
	require.NoError(t, second.err)
	require.Equal(t, eventID, second.id)
	require.False(t, second.claimed)
	var runCount int64
	require.NoError(t, db.Table("runtime_checkpoint").Where("scope=? AND assessment_id=?", "evaluation_run", assessmentID).Count(&runCount).Error)
	require.EqualValues(t, 1, runCount)
	t.Logf("original_event_id=%s relay_attempts=2 first_claim=true duplicate_claim=false run_count=%d", eventID, runCount)
}

func waitQS04Claim(t *testing.T, ctx context.Context, observed <-chan observedClaim) observedClaim {
	t.Helper()
	select {
	case outcome := <-observed:
		return outcome
	case <-ctx.Done():
		t.Fatal("timed out waiting for QS-04 recovered message", ctx.Err())
		return observedClaim{}
	}
}
