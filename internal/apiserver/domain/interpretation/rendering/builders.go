package rendering

import (
	"context"
	"fmt"

	interpinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/input"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	reportscore "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/scoring"
	reporttypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/typology/patterns"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/outcome"
)

type FactorScoringBuilder struct {
	composer report.DraftBuilder
}

func NewFactorScoringBuilder(composer report.DraftBuilder) FactorScoringBuilder {
	return FactorScoringBuilder{composer: composer}
}
func (FactorScoringBuilder) ReportType() policy.ReportType {
	return policy.ReportTypeStandard
}
func (FactorScoringBuilder) TemplateVersion() policy.TemplateVersion { return policy.TemplateVersionV1 }
func (FactorScoringBuilder) BuilderIdentity() string                 { return "factor-scoring" }
func (FactorScoringBuilder) ContentSchemaVersion() string            { return "report-content/v1" }
func (b FactorScoringBuilder) MechanismKey() Key {
	return Key{AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorScoring, DecisionKind: modelcatalog.DecisionKindScoreRange, ReportType: b.ReportType()}
}
func (b FactorScoringBuilder) Build(_ context.Context, input interpinput.InterpretationInput) (*report.Draft, error) {
	if b.composer == nil {
		return nil, fmt.Errorf("factor_scoring report builder is not configured")
	}
	if input.FactorScoring == nil {
		return nil, fmt.Errorf("factor_scoring interpretation facts are required")
	}
	draft, err := reportscore.BuildFactorScoringDraft(b.composer, reportscore.FactorScoringReportInput{
		AssessmentID: report.ID(input.Association.AssessmentID), Scale: input.FactorScoring.Model,
		TotalScore: primaryValue(input), RiskLevel: riskLevel(input), FactorScores: input.FactorScoring.Factors,
	})
	if err != nil {
		return nil, err
	}
	return withInputSummary(input, draft), nil
}

type NormProfileBuilder struct{ scoring FactorScoringBuilder }

func NewNormProfileBuilder(composer report.DraftBuilder) NormProfileBuilder {
	return NormProfileBuilder{scoring: NewFactorScoringBuilder(composer)}
}
func (NormProfileBuilder) ReportType() policy.ReportType {
	return policy.ReportTypeStandard
}
func (NormProfileBuilder) TemplateVersion() policy.TemplateVersion { return policy.TemplateVersionV1 }
func (NormProfileBuilder) BuilderIdentity() string                 { return "norm-profile" }
func (NormProfileBuilder) ContentSchemaVersion() string            { return "report-content/v1" }
func (b NormProfileBuilder) MechanismKey() Key {
	return Key{AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorNorm, DecisionKind: modelcatalog.DecisionKindNormLookup, ReportType: b.ReportType()}
}
func (b NormProfileBuilder) Build(ctx context.Context, input interpinput.InterpretationInput) (*report.Draft, error) {
	return b.scoring.Build(ctx, input)
}

type TaskPerformanceBuilder struct{ scoring FactorScoringBuilder }

func NewTaskPerformanceBuilder(composer report.DraftBuilder) TaskPerformanceBuilder {
	return TaskPerformanceBuilder{scoring: NewFactorScoringBuilder(composer)}
}
func (TaskPerformanceBuilder) ReportType() policy.ReportType {
	return policy.ReportTypeStandard
}
func (TaskPerformanceBuilder) TemplateVersion() policy.TemplateVersion {
	return policy.TemplateVersionV1
}
func (TaskPerformanceBuilder) BuilderIdentity() string      { return "task-performance" }
func (TaskPerformanceBuilder) ContentSchemaVersion() string { return "report-content/v1" }
func (b TaskPerformanceBuilder) MechanismKey() Key {
	return Key{AlgorithmFamily: modelcatalog.AlgorithmFamilyTaskPerformance, DecisionKind: modelcatalog.DecisionKindAbilityLevel, ReportType: b.ReportType()}
}
func (b TaskPerformanceBuilder) Build(ctx context.Context, input interpinput.InterpretationInput) (*report.Draft, error) {
	return b.scoring.Build(ctx, input)
}

type TypologyBuilder struct {
	adapters map[reporttypology.ReportAdapterKey]struct{}
}

