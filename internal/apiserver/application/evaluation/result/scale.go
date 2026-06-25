package result

import (
	"context"

	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/report"
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
	builder domainReport.ReportBuilder
}

func NewScaleReportBuilder(builder domainReport.ReportBuilder) ScaleReportBuilder {
	return ScaleReportBuilder{builder: builder}
}

func (b ScaleReportBuilder) Kind() assessment.EvaluationModelKind {
	return assessment.EvaluationModelKindScale
}

func (ScaleReportBuilder) ReportType() domainReport.ReportType {
	return domainReport.ReportTypeStandard
}

func (b ScaleReportBuilder) Build(ctx context.Context, outcome Outcome) (*domainReport.InterpretReport, error) {
	if b.builder == nil {
		return nil, evalerrors.ModuleNotConfigured("scale report builder is not configured")
	}
	_ = ctx
	return b.builder.Build(scaleReportInputFromOutcome(outcome))
}

func scaleReportInputFromOutcome(outcome Outcome) domainReport.GenerateReportInput {
	input := domainReport.GenerateReportInput{}
	if outcome.Assessment != nil {
		input.AssessmentID = domainReport.ID(outcome.Assessment.ID())
	}
	scaleSnapshot := scaleSnapshotFromOutcome(outcome)
	if scaleSnapshot != nil {
		input.ScaleName = scaleSnapshot.Title
		input.ScaleCode = scaleSnapshot.Code
	}
	if outcome.Result != nil {
		input.TotalScore = outcome.Result.TotalScore
		input.RiskLevel = domainReport.RiskLevel(outcome.Result.RiskLevel)
		input.Conclusion = outcome.Result.Conclusion
		input.Suggestion = outcome.Result.Suggestion
		input.FactorScores = scaleReportFactorScoreInputs(outcome.Result.FactorScores, scaleSnapshot)
	}
	return input
}

func scaleReportFactorScoreInputs(
	factorScores []assessment.FactorScoreResult,
	scaleSnapshot *evaluationinput.ScaleSnapshot,
) []domainReport.FactorScoreInput {
	factorMeta := make(map[string]evaluationinput.FactorSnapshot)
	if scaleSnapshot != nil {
		for _, f := range scaleSnapshot.Factors {
			factorMeta[f.Code] = f
		}
	}
	inputs := make([]domainReport.FactorScoreInput, 0, len(factorScores))
	for _, fs := range factorScores {
		meta, ok := factorMeta[string(fs.FactorCode)]
		factorName := fs.FactorName
		var maxScore *float64
		if ok {
			if factorName == "" {
				factorName = meta.Title
			}
			maxScore = meta.MaxScore
		}
		if factorName == "" {
			factorName = string(fs.FactorCode)
		}
		inputs = append(inputs, domainReport.FactorScoreInput{
			FactorCode:   domainReport.FactorCode(fs.FactorCode),
			FactorName:   factorName,
			RawScore:     fs.RawScore,
			MaxScore:     maxScore,
			RiskLevel:    domainReport.RiskLevel(fs.RiskLevel),
			Description:  fs.Conclusion,
			Suggestion:   fs.Suggestion,
			IsTotalScore: fs.IsTotalScore,
		})
	}
	return inputs
}

func scaleSnapshotFromOutcome(outcome Outcome) *evaluationinput.ScaleSnapshot {
	if outcome.Input == nil {
		return nil
	}
	snapshot, _ := evaluationinput.ScalePayload(outcome.Input)
	return snapshot
}
