package result

import (
	"context"

	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	reportscore "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/score"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scale/snapshot"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

type ScaleScoreProjector struct {
	scoreRepo assessment.ScoreRepository
}

func NewScaleScoreProjector(scoreRepo assessment.ScoreRepository) ScaleScoreProjector {
	return ScaleScoreProjector{scoreRepo: scoreRepo}
}

func (p ScaleScoreProjector) Key() evaluation.EvaluatorKey {
	return evaluation.EvaluatorKeyScaleDefault
}

func (p ScaleScoreProjector) Project(ctx context.Context, outcome Outcome) error {
	if p.scoreRepo == nil || outcome.Assessment == nil || outcome.Execution == nil {
		return nil
	}
	score := assessment.ScaleScoreProjectionFromOutcome(outcome.Assessment.ID(), outcome.Execution)
	if err := p.scoreRepo.SaveScoresWithContext(ctx, outcome.Assessment, score); err != nil {
		return evalerrors.Database(err, "保存测评得分失败")
	}
	return nil
}

type ScaleReportBuilder struct {
	composer domainReport.ReportBuilder
}

func NewScaleReportBuilder(composer domainReport.ReportBuilder) ScaleReportBuilder {
	return ScaleReportBuilder{composer: composer}
}

func (b ScaleReportBuilder) Key() evaluation.EvaluatorKey {
	return evaluation.EvaluatorKeyScaleDefault
}

func (ScaleReportBuilder) ReportType() domainReport.ReportType {
	return domainReport.ReportTypeStandard
}

func (b ScaleReportBuilder) Build(ctx context.Context, outcome Outcome) (*domainReport.InterpretReport, error) {
	if b.composer == nil {
		return nil, evalerrors.ModuleNotConfigured("scale report builder is not configured")
	}
	_ = ctx
	rpt, err := reportscore.BuildScaleReport(b.composer, scaleReportInputFromOutcome(outcome))
	if err != nil {
		return nil, err
	}
	return attachOutcomeSummary(outcome, rpt), nil
}

func scaleReportInputFromOutcome(outcome Outcome) reportscore.ScaleReportInput {
	input := reportscore.ScaleReportInput{}
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
		if scores, ok := execution.Detail.Payload.([]assessment.FactorScoreResult); ok {
			input.FactorScores = scaleFactorReportScores(scores)
		}
	}
	return input
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

func scaleReportModelFromOutcome(outcome Outcome) *reportscore.ReportModel {
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
