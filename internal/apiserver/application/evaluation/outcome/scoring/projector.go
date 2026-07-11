package scoring

import (
	"context"

	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
)

// Projector persists Evaluation-owned score facts derived from a canonical outcome.
type Projector interface {
	Project(ctx context.Context, outcome evaloutcome.Outcome) error
}

type assessmentScoreProjector struct {
	repo assessment.ScoreRepository
}

func NewAssessmentScoreProjector(repo assessment.ScoreRepository) Projector {
	return &assessmentScoreProjector{repo: repo}
}

func (p *assessmentScoreProjector) Project(ctx context.Context, outcome evaloutcome.Outcome) error {
	if p == nil || p.repo == nil || outcome.Assessment == nil || outcome.Execution == nil {
		return nil
	}
	projection := assessment.ScaleScoreProjectionFromOutcome(outcome.Assessment.ID(), outcome.Execution)
	if projection == nil {
		return nil
	}
	if err := p.repo.SaveScoresWithContext(ctx, outcome.Assessment, projection); err != nil {
		return evalerrors.Database(err, "保存测评得分失败")
	}
	return nil
}
