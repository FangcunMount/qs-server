package result

import (
	"context"

	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	evaluationdomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/report"
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
	return evaluationdomain.BuildScaleReport(b.composer, scaleReportInputFromOutcome(outcome))
}

func scaleReportInputFromOutcome(outcome Outcome) evaluationdomain.ScaleReportInput {
	input := evaluationdomain.ScaleReportInput{}
	if outcome.Assessment != nil {
		input.AssessmentID = domainReport.ID(outcome.Assessment.ID())
	}
	input.Scale = scaleSnapshotFromOutcome(outcome)
	if outcome.Result != nil {
		input.TotalScore = outcome.Result.TotalScore
		input.RiskLevel = domainReport.RiskLevel(outcome.Result.RiskLevel)
		input.Conclusion = outcome.Result.Conclusion
		input.Suggestion = outcome.Result.Suggestion
		input.FactorScores = scaleFactorReportScores(outcome.Result.FactorScores)
	}
	return input
}

func scaleFactorReportScores(factorScores []assessment.FactorScoreResult) []evaluationdomain.ScaleFactorReportScore {
	scores := make([]evaluationdomain.ScaleFactorReportScore, 0, len(factorScores))
	for _, fs := range factorScores {
		scores = append(scores, evaluationdomain.ScaleFactorReportScore{
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

func scaleSnapshotFromOutcome(outcome Outcome) *evaluationinput.ScaleSnapshot {
	if outcome.Input == nil {
		return nil
	}
	snapshot, _ := evaluationinput.ScalePayload(outcome.Input)
	return snapshot
}
