package response

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/application/plan"
)

// ============= Plan Response =============

// PlanResponse 计划响应
type PlanResponse struct {
	ID            string   `json:"id"`
	OrgID         int64    `json:"org_id"`
	ScaleID       string   `json:"scale_id"`
	ScheduleType  string   `json:"schedule_type"`
	Interval      int      `json:"interval"`
	TotalTimes    int      `json:"total_times"`
	FixedDates    []string `json:"fixed_dates,omitempty"`
	RelativeWeeks []int    `json:"relative_weeks,omitempty"`
	Status        string   `json:"status"`
}

// TaskResponse 任务响应
type TaskResponse struct {
	ID           string  `json:"id"`
	PlanID       string  `json:"plan_id"`
	Seq          int     `json:"seq"`
	OrgID        int64   `json:"org_id"`
	TesteeID     string  `json:"testee_id"`
	ScaleID      string  `json:"scale_id"`
	PlannedAt    string  `json:"planned_at"`
	OpenAt       *string `json:"open_at,omitempty"`
	ExpireAt     *string `json:"expire_at,omitempty"`
	CompletedAt  *string `json:"completed_at,omitempty"`
	Status       string  `json:"status"`
	AssessmentID *string `json:"assessment_id,omitempty"`
	EntryToken   string  `json:"entry_token,omitempty"`
	EntryURL     string  `json:"entry_url,omitempty"`
}

// EnrollmentResponse 加入计划响应
type EnrollmentResponse struct {
	PlanID string         `json:"plan_id"`
	Tasks  []TaskResponse `json:"tasks"`
}

// PlanListResponse 计划列表响应
type PlanListResponse struct {
	Plans      []PlanResponse `json:"plans"`
	TotalCount int64          `json:"total_count"`
	Page       int            `json:"page"`
	PageSize   int            `json:"page_size"`
}

// TaskListResponse 任务列表响应
type TaskListResponse struct {
	Tasks      []TaskResponse `json:"tasks"`
	TotalCount int64          `json:"total_count"`
	Page       int            `json:"page"`
	PageSize   int            `json:"page_size"`
}

// ============= Converters =============

// NewPlanResponse 从 PlanResult 创建 PlanResponse
func NewPlanResponse(result *plan.PlanResult) *PlanResponse {
	if result == nil {
		return nil
	}

	return &PlanResponse{
		ID:            result.ID,
		OrgID:         result.OrgID,
		ScaleID:       result.ScaleID,
		ScheduleType:  result.ScheduleType,
		Interval:      result.Interval,
		TotalTimes:    result.TotalTimes,
		FixedDates:    result.FixedDates,
		RelativeWeeks: result.RelativeWeeks,
		Status:        result.Status,
	}
}

// NewTaskResponse 从 TaskResult 创建 TaskResponse
func NewTaskResponse(result *plan.TaskResult) *TaskResponse {
	if result == nil {
		return nil
	}

	return &TaskResponse{
		ID:           result.ID,
		PlanID:       result.PlanID,
		Seq:          result.Seq,
		OrgID:        result.OrgID,
		TesteeID:     result.TesteeID,
		ScaleID:      result.ScaleID,
		PlannedAt:    result.PlannedAt,
		OpenAt:       result.OpenAt,
		ExpireAt:     result.ExpireAt,
		CompletedAt:  result.CompletedAt,
		Status:       result.Status,
		AssessmentID: result.AssessmentID,
		EntryToken:   result.EntryToken,
		EntryURL:     result.EntryURL,
	}
}

// NewEnrollmentResponse 从 EnrollmentResult 创建 EnrollmentResponse
func NewEnrollmentResponse(result *plan.EnrollmentResult) *EnrollmentResponse {
	if result == nil {
		return nil
	}

	tasks := make([]TaskResponse, 0, len(result.Tasks))
	for _, task := range result.Tasks {
		if resp := NewTaskResponse(task); resp != nil {
			tasks = append(tasks, *resp)
		}
	}

	return &EnrollmentResponse{
		PlanID: result.PlanID,
		Tasks:  tasks,
	}
}

// NewPlanListResponse 从 PlanListResult 创建 PlanListResponse
func NewPlanListResponse(result *plan.PlanListResult) *PlanListResponse {
	if result == nil {
		return &PlanListResponse{
			Plans:      []PlanResponse{},
			TotalCount: 0,
			Page:       1,
			PageSize:   10,
		}
	}

	plans := make([]PlanResponse, 0, len(result.Items))
	for _, item := range result.Items {
		if resp := NewPlanResponse(item); resp != nil {
			plans = append(plans, *resp)
		}
	}

	return &PlanListResponse{
		Plans:      plans,
		TotalCount: result.Total,
		Page:       result.Page,
		PageSize:   result.PageSize,
	}
}

// NewTaskListResponse 从 TaskListResult 创建 TaskListResponse
func NewTaskListResponse(result *plan.TaskListResult) *TaskListResponse {
	if result == nil {
		return &TaskListResponse{
			Tasks:      []TaskResponse{},
			TotalCount: 0,
			Page:       1,
			PageSize:   10,
		}
	}

	tasks := make([]TaskResponse, 0, len(result.Items))
	for _, item := range result.Items {
		if resp := NewTaskResponse(item); resp != nil {
			tasks = append(tasks, *resp)
		}
	}

	return &TaskListResponse{
		Tasks:      tasks,
		TotalCount: result.Total,
		Page:       result.Page,
		PageSize:   result.PageSize,
	}
}

// NewTaskListResponseFromSlice 从 TaskResult 切片创建 TaskListResponse
func NewTaskListResponseFromSlice(tasks []*plan.TaskResult) *TaskListResponse {
	taskResponses := make([]TaskResponse, 0, len(tasks))
	for _, task := range tasks {
		if resp := NewTaskResponse(task); resp != nil {
			taskResponses = append(taskResponses, *resp)
		}
	}

	return &TaskListResponse{
		Tasks:      taskResponses,
		TotalCount: int64(len(taskResponses)),
		Page:       1,
		PageSize:   len(taskResponses),
	}
}
