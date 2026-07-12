package generation

import (
	"context"
	"errors"
	"testing"
	"time"

	domaininterpretation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	domaingeneration "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/generation"
	interpinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/input"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/rendering"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	interpretationrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/run"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/pkg/event"
)

type executorBuilder struct {
	err   error
	calls int
}

func (*executorBuilder) ReportType() policy.ReportType           { return policy.ReportTypeStandard }
func (*executorBuilder) TemplateVersion() policy.TemplateVersion { return policy.TemplateVersionV1 }
func (*executorBuilder) BuilderIdentity() string                 { return "test-executor-builder" }
func (*executorBuilder) ContentSchemaVersion() string            { return "report-content/v1" }
func (*executorBuilder) MechanismKey() rendering.Key {
	return rendering.Key{AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorScoring, DecisionKind: modelcatalog.DecisionKindScoreRange, ReportType: policy.ReportTypeStandard}
}
func (b *executorBuilder) Build(context.Context, interpinput.InterpretationInput) (*report.Draft, error) {
	b.calls++
	if b.err != nil {
		return nil, b.err
	}
	return report.NewDraft(report.Content{Model: report.ModelIdentity{Kind: "scale", Code: "S-1", Title: "Scale"}, PrimaryScore: report.NewRawTotalScore(12, nil), Level: report.LevelFromRisk(report.RiskLevelLow), Conclusion: "ok"}), nil
}

type eventStagerStub struct {
	events [][]event.DomainEvent
	err    error
}

func (s *eventStagerStub) Stage(_ context.Context, events ...event.DomainEvent) error {
	if s.err != nil {
		return s.err
	}
	s.events = append(s.events, append([]event.DomainEvent(nil), events...))
	return nil
}

func executorInput() interpinput.InterpretationInput {
	return interpinput.InterpretationInput{OutcomeID: meta.FromUint64(42), Association: report.Association{OrgID: 1, AssessmentID: meta.FromUint64(7), TesteeID: 8}, Runtime: interpinput.RuntimeIdentity{AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorScoring, DecisionKind: modelcatalog.DecisionKindScoreRange}, Report: interpinput.ReportSpec{ReportType: policy.ReportTypeStandard, TemplateVersion: policy.TemplateVersionV1}, FactorScoring: &interpinput.FactorScoringFacts{}}
}

func newExecutorFixture(t *testing.T, builder *executorBuilder) (*executor, *memoryGenerationRepo, *memoryRunRepo, *memoryArtifactRepo, *eventStagerStub, *starterTx) {
	t.Helper()
	now := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	gens := newMemoryGenerationRepo()
	runs := newMemoryRunRepo()
	reports := &memoryArtifactRepo{items: map[meta.ID]*report.InterpretReport{}}
	tx := &starterTx{}
	starter, err := NewStarter(tx, gens, runs, reports, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := rendering.NewRegistry(builder)
	if err != nil {
		t.Fatal(err)
	}
	stager := &eventStagerStub{}
	committer, err := NewInterpretationCommitter(tx, gens, runs, reports, stager, nil)
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewExecutor(starter, registry, committer)
	if err != nil {
		t.Fatal(err)
	}
	impl := service.(*executor)
	impl.now = func() time.Time { return now }
	impl.newID = meta.New
	return impl, gens, runs, reports, stager, tx
}

func TestExecutorCommitsReportRunGenerationAndEvents(t *testing.T) {
	builder := &executorBuilder{}
	service, gens, runs, reports, stager, tx := newExecutorFixture(t, builder)
	result, err := service.Execute(context.Background(), executorInput(), "trace")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != ExecuteStatusGenerated || result.InterpretReport == nil || result.Generation.Status() != domaingeneration.StatusGenerated {
		t.Fatalf("result=%#v", result)
	}
	if run, err := runs.FindByID(context.Background(), result.InterpretReport.InterpretationRunID()); err != nil || run.Status() != interpretationrun.StatusSucceeded {
		t.Fatalf("run=%#v err=%v", run, err)
	}
	if len(reports.items) != 1 || len(stager.events) != 1 || len(stager.events[0]) != 2 || tx.calls != 2 {
		t.Fatalf("reports=%d events=%#v tx=%d", len(reports.items), stager.events, tx.calls)
	}
	if _, err := service.Execute(context.Background(), executorInput(), "duplicate"); err != nil {
		t.Fatal(err)
	}
	if builder.calls != 1 || len(stager.events) != 1 || len(gens.items) != 1 {
		t.Fatalf("duplicate rebuilt builder=%d events=%d gens=%d", builder.calls, len(stager.events), len(gens.items))
	}
}

func TestExecutorPersistsFailedRunThenRetriesWithoutEvaluation(t *testing.T) {
	builder := &executorBuilder{err: errors.New("boom")}
	service, gens, runs, _, stager, _ := newExecutorFixture(t, builder)
	_, executeErr := service.Execute(context.Background(), executorInput(), "trace")
	if executeErr == nil {
		t.Fatal("first execution error = nil")
	}
	failedError, ok := FailureFrom(executeErr)
	if !ok || failedError.Failure.Code != "build_failed" || !failedError.Failure.Retryable || failedError.GenerationID.IsZero() || failedError.RunID.IsZero() {
		t.Fatalf("failure metadata=%#v ok=%v", failedError, ok)
	}
	var generationRecord *domaingeneration.ReportGeneration
	for _, item := range gens.items {
		generationRecord = item
	}
	if generationRecord == nil || generationRecord.Status() != domaingeneration.StatusFailed || len(stager.events) != 1 || stager.events[0][0].EventType() != domaininterpretation.EventTypeReportFailed {
		t.Fatalf("generation=%#v events=%#v", generationRecord, stager.events)
	}
	failed, ok := stager.events[0][0].(domaininterpretation.ReportFailedOutcomeEvent)
	if !ok {
		t.Fatalf("failed event type=%T", stager.events[0][0])
	}
	payload := failed.Payload()
	if failed.AggregateType() != domaininterpretation.AggregateType || failed.AggregateID() != generationRecord.ID().String() || payload.GenerationID != generationRecord.ID().String() || payload.RunID == "" || payload.ReportType != "standard" || payload.TemplateVersion != policy.TemplateVersionV1.String() || payload.FailureKind != string(interpretationrun.FailureKindBuild) || payload.FailureCode != "build_failed" || !payload.Retryable || payload.SafeReason == "" {
		t.Fatalf("failed payload=%#v aggregate=%s/%s", payload, failed.AggregateType(), failed.AggregateID())
	}
	builder.err = nil
	result, err := service.Execute(context.Background(), executorInput(), "retry")
	if err != nil {
		t.Fatal(err)
	}
	if result.InterpretReport == nil || builder.calls != 2 {
		t.Fatalf("result=%#v calls=%d", result, builder.calls)
	}
	run, err := runs.FindByID(context.Background(), result.InterpretReport.InterpretationRunID())
	if err != nil || run.Attempt() != 2 {
		t.Fatalf("retry run=%#v err=%v", run, err)
	}
}
