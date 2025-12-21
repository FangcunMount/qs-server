package response

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/application/plan"
)

// ============= Plan Response =============

// PlanResponse 计划响应
type PlanResponse struct {
	ID            string   `json:"id"`                       // 计划ID
	OrgID         int64    `json:"org_id"`                   // 机构ID
	ScaleCode     string   `json:"scale_code"`               // 量表编码（如 "3adyDE"）
	ScheduleType  string   `json:"schedule_type"`            // 周期类型：by_week/by_day/fixed_date/custom
	Interval      int      `json:"interval"`                 // 间隔（周/天，用于 by_week/by_day）
	TotalTimes    int      `json:"total_times"`              // 总次数（用于 by_week/by_day）
	FixedDates    []string `json:"fixed_dates,omitempty"`    // 固定日期列表（用于 fixed_date）
	RelativeWeeks []int    `json:"relative_weeks,omitempty"` // 相对周次列表（用于 custom）
	Status        string   `json:"status"`                   // 状态：active/paused/finished/canceled
}

// TaskResponse 任务响应
type TaskResponse struct {
	ID           string  `json:"id"`                      // 任务ID
	PlanID       string  `json:"plan_id"`                 // 计划ID
	Seq          int     `json:"seq"`                     // 序号（计划内的第N次测评）
	OrgID        int64   `json:"org_id"`                  // 机构ID
	TesteeID     string  `json:"testee_id"`               // 受试者ID
	ScaleCode    string  `json:"scale_code"`              // 量表编码（如 "3adyDE"）
	PlannedAt    string  `json:"planned_at"`              // 计划时间点
	OpenAt       *string `json:"open_at,omitempty"`       // 实际开放时间
	ExpireAt     *string `json:"expire_at,omitempty"`     // 截止时间
	CompletedAt  *string `json:"completed_at,omitempty"`  // 完成时间
	Status       string  `json:"status"`                  // 状态：pending/opened/completed/expired/canceled
	AssessmentID *string `json:"assessment_id,omitempty"` // 关联的测评ID
	EntryToken   string  `json:"entry_token,omitempty"`   // 入口令牌
	EntryURL     string  `json:"entry_url,omitempty"`     // 入口URL
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
		ScaleCode:     result.ScaleCode,
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
		ScaleCode:    result.ScaleCode,
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
