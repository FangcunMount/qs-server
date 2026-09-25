//go:build integration && reliable_messaging_m4

package interpretation_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/FangcunMount/component-base/pkg/event"
	"github.com/FangcunMount/component-base/pkg/messaging"
	interpretationpb "github.com/FangcunMount/qs-server/api/grpc/gen/interpretation"
	appautomation "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/automation"
	execution "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/automation/execution"
	domaingeneration "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/generation"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	interpretationrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/run"
	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	mongostandard "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/standardoutbox"
	evaluationfact "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/mongodbtest"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	"github.com/FangcunMount/qs-server/internal/worker/handlers"
	"github.com/FangcunMount/reliable-messaging/relay"
	sdkmongo "github.com/FangcunMount/reliable-messaging/storage/mongo"
	sdknsq "github.com/FangcunMount/reliable-messaging/transport/nsq"
	"github.com/nsqio/go-nsq"
	"go.mongodb.org/mongo-driver/bson"
	"google.golang.org/grpc/metadata"
)

func TestInterpretationAutomaticFailureCommitsWithStandardMongoScheduledRetry(t *testing.T) {
	fixture, stager := newStandardInterpretationFixture(t)
	db := fixture.db
	generation, run := fixture.start(t)
	fixture.now = time.Now().Add(time.Second).Truncate(time.Millisecond)
	committer, err := execution.NewInterpretationCommitter(
		fixture.runner, fixture.generations, fixture.runs, fixture.reports, stager, nil, fixture.catalog,
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := committer.CommitFailure(t.Context(), execution.CommitFailureRequest{
		Generation: generation, Run: run, OutcomeID: generation.Key().OutcomeID,
		Association: domainreport.Association{OrgID: 1, AssessmentID: meta.New(), TesteeID: 8},
		Failure: interpretationrun.Failure{
			Kind: interpretationrun.FailureKindBuild, Code: "build_failed", SafeMessage: "failed", Retryable: true,
		},
		FailedAt: fixture.now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Generation.Status() != domaingeneration.StatusFailed || result.Run.Status() != interpretationrun.StatusFailed {
		t.Fatalf("committed state generation=%s run=%s", result.Generation.Status(), result.Run.Status())
	}
	decision := result.Run.RetryDecision()
	if decision == nil || decision.Disposition != retrygovernance.DispositionAutomatic || decision.NextAttemptAt == nil || !decision.NextAttemptAt.After(fixture.now) {
		t.Fatalf("automatic retry decision=%+v", decision)
	}
	persistedGeneration, err := fixture.generations.FindByID(t.Context(), generation.ID())
	if err != nil {
		t.Fatal(err)
	}
	persistedRun, err := fixture.runs.FindByID(t.Context(), run.ID())
	if err != nil {
		t.Fatal(err)
	}
	if persistedGeneration.Status() != domaingeneration.StatusFailed || persistedRun.Status() != interpretationrun.StatusFailed {
		t.Fatalf("persisted state generation=%s run=%s", persistedGeneration.Status(), persistedRun.Status())
	}
	collection := db.Collection("rm_outbox")
	count, err := collection.CountDocuments(t.Context(), bson.M{})
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("standard Mongo intents=%d, want failure and scheduled retry", count)
	}
	for _, eventType := range []string{eventcatalog.InterpretationReportFailed, eventcatalog.InterpretationRetryRequested} {
		var row struct {
			MessageID     string    `bson:"message_id"`
			Scope         string    `bson:"scope"`
			State         string    `bson:"state"`
			NextAttemptAt time.Time `bson:"next_attempt_at"`
		}
		if err := collection.FindOne(t.Context(), bson.M{"event_type": eventType}).Decode(&row); err != nil {
			t.Fatal(err)
		}
		if row.Scope != "org:1" || row.State != "pending" {
			t.Fatalf("%s scope=%s state=%s", eventType, row.Scope, row.State)
		}
		if eventType == eventcatalog.InterpretationRetryRequested {
			if row.MessageID != decision.RetryEventID || !row.NextAttemptAt.Equal(decision.NextAttemptAt.UTC().Truncate(time.Millisecond)) {
				t.Fatalf("scheduled retry message=%s due=%s; decision=%+v", row.MessageID, row.NextAttemptAt, decision)
			}
		}
	}
	legacyCount, err := db.Collection("domain_event_outbox").CountDocuments(t.Context(), bson.M{})
	if err != nil {
		t.Fatal(err)
	}
	if legacyCount != 0 {
		t.Fatalf("standard failure wrote %d legacy intents", legacyCount)
	}

	abortedGeneration, abortedRun := fixture.start(t)
	fixture.now = time.Now().Add(time.Second).Truncate(time.Millisecond)
	abort := errors.New("abort after scheduled stage")
	failing, err := execution.NewInterpretationCommitter(
		fixture.runner, fixture.generations, fixture.runs, fixture.reports,
		failAfterScheduledStage{inner: stager, failure: abort}, nil, fixture.catalog,
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = failing.CommitFailure(t.Context(), execution.CommitFailureRequest{
		Generation: abortedGeneration, Run: abortedRun, OutcomeID: abortedGeneration.Key().OutcomeID,
		Association: domainreport.Association{OrgID: 1, AssessmentID: meta.New(), TesteeID: 8},
		Failure: interpretationrun.Failure{
			Kind: interpretationrun.FailureKindBuild, Code: "build_failed", SafeMessage: "failed", Retryable: true,
		},
		FailedAt: fixture.now,
	})
	if !errors.Is(err, abort) {
		t.Fatalf("scheduled-stage failure error=%v", err)
	}
	fixture.assertRunningAndNoOutbox(t, abortedGeneration, abortedRun)
	count, err = collection.CountDocuments(t.Context(), bson.M{})
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("aborted failure left %d standard intents; want prior 2", count)
	}
}

func TestInterpretationSuccessCommitsReportWithStandardMongoIntent(t *testing.T) {
	fixture, stager := newStandardInterpretationFixture(t)
	generation, run := fixture.start(t)
	at := time.Now().Add(time.Second).Truncate(time.Millisecond)
	artifact := integrationArtifact(t, generation, run, at)
	committer, err := execution.NewInterpretationCommitter(
		fixture.runner, fixture.generations, fixture.runs, fixture.reports, stager, nil, fixture.catalog,
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := committer.CommitSuccess(t.Context(), execution.CommitSuccessRequest{
		Generation: generation, Run: run, InterpretReport: artifact,
		BuilderIdentity:      domainreport.BuilderIdentityFactorScoring,
		ContentSchemaVersion: domainreport.ContentSchemaVersionV1,
		CompletedAt:          at,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Generation.Status() != domaingeneration.StatusGenerated || result.Run.Status() != interpretationrun.StatusSucceeded {
		t.Fatalf("committed state generation=%s run=%s", result.Generation.Status(), result.Run.Status())
	}
	persistedReport, err := fixture.reports.FindByID(t.Context(), artifact.ID())
	if err != nil || persistedReport == nil {
		t.Fatalf("persisted report=%v err=%v", persistedReport, err)
	}
	assertMongoDocumentCount(t, fixture.db.Collection("rm_outbox"), bson.M{"event_type": eventcatalog.InterpretationReportGenerated}, 1)
	assertMongoDocumentCount(t, fixture.db.Collection("domain_event_outbox"), bson.M{}, 0)
}

// A real committed failure must reach the existing Worker through the SDK
// relay and NSQ. The retry remains unpublished until its durable due time.
// The automation client here records the call boundary; business attempt
// idempotency remains the apiserver's responsibility and needs separate proof.
func TestInterpretationReportEventsReachWorkerThroughStandardMongoAndNSQ(t *testing.T) {
	if os.Getenv("RM_QS_NSQ_TCP") != "nsqd:4150" {
		t.Fatal("disposable nsqd:4150 required")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	fixture, stager := newStandardInterpretationFixture(t)
	collection := fixture.db.Collection("rm_outbox")
	if _, err := collection.Indexes().CreateMany(ctx, sdkmongo.Indexes()); err != nil {
		t.Fatal(err)
	}
	generation, run := fixture.start(t)
	failedAt := time.Now().Truncate(time.Millisecond)
	committer, err := execution.NewInterpretationCommitter(
		fixture.runner, fixture.generations, fixture.runs, fixture.reports, stager, nil, fixture.catalog,
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := committer.CommitFailure(ctx, execution.CommitFailureRequest{
		Generation: generation, Run: run, OutcomeID: generation.Key().OutcomeID,
		Association: domainreport.Association{OrgID: 1, AssessmentID: meta.New(), TesteeID: 8},
		Failure: interpretationrun.Failure{
			Kind: interpretationrun.FailureKindBuild, Code: "build_failed", SafeMessage: "failed", Retryable: true,
		},
		FailedAt: failedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	decision := result.Run.RetryDecision()
	if decision == nil || decision.NextAttemptAt == nil || !decision.NextAttemptAt.After(time.Now()) {
		t.Fatalf("retry is not durably scheduled in the future: %+v", decision)
	}

	const topic, channel = "qs.evaluation.lifecycle", "m5-standard-report-proof"
	nsqConfig := nsq.NewConfig()
	nsqConfig.HeartbeatInterval = time.Second
	nsqConfig.ReadTimeout, nsqConfig.WriteTimeout = 3*time.Second, time.Second
	consumer, err := nsq.NewConsumer(topic, channel, nsqConfig)
	if err != nil {
		t.Fatal(err)
	}
	consumer.SetLogger(nil, nsq.LogLevelError)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	automation := &reportRetryCallRecorder{}
	projection := &reportStatusCallRecorder{}
	deps := &handlers.Dependencies{Logger: logger, InterpretationAutomationClient: automation, ReportStatusReporter: projection}
	registry := handlers.NewRegistry()
	type delivery struct {
		eventType string
		messageID string
		err       error
	}
	deliveries := make(chan delivery, 4)
	consumer.AddHandler(nsq.HandlerFunc(func(raw *nsq.Message) error {
		decoded, recognized, decodeErr := messaging.DecodeMessagePayload(raw.Body)
		if decodeErr == nil && !recognized {
			decodeErr = errors.New("standard NSQ envelope not recognized")
		}
		eventType, messageID := "", ""
		if decodeErr == nil {
			eventType, messageID = decoded.Metadata["event_type"], decoded.UUID
			var name string
			switch eventType {
			case eventcatalog.InterpretationReportFailed:
				name = "interpretation_report_failed_handler"
			case eventcatalog.InterpretationRetryRequested:
				name = "interpretation_retry_requested_handler"
			case eventcatalog.InterpretationReportGenerated:
				name = "interpretation_report_generated_handler"
			default:
				decodeErr = fmt.Errorf("unexpected event type %q", eventType)
			}
			if decodeErr == nil {
				handler, ok := registry.Create(name, deps)
				if !ok {
					decodeErr = fmt.Errorf("Worker handler %q is missing", name)
				} else {
					decodeErr = handler(ctx, eventType, decoded.Payload)
				}
			}
		}
		select {
		case deliveries <- delivery{eventType: eventType, messageID: messageID, err: decodeErr}:
		case <-ctx.Done():
			return ctx.Err()
		}
		return decodeErr
	}))
	if err := consumer.ConnectToNSQD(os.Getenv("RM_QS_NSQ_TCP")); err != nil {
		t.Fatal(err)
	}
	defer func() {
		consumer.Stop()
		select {
		case <-consumer.StopChan:
		case <-time.After(5 * time.Second):
			t.Error("NSQ consumer did not stop")
		}
	}()
	producer, err := nsq.NewProducer(os.Getenv("RM_QS_NSQ_TCP"), nsqConfig)
	if err != nil {
		t.Fatal(err)
	}
	producer.SetLogger(nil, nsq.LogLevelError)
	defer producer.Stop()
	publisher, err := sdknsq.New(producer, map[string]string{topic: topic}, 1)
	if err != nil {
		t.Fatal(err)
	}
	store, err := sdkmongo.New(collection)
	if err != nil {
		t.Fatal(err)
	}
	forwarder, err := relay.New(store, publisher, relay.Config{
		Concurrency: 1, PollInterval: 50 * time.Millisecond, Lease: 5 * time.Second,
		PublishTimeout: 2 * time.Second, WriteTimeout: time.Second,
		Retry: standardoutbox.SDKRetryPolicy(), Observe: func(relay.Event) {},
	})
	if err != nil {
		t.Fatal(err)
	}
	relayCtx, stopRelay := context.WithCancel(ctx)
	relayDone := make(chan error, 1)
	go func() { relayDone <- forwarder.Run(relayCtx) }()
	defer func() {
		stopRelay()
		if err := <-relayDone; err != nil {
			t.Errorf("standard relay: %v", err)
		}
		if err := publisher.Drain(ctx); err != nil {
			t.Errorf("publisher drain: %v", err)
		}
	}()

	select {
	case got := <-deliveries:
		if got.err != nil || got.eventType != eventcatalog.InterpretationReportFailed || got.messageID == decision.RetryEventID {
			t.Fatalf("failed event delivery: %+v", got)
		}
	case <-ctx.Done():
		t.Fatal("report.failed not delivered", ctx.Err())
	}
	if automation.Count() != 0 || projection.Count() != 0 {
		t.Fatal("automatic report.failed invoked generation or terminal status projection")
	}
	var scheduled struct {
		NextAttemptAt time.Time `bson:"next_attempt_at"`
		State         string    `bson:"state"`
	}
	if err := collection.FindOne(ctx, bson.M{"message_id": decision.RetryEventID}).Decode(&scheduled); err != nil {
		t.Fatal(err)
	}
	if scheduled.State != "pending" || !scheduled.NextAttemptAt.Equal(decision.NextAttemptAt.UTC().Truncate(time.Millisecond)) {
		t.Fatalf("retry was not left pending until original due time: %+v decision=%+v", scheduled, decision)
	}
	select {
	case got := <-deliveries:
		t.Fatalf("retry delivered before due: %+v", got)
	case <-time.After(200 * time.Millisecond):
	}
	// Advance the isolated due index after proving the original schedule. This
	// avoids waiting for production retry backoff and does not change the payload.
	if _, err := collection.UpdateOne(ctx, bson.M{"message_id": decision.RetryEventID, "state": "pending"}, bson.M{"$set": bson.M{"next_attempt_at": time.Now().Add(-time.Second)}}); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-deliveries:
		if got.err != nil || got.eventType != eventcatalog.InterpretationRetryRequested || got.messageID != decision.RetryEventID {
			t.Fatalf("retry event delivery: %+v", got)
		}
	case <-ctx.Done():
		t.Fatal("retry.requested not delivered", ctx.Err())
	}
	if automation.Count() != 1 || projection.Count() != 0 {
		t.Fatalf("Worker effects: generation calls=%d status projections=%d", automation.Count(), projection.Count())
	}
	call := automation.Last()
	if call.outcomeID != generation.Key().OutcomeID.String() ||
		call.metadata.Get("x-retry-event-id")[0] != decision.RetryEventID ||
		call.metadata.Get("x-retry-origin")[0] != "automatic" ||
		call.metadata.Get("x-retry-expected-attempt")[0] != fmt.Sprint(decision.Attempt) {
		t.Fatalf("Worker lost retry authority: outcome=%q metadata=%v decision=%+v", call.outcomeID, call.metadata, decision)
	}

	generated, generatedRun := fixture.start(t)
	completedAt := time.Now().Truncate(time.Millisecond)
	artifact := integrationArtifact(t, generated, generatedRun, completedAt)
	if _, err := committer.CommitSuccess(ctx, execution.CommitSuccessRequest{
		Generation: generated, Run: generatedRun, InterpretReport: artifact,
		BuilderIdentity: domainreport.BuilderIdentityFactorScoring, ContentSchemaVersion: domainreport.ContentSchemaVersionV1,
		CompletedAt: completedAt,
	}); err != nil {
		t.Fatal(err)
	}
	var generatedRow struct {
		MessageID string `bson:"message_id"`
	}
	if err := collection.FindOne(ctx, bson.M{"event_type": eventcatalog.InterpretationReportGenerated}).Decode(&generatedRow); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-deliveries:
		if got.err != nil || got.eventType != eventcatalog.InterpretationReportGenerated || got.messageID != generatedRow.MessageID {
			t.Fatalf("generated event delivery: %+v", got)
		}
	case <-ctx.Done():
		t.Fatal("report.generated not delivered", ctx.Err())
	}
	if projection.Completed() != 1 || automation.Count() != 1 {
		t.Fatalf("report.generated effects: completed=%d generation calls=%d", projection.Completed(), automation.Count())
	}

	terminalGeneration, terminalRun := fixture.start(t)
	terminalAssessmentID := meta.New()
	terminal, err := committer.CommitFailure(ctx, execution.CommitFailureRequest{
		Generation: terminalGeneration, Run: terminalRun, OutcomeID: terminalGeneration.Key().OutcomeID,
		Association: domainreport.Association{OrgID: 1, AssessmentID: terminalAssessmentID, TesteeID: 8},
		Failure: interpretationrun.Failure{
			Kind: interpretationrun.FailureKindBuild, Code: "build_failed", SafeMessage: "failed", Retryable: false,
		},
		FailedAt: time.Now().Truncate(time.Millisecond),
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision := terminal.Run.RetryDecision(); decision == nil || decision.Disposition != retrygovernance.DispositionTerminal {
		t.Fatalf("terminal failure retry decision=%+v", decision)
	}
	select {
	case got := <-deliveries:
		if got.err != nil || got.eventType != eventcatalog.InterpretationReportFailed {
			t.Fatalf("terminal report.failed delivery: %+v", got)
		}
	case <-ctx.Done():
		t.Fatal("terminal report.failed not delivered", ctx.Err())
	}
	if automation.Count() != 1 {
		t.Fatal("terminal failure invoked automatic generation")
	}
	forceOutcome := evaluationfact.NewRecord(evaluationfact.NewRecordInput{
		ID: terminalGeneration.Key().OutcomeID, OrgID: 1, AssessmentID: terminalAssessmentID, TesteeID: 8,
	})
	forceService := appautomation.NewGovernedRetryService(
		fixture.generations, fixture.runs, manualRetryOutcomeRepo{record: forceOutcome}, fixture.runner, stager,
	)
	const forceRequestID = "m5-force-nsq-request"
	forced, err := forceService.Authorize(ctx, appautomation.GovernedRetryCommand{
		OrgID: 1, GenerationID: terminalRun.GenerationID(), ExpectedAttempt: terminalRun.Attempt(),
		Origin: retrygovernance.AttemptOriginForce, RequestID: forceRequestID, Reason: "operator review",
	})
	if err != nil {
		t.Fatal(err)
	}
	forceDecision := forced.RetryDecision()
	if forceDecision == nil || forceDecision.RetryEventID == "" || forceDecision.ActionRequestID != forceRequestID {
		t.Fatalf("force authorization decision=%+v", forceDecision)
	}
	select {
	case got := <-deliveries:
		if got.err != nil || got.eventType != eventcatalog.InterpretationRetryRequested || got.messageID != forceDecision.RetryEventID {
			t.Fatalf("force retry delivery: %+v", got)
		}
	case <-ctx.Done():
		t.Fatal("force retry.requested not delivered", ctx.Err())
	}
	if automation.Count() != 2 {
		t.Fatalf("force retry Worker calls=%d, want 2 total", automation.Count())
	}
	forceCall := automation.Last()
	if got := forceCall.metadata.Get("x-retry-origin"); len(got) != 1 || got[0] != "force" {
		t.Fatalf("force origin lost across NSQ: %v", forceCall.metadata)
	}
	if got := forceCall.metadata.Get("x-retry-action-request-id"); len(got) != 1 || got[0] != forceRequestID {
		t.Fatalf("force request ID lost across NSQ: %v", forceCall.metadata)
	}
}

type reportRetryCall struct {
	outcomeID string
	metadata  metadata.MD
}

type reportRetryCallRecorder struct {
	mu    sync.Mutex
	calls []reportRetryCall
}

func (r *reportRetryCallRecorder) GenerateReportFromOutcome(ctx context.Context, outcomeID string) (*interpretationpb.GenerateReportFromAssessmentResponse, error) {
	md, _ := metadata.FromOutgoingContext(ctx)
	r.mu.Lock()
	r.calls = append(r.calls, reportRetryCall{outcomeID: outcomeID, metadata: md.Copy()})
	r.mu.Unlock()
	return &interpretationpb.GenerateReportFromAssessmentResponse{Status: "already_generated"}, nil
}

func (r *reportRetryCallRecorder) Count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.calls)
}

func (r *reportRetryCallRecorder) Last() reportRetryCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls[len(r.calls)-1]
}

type reportStatusCallRecorder struct {
	count     int
	completed int
	temporary int
}

func (r *reportStatusCallRecorder) SetProcessing(context.Context, string, string, string) { r.count++ }
func (r *reportStatusCallRecorder) SetCompleted(context.Context, string, string, string) {
	r.count++
	r.completed++
}
func (r *reportStatusCallRecorder) SetFailed(context.Context, string, string, string, string) {
	r.count++
}
func (r *reportStatusCallRecorder) SetTemporarilyUnavailable(context.Context, string, string, string, string) {
	r.count++
	r.temporary++
}
func (r *reportStatusCallRecorder) Count() int     { return r.count }
func (r *reportStatusCallRecorder) Completed() int { return r.completed }
func (r *reportStatusCallRecorder) Temporary() int { return r.temporary }

func newStandardInterpretationFixture(t *testing.T) (interpretationMongoFixture, *mongostandard.Stager) {
	t.Helper()
	_, db := mongodbtest.ReplicaSetDatabase(t)
	fixture := newInterpretationMongoFixture(t, db)
	if err := db.CreateCollection(t.Context(), "rm_outbox"); err != nil {
		t.Fatal(err)
	}
	wire, err := eventcatalog.Load("../../../../../configs/events.yaml")
	if err != nil {
		t.Fatal(err)
	}
	stager, err := mongostandard.NewStager(db.Collection("rm_outbox"), eventcatalog.NewCatalog(wire), eventruntime.SourceAPIServer)
	if err != nil {
		t.Fatal(err)
	}
	return fixture, stager
}

type failAfterScheduledStage struct {
	inner   *mongostandard.Stager
	failure error
}

func (s failAfterScheduledStage) Stage(ctx context.Context, events ...event.DomainEvent) error {
	return s.inner.Stage(ctx, events...)
}

func (s failAfterScheduledStage) StageAt(ctx context.Context, dueAt time.Time, events ...event.DomainEvent) error {
	if err := s.inner.StageAt(ctx, dueAt, events...); err != nil {
		return err
	}
	return s.failure
}
