//go:build integration && reliable_messaging_m4

package interpretation_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/FangcunMount/component-base/pkg/eventcodec"
	"github.com/FangcunMount/component-base/pkg/messaging"
	automation "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/automation"
	execution "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/automation/execution"
	domaingeneration "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/generation"
	interpinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/input"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/rendering"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	interpretationrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/run"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/interpretationassets"
	evaluationfact "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
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
	assertStandardRetryClaimOnce(t, fixture, run.GenerationID(), decoded.Payload, command.ExpectedAttempt, retrygovernance.AttemptOriginManual)
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
	assertStandardRetryClaimOnce(t, fixture, terminalRun.GenerationID(), forceWire.Payload, forceCommand.ExpectedAttempt, retrygovernance.AttemptOriginForce)
	assertMongoDocumentCount(t, fixture.db.Collection("rm_outbox"), bson.M{"event_type": eventcatalog.InterpretationRetryRequested}, 2)

	fullRun, fullOutcomeID, fullAssessmentID := makeFailed(t, true, retrygovernance.DispositionManualRequired)
	fullOutcome := standardRetryOutcome(t, fullOutcomeID, fullAssessmentID)
	fullService := automation.NewGovernedRetryService(fixture.generations, fixture.runs, manualRetryOutcomeRepo{record: fullOutcome}, fixture.runner, stager)
	fullCommand := command
	fullCommand.GenerationID = fullRun.GenerationID()
	fullCommand.RequestID = "m5-full-business-request"
	fullAuthorized, err := fullService.Authorize(t.Context(), fullCommand)
	if err != nil {
		t.Fatal(err)
	}
	var fullRow struct {
		Payload []byte `bson:"payload"`
	}
	if err := fixture.db.Collection("rm_outbox").FindOne(t.Context(), bson.M{"message_id": fullAuthorized.RetryDecision().RetryEventID}).Decode(&fullRow); err != nil {
		t.Fatal(err)
	}
	fullWire, recognized, err := messaging.DecodeMessagePayload(fullRow.Payload)
	if err != nil || !recognized {
		t.Fatalf("full business retry wire decode: recognized=%t err=%v", recognized, err)
	}
	if err := retryHandler(t.Context(), eventcatalog.InterpretationRetryRequested, fullWire.Payload); err != nil {
		t.Fatal(err)
	}
	if workerAutomation.Count() != 3 {
		t.Fatalf("full business retry Worker calls=%d, want 3 total", workerAutomation.Count())
	}
	assertStandardRetryFullBusinessOnce(t, fixture, committer, fullRun.GenerationID(), fullOutcome, fullWire.Payload, fullCommand.ExpectedAttempt, false)
	assertMongoDocumentCount(t, fixture.db.Collection("rm_outbox"), bson.M{"event_type": eventcatalog.InterpretationReportGenerated}, 1)

	lostRun, lostOutcomeID, lostAssessmentID := makeFailed(t, true, retrygovernance.DispositionManualRequired)
	lostOutcome := standardRetryOutcome(t, lostOutcomeID, lostAssessmentID)
	lostService := automation.NewGovernedRetryService(fixture.generations, fixture.runs, manualRetryOutcomeRepo{record: lostOutcome}, fixture.runner, stager)
	lostCommand := command
	lostCommand.GenerationID = lostRun.GenerationID()
	lostCommand.RequestID = "m5-lost-commit-reply-request"
	lostAuthorized, err := lostService.Authorize(t.Context(), lostCommand)
	if err != nil {
		t.Fatal(err)
	}
	var lostRow struct {
		Payload []byte `bson:"payload"`
	}
	if err := fixture.db.Collection("rm_outbox").FindOne(t.Context(), bson.M{"message_id": lostAuthorized.RetryDecision().RetryEventID}).Decode(&lostRow); err != nil {
		t.Fatal(err)
	}
	lostWire, recognized, err := messaging.DecodeMessagePayload(lostRow.Payload)
	if err != nil || !recognized {
		t.Fatalf("lost reply retry wire decode: recognized=%t err=%v", recognized, err)
	}
	assertStandardRetryFullBusinessOnce(t, fixture, committer, lostRun.GenerationID(), lostOutcome, lostWire.Payload, lostCommand.ExpectedAttempt, true)
	assertMongoDocumentCount(t, fixture.db.Collection("rm_outbox"), bson.M{"event_type": eventcatalog.InterpretationReportGenerated}, 2)
}

