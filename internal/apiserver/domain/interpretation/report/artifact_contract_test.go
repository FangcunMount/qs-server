package report_test

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestCrossMechanismArtifactContractRejectsEmptyContent(t *testing.T) {
	if err := report.CrossMechanismArtifactContract(report.Content{}); err == nil {
		t.Fatal("empty content must be rejected")
	}
}

func TestCrossMechanismArtifactContractRejectsIncompleteModelIdentity(t *testing.T) {
	content := report.Content{
		Model:        report.ModelIdentity{Kind: "scale", Code: "PHQ9"},
		PrimaryScore: report.NewRawTotalScore(8, nil),
		Dimensions: []report.DimensionInterpret{
			report.NewDimensionInterpret(report.NewFactorCode("TOTAL"), "总分", 8, nil, report.RiskLevelLow, "ok", "ok"),
		},
	}
	if err := report.CrossMechanismArtifactContract(content); err == nil {
		t.Fatal("incomplete model identity must be rejected")
	}
}

func TestCrossMechanismArtifactContractRejectsMismatchedModelExtraKind(t *testing.T) {
	content := report.Content{
		Model: report.ModelIdentity{
			Kind: string(modelcatalog.KindScale), Code: "PHQ9", Version: "v1",
		},
		PrimaryScore: report.NewRawTotalScore(8, nil),
		Dimensions: []report.DimensionInterpret{
			report.NewDimensionInterpret(report.NewFactorCode("TOTAL"), "总分", 8, nil, report.RiskLevelLow, "ok", "ok"),
		},
		ModelExtra: &report.ModelExtra{Kind: "personality_type", TypeCode: "INTJ"},
	}
	if err := report.CrossMechanismArtifactContract(content); err == nil {
		t.Fatal("scale model with personality extra must be rejected")
	}
}

func TestBuilderSpecificDraftContractFactorScoringGolden(t *testing.T) {
	content := factorScoringMinimalContent()
	if err := report.BuilderSpecificDraftContract(report.BuilderIdentityFactorScoring, content); err != nil {
		t.Fatalf("factor scoring golden content rejected: %v", err)
	}
	if _, err := report.NewInterpretReport(newWriteInput(report.BuilderIdentityFactorScoring, content)); err != nil {
		t.Fatalf("factor scoring artifact rejected: %v", err)
	}
}

func TestBuilderSpecificDraftContractNormProfileGolden(t *testing.T) {
	content := normProfileMinimalContent()
	if err := report.BuilderSpecificDraftContract(report.BuilderIdentityNormProfile, content); err != nil {
		t.Fatalf("norm profile golden content rejected: %v", err)
	}
	if _, err := report.NewInterpretReport(newWriteInput(report.BuilderIdentityNormProfile, content)); err != nil {
		t.Fatalf("norm profile artifact rejected: %v", err)
	}
}

func TestBuilderSpecificDraftContractTypologyGolden(t *testing.T) {
	content := typologyMinimalContent()
	if err := report.BuilderSpecificDraftContract(report.BuilderIdentityTypology, content); err != nil {
		t.Fatalf("typology golden content rejected: %v", err)
	}
	if _, err := report.NewInterpretReport(newWriteInput(report.BuilderIdentityTypology, content)); err != nil {
		t.Fatalf("typology artifact rejected: %v", err)
	}
}

func TestBuilderSpecificDraftContractTaskPerformanceGolden(t *testing.T) {
	content := taskPerformanceMinimalContent()
	if err := report.BuilderSpecificDraftContract(report.BuilderIdentityTaskPerformance, content); err != nil {
		t.Fatalf("task performance golden content rejected: %v", err)
	}
	if _, err := report.NewInterpretReport(newWriteInput(report.BuilderIdentityTaskPerformance, content)); err != nil {
		t.Fatalf("task performance artifact rejected: %v", err)
	}
}

func TestNewInterpretReportRequiresProvenanceAndContract(t *testing.T) {
	content := factorScoringMinimalContent()
	_, err := report.NewInterpretReport(report.InterpretReportInput{
		ID: meta.FromUint64(1), GenerationID: meta.FromUint64(2), OutcomeID: meta.FromUint64(3),
		InterpretationRunID: meta.FromUint64(4),
		Association:         report.Association{OrgID: 1, AssessmentID: meta.FromUint64(5), TesteeID: 6},
		ReportType:          policy.ReportTypeStandard, TemplateVersion: policy.TemplateVersionV1,
		Content: content, GeneratedAt: time.Now(),
	})
	if err == nil {
		t.Fatal("write path must require provenance")
	}
}

