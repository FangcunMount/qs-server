//go:build integration && reliable_messaging && reliable_messaging_m4 && reliable_messaging_m5

package transaction

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/FangcunMount/component-base/pkg/messaging"
	evalpb "github.com/FangcunMount/qs-server/api/grpc/gen/evaluation"
	appauthz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	appexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	appintake "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/intake"
	appoperator "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/operator"
	evalworker "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/worker"
	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/checkpoint"
	assessmentmysql "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	mysqlstandard "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/standardoutbox"
	grpcservice "github.com/FangcunMount/qs-server/internal/apiserver/transport/grpc/service"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
	"github.com/FangcunMount/qs-server/internal/pkg/reportstatus"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	"github.com/FangcunMount/qs-server/internal/worker/handlers"
	"github.com/FangcunMount/reliable-messaging/relay"
	sdkmysql "github.com/FangcunMount/reliable-messaging/storage/mysql"
	sdknsq "github.com/FangcunMount/reliable-messaging/transport/nsq"
	"github.com/alicebob/miniredis/v2"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/nsqio/go-nsq"
	redis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type m5FailedProjection struct {
	mu           sync.Mutex
	reporter     *reportstatus.Reporter
	assessmentID string
	reason       string
	calls        int
}

func (*m5FailedProjection) SetProcessing(context.Context, string, string, string) {}
func (*m5FailedProjection) SetCompleted(context.Context, string, string, string)  {}
func (*m5FailedProjection) SetTemporarilyUnavailable(context.Context, string, string, string, string) {
}
func (p *m5FailedProjection) SetFailed(ctx context.Context, assessmentID, answerSheetID, reason, message string) {
	p.reporter.SetFailed(ctx, assessmentID, answerSheetID, reason, message)
	p.mu.Lock()
	defer p.mu.Unlock()
	p.assessmentID, p.reason = assessmentID, reason
	p.calls++
}

type m5FailedDelivery struct {
	messageID string
	brokerID  nsq.MessageID
	attempts  uint16
	err       error
}

type m5EvaluationGRPCClient struct {
	client evalpb.EvaluationWorkerServiceClient
}

func (c m5EvaluationGRPCClient) ExecuteEvaluation(ctx context.Context, assessmentID uint64) (*evalpb.ExecuteEvaluationResponse, error) {
	return c.client.ExecuteEvaluation(ctx, &evalpb.ExecuteEvaluationRequest{AssessmentId: assessmentID})
}

type m5RetryMetadata struct {
	eventID, expectedAttempt, origin, mode, actionRequestID string
}

