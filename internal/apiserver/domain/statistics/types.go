package statistics

import "time"

// StatisticType 统计类型
type StatisticType string

const (
	StatisticTypeSystem        StatisticType = "system"        // 系统整体统计
	StatisticTypeQuestionnaire StatisticType = "questionnaire" // 问卷/量表统计
	StatisticTypeTestee        StatisticType = "testee"        // 受试者统计
	StatisticTypePlan          StatisticType = "plan"          // 计划统计
	StatisticTypeScreening     StatisticType = "screening"     // 筛查项目统计
)

// DailyCount 每日计数
type DailyCount struct {
	Date  time.Time `json:"date"`
	Count int64     `json:"count"`
}

// SystemStatistics 系统整体统计
type SystemStatistics struct {
	OrgID int64 `json:"org_id"`

	// 基础数量统计
	QuestionnaireCount int64 `json:"questionnaire_count"` // 问卷总数
	AnswerSheetCount   int64 `json:"answer_sheet_count"`  // 答卷总数
	TesteeCount        int64 `json:"testee_count"`        // 受试者总数
	AssessmentCount    int64 `json:"assessment_count"`    // 测评总数

	// 状态分布
	AssessmentStatusDistribution map[string]int64 `json:"assessment_status_distribution"` // 按状态统计

	// 今日新增（每日凌晨清零）
	TodayNewAssessments  int64 `json:"today_new_assessments"`   // 今日新增测评
	TodayNewAnswerSheets int64 `json:"today_new_answer_sheets"` // 今日新增答卷
	TodayNewTestees      int64 `json:"today_new_testees"`       // 今日新增受试者

	// 趋势数据（近30天）
	AssessmentTrend []DailyCount `json:"assessment_trend"` // 每日测评数量趋势
}

// QuestionnaireStatistics 问卷/量表统计
type QuestionnaireStatistics struct {
	OrgID             int64  `json:"org_id"`
	QuestionnaireCode string `json:"questionnaire_code"`

	// 基础统计
	TotalSubmissions int64   `json:"total_submissions"` // 总提交数
	TotalCompletions int64   `json:"total_completions"` // 总完成数（已解读）
	CompletionRate   float64 `json:"completion_rate"`   // 完成率 = TotalCompletions / TotalSubmissions

	// 时间维度统计
	Last7DaysCount  int64 `json:"last_7_days_count"`  // 近7天提交数
	Last15DaysCount int64 `json:"last_15_days_count"` // 近15天提交数
	Last30DaysCount int64 `json:"last_30_days_count"` // 近30天提交数

	// 趋势数据（近30天）
	DailyTrend []DailyCount `json:"daily_trend"` // 每日提交趋势

	// 来源分布
	OriginDistribution map[string]int64 `json:"origin_distribution"` // 按来源统计（adhoc/plan/screening）
}

// TesteeStatistics 受试者统计
type TesteeStatistics struct {
	OrgID    int64  `json:"org_id"`
	TesteeID uint64 `json:"testee_id"`

	// 测评统计
	TotalAssessments     int64 `json:"total_assessments"`     // 总测评数
	CompletedAssessments int64 `json:"completed_assessments"` // 已完成测评数
	PendingAssessments   int64 `json:"pending_assessments"`   // 待完成测评数

	// 风险分布
	RiskDistribution map[string]int64 `json:"risk_distribution"` // 按风险等级统计

	// 时间维度
	LastAssessmentDate  *time.Time `json:"last_assessment_date"`  // 最近测评日期
	FirstAssessmentDate *time.Time `json:"first_assessment_date"` // 首次测评日期
}

// PlanStatistics 测评计划统计
type PlanStatistics struct {
	OrgID  int64  `json:"org_id"`
	PlanID uint64 `json:"plan_id"`

	// 任务统计
	TotalTasks     int64 `json:"total_tasks"`     // 总任务数
	CompletedTasks int64 `json:"completed_tasks"` // 已完成任务数
	PendingTasks   int64 `json:"pending_tasks"`   // 待完成任务数
	ExpiredTasks   int64 `json:"expired_tasks"`   // 已过期任务数

	// 完成率
	CompletionRate float64 `json:"completion_rate"` // 完成率 = CompletedTasks / TotalTasks

	// 受试者统计
	EnrolledTestees int64 `json:"enrolled_testees"` // 已加入计划的受试者数
	ActiveTestees   int64 `json:"active_testees"`   // 活跃受试者数（有完成任务的）
}

// ScreeningStatistics 筛查项目统计
type ScreeningStatistics struct {
	OrgID       int64  `json:"org_id"`
	ScreeningID uint64 `json:"screening_id"`

	// 参与统计
	TotalParticipants     int64   `json:"total_participants"`     // 总参与人数
	CompletedParticipants int64   `json:"completed_participants"` // 已完成人数
	ParticipationRate     float64 `json:"participation_rate"`     // 参与率 = TotalParticipants / TargetParticipants

	// 风险分布
	RiskDistribution map[string]int64 `json:"risk_distribution"` // 按风险等级统计

	// 目标人数
	TargetParticipants int64 `json:"target_participants"` // 目标参与人数
}
