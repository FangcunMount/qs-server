package scoring

import (
	"context"

	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
)

// Projector persists Evaluation-owned score query projections derived from a canonical outcome.
type Projector interface {
	Project(ctx context.Context, record *domainoutcome.Record, assessed *assessment.Assessment, execution *domainoutcome.Execution) error
}

type assessmentScoreProjector struct {
	repo assessment.ScoreRepository
}

func NewAssessmentScoreProjector(repo assessment.ScoreRepository) Projector {
	return &assessmentScoreProjector{repo: repo}
}

func (p *assessmentScoreProjector) Project(ctx context.Context, record *domainoutcome.Record, assessed *assessment.Assessment, execution *domainoutcome.Execution) error {
	if p == nil || p.repo == nil || record == nil || assessed == nil || execution == nil {
		return nil
	}
	projection := evaloutcome.ScaleScoreProjectionFromExecution(assessed.ID(), execution)
	if projection == nil {
		return nil
	}
	if err := p.repo.SaveProjectionFromOutcome(ctx, record.ID(), assessed, projection); err != nil {
		return evalerrors.Database(err, "保存测评得分失败")
	}
	return nil
}
