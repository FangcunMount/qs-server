package testee

import (
	"context"
	"time"
)

// TesteeStatisticsService 受试者统计服务
// 行为者：管理员、数据分析系统
// 职责：提供受试者的测评数据统计和分析能力
// 变更来源：数据分析和可视化需求变化
type TesteeStatisticsService interface {
	// GetScaleAnalysis 获取受试者的量表趋势分析
	// 场景：查看受试者在各个量表上的历史得分变化
	// 用于绘制趋势图表，分析干预效果
	GetScaleAnalysis(ctx context.Context, testeeID uint64) (*ScaleAnalysisResult, error)

	// GetPeriodicStats 获取受试者参与的周期性测评项目统计
	// 场景：查看受试者在周期性干预项目中的完成进度
	// 用于监控长期干预计划的执行情况
	GetPeriodicStats(ctx context.Context, testeeID uint64) (*PeriodicStatsResult, error)
}

// ============= Statistics DTOs =============

// ScaleAnalysisResult 量表趋势分析结果
type ScaleAnalysisResult struct {
	TesteeID uint64               `json:"testee_id"` // 受试者ID
	Scales   []ScaleTrendAnalysis `json:"scales"`    // 各量表的趋势分析
}

// ScaleTrendAnalysis 量表趋势分析
type ScaleTrendAnalysis struct {
	ScaleID   uint64           `json:"scale_id"`   // 量表ID
	ScaleCode string           `json:"scale_code"` // 量表编码
	ScaleName string           `json:"scale_name"` // 量表名称
	Tests     []TestRecordData `json:"tests"`      // 测评历史记录（按时间升序）
}

// TestRecordData 测评记录数据
type TestRecordData struct {
	AssessmentID uint64            `json:"assessment_id"` // 测评ID
	TestDate     time.Time         `json:"test_date"`     // 测评日期
	TotalScore   float64           `json:"total_score"`   // 总分
	RiskLevel    string            `json:"risk_level"`    // 风险等级
	Result       string            `json:"result"`        // 结果描述
	Factors      []FactorScoreData `json:"factors"`       // 各因子得分
}

// FactorScoreData 因子得分数据
type FactorScoreData struct {
	FactorCode string   `json:"factor_code"`          // 因子编码
	FactorName string   `json:"factor_name"`          // 因子名称
	RawScore   float64  `json:"raw_score"`            // 原始分
	TScore     *float64 `json:"t_score,omitempty"`    // T分
	Percentile *float64 `json:"percentile,omitempty"` // 百分位
	RiskLevel  string   `json:"risk_level,omitempty"` // 风险等级
}

// PeriodicStatsResult 周期性测评统计结果
type PeriodicStatsResult struct {
	TesteeID       uint64                 `json:"testee_id"`       // 受试者ID
	Projects       []PeriodicProjectStats `json:"projects"`        // 周期性项目列表
	TotalProjects  int                    `json:"total_projects"`  // 项目总数
	ActiveProjects int                    `json:"active_projects"` // 进行中的项目数
}

// PeriodicProjectStats 周期性项目统计
type PeriodicProjectStats struct {
	ProjectID      uint64         `json:"project_id"`           // 项目ID
	ProjectName    string         `json:"project_name"`         // 项目名称
	ScaleName      string         `json:"scale_name"`           // 关联的量表名称
	TotalWeeks     int            `json:"total_weeks"`          // 总周数
	CompletedWeeks int            `json:"completed_weeks"`      // 已完成周数
	CompletionRate float64        `json:"completion_rate"`      // 完成率（0-100）
	CurrentWeek    int            `json:"current_week"`         // 当前应该完成的周次
	Tasks          []PeriodicTask `json:"tasks"`                // 各周任务状态
	StartDate      *time.Time     `json:"start_date,omitempty"` // 项目开始日期
	EndDate        *time.Time     `json:"end_date,omitempty"`   // 项目结束日期
}

// PeriodicTask 周期任务
type PeriodicTask struct {
	Week         int        `json:"week"`                    // 第几周（从1开始）
	Status       string     `json:"status"`                  // 状态：completed/pending/overdue
	CompletedAt  *time.Time `json:"completed_at,omitempty"`  // 完成时间
	DueDate      *time.Time `json:"due_date,omitempty"`      // 截止时间
	AssessmentID *uint64    `json:"assessment_id,omitempty"` // 关联的测评ID
}
