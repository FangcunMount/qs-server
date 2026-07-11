package consistency

import (
	"context"

	outcomescoring "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome/scoring"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
)

// CompositeScoringArtifactChecker checks snapshot store and score read model.
type CompositeScoringArtifactChecker struct {
	SnapshotStore outcomescoring.SnapshotStore
	ScoreReader   evaluationreadmodel.ScoreReader
}

func (c CompositeScoringArtifactChecker) HasScoringArtifact(ctx context.Context, assessmentID uint64) (bool, error) {
	if c.SnapshotStore != nil {
		outcome, err := c.SnapshotStore.Load(ctx, assessmentID)
		if err != nil {
			return false, err
		}
		if outcome != nil {
			return true, nil
		}
	}
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
