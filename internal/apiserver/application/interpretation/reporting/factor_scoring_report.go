package reporting

import (
	"context"

	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	reportscore "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/scoring"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/scale"
)

type FactorScoringScoreProjector struct {
	scoreRepo assessment.ScoreRepository
}

func NewFactorScoringScoreProjector(scoreRepo assessment.ScoreRepository) FactorScoringScoreProjector {
	return FactorScoringScoreProjector{scoreRepo: scoreRepo}
}

func (p FactorScoringScoreProjector) ExecutionIdentity() evaluation.ExecutionIdentity {
	return evaluation.ExecutionIdentityScaleDefault
}

func (p FactorScoringScoreProjector) Key() evaluation.ExecutionIdentity {
	return p.ExecutionIdentity()
}

func (p FactorScoringScoreProjector) Project(ctx context.Context, outcome evaloutcome.Outcome) error {
	if p.scoreRepo == nil || outcome.Assessment == nil || outcome.Execution == nil {
		return nil
	}
	score := assessment.ScaleScoreProjectionFromOutcome(outcome.Assessment.ID(), evaloutcome.AssessmentOutcomeFromExecution(outcome.Execution))
	if err := p.scoreRepo.SaveScoresWithContext(ctx, outcome.Assessment, score); err != nil {
		return evalerrors.Database(err, "保存测评得分失败")
	}
	return nil
}

type FactorScoringReportBuilder struct {
	composer domainReport.ReportBuilder
}

func NewFactorScoringReportBuilder(composer domainReport.ReportBuilder) FactorScoringReportBuilder {
	return FactorScoringReportBuilder{composer: composer}
}

func (b FactorScoringReportBuilder) ExecutionIdentity() evaluation.ExecutionIdentity {
	return evaluation.ExecutionIdentityScaleDefault
}

func (b FactorScoringReportBuilder) Key() evaluation.ExecutionIdentity {
	return b.ExecutionIdentity()
}

func (FactorScoringReportBuilder) ReportType() domainReport.ReportType {
	return domainReport.ReportTypeStandard
}

func (b FactorScoringReportBuilder) Build(ctx context.Context, outcome evaloutcome.Outcome) (*domainReport.InterpretReport, error) {
	if b.composer == nil {
		return nil, evalerrors.ModuleNotConfigured("factor_scoring report builder is not configured")
	}
	_ = ctx
	rpt, err := reportscore.BuildFactorScoringReport(b.composer, factorScoringReportInputFromOutcome(outcome))
	if err != nil {
		return nil, err
	}
	return AttachReportOutcomeSummary(outcome, rpt), nil
}

func factorScoringReportInputFromOutcome(outcome evaloutcome.Outcome) reportscore.FactorScoringReportInput {
	input := reportscore.FactorScoringReportInput{}
	if outcome.Assessment != nil {
		input.AssessmentID = domainReport.ID(outcome.Assessment.ID())
	}
	input.Scale = scaleReportModelFromOutcome(outcome)
	if execution := outcome.Execution; execution != nil {
		if execution.Primary != nil {
			input.TotalScore = execution.Primary.Value
		}
		if execution.Level != nil {
			input.RiskLevel = domainReport.RiskLevel(execution.Level.Code)
		}
		if outcomeDimensionsPreferredForReporting(execution.Dimensions) {
			input.FactorScores = scaleDimensionReportScores(execution.Dimensions, input.Scale)
		} else if scores, ok := execution.Detail.Payload.([]assessment.FactorScoreResult); ok && len(scores) > 0 {
			input.FactorScores = scaleFactorReportScores(scores)
		} else if len(execution.Dimensions) > 0 {
			input.FactorScores = scaleDimensionReportScores(execution.Dimensions, input.Scale)
		}
	}
	return input
}

func outcomeDimensionsPreferredForReporting(dimensions []domainoutcome.DimensionResult) bool {
	for _, dim := range dimensions {
		if dim.Role != "" || dim.ParentCode != "" || dim.HierarchyLevel > 0 || dim.SortOrder > 0 {
			return true
		}
	}
	return false
}

