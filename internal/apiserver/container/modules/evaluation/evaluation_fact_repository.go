package evaluation

import (
	"context"

	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type evaluationFactRepository struct{ source domainoutcome.Repository }

func newEvaluationFactRepository(source domainoutcome.Repository) evaluationfact.Repository {
	if source == nil {
		return nil
	}
	return &evaluationFactRepository{source: source}
}

func (r *evaluationFactRepository) FindByID(ctx context.Context, id meta.ID) (*evaluationfact.Record, error) {
	record, err := r.source.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return evaloutcome.FactRecord(record), nil
}

func (r *evaluationFactRepository) FindByAssessmentID(ctx context.Context, assessmentID meta.ID) (*evaluationfact.Record, error) {
	record, err := r.source.FindByAssessmentID(ctx, assessmentID)
	if err != nil {
		return nil, err
	}
	return evaloutcome.FactRecord(record), nil
}
