//go:build integration && reliable_messaging_m4

package interpretation_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/FangcunMount/component-base/pkg/eventcodec"
	"github.com/FangcunMount/component-base/pkg/messaging"
	automation "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/automation"
	execution "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/automation/execution"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	interpretationrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/run"
	evaluationfact "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventoutcome "github.com/FangcunMount/qs-server/internal/pkg/eventing/outcome"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	"github.com/FangcunMount/qs-server/internal/worker/handlers"
	"go.mongodb.org/mongo-driver/bson"
)

func TestInterpretationGovernedRetryAuthorizationAndStandardIntentCommitTogether(t *testing.T) {
	originalBusiness, originalOutbox := retrygovernance.BusinessPolicy(), retrygovernance.OutboxPolicy()
	proofPolicy := originalBusiness
	proofPolicy.Version = "m5-manual-exhaustion-proof"
	proofPolicy.MaxAutomaticAttempts = 1
	if err := retrygovernance.ConfigurePolicies(proofPolicy, originalOutbox); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := retrygovernance.ConfigurePolicies(originalBusiness, originalOutbox); err != nil {
			t.Errorf("restore retry policy: %v", err)
		}
	})

	fixture, stager := newStandardInterpretationFixture(t)
	committer, err := execution.NewInterpretationCommitter(
		fixture.runner, fixture.generations, fixture.runs, fixture.reports, stager, nil, fixture.catalog,
	)
	if err != nil {
		t.Fatal(err)
	}
	makeFailed := func(t *testing.T, retryable bool, want retrygovernance.Disposition) (*interpretationrun.InterpretationRun, meta.ID, meta.ID) {
		t.Helper()
		generation, run := fixture.start(t)
		assessmentID := meta.New()
		result, err := committer.CommitFailure(t.Context(), execution.CommitFailureRequest{
			Generation: generation, Run: run, OutcomeID: generation.Key().OutcomeID,
			Association: domainreport.Association{OrgID: 1, AssessmentID: assessmentID, TesteeID: 8},
			Failure: interpretationrun.Failure{
				Kind: interpretationrun.FailureKindBuild, Code: "build_failed", SafeMessage: "failed", Retryable: retryable,
			},
			FailedAt: time.Now(),
		})
		if err != nil {
			t.Fatal(err)
		}
		if decision := result.Run.RetryDecision(); decision == nil || decision.Disposition != want {
			t.Fatalf("failure retry decision = %+v, want %s", decision, want)
		}
		return result.Run, generation.Key().OutcomeID, assessmentID
	}

	run, outcomeID, assessmentID := makeFailed(t, true, retrygovernance.DispositionManualRequired)
	assertMongoDocumentCount(t, fixture.db.Collection("rm_outbox"), bson.M{"event_type": eventcatalog.InterpretationRetryRequested}, 0)
	var failedRow struct {
		Payload []byte `bson:"payload"`
	}
	if err := fixture.db.Collection("rm_outbox").FindOne(t.Context(), bson.M{"event_type": eventcatalog.InterpretationReportFailed}).Decode(&failedRow); err != nil {
		t.Fatal(err)
	}
	failedWire, recognized, err := messaging.DecodeMessagePayload(failedRow.Payload)
	if err != nil || !recognized {
		t.Fatalf("failed event wire decode: recognized=%t err=%v", recognized, err)
	}
	workerAutomation := &reportRetryCallRecorder{}
	projection := &reportStatusCallRecorder{}
	workerDeps := &handlers.Dependencies{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), InterpretationAutomationClient: workerAutomation,
		ReportStatusReporter: projection, DisableAutomaticRetry: true,
	}
	registry := handlers.NewRegistry()
	failedHandler, ok := registry.Create("interpretation_report_failed_handler", workerDeps)
	if !ok {
		t.Fatal("Worker report.failed handler is missing")
	}
	if err := failedHandler(t.Context(), eventcatalog.InterpretationReportFailed, failedWire.Payload); err != nil {
		t.Fatal(err)
	}
	if projection.Temporary() != 1 || workerAutomation.Count() != 0 {
		t.Fatalf("manual-required failure projection=%d automation calls=%d", projection.Temporary(), workerAutomation.Count())
	}
	outcome := evaluationfact.NewRecord(evaluationfact.NewRecordInput{ID: outcomeID, OrgID: 1, AssessmentID: assessmentID, TesteeID: 8})
	service := automation.NewGovernedRetryService(fixture.generations, fixture.runs, manualRetryOutcomeRepo{record: outcome}, fixture.runner, stager)
	command := automation.GovernedRetryCommand{
		OrgID: 1, GenerationID: run.GenerationID(), ExpectedAttempt: run.Attempt(),
		Origin: retrygovernance.AttemptOriginManual, RequestID: "m5-manual-request", Reason: "operator review",
	}
	wrongOrg := command
	wrongOrg.OrgID = 2
	if _, err := service.Authorize(t.Context(), wrongOrg); err == nil {
		t.Fatal("cross-organization retry authorization succeeded")
	}
	authorized, err := service.Authorize(t.Context(), command)
	if err != nil {
		t.Fatal(err)
	}
	decision := authorized.RetryDecision()
	if decision == nil || decision.Disposition != retrygovernance.DispositionAutomatic ||
		decision.RetryEventID == "" || decision.ActionRequestID != command.RequestID || decision.NextAttemptAt == nil {
		t.Fatalf("authorized retry decision = %+v", decision)
	}
	persisted, err := fixture.runs.FindByID(t.Context(), run.ID())
	if err != nil {
		t.Fatal(err)
	}
	if persisted.RetryDecision() == nil || persisted.RetryDecision().RetryEventID != decision.RetryEventID {
		t.Fatalf("retry authority was not persisted with intent: %+v", persisted.RetryDecision())
	}
	var row struct {
		MessageID     string    `bson:"message_id"`
		NextAttemptAt time.Time `bson:"next_attempt_at"`
		Payload       []byte    `bson:"payload"`
	}
	if err := fixture.db.Collection("rm_outbox").FindOne(t.Context(), bson.M{"message_id": decision.RetryEventID}).Decode(&row); err != nil {
		t.Fatal(err)
	}
	if row.MessageID != decision.RetryEventID || !row.NextAttemptAt.Equal(decision.NextAttemptAt.UTC().Truncate(time.Millisecond)) {
		t.Fatalf("manual retry intent = %+v; decision = %+v", row, decision)
	}
	decoded, recognized, err := messaging.DecodeMessagePayload(row.Payload)
	if err != nil || !recognized {
		t.Fatalf("standard message envelope decode: recognized=%t err=%v", recognized, err)
	}
	envelope, err := eventcodec.DecodeEnvelope(decoded.Payload)
	if err != nil {
		t.Fatal(err)
	}
	var payload eventoutcome.InterpretationRetryRequestedPayload
	if err := json.Unmarshal(envelope.Data, &payload); err != nil {
		t.Fatal(err)
	}
	if envelope.ID != decision.RetryEventID || payload.AttemptOrigin != "manual" ||
		payload.ActionRequestID != command.RequestID || payload.ExpectedAttempt != run.Attempt() {
		t.Fatalf("manual retry wire lost authorization: event=%s payload=%+v", envelope.ID, payload)
	}
	retryHandler, ok := registry.Create("interpretation_retry_requested_handler", workerDeps)
	if !ok {
		t.Fatal("Worker retry.requested handler is missing")
	}
	if err := retryHandler(t.Context(), eventcatalog.InterpretationRetryRequested, decoded.Payload); err != nil {
		t.Fatal(err)
	}
	if workerAutomation.Count() != 1 {
		t.Fatalf("manual Worker retry calls=%d, want 1", workerAutomation.Count())
	}
	metadata := workerAutomation.Last().metadata
	if action := metadata.Get("x-retry-action-request-id"); len(action) != 1 || action[0] != command.RequestID {
		t.Fatalf("manual Worker lost action request ID: %v", metadata)
	}
	if origin := metadata.Get("x-retry-origin"); len(origin) != 1 || origin[0] != "manual" {
		t.Fatalf("manual Worker lost origin: %v", metadata)
	}
	if _, err := service.Authorize(t.Context(), command); err == nil {
		t.Fatal("duplicate manual authorization succeeded")
	}
	assertMongoDocumentCount(t, fixture.db.Collection("rm_outbox"), bson.M{"event_type": eventcatalog.InterpretationRetryRequested}, 1)

	rollbackRun, rollbackOutcomeID, rollbackAssessmentID := makeFailed(t, true, retrygovernance.DispositionManualRequired)
	rollbackOutcome := evaluationfact.NewRecord(evaluationfact.NewRecordInput{ID: rollbackOutcomeID, OrgID: 1, AssessmentID: rollbackAssessmentID, TesteeID: 8})
	rollbackService := automation.NewGovernedRetryService(
		fixture.generations, fixture.runs, manualRetryOutcomeRepo{record: rollbackOutcome}, fixture.runner,
		failAfterScheduledStage{inner: stager, failure: errors.New("injected stage failure")},
	)
	rollbackCommand := command
	rollbackCommand.GenerationID = rollbackRun.GenerationID()
	rollbackCommand.RequestID = "m5-rollback-request"
	if _, err := rollbackService.Authorize(t.Context(), rollbackCommand); err == nil {
		t.Fatal("injected stage failure did not abort authorization")
	}
	rolledBack, err := fixture.runs.FindByID(t.Context(), rollbackRun.ID())
	if err != nil {
		t.Fatal(err)
	}
	if rolledBack.RetryDecision() == nil || rolledBack.RetryDecision().Disposition != retrygovernance.DispositionManualRequired ||
		rolledBack.RetryDecision().RetryEventID != "" || rolledBack.RetryDecision().ActionRequestID != "" {
		t.Fatalf("failed staging left retry authority behind: %+v", rolledBack.RetryDecision())
	}
	assertMongoDocumentCount(t, fixture.db.Collection("rm_outbox"), bson.M{"event_type": eventcatalog.InterpretationRetryRequested}, 1)

	terminalRun, terminalOutcomeID, terminalAssessmentID := makeFailed(t, false, retrygovernance.DispositionTerminal)
	assertMongoDocumentCount(t, fixture.db.Collection("rm_outbox"), bson.M{"event_type": eventcatalog.InterpretationRetryRequested}, 1)
	terminalOutcome := evaluationfact.NewRecord(evaluationfact.NewRecordInput{ID: terminalOutcomeID, OrgID: 1, AssessmentID: terminalAssessmentID, TesteeID: 8})
	forceService := automation.NewGovernedRetryService(fixture.generations, fixture.runs, manualRetryOutcomeRepo{record: terminalOutcome}, fixture.runner, stager)
	forceCommand := command
	forceCommand.GenerationID = terminalRun.GenerationID()
	forceCommand.ExpectedAttempt = terminalRun.Attempt()
	forceCommand.Origin = retrygovernance.AttemptOriginForce
	forceCommand.RequestID = "m5-force-request"
	forced, err := forceService.Authorize(t.Context(), forceCommand)
	if err != nil {
		t.Fatal(err)
	}
	forceDecision := forced.RetryDecision()
	if forceDecision == nil || forceDecision.Disposition != retrygovernance.DispositionAutomatic ||
		forceDecision.RetryEventID == "" || forceDecision.ActionRequestID != forceCommand.RequestID {
		t.Fatalf("force authorization decision = %+v", forceDecision)
	}
	var forceRow struct {
		Payload []byte `bson:"payload"`
	}
	if err := fixture.db.Collection("rm_outbox").FindOne(t.Context(), bson.M{"message_id": forceDecision.RetryEventID}).Decode(&forceRow); err != nil {
		t.Fatal(err)
	}
	forceWire, recognized, err := messaging.DecodeMessagePayload(forceRow.Payload)
	if err != nil || !recognized {
		t.Fatalf("force retry wire decode: recognized=%t err=%v", recognized, err)
	}
	if err := retryHandler(t.Context(), eventcatalog.InterpretationRetryRequested, forceWire.Payload); err != nil {
		t.Fatal(err)
	}
	if workerAutomation.Count() != 2 {
		t.Fatalf("force retry Worker calls=%d, want 2 total", workerAutomation.Count())
	}
	forceMetadata := workerAutomation.Last().metadata
	if origin := forceMetadata.Get("x-retry-origin"); len(origin) != 1 || origin[0] != "force" {
		t.Fatalf("force Worker lost origin: %v", forceMetadata)
	}
	if action := forceMetadata.Get("x-retry-action-request-id"); len(action) != 1 || action[0] != forceCommand.RequestID {
		t.Fatalf("force Worker lost action request ID: %v", forceMetadata)
	}
	assertMongoDocumentCount(t, fixture.db.Collection("rm_outbox"), bson.M{"event_type": eventcatalog.InterpretationRetryRequested}, 2)
}

type manualRetryOutcomeRepo struct{ record *evaluationfact.Record }

func (r manualRetryOutcomeRepo) FindByID(_ context.Context, id meta.ID) (*evaluationfact.Record, error) {
	if r.record == nil || r.record.ID() != id {
		return nil, evaluationfact.ErrNotFound
	}
	return r.record, nil
}

func (r manualRetryOutcomeRepo) FindByAssessmentID(_ context.Context, id meta.ID) (*evaluationfact.Record, error) {
	if r.record == nil || r.record.AssessmentID() != id {
		return nil, evaluationfact.ErrNotFound
	}
	return r.record, nil
}