func scaleDimensionReportScores(dimensions []domainoutcome.DimensionResult, model *reportscore.ReportModel) []reportscore.FactorReportScore {
	totalFactors := scaleTotalFactorCodes(model)
	scores := make([]reportscore.FactorReportScore, 0, len(dimensions))
	for _, dim := range dimensions {
		if dim.Score == nil {
			continue
		}
		risk := domainReport.RiskLevelNone
		if dim.Level != nil && domainReport.IsRiskLevelCode(dim.Level.Code) {
			risk = domainReport.RiskLevel(dim.Level.Code)
		}
		scores = append(scores, reportscore.FactorReportScore{
			FactorCode:     dim.Code,
			FactorName:     dim.Name,
			RawScore:       dim.Score.Value,
			RiskLevel:      risk,
			Conclusion:     dim.Description,
			Suggestion:     dim.Suggestion,
			IsTotalScore:   totalFactors[dim.Code],
			Role:           dim.Role,
			ParentCode:     dim.ParentCode,
			HierarchyLevel: dim.HierarchyLevel,
			SortOrder:      dim.SortOrder,
		})
	}
	return scores
}

func scaleTotalFactorCodes(model *reportscore.ReportModel) map[string]bool {
	if model == nil {
		return nil
	}
	codes := make(map[string]bool, len(model.Factors))
	for _, factor := range model.Factors {
		if factor.IsTotalScore {
			codes[factor.Code] = true
		}
	}
	return codes
}

func scaleFactorReportScores(factorScores []assessment.FactorScoreResult) []reportscore.FactorReportScore {
	scores := make([]reportscore.FactorReportScore, 0, len(factorScores))
	for _, fs := range factorScores {
		scores = append(scores, reportscore.FactorReportScore{
			FactorCode:   string(fs.FactorCode),
			FactorName:   fs.FactorName,
			RawScore:     fs.RawScore,
			RiskLevel:    domainReport.RiskLevel(fs.RiskLevel),
			Conclusion:   fs.Conclusion,
			Suggestion:   fs.Suggestion,
			IsTotalScore: fs.IsTotalScore,
		})
	}
	return scores
}

func scaleReportModelFromOutcome(outcome evaloutcome.Outcome) *reportscore.ReportModel {
	if outcome.Input == nil {
		return nil
	}
	snapshot, _ := evaluationinput.ScalePayload(outcome.Input)
	return scaleReportModelFromSnapshot(snapshot)
}

func scaleReportModelFromSnapshot(snapshot *scalesnapshot.ScaleSnapshot) *reportscore.ReportModel {
	if snapshot == nil {
		return nil
	}
	factors := make([]reportscore.FactorReportModel, 0, len(snapshot.Factors))
	for _, factor := range snapshot.Factors {
		factors = append(factors, reportscore.FactorReportModel{
			Code:           factor.Code,
			Title:          factor.Title,
			MaxScore:       factor.MaxScore,
			IsTotalScore:   factor.IsTotalScore,
			InterpretRules: scaleFactorInterpretRules(factor.InterpretRules),
		})
	}
	return &reportscore.ReportModel{
		Code:    snapshot.Code,
		Title:   snapshot.Title,
		Factors: factors,
	}
}

func scaleFactorInterpretRules(rules []scalesnapshot.InterpretRuleSnapshot) []reportscore.FactorInterpretRule {
	if len(rules) == 0 {
		return nil
	}
	converted := make([]reportscore.FactorInterpretRule, 0, len(rules))
	for _, rule := range rules {
		converted = append(converted, reportscore.FactorInterpretRule{
			Min:        rule.Min,
			Max:        rule.Max,
			RiskLevel:  rule.RiskLevel,
			Conclusion: rule.Conclusion,
			Suggestion: rule.Suggestion,
		})
	}
	return converted
}
