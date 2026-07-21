package interpretation

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/generation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	interpretationrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/run"
	base "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
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

	artifact, err := domainreport.NewInterpretReport(domainreport.InterpretReportInput{
		ID:                   meta.FromUint64(3),
		GenerationID:         generationRecord.ID(),
		OutcomeID:            key.OutcomeID,
		InterpretationRunID:  meta.FromUint64(2),
		Association:          domainreport.Association{OrgID: 11, AssessmentID: meta.FromUint64(7), TesteeID: 8},
		ReportType:           key.ReportType,
		TemplateVersion:      key.TemplateVersion,
		BuilderIdentity:      domainreport.BuilderIdentityFactorScoring,
		ContentSchemaVersion: domainreport.ContentSchemaVersionV1,
		Content: domainreport.Content{
			Model:        domainreport.ModelIdentity{Kind: "scale", Code: "SDS", Version: "v1", Title: "抑郁自评"},
			PrimaryScore: &domainreport.ScoreValue{Kind: domainreport.ScoreKindRawTotal, Value: 42},
			Level:        &domainreport.ResultLevel{Code: "high", Severity: "high"},
			Conclusion:   "高风险",
			Dimensions: []domainreport.DimensionInterpret{
				domainreport.NewDimensionInterpret(domainreport.NewFactorCode("gec"), "GEC", 12, nil, domainreport.RiskLevelHigh, "偏高", "建议").WithScoreContext(
					[]domainreport.ScoreValue{{Kind: domainreport.ScoreKindTScore, Value: 65}, {Kind: domainreport.ScoreKindPercentile, Value: 90}},
					&domainreport.ResultLevel{Code: "elevated", Label: "偏高", Severity: "high"},
					&domainreport.NormReference{ScoreKind: domainreport.ScoreKindTScore, Benchmark: 50, TableVersion: "2026", FormVariant: "teacher", MinAgeMonths: 60, MaxAgeMonths: 95},
				),
			},
		},
		GeneratedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	restoredArtifact, err := mapper.ReportToDomain(mapper.ReportToPO(artifact))
	if err != nil || restoredArtifact.Association().AssessmentID != meta.FromUint64(7) || restoredArtifact.Content().Model.Code != "SDS" || restoredArtifact.Content().PrimaryScore.Value != 42 {
		t.Fatalf("artifact round trip = %#v err=%v", restoredArtifact, err)
	}
	dimension := restoredArtifact.Content().Dimensions[0]
	if len(dimension.DerivedScores()) != 2 || dimension.Level() == nil || dimension.Level().Code != "elevated" || dimension.NormReference() == nil || dimension.NormReference().TableVersion != "2026" {
		t.Fatalf("dimension score context did not round trip: %#v %#v %#v", dimension.DerivedScores(), dimension.Level(), dimension.NormReference())
	}
	if restoredArtifact.BuilderIdentity() != domainreport.BuilderIdentityFactorScoring || restoredArtifact.ContentSchemaVersion() != domainreport.ContentSchemaVersionV1 {
		t.Fatalf("artifact provenance round trip = %q/%q", restoredArtifact.BuilderIdentity(), restoredArtifact.ContentSchemaVersion())
	}
}

func TestLifecycleMapperRestoresLegacyArtifactProvenance(t *testing.T) {
	mapper := NewLifecycleMapper()
	now := time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC)
	restored, err := mapper.ReportToDomain(&InterpretReportPO{
		BaseDocument:        base.BaseDocument{DomainID: meta.FromUint64(3), CreatedAt: now, UpdatedAt: now},
		GenerationID:        1,
		OutcomeID:           9,
		InterpretationRunID: 2,
		ReportType:          string(policy.ReportTypeStandard),
		TemplateVersion:     "v1",
		GeneratedAt:         now,
		OrgID:               11,
		AssessmentID:        7,
		TesteeID:            8,
		ScaleCode:           "SDS",
		ScaleName:           "抑郁自评",
	})
	if err != nil {
		t.Fatal(err)
	}
	if restored.BuilderIdentity() != domainreport.UnknownBuilderIdentity {
		t.Fatalf("builder identity = %q, want %q", restored.BuilderIdentity(), domainreport.UnknownBuilderIdentity)
	}
	if restored.ContentSchemaVersion() != domainreport.LegacyContentSchemaVersion {
		t.Fatalf("content schema = %q, want %q", restored.ContentSchemaVersion(), domainreport.LegacyContentSchemaVersion)
	}
}

func TestLifecycleMapperRoundTripsClaimHistory(t *testing.T) {
	now := time.Date(2026, 7, 21, 10, 0, 0, 0, time.UTC)
	mapper := NewLifecycleMapper()
	runRecord, err := interpretationrun.NewPending(meta.FromUint64(2), meta.FromUint64(1), 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := runRecord.StartWithLease(now.Add(-2*time.Minute), "initial", now.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := runRecord.ReclaimExpiredLease(now, "recovery", now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	restored, err := mapper.RunToDomain(mapper.RunToPO(runRecord))
	if err != nil || restored.RecoveryCount() != 1 || len(restored.ClaimHistory()) != 1 || restored.ClaimHistory()[0].TraceID != "recovery" {
		t.Fatalf("claim history round trip = count:%d history:%#v err:%v", restored.RecoveryCount(), restored.ClaimHistory(), err)
	}
}
