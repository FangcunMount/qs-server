package plan

import (
	"context"
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
	ID           uint64     `json:"id"`
	Seq          int        `json:"seq"`
	ScaleCode    string     `json:"scale_code"`
	Status       string     `json:"status"`
	PlannedAt    time.Time  `json:"planned_at"`
	OpenAt       *time.Time `json:"open_at,omitempty"`
	ExpireAt     *time.Time `json:"expire_at,omitempty"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
	ExpiredAt    *time.Time `json:"expired_at,omitempty"`
	CanceledAt   *time.Time `json:"canceled_at,omitempty"`
	AssessmentID *string    `json:"assessment_id,omitempty"`
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

type enrollmentQueryService struct {
	store   EnrollmentQueryStore
	catalog ScaleCatalog
}

func NewEnrollmentQueryService(store EnrollmentQueryStore, catalog ScaleCatalog) EnrollmentQueryService {
	return &enrollmentQueryService{store: store, catalog: catalog}
}

func (s *enrollmentQueryService) ListEnrollments(ctx context.Context, query EnrollmentQuery) (*EnrollmentPage, error) {
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
