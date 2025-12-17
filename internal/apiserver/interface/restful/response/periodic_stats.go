package response

import "time"

// PeriodicStatsResponse 周期性测评统计响应
type PeriodicStatsResponse struct {
	Projects       []PeriodicProjectResponse `json:"projects"`        // 周期性项目列表
	TotalProjects  int                       `json:"total_projects"`  // 项目总数
	ActiveProjects int                       `json:"active_projects"` // 进行中的项目数
}

// PeriodicProjectResponse 周期性项目响应
type PeriodicProjectResponse struct {
	ProjectID      string                 `json:"project_id"`           // 项目ID
	ProjectName    string                 `json:"project_name"`         // 项目名称
	ScaleName      string                 `json:"scale_name"`           // 关联的量表名称
	TotalWeeks     int                    `json:"total_weeks"`          // 总周数
	CompletedWeeks int                    `json:"completed_weeks"`      // 已完成周数
	CompletionRate float64                `json:"completion_rate"`      // 完成率（0-100）
	CurrentWeek    int                    `json:"current_week"`         // 当前应该完成的周次
	Tasks          []PeriodicTaskResponse `json:"tasks"`                // 各周任务状态（按周次升序排列）
	StartDate      *time.Time             `json:"start_date,omitempty"` // 项目开始日期
	EndDate        *time.Time             `json:"end_date,omitempty"`   // 项目结束日期
}

// PeriodicTaskResponse 周期任务响应
type PeriodicTaskResponse struct {
	Week         int        `json:"week"`                    // 第几周（从1开始）
	Status       string     `json:"status"`                  // 状态：completed/pending/overdue
	CompletedAt  *time.Time `json:"completed_at,omitempty"`  // 完成时间
	DueDate      *time.Time `json:"due_date,omitempty"`      // 截止时间
	AssessmentID *string    `json:"assessment_id,omitempty"` // 关联的测评ID（如已完成）
}
