package interpretation

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/generation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	interpretationrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/run"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestLifecycleMapperRoundTripsThreeInterpretationObjects(t *testing.T) {
	now := time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC)
	mapper := NewLifecycleMapper()
	key := generation.Key{OutcomeID: meta.FromUint64(9), ReportType: policy.ReportTypeStandard, TemplateVersion: policy.TemplateVersion("v1")}
	generationRecord, err := generation.New(meta.FromUint64(1), key, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := generationRecord.Begin(meta.FromUint64(2), now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := generationRecord.Succeed(meta.FromUint64(2), meta.FromUint64(3), now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	restoredGeneration, err := mapper.GenerationToDomain(mapper.GenerationToPO(generationRecord))
	if err != nil || restoredGeneration.Status() != generation.StatusGenerated || restoredGeneration.ReportID() != meta.FromUint64(3) || restoredGeneration.Key() != key {
		t.Fatalf("generation round trip = %#v err=%v", restoredGeneration, err)
	}

	runRecord, err := interpretationrun.NewPending(meta.FromUint64(2), generationRecord.ID(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := runRecord.Start(now, "trace-1"); err != nil {
		t.Fatal(err)
	}
	if err := runRecord.Fail(now.Add(time.Second), interpretationrun.Failure{Kind: interpretationrun.FailureKindBuild, Code: "build_failed", SafeMessage: "报告生成失败", Retryable: true}); err != nil {
		t.Fatal(err)
	}
	restoredRun, err := mapper.RunToDomain(mapper.RunToPO(runRecord))
	if err != nil || restoredRun.Attempt() != 1 || restoredRun.Failure() == nil || restoredRun.Failure().Code != "build_failed" {
		t.Fatalf("run round trip = %#v err=%v", restoredRun, err)
	}

	artifact, err := domainreport.NewArtifact(domainreport.ArtifactInput{
		ID:                  meta.FromUint64(3),
		GenerationID:        generationRecord.ID(),
		OutcomeID:           key.OutcomeID,
		InterpretationRunID: meta.FromUint64(2),
		Association:         domainreport.Association{OrgID: 11, AssessmentID: meta.FromUint64(7), TesteeID: 8},
		ReportType:          key.ReportType,
		TemplateVersion:     key.TemplateVersion,
		Content: domainreport.Content{
			Model:        domainreport.ModelIdentity{Code: "SDS", Title: "抑郁自评"},
			PrimaryScore: &domainreport.ScoreValue{Kind: domainreport.ScoreKindRawTotal, Value: 42},
			Level:        &domainreport.ResultLevel{Code: "high", Severity: "high"},
			Conclusion:   "高风险",
		},
		GeneratedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	restoredArtifact, err := mapper.ArtifactToDomain(mapper.ArtifactToPO(artifact))
	if err != nil || restoredArtifact.Association().AssessmentID != meta.FromUint64(7) || restoredArtifact.Content().Model.Code != "SDS" || restoredArtifact.Content().PrimaryScore.Value != 42 {
		t.Fatalf("artifact round trip = %#v err=%v", restoredArtifact, err)
	}
}
