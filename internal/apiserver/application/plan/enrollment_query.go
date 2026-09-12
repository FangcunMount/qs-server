package plan

import (
	"context"
	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/actor/actorctx"
	appauthz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"math"
	"time"
)

type EnrollmentQuery struct {
	OrgID    int64
	TesteeID uint64
	PlanID   *uint64
	Status   string
	Page     int
	PageSize int
}

type EnrollmentTaskItem struct {
	ID               uint64     `json:"id"`
	Seq              int        `json:"seq"`
	ScaleCode        string     `json:"scale_code"`
	Status           string     `json:"status"`
	PlannedAt        time.Time  `json:"planned_at"`
	DueAt            *time.Time `json:"due_at,omitempty"`
	OpenAt           *time.Time `json:"open_at,omitempty"`
	ExpireAt         *time.Time `json:"expire_at,omitempty"`
	CompletedAt      *time.Time `json:"completed_at,omitempty"`
	ExpiredAt        *time.Time `json:"expired_at,omitempty"`
	ExpirationReason *string    `json:"expiration_reason,omitempty"`
	CanceledAt       *time.Time `json:"canceled_at,omitempty"`
	AssessmentID     *string    `json:"assessment_id,omitempty"`
}

type EnrollmentItem struct {
	ID                 uint64               `json:"id"`
	OrgID              int64                `json:"org_id"`
	PlanID             uint64               `json:"plan_id"`
	TesteeID           uint64               `json:"testee_id"`
	Round              uint32               `json:"round"`
	StartDate          time.Time            `json:"start_date"`
	Status             string               `json:"status"`
	JoinedAt           time.Time            `json:"joined_at"`
	ClosedAt           *time.Time           `json:"closed_at,omitempty"`
	TerminatedAt       *time.Time           `json:"terminated_at,omitempty"`
	TerminatedReason   string               `json:"terminated_reason,omitempty"`
	RecordOrigin       string               `json:"record_origin"`
	ScaleCode          string               `json:"scale_code"`
	ScaleTitle         string               `json:"scale_title"`
	TaskCount          int                  `json:"task_count"`
	CompletedTaskCount int                  `json:"completed_task_count"`
	CompletionRate     float64              `json:"completion_rate"`
	Tasks              []EnrollmentTaskItem `json:"tasks"`
}

type EnrollmentPage struct {
	Items      []EnrollmentItem `json:"items"`
	Total      int64            `json:"total"`
	Page       int              `json:"page"`
	PageSize   int              `json:"page_size"`
	TotalPages int              `json:"total_pages"`
}

type EnrollmentQueryStore interface {
	ListEnrollments(context.Context, EnrollmentQuery) ([]EnrollmentItem, int64, error)
}

type EnrollmentQueryService interface {
	ListEnrollments(context.Context, EnrollmentQuery) (*EnrollmentPage, error)
}

type EnrollmentScopeChecker interface {
	ValidateTesteeStoreAccess(context.Context, int64, int64, uint64, string, string) error
}

type enrollmentQueryService struct {
	access  EnrollmentScopeChecker
	store   EnrollmentQueryStore
	catalog ScaleCatalog
}

func NewEnrollmentQueryService(store EnrollmentQueryStore, catalog ScaleCatalog, access ...EnrollmentScopeChecker) EnrollmentQueryService {
	var checker EnrollmentScopeChecker
	if len(access) > 0 {
		checker = access[0]
	}
	return &enrollmentQueryService{store: store, catalog: catalog, access: checker}
}

func (s *enrollmentQueryService) ListEnrollments(ctx context.Context, query EnrollmentQuery) (*EnrollmentPage, error) {
	orgID := actorctx.OperatorOrgID(ctx)
	userID := actorctx.GrantingUserID(ctx)
	if orgID <= 0 || userID == 0 || userID > math.MaxInt64 || orgID != query.OrgID || query.TesteeID == 0 || s.access == nil {
		return nil, errors.WithCode(code.ErrPermissionDenied, "trusted operator and enrollment scope are required")
	}
	if err := appauthz.RequirePermission(ctx, appauthz.EvaluationPlanTaskResource, "list"); err != nil {
		return nil, err
	}
	if err := s.access.ValidateTesteeStoreAccess(ctx, orgID, int64(userID), query.TesteeID, appauthz.EvaluationPlanTaskResource, "list"); err != nil {
		return nil, err
	}

	if query.Page < 1 {
		query.Page = 1
	}
	if query.PageSize <= 0 {
		query.PageSize = 20
	}
	if query.PageSize > 100 {
		query.PageSize = 100
	}
	items, total, err := s.store.ListEnrollments(ctx, query)
	if err != nil {
		return nil, err
	}
	codes := make([]string, 0, len(items))
	for index := range items {
		item := &items[index]
		item.TaskCount = len(item.Tasks)
		for _, task := range item.Tasks {
			if item.ScaleCode == "" {
				item.ScaleCode = task.ScaleCode
			}
			if task.Status == "completed" {
				item.CompletedTaskCount++
			}
		}
		if item.TaskCount > 0 {
			item.CompletionRate = float64(item.CompletedTaskCount) / float64(item.TaskCount)
		}
		if item.ScaleCode != "" {
			codes = append(codes, item.ScaleCode)
		}
	}
	if s.catalog != nil {
		titles := s.catalog.ResolveTitles(ctx, codes)
		for index := range items {
			items[index].ScaleTitle = titles[items[index].ScaleCode]
		}
	}
	return &EnrollmentPage{Items: items, Total: total, Page: query.Page, PageSize: query.PageSize, TotalPages: int((total + int64(query.PageSize) - 1) / int64(query.PageSize))}, nil
}