// The failed fact and standard intent are committed by the real Evaluation
// finalizer. Relay publishes the original envelope to NSQ; losing the first
// consumer ACK redelivers that same broker message to the original Worker
// handler without starting another business attempt.
func TestM5StandardEvaluationFailureAndGovernedRetriesAcrossNSQ(t *testing.T) {
	dsn := os.Getenv("RM_QS_M5_FAILED_DSN")
	parsed, err := mysqldriver.ParseDSN(dsn)
	if err != nil || parsed.Net != "tcp" || parsed.Addr != "mysql:3306" || parsed.DBName != "m5_qs_failed_delivery" {
		t.Fatal("disposable m5_qs_failed_delivery MySQL required")
	}
	address := os.Getenv("RM_QS_M5_NSQ_TCP")
	if address != "nsqd:4150" {
		t.Fatal("disposable nsqd:4150 required")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Second)
	defer cancel()
	db, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&assessmentmysql.AssessmentPO{}, &checkpoint.RuntimeCheckpointPO{}))
	_, err = sqlDB.ExecContext(ctx, sdkmysql.Schema)
	require.NoError(t, err)

	parsedCatalog, err := eventcatalog.Parse([]byte(`version: "1"
topics:
  evaluation:
    name: qs.evaluation.lifecycle
events:
  evaluation.requested:
    topic: evaluation
    delivery: durable_outbox
    aggregate: Evaluation
    domain: evaluation
    handler: evaluation_requested_handler
  evaluation.failed:
    topic: evaluation
    delivery: durable_outbox
    aggregate: Evaluation
    domain: evaluation
    handler: evaluation_failed_handler
  evaluation.retry.requested:
    topic: evaluation
    delivery: durable_outbox
    aggregate: Evaluation
    domain: evaluation
    handler: evaluation_retry_requested_handler
`))
	require.NoError(t, err)
	stager, err := mysqlstandard.NewStager(eventcatalog.NewCatalog(parsedCatalog), eventruntime.SourceAPIServer)
	require.NoError(t, err)
	assessmentRepo := assessmentmysql.NewAssessmentRepository(db)
	runRepo := checkpoint.NewRunRepository(db)
	runner := NewMySQLRunner(db)
	intake := appintake.NewService(assessmentRepo, proofModelValidator{}, runner, stager)
	kind, code, version := "scale", "MODEL-1", "1.0.0"
	created, err := intake.CreateForAnswerSheet(ctx, appintake.CreateCommand{
		OrgID: 1, TesteeID: 2, AnswerSheetID: 701,
		QuestionnaireCode: "Q-001", QuestionnaireVersion: "v1", OriginType: "adhoc",
		ModelKind: &kind, ModelCode: &code, ModelVersion: &version,
	})
	require.NoError(t, err)
	_, err = intake.SubmitForEvaluation(ctx, created.ID)
	require.NoError(t, err)
	engine := appexecute.NewEngine(assessmentRepo, m5UnavailableInput{},
		appexecute.WithRunRepository(runRepo), appexecute.WithTransactionalOutbox(runner, stager))
	require.Error(t, engine.Evaluate(ctx, created.ID))
	failed, err := assessmentRepo.FindByID(ctx, meta.FromUint64(created.ID))
	require.NoError(t, err)
	require.True(t, failed.Status().IsFailed())
	run, err := runRepo.FindLatestByAssessmentID(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, "failed", run.Attempt().Status.String())
	decision := run.RetryDecision()
	require.NotNil(t, decision)
	require.NotNil(t, decision.NextAttemptAt)
	require.Greater(t, time.Until(*decision.NextAttemptAt), 15*time.Second)
	require.NotEmpty(t, decision.RetryEventID)
	var intent struct{ MessageID, State string }
	require.NoError(t, db.Table("rm_outbox").Select("message_id,state").
		Where("event_type = ?", eventcatalog.EvaluationFailed).Take(&intent).Error)
	require.Equal(t, "pending", intent.State)
	var retryIntent struct{ MessageID, State string }
	require.NoError(t, db.Table("rm_outbox").Select("message_id,state").
		Where("event_type = ?", eventcatalog.EvaluationRetryRequested).Take(&retryIntent).Error)
	require.Equal(t, decision.RetryEventID, retryIntent.MessageID)
	require.Equal(t, "pending", retryIntent.State)

	statusRedis := miniredis.RunT(t)
	statusClient := redis.NewClient(&redis.Options{Addr: statusRedis.Addr()})
	t.Cleanup(func() { _ = statusClient.Close() })
	statusHandle := &redisruntime.Handle{Namespace: "m5-evaluation-failed", Client: statusClient}
	reporter, err := reportstatus.NewReporter(statusHandle, reportstatus.Config{TTL: time.Hour, Service: "qs-worker"})
	require.NoError(t, err)
	projection := &m5FailedProjection{reporter: reporter}
	metadataSeen := make(chan m5RetryMetadata, 8)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	server := grpc.NewServer(grpc.UnaryInterceptor(func(ctx context.Context, request any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
		incoming, _ := metadata.FromIncomingContext(ctx)
		select {
		case metadataSeen <- m5RetryMetadata{
			eventID: firstValue(incoming, "x-retry-event-id"), expectedAttempt: firstValue(incoming, "x-retry-expected-attempt"),
			origin: firstValue(incoming, "x-retry-origin"), mode: firstValue(incoming, "x-retry-mode"),
			actionRequestID: firstValue(incoming, "x-retry-action-request-id"),
		}:
		case <-ctx.Done():
		}
		return next(ctx, request)
	}))
	grpcservice.NewEvaluationWorkerService(evalworker.NewService(engine, assessmentRepo, assessmentmysql.NewOutcomeRepository(db), runRepo)).RegisterService(server)
	serverDone := make(chan error, 1)
	go func() { serverDone <- server.Serve(listener) }()
	t.Cleanup(func() {
		server.Stop()
		if err := <-serverDone; err != nil {
			t.Errorf("stop Evaluation gRPC server: %v", err)
		}
	})
	connection, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = connection.Close() })
	workerDeps := &handlers.Dependencies{
		Logger:                 slog.New(slog.NewTextHandler(io.Discard, nil)),
		ReportStatusReporter:   projection,
		EvaluationWorkerClient: m5EvaluationGRPCClient{client: evalpb.NewEvaluationWorkerServiceClient(connection)},
	}
	handler, ok := handlers.NewRegistry().Create("evaluation_failed_handler", workerDeps)
	require.True(t, ok)
	retryHandler, ok := handlers.NewRegistry().Create("evaluation_retry_requested_handler", workerDeps)
	require.True(t, ok)
	config := nsq.NewConfig()
	config.HeartbeatInterval, config.MsgTimeout = time.Second, 5*time.Second
	config.ReadTimeout, config.WriteTimeout = 3*time.Second, time.Second
	config.DefaultRequeueDelay = 100 * time.Millisecond
	consumer, err := nsq.NewConsumer("qs.evaluation.lifecycle", "rm-m5-failed-delivery", config)
	require.NoError(t, err)
	consumer.SetLogger(nil, nsq.LogLevelError)
	deliveries := make(chan m5FailedDelivery, 2)
	retryDeliveries := make(chan m5FailedDelivery, 8)
	consumer.AddHandler(nsq.HandlerFunc(func(raw *nsq.Message) error {
		decoded, recognized, decodeErr := messaging.DecodeMessagePayload(raw.Body)
		if decodeErr != nil {
			return fmt.Errorf("decode original QS envelope: %w", decodeErr)
		}
		if !recognized {
			return errors.New("NSQ message has no original QS envelope")
		}
		if decoded.Metadata["event_type"] == eventcatalog.EvaluationRetryRequested {
			handleErr := retryHandler(ctx, eventcatalog.EvaluationRetryRequested, decoded.Payload)
			if handleErr == nil && raw.Attempts == 1 {
				handleErr = errors.New("controlled lost retry consumer ACK")
			}
			select {
			case retryDeliveries <- m5FailedDelivery{messageID: decoded.UUID, brokerID: raw.ID, attempts: raw.Attempts, err: handleErr}:
			case <-ctx.Done():
			}
			return handleErr
		}
		if decoded.Metadata["event_type"] != eventcatalog.EvaluationFailed {
			// The requested event was already executed directly above; this
			// proof observes only the failure consequence.
			return nil
		}
		handleErr := handler(ctx, eventcatalog.EvaluationFailed, decoded.Payload)
		if handleErr == nil && raw.Attempts == 1 {
			handleErr = errors.New("controlled lost consumer ACK")
		}
		if decoded.UUID == intent.MessageID {
			select {
			case deliveries <- m5FailedDelivery{messageID: decoded.UUID, brokerID: raw.ID, attempts: raw.Attempts, err: handleErr}:
			case <-ctx.Done():
			}
		}
		return handleErr
	}))
	require.NoError(t, consumer.ConnectToNSQD(address))
	t.Cleanup(func() {
		consumer.Stop()
		select {
		case <-consumer.StopChan:
		case <-time.After(5 * time.Second):
			t.Error("NSQ failure consumer did not stop")
		}
	})
	producer, err := nsq.NewProducer(address, config)
	require.NoError(t, err)
	producer.SetLogger(nil, nsq.LogLevelError)
	require.NoError(t, producer.Ping())
	publisher, err := sdknsq.New(producer, map[string]string{"qs.evaluation.lifecycle": "qs.evaluation.lifecycle"}, 1)
	require.NoError(t, err)
	t.Cleanup(func() {
		drainCtx, finish := context.WithTimeout(context.Background(), 5*time.Second)
		defer finish()
		if err := publisher.Drain(drainCtx); err != nil {
			t.Errorf("drain NSQ publisher: %v", err)
		}
		producer.Stop()
	})
	store, err := sdkmysql.New(sqlDB)
	require.NoError(t, err)
	r, err := relay.New(store, publisher, relay.Config{
		Concurrency: 1, PollInterval: 30 * time.Millisecond, Lease: 5 * time.Second,
		PublishTimeout: 2 * time.Second, WriteTimeout: time.Second,
		Retry: standardoutbox.SDKRetryPolicy(), Observe: func(relay.Event) {},
	})
	require.NoError(t, err)
	relayCtx, stopRelay := context.WithCancel(ctx)
	relayDone := make(chan error, 1)
	go func() { relayDone <- r.Run(relayCtx) }()
	t.Cleanup(func() {
		stopRelay()
		if err := <-relayDone; err != nil {
			t.Errorf("standard MySQL Relay: %v", err)
		}
	})

	var first, second m5FailedDelivery
	select {
	case first = <-deliveries:
	case <-ctx.Done():
		t.Fatalf("first failure delivery missing: %v", ctx.Err())
	}
	select {
	case second = <-deliveries:
	case <-ctx.Done():
		t.Fatalf("failure redelivery missing: %v", ctx.Err())
	}
	require.Equal(t, intent.MessageID, first.messageID)
	require.Equal(t, first.messageID, second.messageID)
	require.Equal(t, first.brokerID, second.brokerID)
	require.EqualValues(t, 1, first.attempts)
	require.Greater(t, second.attempts, first.attempts)
	require.ErrorContains(t, first.err, "controlled lost consumer ACK")
	require.NoError(t, second.err)
	projection.mu.Lock()
	require.Equal(t, fmt.Sprint(created.ID), projection.assessmentID)
	require.Equal(t, "evaluation_failed", projection.reason)
	require.Equal(t, 2, projection.calls)
	projection.mu.Unlock()
	snapshot, err := reportstatus.NewCache(statusHandle).Get(ctx, fmt.Sprint(created.ID))
	require.NoError(t, err)
	require.NotNil(t, snapshot)
	require.Equal(t, "failed", snapshot.Status)
	require.Equal(t, "evaluation_failed", snapshot.Reason)
	var count int64
	require.NoError(t, db.Table("runtime_checkpoint").Where("scope = ? AND assessment_id = ?", "evaluation_run", created.ID).Count(&count).Error)
	require.EqualValues(t, 1, count)
	require.NoError(t, db.Table("rm_outbox").Where("event_type = ?", eventcatalog.EvaluationOutcomeCommitted).Count(&count).Error)
	require.Zero(t, count)
	require.Eventually(t, func() bool {
		var state string
		return db.Table("rm_outbox").Select("state").Where("message_id = ?", intent.MessageID).Scan(&state).Error == nil && state == "published"
	}, 5*time.Second, 25*time.Millisecond)
	// The original decision stays frozen. Relay itself waits for its persisted
	// next_attempt_at, then the original Worker forwards authorization over
	// actual gRPC before the MySQL claim can create attempt 2.
	var firstRetry, secondRetry m5FailedDelivery
	select {
	case firstRetry = <-retryDeliveries:
	case <-ctx.Done():
		t.Fatalf("scheduled retry did not arrive: %v", ctx.Err())
	}
	require.False(t, time.Now().Before(decision.NextAttemptAt.Add(-200*time.Millisecond)),
		"standard Relay must not deliver the authorized retry before its persisted due time")
	select {
	case secondRetry = <-retryDeliveries:
	case <-ctx.Done():
		t.Fatalf("retry redelivery did not arrive: %v", ctx.Err())
	}
	require.Equal(t, retryIntent.MessageID, firstRetry.messageID)
	require.Equal(t, firstRetry.messageID, secondRetry.messageID)
	require.Equal(t, firstRetry.brokerID, secondRetry.brokerID)
	require.EqualValues(t, 1, firstRetry.attempts)
	require.Greater(t, secondRetry.attempts, firstRetry.attempts)
	require.ErrorContains(t, firstRetry.err, "controlled lost retry consumer ACK")
	require.NoError(t, secondRetry.err)
	for range 2 {
		select {
		case seen := <-metadataSeen:
			require.Equal(t, retryIntent.MessageID, seen.eventID)
			require.Equal(t, "1", seen.expectedAttempt)
			require.Equal(t, "automatic", seen.origin)
			require.Equal(t, "next_attempt", seen.mode)
		case <-ctx.Done():
			t.Fatalf("retry authorization did not reach gRPC: %v", ctx.Err())
		}
	}
	retriedRun, err := runRepo.FindLatestByAssessmentID(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, 2, retriedRun.Attempt().Number)
	require.Equal(t, "failed", retriedRun.Attempt().Status.String())
	require.NoError(t, db.Table("runtime_checkpoint").Where("scope = ? AND assessment_id = ?", "evaluation_run", created.ID).Count(&count).Error)
	require.EqualValues(t, 2, count, "NSQ redelivery must not create attempt 3")
	require.Eventually(t, func() bool {
		var state string
		return db.Table("rm_outbox").Select("state").Where("message_id = ?", retryIntent.MessageID).Scan(&state).Error == nil && state == "published"
	}, 5*time.Second, 25*time.Millisecond)

	// Exercise the two operator-owned branches only after the automatic
	// path has finished. The disposable policy is reduced to one automatic
	// attempt so the original failed Run requires manual approval.
	originalBusiness, originalOutbox := retrygovernance.BusinessPolicy(), retrygovernance.OutboxPolicy()
	limitedBusiness := originalBusiness
	limitedBusiness.Version += "-m5-governed-delivery"
	limitedBusiness.MaxAutomaticAttempts = 1
	require.NoError(t, retrygovernance.ConfigurePolicies(limitedBusiness, originalOutbox))
	t.Cleanup(func() { require.NoError(t, retrygovernance.ConfigurePolicies(originalBusiness, originalOutbox)) })
	createSubmitted := func(answerSheetID uint64) uint64 {
		record, createErr := intake.CreateForAnswerSheet(ctx, appintake.CreateCommand{
			OrgID: 1, TesteeID: 2, AnswerSheetID: answerSheetID,
			QuestionnaireCode: "Q-001", QuestionnaireVersion: "v1", OriginType: "adhoc",
			ModelKind: &kind, ModelCode: &code, ModelVersion: &version,
		})
		require.NoError(t, createErr)
		_, submitErr := intake.SubmitForEvaluation(ctx, record.ID)
		require.NoError(t, submitErr)
		return record.ID
	}
	governed := appoperator.NewGovernedRetryService(assessmentRepo, runRepo, runner, stager, m5GovernedAccess{})
	actor := appoperator.Actor{OrgID: 1, OperatorUserID: 9}
	permissionContext := func(action string) context.Context {
		return appauthz.WithSnapshot(ctx, &appauthz.Snapshot{Permissions: []appauthz.Permission{{
			Resource: appauthz.AssessmentResource, Action: action, Mode: appauthz.AuthorizationModeUnconditional,
		}}})
	}
	assertGovernedDelivery := func(assessmentID uint64, eventID, origin, actionRequestID string) {
		t.Helper()
		var first, second m5FailedDelivery
		for first.messageID != eventID {
			select {
			case first = <-retryDeliveries:
			case <-ctx.Done():
				t.Fatalf("%s retry delivery missing: %v", origin, ctx.Err())
			}
		}
		for second.messageID != eventID {
			select {
			case second = <-retryDeliveries:
			case <-ctx.Done():
				t.Fatalf("%s retry redelivery missing: %v", origin, ctx.Err())
			}
		}
		require.Equal(t, first.brokerID, second.brokerID)
		require.EqualValues(t, 1, first.attempts)
		require.Greater(t, second.attempts, first.attempts)
		require.ErrorContains(t, first.err, "controlled lost retry consumer ACK")
		require.NoError(t, second.err)
		seenCount := 0
		for seenCount < 2 {
			select {
			case seen := <-metadataSeen:
				if seen.eventID != eventID {
					continue
				}
				require.Equal(t, "1", seen.expectedAttempt)
				require.Equal(t, origin, seen.origin)
				require.Equal(t, "next_attempt", seen.mode)
				require.Equal(t, actionRequestID, seen.actionRequestID)
				seenCount++
			case <-ctx.Done():
				t.Fatalf("%s gRPC authorization missing: %v", origin, ctx.Err())
			}
		}
		latest, latestErr := runRepo.FindLatestByAssessmentID(ctx, assessmentID)
		require.NoError(t, latestErr)
		require.Equal(t, 2, latest.Attempt().Number)
		require.Equal(t, "failed", latest.Attempt().Status.String())
		require.Equal(t, retrygovernance.AttemptOrigin(origin), latest.Origin())
		require.Equal(t, actionRequestID, latest.ActionRequestID())
		require.Equal(t, retrygovernance.DispositionManualRequired, latest.RetryDecision().Disposition)
		require.NoError(t, db.Table("runtime_checkpoint").Where("scope = ? AND assessment_id = ?", "evaluation_run", assessmentID).Count(&count).Error)
		require.EqualValues(t, 2, count, "redelivery must not create an extra governed attempt")
		require.Eventually(t, func() bool {
			var state string
			return db.Table("rm_outbox").Select("state").Where("message_id = ?", eventID).Scan(&state).Error == nil && state == "published"
		}, 5*time.Second, 25*time.Millisecond)
	}

	manualID := createSubmitted(702)
	require.Error(t, engine.Evaluate(ctx, manualID))
	manualRun, err := runRepo.FindLatestByAssessmentID(ctx, manualID)
	require.NoError(t, err)
	require.Equal(t, retrygovernance.DispositionManualRequired, manualRun.RetryDecision().Disposition)
	manual := appoperator.GovernedRetryCommand{
		AssessmentID: manualID, ExpectedAttempt: 1, Origin: retrygovernance.AttemptOriginManual,
		RequestID: "m5-manual-business-delivery", Reason: "controlled retry",
		AuthorizationSubject: "user:9", AuthorizationAction: "retry",
	}
	_, err = governed.Authorize(ctx, actor, manual)
	require.Error(t, err, "retry requires an authorization snapshot")
	_, err = governed.Authorize(permissionContext("retry"), actor, manual)
	require.NoError(t, err)
	manualAuthorized, err := runRepo.FindLatestByAssessmentID(ctx, manualID)
	require.NoError(t, err)
	manualEventID := manualAuthorized.RetryDecision().RetryEventID
	require.NotEmpty(t, manualEventID)
	assertGovernedDelivery(manualID, manualEventID, "manual", manual.RequestID)

	forceID := createSubmitted(703)
	terminalEngine := appexecute.NewEngine(assessmentRepo, m5TerminalInput{},
		appexecute.WithRunRepository(runRepo), appexecute.WithTransactionalOutbox(runner, stager))
	require.Error(t, terminalEngine.Evaluate(ctx, forceID))
	terminalRun, err := runRepo.FindLatestByAssessmentID(ctx, forceID)
	require.NoError(t, err)
	require.Equal(t, retrygovernance.DispositionTerminal, terminalRun.RetryDecision().Disposition)
	force := appoperator.GovernedRetryCommand{
		AssessmentID: forceID, ExpectedAttempt: 1, Origin: retrygovernance.AttemptOriginForce,
		RequestID: "m5-force-business-delivery", Reason: "controlled force retry",
		AuthorizationSubject: "user:9", AuthorizationAction: "force_retry",
	}
	_, err = governed.Authorize(permissionContext("retry"), actor, force)
	require.Error(t, err, "ordinary retry permission cannot authorize force")
	_, err = governed.Authorize(permissionContext("force_retry"), actor, force)
	require.NoError(t, err)
	forceAuthorized, err := runRepo.FindLatestByAssessmentID(ctx, forceID)
	require.NoError(t, err)
	forceEventID := forceAuthorized.RetryDecision().RetryEventID
	require.NotEmpty(t, forceEventID)
	assertGovernedDelivery(forceID, forceEventID, "force", force.RequestID)
}

func firstValue(md metadata.MD, key string) string {
	if values := md.Get(key); len(values) > 0 {
		return values[0]
	}
	return ""
}
