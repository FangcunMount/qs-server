package generation

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
	artifacts := &memoryArtifactRepo{items: map[meta.ID]*domainreport.Artifact{}}
	tx := &starterTx{}
	starterService, err := NewStarter(tx, gens, runs, artifacts, time.Minute)
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
	committer, err := NewInterpretationCommitter(tx, gens, runs, artifacts, stager, nil)
	if err != nil {
		t.Fatal(err)
	}
	return committer.(*interpretationCommitter), started.Generation, started.Run, gens, runs, artifacts, tx, now
}

func commitArtifact(t *testing.T, generation *domaingeneration.ReportGeneration, run *interpretationrun.InterpretationRun, now time.Time) *domainreport.Artifact {
	t.Helper()
	artifact, err := domainreport.NewArtifact(domainreport.ArtifactInput{
		ID: meta.FromUint64(900), GenerationID: generation.ID(), OutcomeID: meta.FromUint64(42), InterpretationRunID: run.ID(),
		Association: domainreport.Association{OrgID: 1, AssessmentID: meta.FromUint64(7), TesteeID: 8},
		ReportType:  policy.ReportTypeStandard, TemplateVersion: policy.TemplateVersion("v1"), GeneratedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	return artifact
}

func TestInterpretationCommitterCommitsArtifactTerminalStateAndOutbox(t *testing.T) {
	stager := &eventStagerStub{}
	committer, generationRecord, runRecord, gens, runs, artifacts, tx, now := commitFixture(t, stager)
	artifact := commitArtifact(t, generationRecord, runRecord, now)

	result, err := committer.CommitSuccess(context.Background(), CommitSuccessRequest{Generation: generationRecord, Run: runRecord, Artifact: artifact, BuilderIdentity: "test-committer-builder", ContentSchemaVersion: "report-content/v1", CompletedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	if result.Generation.Status() != domaingeneration.StatusGenerated || result.Run.Status() != interpretationrun.StatusSucceeded || result.Artifact != artifact {
		t.Fatalf("commit result = %#v", result)
	}
	if generationRecord.Status() != domaingeneration.StatusGenerating || runRecord.Status() != interpretationrun.StatusRunning {
		t.Fatalf("caller records were not updated only after commit: generation=%s run=%s", generationRecord.Status(), runRecord.Status())
	}
	if len(artifacts.items) != 1 || len(stager.events) != 1 || len(stager.events[0]) != 2 || tx.calls != 2 {
		t.Fatalf("artifacts=%d events=%#v tx=%d", len(artifacts.items), stager.events, tx.calls)
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

	_, err := committer.CommitSuccess(context.Background(), CommitSuccessRequest{Generation: generationRecord, Run: runRecord, Artifact: artifact, BuilderIdentity: "test-committer-builder", ContentSchemaVersion: "report-content/v1", CompletedAt: now})
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
