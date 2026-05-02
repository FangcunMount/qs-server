package pipeline

import (
	"context"

	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
)

// AssessmentScoreWriter 封装 risk 计算后的 score 持久化副作用。
type AssessmentScoreWriter interface {
	SaveAssessmentScore(ctx context.Context, evalCtx *Context) error
}

type repositoryAssessmentScoreWriter struct {
	scoreRepo assessment.ScoreRepository
}

func NewAssessmentScoreWriter(scoreRepo assessment.ScoreRepository) AssessmentScoreWriter {
	return repositoryAssessmentScoreWriter{scoreRepo: scoreRepo}
}

// SaveAssessmentScore 保存测评得分。
func (w repositoryAssessmentScoreWriter) SaveAssessmentScore(ctx context.Context, evalCtx *Context) error {
	factorScores := make([]assessment.FactorScore, 0, len(evalCtx.FactorScores))
	for _, fs := range evalCtx.FactorScores {
		factorScores = append(factorScores, assessment.NewFactorScore(
			fs.FactorCode,
			fs.FactorName,
			fs.RawScore,
			fs.RiskLevel,
			fs.IsTotalScore,
		))
	}

	score := assessment.NewAssessmentScore(
		evalCtx.Assessment.ID(),
		evalCtx.TotalScore,
		evalCtx.RiskLevel,
		factorScores,
	)

	if err := w.scoreRepo.SaveScoresWithContext(ctx, evalCtx.Assessment, score); err != nil {
		return evalerrors.Database(err, "保存测评得分失败")
	}
	return nil
}