func assertStandardRetryClaimOnce(t *testing.T, fixture interpretationMongoFixture, generationID meta.ID, payload []byte, expectedAttempt int, origin retrygovernance.AttemptOrigin) {
	t.Helper()
	ctx := standardRetryAuthorizationContext(t, payload)
	generation, err := fixture.generations.FindByID(ctx, generationID)
	if err != nil {
		t.Fatal(err)
	}
	starter, err := execution.NewStarter(fixture.runner, fixture.generations, fixture.runs, fixture.reports, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	request := execution.StartRequest{Key: generation.Key(), TraceID: "m5-governed-retry-proof"}
	first, err := starter.Start(ctx, request)
	if err != nil || first == nil || first.Status != execution.StartStatusStarted || first.Run == nil || first.Run.Attempt() != expectedAttempt+1 || first.Run.Origin() != origin {
		t.Fatalf("first authorized retry claim=%+v err=%v", first, err)
	}
	again, err := starter.Start(ctx, request)
	if err != nil || again == nil || again.Status != execution.StartStatusProcessing || again.Run == nil || again.Run.ID() != first.Run.ID() {
		t.Fatalf("duplicate retry claim=%+v first=%+v err=%v", again, first, err)
	}
	latest, err := fixture.runs.FindLatestByGenerationID(ctx, generationID)
	if err != nil || latest == nil || latest.ID() != first.Run.ID() || latest.Attempt() != expectedAttempt+1 {
		t.Fatalf("persisted retry run=%+v first=%+v err=%v", latest, first, err)
	}
}

func standardRetryAuthorizationContext(t *testing.T, payload []byte) context.Context {
	t.Helper()
	envelope, err := eventcodec.DecodeEnvelope(payload)
	if err != nil {
		t.Fatal(err)
	}
	var data eventoutcome.InterpretationRetryRequestedPayload
	if err := json.Unmarshal(envelope.Data, &data); err != nil {
		t.Fatal(err)
	}
	origin := retrygovernance.AttemptOrigin(data.AttemptOrigin)
	if !origin.IsValid() || data.ExpectedAttempt < 1 {
		t.Fatalf("invalid retry authorization payload: %+v", data)
	}
	return retrygovernance.WithAuthorization(t.Context(), retrygovernance.Authorization{
		EventID: envelope.ID, ExpectedAttempt: data.ExpectedAttempt, Origin: origin,
		ActionRequestID: data.ActionRequestID, Mode: data.Mode,
	})
}

func standardRetryOutcome(t *testing.T, outcomeID, assessmentID meta.ID) *evaluationfact.Record {
	t.Helper()
	reportInput, err := evaluationinput.MarshalReportInput(evaluationinput.ReportInputFreezeOptions{
		Assets: &interpretationassets.Assets{ReportSpec: interpretationassets.ReportSpec{Sections: []interpretationassets.ReportSection{{
			Code: "standard", Kind: "factor_scoring", TemplateID: "standard", TemplateVersion: "v1",
		}}}},
		ModelRef: evaluationinput.ModelRef{
			Kind: evaluationinput.EvaluationModelKindScale, Algorithm: string(modelcatalog.AlgorithmScaleDefault),
			Code: "SCALE-1", Version: "v1", Title: "Scale",
		},
		DecisionKind:  modelcatalog.DecisionKindScoreRange,
		FactorCatalog: []evaluationinput.FactorCatalogEntry{{Code: "TOTAL", Title: "总分", IsTotalScore: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return evaluationfact.NewRecord(evaluationfact.NewRecordInput{
		ID: outcomeID, OrgID: 1, AssessmentID: assessmentID, TesteeID: 8,
		Model: evaluationfact.ModelIdentity{
			Kind: modelcatalog.KindScale, Algorithm: modelcatalog.AlgorithmScaleDefault,
			Code: "SCALE-1", Version: "v1", Title: "Scale",
		},
		Runtime:       evaluationfact.RuntimeIdentity{DecisionKind: modelcatalog.DecisionKindScoreRange},
		SchemaVersion: 2, ReportInput: reportInput, EvaluatedAt: time.Now(),
		Payload: []byte(`{"Primary":{"Kind":"raw_total","Value":12},"Level":{"Code":"low"},"Dimensions":[{"Code":"TOTAL","Role":"total","Score":{"Kind":"raw_total","Value":12},"Level":{"Code":"low"}}]}`),
	})
}

func assertStandardRetryFullBusinessOnce(
	t *testing.T, fixture interpretationMongoFixture, committer execution.InterpretationCommitter,
	generationID meta.ID, outcome *evaluationfact.Record, payload []byte, expectedAttempt int, loseCommitReply bool,
) {
	t.Helper()
	starter, err := execution.NewStarter(fixture.runner, fixture.generations, fixture.runs, fixture.reports, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	builder := &standardRetryBuilder{}
	registry, err := rendering.NewRegistry(builder)
	if err != nil {
		t.Fatal(err)
	}
	if loseCommitReply {
		committer = &standardRetryLostCommitReply{InterpretationCommitter: committer}
	}
	executor, err := execution.NewExecutor(starter, registry, committer)
	if err != nil {
		t.Fatal(err)
	}
	service, err := automation.NewService(manualRetryOutcomeRepo{record: outcome}, executor)
	if err != nil {
		t.Fatal(err)
	}
	ctx := standardRetryAuthorizationContext(t, payload)
	command := automation.GenerateCommand{Actor: automation.TrustedServiceActor("m5-full-business-proof"), OutcomeID: outcome.ID()}
	var first *automation.Result
	var committedReportID meta.ID
	for i := 0; i < 2; i++ {
		result, err := service.Generate(ctx, command)
		if loseCommitReply && i == 0 {
			if err == nil || result != nil {
				t.Fatalf("lost commit reply must leave caller uncertain: result=%+v err=%v", result, err)
			}
			committed, findErr := fixture.generations.FindByID(t.Context(), generationID)
			if findErr != nil || committed == nil || committed.Status() != domaingeneration.StatusGenerated || committed.ReportID().IsZero() {
				t.Fatalf("lost reply did not leave a durable report: generation=%+v err=%v", committed, findErr)
			}
			committedReportID = committed.ReportID()
			continue
		}
		if err != nil || result == nil || result.Status != automation.StatusGenerated {
			t.Fatalf("full report retry delivery %d: result=%+v err=%v", i+1, result, err)
		}
		if loseCommitReply && result.ReportID != committedReportID {
			t.Fatalf("retry after lost reply changed report: persisted=%s returned=%s", committedReportID, result.ReportID)
		}
		if first == nil {
			first = result
		} else if result.RunID != first.RunID || result.ReportID != first.ReportID {
			t.Fatalf("duplicate full report changed result: first=%+v again=%+v", first, result)
		}
	}
	builder.mu.Lock()
	buildCalls := builder.calls
	builder.mu.Unlock()
	if buildCalls != 1 {
		t.Fatalf("duplicate retry invoked builder %d times, want 1", buildCalls)
	}
	latest, err := fixture.runs.FindLatestByGenerationID(t.Context(), generationID)
	if err != nil || latest == nil || latest.ID() != first.RunID || latest.Attempt() != expectedAttempt+1 || latest.Status() != interpretationrun.StatusSucceeded {
		t.Fatalf("full report retry run=%+v err=%v", latest, err)
	}
	generated, err := fixture.generations.FindByID(t.Context(), generationID)
	if err != nil || generated == nil || generated.Status() != domaingeneration.StatusGenerated {
		t.Fatalf("full report retry generation=%+v err=%v", generated, err)
	}
}

type standardRetryLostCommitReply struct {
	execution.InterpretationCommitter
	lost bool
}

func (c *standardRetryLostCommitReply) CommitSuccess(ctx context.Context, request execution.CommitSuccessRequest) (*execution.CommitResult, error) {
	result, err := c.InterpretationCommitter.CommitSuccess(ctx, request)
	if err != nil {
		return result, err
	}
	if !c.lost {
		c.lost = true
		return nil, errors.New("simulated reply loss after durable report commit")
	}
	return result, nil
}

type standardRetryBuilder struct {
	mu    sync.Mutex
	calls int
}

func (*standardRetryBuilder) ReportType() policy.ReportType { return policy.ReportTypeStandard }
func (*standardRetryBuilder) TemplateVersion() policy.TemplateVersion {
	return policy.TemplateVersion("v1")
}
func (*standardRetryBuilder) BuilderIdentity() string {
	return domainreport.BuilderIdentityFactorScoring
}
func (*standardRetryBuilder) ContentSchemaVersion() string { return "report-content/v1" }
func (*standardRetryBuilder) MechanismKey() rendering.Key {
	return rendering.Key{DecisionKind: modelcatalog.DecisionKindScoreRange, ReportType: policy.ReportTypeStandard}
}
func (b *standardRetryBuilder) Build(context.Context, interpinput.InterpretationInput) (*domainreport.Draft, error) {
	b.mu.Lock()
	b.calls++
	b.mu.Unlock()
	return domainreport.NewDraft(domainreport.Content{
		Model:        domainreport.ModelIdentity{Kind: "scale", Code: "SCALE-1", Version: "v1", Title: "Scale"},
		PrimaryScore: domainreport.NewRawTotalScore(12, nil),
		Level:        domainreport.LevelFromRisk(domainreport.RiskLevelLow),
		Conclusion:   "ok",
		Dimensions: []domainreport.DimensionInterpret{
			domainreport.NewDimensionInterpret(domainreport.NewFactorCode("TOTAL"), "总分", 12, nil, domainreport.RiskLevelLow, "ok", "ok"),
		},
	}), nil
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
