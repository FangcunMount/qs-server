package lifecycle

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/assessmentstore"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/shared"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func (s *lifecycleService) usesAssessmentModelStore() bool {
	return s != nil && s.modelRepo != nil
}

func (s *lifecycleService) loadAssessmentModel(ctx context.Context, code string) (*domain.AssessmentModel, error) {
	return assessmentstore.LoadScale(ctx, s.modelRepo, code)
}

func (s *lifecycleService) ensureAssessmentModelHeadEditable(ctx context.Context, model *domain.AssessmentModel) error {
	return assessmentstore.EnsureHeadEditable(ctx, s.modelRepo, model)
}

func (s *lifecycleService) saveAssessmentModel(ctx context.Context, model *domain.AssessmentModel) error {
	return assessmentstore.SaveScale(ctx, s.modelRepo, model)
}

func (s *lifecycleService) scaleResultFromAssessmentModel(model *domain.AssessmentModel) (*shared.ScaleResult, error) {
	return assessmentstore.ScaleResult(model)
}
