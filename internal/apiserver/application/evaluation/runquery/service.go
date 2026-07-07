package runquery

import (
	"context"

	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationrun"
)

const (
	defaultAssessmentRunLimit = 20
	maxAssessmentRunLimit     = 100
	defaultFailedRunLimit     = 50
	maxFailedRunLimit         = 200
)

// Service reads persisted 评估执行。
type Service interface {
	ListByAssessmentID(ctx context.Context, assessmentID uint64, limit int) (*RunListResult, error)
	FindLatestByAssessmentID(ctx context.Context, assessmentID uint64) (*RunResult, error)
	ListRetryableFailed(ctx context.Context, orgID int64, limit int, cursor uint64) (*RetryableFailedListResult, error)
}

type service struct {
	runRepo evaluationrun.Repository
}

// NewService 创建评估执行 查询服务。
func NewService(runRepo evaluationrun.Repository) Service {
	return &service{runRepo: runRepo}
}

func (s *service) ListByAssessmentID(ctx context.Context, assessmentID uint64, limit int) (*RunListResult, error) {
	if s == nil || s.runRepo == nil {
		return nil, evalerrors.ModuleNotConfigured("evaluation run repository is not configured")
	}
	if assessmentID == 0 {
		return nil, evalerrors.InvalidArgument("assessment id is required")
	}
	limit = normalizeAssessmentRunLimit(limit)
	runs, err := s.runRepo.ListByAssessmentID(ctx, assessmentID, limit)
	if err != nil {
		return nil, err
	}
	items := make([]*RunResult, 0, len(runs))
	for _, run := range runs {
		items = append(items, runResultFromDomain(run))
	}
	return &RunListResult{Items: items}, nil
}

func (s *service) FindLatestByAssessmentID(ctx context.Context, assessmentID uint64) (*RunResult, error) {
	if s == nil || s.runRepo == nil {
		return nil, evalerrors.ModuleNotConfigured("evaluation run repository is not configured")
	}
	if assessmentID == 0 {
		return nil, evalerrors.InvalidArgument("assessment id is required")
	}
	run, err := s.runRepo.FindLatestByAssessmentID(ctx, assessmentID)
	if err != nil {
		return nil, err
	}
	if run == nil {
		return nil, nil
	}
	return runResultFromDomain(*run), nil
}

func (s *service) ListRetryableFailed(ctx context.Context, orgID int64, limit int, cursor uint64) (*RetryableFailedListResult, error) {
	if s == nil || s.runRepo == nil {
		return nil, evalerrors.ModuleNotConfigured("evaluation run repository is not configured")
	}
	if orgID <= 0 {
		return nil, evalerrors.InvalidArgument("org id is required")
	}
	limit = normalizeFailedRunLimit(limit)
	page, err := s.runRepo.ListRetryableFailed(ctx, evaluationrun.ListRetryableFailedParams{
		OrgID:  orgID,
		Limit:  limit,
		Cursor: cursor,
	})
	if err != nil {
		return nil, err
	}
	if page == nil {
		return &RetryableFailedListResult{}, nil
	}
	items := make([]*RetryableFailedRunResult, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, &RetryableFailedRunResult{
			RunResult: *runResultFromDomain(item.Run),
			OrgID:     item.OrgID,
		})
	}
	return &RetryableFailedListResult{
		Items:      items,
		NextCursor: page.NextCursor,
	}, nil
}

func normalizeAssessmentRunLimit(limit int) int {
	if limit <= 0 {
		return defaultAssessmentRunLimit
	}
	if limit > maxAssessmentRunLimit {
		return maxAssessmentRunLimit
	}
	return limit
}

func normalizeFailedRunLimit(limit int) int {
	if limit <= 0 {
		return defaultFailedRunLimit
	}
	if limit > maxFailedRunLimit {
		return maxFailedRunLimit
	}
	return limit
}