func TestRestoreInterpretReportMapsLegacyProvenance(t *testing.T) {
	artifact, err := report.RestoreInterpretReport(report.InterpretReportInput{
		ID: meta.FromUint64(1), GenerationID: meta.FromUint64(2), OutcomeID: meta.FromUint64(3),
		InterpretationRunID: meta.FromUint64(4),
		Association:         report.Association{OrgID: 1, AssessmentID: meta.FromUint64(5), TesteeID: 6},
		ReportType:          policy.ReportTypeStandard, TemplateVersion: policy.TemplateVersionV1,
		GeneratedAt: time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if artifact.BuilderIdentity() != report.UnknownBuilderIdentity {
		t.Fatalf("builder identity = %q, want %q", artifact.BuilderIdentity(), report.UnknownBuilderIdentity)
	}
	if artifact.ContentSchemaVersion() != report.LegacyContentSchemaVersion {
		t.Fatalf("content schema = %q, want %q", artifact.ContentSchemaVersion(), report.LegacyContentSchemaVersion)
	}
}

func TestRestoreInterpretReportAllowsEmptyContent(t *testing.T) {
	_, err := report.RestoreInterpretReport(report.InterpretReportInput{
		ID: meta.FromUint64(1), GenerationID: meta.FromUint64(2), OutcomeID: meta.FromUint64(3),
		InterpretationRunID: meta.FromUint64(4),
		Association:         report.Association{OrgID: 1, AssessmentID: meta.FromUint64(5), TesteeID: 6},
		ReportType:          policy.ReportTypeStandard, TemplateVersion: policy.TemplateVersionV1,
		BuilderIdentity:      report.BuilderIdentityFactorScoring,
		ContentSchemaVersion: report.ContentSchemaVersionV1,
		GeneratedAt:          time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("historical read must not enforce content contract: %v", err)
	}
}

func newWriteInput(builderIdentity string, content report.Content) report.InterpretReportInput {
	return report.InterpretReportInput{
		ID: meta.FromUint64(1), GenerationID: meta.FromUint64(2), OutcomeID: meta.FromUint64(3),
		InterpretationRunID: meta.FromUint64(4),
		Association:         report.Association{OrgID: 1, AssessmentID: meta.FromUint64(5), TesteeID: 6},
		ReportType:          policy.ReportTypeStandard, TemplateVersion: policy.TemplateVersionV1,
		BuilderIdentity: builderIdentity, ContentSchemaVersion: report.ContentSchemaVersionV1,
		Content: content, GeneratedAt: time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC),
	}
}

func factorScoringMinimalContent() report.Content {
	return report.Content{
		Model: report.ModelIdentity{
			Kind: string(modelcatalog.KindScale), Code: "PHQ9", Version: "v1", Title: "抑郁筛查",
			AlgorithmFamily: string(modelcatalog.AlgorithmFamilyFactorScoring),
		},
		PrimaryScore: report.NewRawTotalScore(8, nil),
		Level:        report.LevelFromRisk(report.RiskLevelLow),
		Dimensions: []report.DimensionInterpret{
			report.NewDimensionInterpret(report.NewFactorCode("TOTAL"), "总分", 8, nil, report.RiskLevelLow, "轻度", "观察"),
		},
	}
}

func normProfileMinimalContent() report.Content {
	dimension := report.NewDimensionInterpret(report.NewFactorCode("GEC"), "GEC", 12, nil, report.RiskLevelHigh, "偏高", "建议")
	return report.Content{
		Model: report.ModelIdentity{
			Kind: string(modelcatalog.KindScale), Code: "CBCL", Version: "v1", Title: "儿童行为",
			AlgorithmFamily: string(modelcatalog.AlgorithmFamilyFactorNorm),
		},
		PrimaryScore: report.NewRawTotalScore(42, nil),
		Level:        report.LevelFromRisk(report.RiskLevelHigh),
		Dimensions: []report.DimensionInterpret{
			dimension.WithScoreContext(
				[]report.ScoreValue{{Kind: report.ScoreKindTScore, Value: 65}},
				&report.ResultLevel{Code: "elevated", Severity: "high"},
				&report.NormReference{ScoreKind: report.ScoreKindTScore, Benchmark: 50, TableVersion: "2026"},
			),
		},
	}
}

func typologyMinimalContent() report.Content {
	return report.Content{
		Model: report.ModelIdentity{
			Kind: string(modelcatalog.KindTypology), Code: "MBTI", Version: "v1", Title: "MBTI",
			AlgorithmFamily: string(modelcatalog.AlgorithmFamilyFactorClassification),
		},
		Conclusion: "INTJ 建筑师",
		Dimensions: []report.DimensionInterpret{
			report.NewNeutralDimensionInterpret(report.NewDimensionCode("EI"), report.DimensionKindPole, "外向/内向", 12, nil, nil, "偏内向", ""),
		},
		ModelExtra: &report.ModelExtra{
			Kind: "personality_type", TypeCode: "INTJ", TypeName: "建筑师", OneLiner: "独立规划者",
		},
	}
}

func taskPerformanceMinimalContent() report.Content {
	return report.Content{
		Model: report.ModelIdentity{
			Kind: string(modelcatalog.KindCognitive), Code: "SPM", Version: "v1", Title: "瑞文推理",
			AlgorithmFamily: string(modelcatalog.AlgorithmFamilyTaskPerformance),
		},
		PrimaryScore: report.NewRawTotalScore(36, nil),
		Dimensions: []report.DimensionInterpret{
			report.NewNeutralDimensionInterpret(report.NewDimensionCode("ACC"), report.DimensionKindAbility, "正确率", 90, nil, nil, "表现稳定", ""),
		},
	}
}
