package checkpoint

import (
	"context"
	"fmt"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationrun"
	"gorm.io/gorm"
)

func NewExactRunReader(db *gorm.DB) evaluationrun.ExactReader { return &Repository{db: db} }

func (r *Repository) FindRunByID(ctx context.Context, id evalrun.ID) (*evalrun.EvaluationRun, error) {
	if r == nil || r.db == nil || id == "" {
		return nil, fmt.Errorf("original run primary and identity required")
	}
	var rows []RuntimeCheckpointPO
	// Always read the original primary at a fresh boundary. Bound the query and
	// reject contradictory duplicates instead of choosing a convenient attempt.
	if err := r.db.WithContext(ctx).Where("scope = ? AND resource_id = ? AND deleted_at IS NULL", scopeEvaluationRun, id.String()).Limit(2).Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	if len(rows) != 1 {
		return nil, fmt.Errorf("evaluation run identity is ambiguous")
	}
	run := runFromPO(rows[0])
	return &run, nil
}
