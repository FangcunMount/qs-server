package consistency

import (
	"context"
	stderrors "errors"

	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"gorm.io/gorm"
)

// OutcomeExistenceChecker checks the canonical immutable evaluation fact.
type OutcomeExistenceChecker struct {
	Repository domainoutcome.Repository
}

func (c OutcomeExistenceChecker) HasOutcome(ctx context.Context, assessmentID uint64) (bool, error) {
	if c.Repository == nil {
		return false, nil
	}
	record, err := c.Repository.FindByAssessmentID(ctx, meta.FromUint64(assessmentID))
	if stderrors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return record != nil, nil
}
