package operator

import (
	"context"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	"time"

	appauthz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	"github.com/FangcunMount/qs-server/internal/pkg/safeconv"
)

// Progress is an explicit allowlist. Never embed Assessment or Run: those
// contain scores, clinical findings and internal failure diagnostics.
type Progress struct {
	// ManualRetryAvailable exposes only current workflow eligibility, not an authorization grant.
	ManualRetryAvailable bool       `json:"manual_retry_available"`
	ID                   uint64     `json:"id,string"`
	TesteeID             uint64     `json:"testee_id,string"`
	QuestionnaireCode    string     `json:"questionnaire_code"`
	QuestionnaireVersion string     `json:"questionnaire_version"`
	OriginType           string     `json:"origin_type"`
	OriginID             *string    `json:"origin_id,omitempty"`
	Status               string     `json:"status"`
	SubmittedAt          *time.Time `json:"submitted_at,omitempty"`
	EvaluatedAt          *time.Time `json:"evaluated_at,omitempty"`
	FailedAt             *time.Time `json:"failed_at,omitempty"`
}
type ProgressList struct {
	Items      []*Progress `json:"items"`
	Total      int         `json:"total"`
	Page       int         `json:"page"`
	PageSize   int         `json:"page_size"`
	TotalPages int         `json:"total_pages"`
}
type ProgressQueryService interface {
	GetProgress(context.Context, Actor, uint64) (*Progress, error)
	ListProgress(context.Context, Actor, ListQuery) (*ProgressList, error)
}

var _ ProgressQueryService = (*queryService)(nil)

func (s *queryService) GetProgress(ctx context.Context, actor Actor, id uint64) (*Progress, error) {
	if err := appauthz.RequirePermission(ctx, appauthz.AssessmentResource, "read_progress"); err != nil {
		return nil, err
	}
	a, err := s.loadAccessible(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	available, err := s.manualRetryAvailable(ctx, a.ID().Uint64(), a.Status().String())
	if err != nil {
		return nil, err
	}
	return &Progress{ManualRetryAvailable: available, ID: a.ID().Uint64(), TesteeID: a.TesteeID().Uint64(), QuestionnaireCode: a.QuestionnaireRef().Code().String(), QuestionnaireVersion: a.QuestionnaireRef().Version(), OriginType: a.OriginType().String(), OriginID: a.OriginID(), Status: a.Status().String(), SubmittedAt: a.SubmittedAt(), EvaluatedAt: a.EvaluatedAt(), FailedAt: a.FailedAt()}, nil
}
func (s *queryService) ListProgress(ctx context.Context, actor Actor, q ListQuery) (*ProgressList, error) {
	if err := appauthz.RequirePermission(ctx, appauthz.AssessmentResource, "list_progress"); err != nil {
		return nil, err
	}
	rows, total, page, size, err := s.listRows(ctx, actor, q)
	if err != nil {
		return nil, err
	}
	count, err := safeconv.Int64ToInt(total)
	if err != nil {
		return nil, evalerrors.DatabaseMessage("测评总数超出安全范围")
	}
	items := make([]*Progress, 0, len(rows))
	for _, row := range rows {
		available, err := s.manualRetryAvailable(ctx, row.ID, row.Status)
		if err != nil {
			return nil, err
		}
		items = append(items, &Progress{ManualRetryAvailable: available, ID: row.ID, TesteeID: row.TesteeID, QuestionnaireCode: row.QuestionnaireCode, QuestionnaireVersion: row.QuestionnaireVersion, OriginType: row.OriginType, OriginID: row.OriginID, Status: row.Status, SubmittedAt: row.SubmittedAt, EvaluatedAt: row.EvaluatedAt, FailedAt: row.FailedAt})
	}
	return &ProgressList{Items: items, Total: count, Page: page, PageSize: size, TotalPages: pages(count, size)}, nil
}

// Called only after permission and business-range filtering. No run details cross
// the progress boundary; retry submission must recheck the current state.
func (s *queryService) manualRetryAvailable(ctx context.Context, id uint64, status string) (bool, error) {
	if status != "failed" || s.runs == nil {
		return false, nil
	}
	run, err := s.runs.FindLatestByAssessmentID(ctx, id)
	if err != nil {
		return false, err
	}
	if run == nil || run.Attempt().Status != evalrun.StatusFailed {
		return false, nil
	}
	decision := run.RetryDecision()
	return decision != nil && decision.Disposition == retrygovernance.DispositionManualRequired, nil
}
