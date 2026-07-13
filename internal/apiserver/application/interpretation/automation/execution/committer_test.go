package execution

import (
	"context"
	"errors"
	"testing"
	"time"

	domaininterpretation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	domaingeneration "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/generation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	interpretationrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/run"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func commitFixture(t *testing.T, stager *eventStagerStub) (*interpretationCommitter, *domaingeneration.ReportGeneration, *interpretationrun.InterpretationRun, *memoryGenerationRepo, *memoryRunRepo, *memoryArtifactRepo, *starterTx, time.Time) {
	t.Helper()
	now := time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC)
	gens := newMemoryGenerationRepo()
	runs := newMemoryRunRepo()
	reports := &memoryArtifactRepo{items: map[meta.ID]*domainreport.InterpretReport{}}
	tx := &starterTx{}
	starterService, err := NewStarter(tx, gens, runs, reports, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	impl := starterService.(*starter)
	impl.now = func() time.Time { return now }
	impl.newID = meta.New
	started, err := starterService.Start(context.Background(), StartRequest{Key: domaingeneration.Key{OutcomeID: meta.FromUint64(42), ReportType: policy.ReportTypeStandard, TemplateVersion: policy.TemplateVersion("v1")}, TraceID: "trace"})
	if err != nil {
		t.Fatal(err)
	}
	committer, err := NewInterpretationCommitter(tx, gens, runs, reports, stager, nil, catalogProjectorStub{})
	if err != nil {
		t.Fatal(err)
	}
	return committer.(*interpretationCommitter), started.Generation, started.Run, gens, runs, reports, tx, now
}

type catalogProjectorStub struct{ err error }

func (s catalogProjectorStub) ProjectCurrent(context.Context, *domainreport.InterpretReport) error {
	return s.err
}

func TestInterpretationCommitterCatalogFailureDoesNotPublishTerminalState(t *testing.T) {
	committer, generationRecord, runRecord, _, _, _, _, now := commitFixture(t, &eventStagerStub{})
	committer.catalog = catalogProjectorStub{err: errors.New("catalog unavailable")}
	_, err := committer.CommitSuccess(context.Background(), CommitSuccessRequest{Generation: generationRecord, Run: runRecord, InterpretReport: commitArtifact(t, generationRecord, runRecord, now), BuilderIdentity: "builder", ContentSchemaVersion: "v1", CompletedAt: now})
	if err == nil {
		t.Fatal("expected catalog error")
	}
	if generationRecord.Status() != domaingeneration.StatusGenerating || runRecord.Status() != interpretationrun.StatusRunning {
		t.Fatal("caller terminal state leaked after catalog failure")
	}
}

func commitArtifact(t *testing.T, generation *domaingeneration.ReportGeneration, run *interpretationrun.InterpretationRun, now time.Time) *domainreport.InterpretReport {
	t.Helper()
	artifact, err := domainreport.NewInterpretReport(domainreport.InterpretReportInput{
		ID: meta.FromUint64(900), GenerationID: generation.ID(), OutcomeID: meta.FromUint64(42), InterpretationRunID: run.ID(),
		Association: domainreport.Association{OrgID: 1, AssessmentID: meta.FromUint64(7), TesteeID: 8},
		ReportType:  policy.ReportTypeStandard, TemplateVersion: policy.TemplateVersion("v1"), GeneratedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	return artifact
}

func TestInterpretationCommitterCommitsReportTerminalStateAndOutbox(t *testing.T) {
	stager := &eventStagerStub{}
	committer, generationRecord, runRecord, gens, runs, reports, tx, now := commitFixture(t, stager)
	artifact := commitArtifact(t, generationRecord, runRecord, now)

	result, err := committer.CommitSuccess(context.Background(), CommitSuccessRequest{Generation: generationRecord, Run: runRecord, InterpretReport: artifact, BuilderIdentity: "test-committer-builder", ContentSchemaVersion: "report-content/v1", CompletedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	if result.Generation.Status() != domaingeneration.StatusGenerated || result.Run.Status() != interpretationrun.StatusSucceeded || result.InterpretReport != artifact {
		t.Fatalf("commit result = %#v", result)
	}
	if generationRecord.Status() != domaingeneration.StatusGenerating || runRecord.Status() != interpretationrun.StatusRunning {
		t.Fatalf("caller records were not updated only after commit: generation=%s run=%s", generationRecord.Status(), runRecord.Status())
	}
	if len(reports.items) != 1 || len(stager.events) != 1 || len(stager.events[0]) != 1 || tx.calls != 2 {
		t.Fatalf("reports=%d events=%#v tx=%d", len(reports.items), stager.events, tx.calls)
	}
	generated, ok := stager.events[0][0].(domaininterpretation.ReportGeneratedOutcomeEvent)
	if !ok {
		t.Fatalf("generated event type=%T", stager.events[0][0])
	}
	payload := generated.Payload()
	if generated.AggregateType() != domaininterpretation.AggregateType || generated.AggregateID() != generationRecord.ID().String() || payload.GenerationID != generationRecord.ID().String() || payload.RunID != runRecord.ID().String() || payload.ReportID != artifact.ID().String() || payload.ReportType != "standard" || payload.TemplateVersion != "v1" || payload.BuilderIdentity != "test-committer-builder" || payload.ContentSchemaVersion != "report-content/v1" {
		t.Fatalf("generated payload=%#v aggregate=%s/%s", payload, generated.AggregateType(), generated.AggregateID())
	}
	if persistedGeneration, err := gens.FindByID(context.Background(), generationRecord.ID()); err != nil || persistedGeneration.Status() != domaingeneration.StatusGenerated {
		t.Fatalf("persisted generation=%#v err=%v", persistedGeneration, err)
	}
	if persistedRun, err := runs.FindByID(context.Background(), runRecord.ID()); err != nil || persistedRun.Status() != interpretationrun.StatusSucceeded {
		t.Fatalf("persisted run=%#v err=%v", persistedRun, err)
	}
}

func TestInterpretationCommitterFailureDoesNotPublishTerminalStateToCaller(t *testing.T) {
	commitErr := errors.New("outbox unavailable")
	stager := &eventStagerStub{err: commitErr}
	committer, generationRecord, runRecord, _, _, _, _, now := commitFixture(t, stager)
	artifact := commitArtifact(t, generationRecord, runRecord, now)

	_, err := committer.CommitSuccess(context.Background(), CommitSuccessRequest{Generation: generationRecord, Run: runRecord, InterpretReport: artifact, BuilderIdentity: "test-committer-builder", ContentSchemaVersion: "report-content/v1", CompletedAt: now})
	if !errors.Is(err, commitErr) {
		t.Fatalf("CommitSuccess error = %v, want %v", err, commitErr)
	}
	if generationRecord.Status() != domaingeneration.StatusGenerating || generationRecord.ReportID() != 0 {
		t.Fatalf("caller generation was polluted: status=%s report=%s", generationRecord.Status(), generationRecord.ReportID())
	}
	if runRecord.Status() != interpretationrun.StatusRunning || runRecord.FinishedAt() != nil || runRecord.Failure() != nil {
		t.Fatalf("caller run was polluted: %#v", runRecord)
	}
}

func TestInterpretationCommitterRejectsMismatchedSuccessReferences(t *testing.T) {
	committer, generationRecord, runRecord, _, _, _, _, now := commitFixture(t, &eventStagerStub{})

	tests := []struct {
		name            string
		outcomeID       meta.ID
		reportType      policy.ReportType
		templateVersion policy.TemplateVersion
		useOtherRun     bool
	}{
		{name: "outcome", outcomeID: meta.FromUint64(43), reportType: policy.ReportTypeStandard, templateVersion: policy.TemplateVersion("v1")},
		{name: "report type", outcomeID: meta.FromUint64(42), reportType: policy.ReportType("clinician"), templateVersion: policy.TemplateVersion("v1")},
		{name: "template version", outcomeID: meta.FromUint64(42), reportType: policy.ReportTypeStandard, templateVersion: policy.TemplateVersion("v2")},
		{name: "latest run", outcomeID: meta.FromUint64(42), reportType: policy.ReportTypeStandard, templateVersion: policy.TemplateVersion("v1"), useOtherRun: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidateRun := runRecord
			if tt.useOtherRun {
				var err error
				candidateRun, err = interpretationrun.NewPending(meta.FromUint64(901), generationRecord.ID(), runRecord.Attempt()+1)
				if err != nil {
					t.Fatal(err)
				}
				if err := candidateRun.Start(now, "other"); err != nil {
					t.Fatal(err)
				}
			}
			artifact := artifactWithIdentity(t, generationRecord, candidateRun, tt.outcomeID, tt.reportType, tt.templateVersion, now)
			_, err := committer.CommitSuccess(context.Background(), CommitSuccessRequest{
				Generation: generationRecord, Run: candidateRun, InterpretReport: artifact,
				BuilderIdentity: "builder", ContentSchemaVersion: "report-content/v1", CompletedAt: now,
			})
			if err == nil {
				t.Fatal("CommitSuccess accepted mismatched references")
			}
		})
	}
}

func TestInterpretationCommitterRejectsFailureForDifferentOutcome(t *testing.T) {
	committer, generationRecord, runRecord, _, _, _, _, now := commitFixture(t, &eventStagerStub{})
	_, err := committer.CommitFailure(context.Background(), CommitFailureRequest{
		Generation: generationRecord,
		Run:        runRecord,
		OutcomeID:  meta.FromUint64(43),
		Association: domainreport.Association{
			OrgID: 1, AssessmentID: meta.FromUint64(7), TesteeID: 8,
		},
		Failure: interpretationrun.Failure{
			Kind: interpretationrun.FailureKindBuild, Code: "build_failed", SafeMessage: "报告生成失败", Retryable: true,
		},
		FailedAt: now,
	})
	if err == nil {
		t.Fatal("CommitFailure accepted a different outcome")
	}
}

func artifactWithIdentity(
	t *testing.T,
	generationRecord *domaingeneration.ReportGeneration,
	runRecord *interpretationrun.InterpretationRun,
	outcomeID meta.ID,
	reportType policy.ReportType,
	templateVersion policy.TemplateVersion,
	at time.Time,
) *domainreport.InterpretReport {
	t.Helper()
	artifact, err := domainreport.NewInterpretReport(domainreport.InterpretReportInput{
		ID: meta.New(), GenerationID: generationRecord.ID(), OutcomeID: outcomeID, InterpretationRunID: runRecord.ID(),
		Association: domainreport.Association{OrgID: 1, AssessmentID: meta.FromUint64(7), TesteeID: 8},
		ReportType:  reportType, TemplateVersion: templateVersion, GeneratedAt: at,
	})
	if err != nil {
		t.Fatal(err)
	}
	return artifact
}
