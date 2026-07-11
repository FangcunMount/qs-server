package consistency

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
)

// CompositeScoringArtifactChecker checks durable score projections.
type CompositeScoringArtifactChecker struct {
	ScoreReader evaluationreadmodel.ScoreReader
}

func (c CompositeScoringArtifactChecker) HasScoringArtifact(ctx context.Context, assessmentID uint64) (bool, error) {
	if c.ScoreReader != nil {
		row, err := c.ScoreReader.GetScoreByAssessmentID(ctx, assessmentID)
		if err != nil {
			return false, err
		}
		if row != nil {
			return true, nil
		}
	}
	return false, nil
}
