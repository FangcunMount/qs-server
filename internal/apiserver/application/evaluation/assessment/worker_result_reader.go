package assessment

import (
	"context"

	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	domainassessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// workerAssessmentResultReader serves the trusted Worker response path. It is
// deliberately separate from the backend-operator query service so the two
// actors can evolve their result and authorization contracts independently.
type workerAssessmentResultReader struct {
	repo domainassessment.Repository
}

// NewWorkerAssessmentResultReader creates the narrow read capability used
// after one Worker evaluation attempt.
func NewWorkerAssessmentResultReader(repo domainassessment.Repository) AssessmentResultReader {
	return &workerAssessmentResultReader{repo: repo}
}

func (r *workerAssessmentResultReader) GetByID(ctx context.Context, id uint64) (*AssessmentResult, error) {
	if r == nil || r.repo == nil {
		return nil, evalerrors.ModuleNotConfigured("assessment repository is not configured")
	}
	a, err := r.repo.FindByID(ctx, meta.FromUint64(id))
	if err != nil {
		return nil, evalerrors.AssessmentNotFound(err, "测评不存在")
	}
	return toAssessmentResult(a)
}

var _ AssessmentResultReader = (*workerAssessmentResultReader)(nil)
