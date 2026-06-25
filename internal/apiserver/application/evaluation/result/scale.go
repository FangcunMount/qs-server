package result

import (
	"context"

	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/report"
	reportscale "github.com/FangcunMount/qs-server/internal/apiserver/domain/report/scale"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/ruleset/scale/snapshot"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

type ScaleScoreProjector struct {
	scoreRepo assessment.ScoreRepository
}

func NewScaleScoreProjector(scoreRepo assessment.ScoreRepository) ScaleScoreProjector {
	return ScaleScoreProjector{scoreRepo: scoreRepo}
}

func (p ScaleScoreProjector) Kind() assessment.EvaluationModelKind {
	return assessment.EvaluationModelKindScale
}

func (p ScaleScoreProjector) Project(ctx context.Context, outcome Outcome) error {
	if p.scoreRepo == nil || outcome.Assessment == nil || outcome.Result == nil {
		return nil
	}
	score := assessment.FromEvaluationResult(outcome.Assessment.ID(), outcome.Result)
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

func (b ScaleReportBuilder) Kind() assessment.EvaluationModelKind {
	return assessment.EvaluationModelKindScale
}

func (ScaleReportBuilder) ReportType() domainReport.ReportType {
	return domainReport.ReportTypeStandard
}

func (b ScaleReportBuilder) Build(ctx context.Context, outcome Outcome) (*domainReport.InterpretReport, error) {
	if b.composer == nil {
		return nil, evalerrors.ModuleNotConfigured("scale report builder is not configured")
	}
	_ = ctx
	return reportscale.BuildReport(b.composer, scaleReportInputFromOutcome(outcome))
}

func scaleReportInputFromOutcome(outcome Outcome) reportscale.ReportInput {
	input := reportscale.ReportInput{}
	if outcome.Assessment != nil {
		input.AssessmentID = domainReport.ID(outcome.Assessment.ID())
	}
	input.Scale = scaleReportModelFromOutcome(outcome)
	if outcome.Result != nil {
		input.TotalScore = outcome.Result.TotalScore
		input.RiskLevel = domainReport.RiskLevel(outcome.Result.RiskLevel)
		input.Conclusion = outcome.Result.Conclusion
		input.Suggestion = outcome.Result.Suggestion
		input.FactorScores = scaleFactorReportScores(outcome.Result.FactorScores)
	}
	return input
}

func scaleFactorReportScores(factorScores []assessment.FactorScoreResult) []reportscale.FactorReportScore {
	scores := make([]reportscale.FactorReportScore, 0, len(factorScores))
	for _, fs := range factorScores {
		scores = append(scores, reportscale.FactorReportScore{
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

func scaleReportModelFromOutcome(outcome Outcome) *reportscale.ReportModel {
	if outcome.Input == nil {
		return nil
	}
	snapshot, _ := evaluationinput.ScalePayload(outcome.Input)
	return scaleReportModelFromSnapshot(snapshot)
}

func scaleReportModelFromSnapshot(snapshot *scalesnapshot.ScaleSnapshot) *reportscale.ReportModel {
	if snapshot == nil {
		return nil
	}
	factors := make([]reportscale.FactorReportModel, 0, len(snapshot.Factors))
	for _, factor := range snapshot.Factors {
		factors = append(factors, reportscale.FactorReportModel{
			Code:     factor.Code,
			Title:    factor.Title,
			MaxScore: factor.MaxScore,
		})
	}
	return &reportscale.ReportModel{
		Code:    snapshot.Code,
		Title:   snapshot.Title,
		Factors: factors,
	}
}