func NewTypologyBuilder() TypologyBuilder {
	return TypologyBuilder{adapters: map[reporttypology.ReportAdapterKey]struct{}{
		reporttypology.ReportAdapterPersonalityType: {}, reporttypology.ReportAdapterTraitProfile: {},
	}}
}
func (TypologyBuilder) ReportType() policy.ReportType {
	return policy.ReportTypeStandard
}
func (TypologyBuilder) TemplateVersion() policy.TemplateVersion { return policy.TemplateVersionV1 }
func (TypologyBuilder) BuilderIdentity() string                 { return "typology" }
func (TypologyBuilder) ContentSchemaVersion() string            { return "report-content/v1" }
func (b TypologyBuilder) MechanismKey() Key                     { return b.MechanismKeys()[0] }
func (TypologyBuilder) MechanismKeys() []Key {
	return []Key{
		{AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorClassification, DecisionKind: modelcatalog.DecisionKindPoleComposition, ReportType: policy.ReportTypeStandard},
		{AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorClassification, DecisionKind: modelcatalog.DecisionKindTraitProfile, ReportType: policy.ReportTypeStandard},
		{AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorClassification, DecisionKind: modelcatalog.DecisionKindNearestPattern, ReportType: policy.ReportTypeStandard},
		{AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorClassification, DecisionKind: modelcatalog.DecisionKindDominantFactor, ReportType: policy.ReportTypeStandard},
	}
}
func (b TypologyBuilder) Build(_ context.Context, input interpinput.InterpretationInput) (*report.Draft, error) {
	if len(b.adapters) == 0 {
		return nil, fmt.Errorf("personality typology report builder is not configured")
	}
	if input.PersonalityType != nil {
		adapter := personalityAdapter(input)
		if _, ok := b.adapters[adapter]; !ok {
			return nil, fmt.Errorf("unsupported report adapter key: %s", adapter)
		}
		tmpl, err := reporttypology.PersonalityTypeTemplateForSpec(reporttypology.ReportSpec{AdapterKey: adapter, TemplateID: input.Report.TemplateID})
		if err != nil {
			return nil, err
		}
		content, err := reporttypology.BuildPersonalityTypeContent(reporttypology.PersonalityTypeReportInput{
			AssessmentID: report.ID(input.Association.AssessmentID), ModelCode: input.Model.Code,
			TotalScore: primaryValue(input), RiskLevel: riskLevel(input), Detail: input.PersonalityType.Detail,
		}, tmpl)
		if err != nil {
			return nil, err
		}
		content.Model, content.PrimaryScore, content.Level = input.Model, input.Result.Primary, input.Result.Level
		return report.NewDraft(content), nil
	}
	if input.TraitProfile != nil {
		adapter := traitAdapter(input)
		if _, ok := b.adapters[adapter]; !ok {
			return nil, fmt.Errorf("unsupported report adapter key: %s", adapter)
		}
		tmpl, err := reporttypology.TraitProfileTemplateForSpec(reporttypology.ReportSpec{AdapterKey: adapter, TemplateID: input.Report.TemplateID})
		if err != nil {
			return nil, err
		}
		content, err := reporttypology.BuildTraitProfileContent(reporttypology.TraitProfileReportInput{
			AssessmentID: report.ID(input.Association.AssessmentID), ModelCode: input.Model.Code,
			TotalScore: primaryValue(input), RiskLevel: riskLevel(input), Detail: input.TraitProfile.Detail,
		}, tmpl)
		if err != nil {
			return nil, err
		}
		content.Model, content.PrimaryScore, content.Level = input.Model, input.Result.Primary, input.Result.Level
		return report.NewDraft(content), nil
	}
	return nil, fmt.Errorf("typology interpretation facts are required")
}

func DefaultBuilders(composer report.DraftBuilder) []Builder {
	return []Builder{NewFactorScoringBuilder(composer), NewTypologyBuilder(), NewNormProfileBuilder(composer), NewTaskPerformanceBuilder(composer)}
}

func withInputSummary(input interpinput.InterpretationInput, draft *report.Draft) *report.Draft {
	if draft == nil {
		return nil
	}
	content := draft.Content()
	if !input.Model.IsEmpty() {
		content.Model = input.Model
	}
	content.PrimaryScore, content.Level = input.Result.Primary, input.Result.Level
	if input.PresentationProfile != nil {
		copy := *input.PresentationProfile
		content.PresentationProfile = &copy
	}
	return report.NewDraft(content)
}
func primaryValue(input interpinput.InterpretationInput) float64 {
	if input.Result.Primary == nil {
		return 0
	}
	return input.Result.Primary.Value
}
func riskLevel(input interpinput.InterpretationInput) report.RiskLevel {
	if input.Result.Level == nil || !eventoutcome.IsRiskLevelCode(input.Result.Level.Code) {
		return report.RiskLevelNone
	}
	return report.RiskLevel(input.Result.Level.Code)
}
func personalityAdapter(input interpinput.InterpretationInput) reporttypology.ReportAdapterKey {
	if input.Report.AdapterKey != "" {
		return reporttypology.ReportAdapterKey(input.Report.AdapterKey)
	}
	return reporttypology.ReportAdapterPersonalityType
}
func traitAdapter(input interpinput.InterpretationInput) reporttypology.ReportAdapterKey {
	if input.Report.AdapterKey != "" {
		return reporttypology.ReportAdapterKey(input.Report.AdapterKey)
	}
	return reporttypology.ReportAdapterTraitProfile
}
