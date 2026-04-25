package response

import domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"

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
	StartDate      *string                `json:"start_date,omitempty"` // 项目开始日期
	EndDate        *string                `json:"end_date,omitempty"`   // 项目结束日期
}

// PeriodicTaskResponse 周期任务响应
type PeriodicTaskResponse struct {
	Week         int     `json:"week"`                    // 第几周（从1开始）
	Status       string  `json:"status"`                  // 状态：completed/pending/overdue
	StatusLabel  string  `json:"status_label,omitempty"`  // 状态中文
	CompletedAt  *string `json:"completed_at,omitempty"`  // 完成时间
	PlannedAt    *string `json:"planned_at,omitempty"`    // 计划时间
	DueDate      *string `json:"due_date,omitempty"`      // 截止时间
	AssessmentID *string `json:"assessment_id,omitempty"` // 关联的测评ID（如已完成）
}

// NewPeriodicStatsResponse 从统计读模型创建 REST 响应。
func NewPeriodicStatsResponse(stats *domainStatistics.TesteePeriodicStatisticsResponse) *PeriodicStatsResponse {
	if stats == nil {
		return &PeriodicStatsResponse{
			Projects:       []PeriodicProjectResponse{},
			TotalProjects:  0,
			ActiveProjects: 0,
		}
	}

	projects := make([]PeriodicProjectResponse, 0, len(stats.Projects))
	for _, project := range stats.Projects {
		tasks := make([]PeriodicTaskResponse, 0, len(project.Tasks))
		for _, task := range project.Tasks {
			tasks = append(tasks, PeriodicTaskResponse{
				Week:         task.Week,
				Status:       task.Status,
				StatusLabel:  LabelForPeriodicTaskStatus(task.Status),
				CompletedAt:  FormatDateTimePtr(task.CompletedAt),
				PlannedAt:    FormatDateTimePtr(task.PlannedAt),
				DueDate:      FormatDatePtr(task.DueDate),
				AssessmentID: task.AssessmentID,
			})
		}

		projects = append(projects, PeriodicProjectResponse{
			ProjectID:      project.ProjectID,
			ProjectName:    project.ProjectName,
			ScaleName:      project.ScaleName,
			TotalWeeks:     project.TotalWeeks,
			CompletedWeeks: project.CompletedWeeks,
			CompletionRate: project.CompletionRate,
			CurrentWeek:    project.CurrentWeek,
			Tasks:          tasks,
			StartDate:      FormatDatePtr(project.StartDate),
			EndDate:        FormatDatePtr(project.EndDate),
		})
	}

	return &PeriodicStatsResponse{
		Projects:       projects,
		TotalProjects:  stats.TotalProjects,
		ActiveProjects: stats.ActiveProjects,
	}
}
