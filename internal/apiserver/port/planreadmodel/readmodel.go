package planreadmodel

import (
	"context"
	"time"
)

// PageRequest carries normalized pagination inputs for plan read queries.
type PageRequest struct {
	Page     int
	PageSize int
}

// PlanFilter describes plan list filters.
type PlanFilter struct {
	OrgID     int64
	ScaleCode string
	Status    string
}

// TaskFilter describes task list filters.
type TaskFilter struct {
	OrgID                 int64
	PlanID                *uint64
	TesteeID              *uint64
	AccessibleTesteeIDs   []uint64
	RestrictToAccessScope bool
	Status                *string
}

// TaskWindowFilter describes task window filters.
type TaskWindowFilter struct {
	OrgID         int64
	PlanID        uint64
	TesteeIDs     []uint64
	Status        *string
	PlannedBefore *time.Time
}

// PlanRow is the read-side projection of an assessment plan.
type PlanRow struct {
	ID            uint64
	OrgID         int64
	ScaleCode     string
	ScheduleType  string
	TriggerTime   string
	Interval      int
	TotalTimes    int
	FixedDates    []string
	RelativeWeeks []int
	Status        string
}

// TaskRow is the read-side projection of an assessment task.
type TaskRow struct {
	ID           uint64
	PlanID       uint64
	Seq          int
	OrgID        int64
	TesteeID     uint64
	ScaleCode    string
	PlannedAt    time.Time
	OpenAt       *time.Time
	ExpireAt     *time.Time
	CompletedAt  *time.Time
	Status       string
	AssessmentID *uint64
	EntryToken   string
	EntryURL     string
}

// PlanPage carries paged plan rows.
type PlanPage struct {
	Items    []PlanRow
	Total    int64
	Page     int
	PageSize int
}

// TaskPage carries paged task rows.
type TaskPage struct {
	Items    []TaskRow
	Total    int64
	Page     int
	PageSize int
}

// TaskWindow carries task rows plus next-page information.
type TaskWindow struct {
	Items    []TaskRow
	Page     int
	PageSize int
	HasMore  bool
}

// PlanReader exposes plan read-model queries.
type PlanReader interface {
	GetPlan(ctx context.Context, orgID int64, planID uint64) (*PlanRow, error)
	ListPlans(ctx context.Context, filter PlanFilter, page PageRequest) (PlanPage, error)
	ListPlansByTesteeID(ctx context.Context, testeeID uint64) ([]PlanRow, error)
}

// TaskReader exposes task read-model queries.
type TaskReader interface {
	GetTask(ctx context.Context, orgID int64, taskID uint64) (*TaskRow, error)
	ListTasks(ctx context.Context, filter TaskFilter, page PageRequest) (TaskPage, error)
	ListTaskWindow(ctx context.Context, filter TaskWindowFilter, page PageRequest) (TaskWindow, error)
	ListTasksByPlanID(ctx context.Context, planID uint64) ([]TaskRow, error)
	ListTasksByPlanIDAndTesteeIDs(ctx context.Context, planID uint64, testeeIDs []uint64) ([]TaskRow, error)
	ListTasksByTesteeID(ctx context.Context, testeeID uint64) ([]TaskRow, error)
	ListTasksByTesteeIDAndPlanID(ctx context.Context, testeeID uint64, planID uint64) ([]TaskRow, error)
}
