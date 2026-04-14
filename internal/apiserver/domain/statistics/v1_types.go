package statistics

import (
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// TimeRangePreset 统计时间窗口预设。
type TimeRangePreset string

const (
	TimeRangePresetToday TimeRangePreset = "today"
	TimeRangePreset7D    TimeRangePreset = "7d"
	TimeRangePreset30D   TimeRangePreset = "30d"
)

// StatisticsTimeRange 统计时间范围。
type StatisticsTimeRange struct {
	Preset TimeRangePreset `json:"preset"`
	From   time.Time       `json:"from"`
	To     time.Time       `json:"to"`
}

// OrgOverviewSnapshot 机构概览快照。
type OrgOverviewSnapshot struct {
	TesteeCount                int64 `json:"testee_count"`
	ClinicianCount             int64 `json:"clinician_count"`
	ActiveEntryCount           int64 `json:"active_entry_count"`
	AssessmentCount            int64 `json:"assessment_count"`
	InterpretedAssessmentCount int64 `json:"interpreted_assessment_count"`
}

// OrgOverviewWindow 机构概览窗口指标。
type OrgOverviewWindow struct {
	NewTestees               int64 `json:"new_testees"`
	EntryCreatedCount        int64 `json:"entry_created_count"`
	EntryResolvedCount       int64 `json:"entry_resolved_count"`
	EntryIntakeCount         int64 `json:"entry_intake_count"`
	RelationAssignedCount    int64 `json:"relation_assigned_count"`
	AssessmentCreatedCount   int64 `json:"assessment_created_count"`
	AssessmentCompletedCount int64 `json:"assessment_completed_count"`
}

// OrgOverviewTrend 机构概览趋势。
type OrgOverviewTrend struct {
	Assessments []DailyCount `json:"assessments"`
	Intakes     []DailyCount `json:"intakes"`
	Assignments []DailyCount `json:"assignments"`
}

// StatisticsOverview 机构统计概览。
type StatisticsOverview struct {
	OrgID     int64               `json:"org_id"`
	TimeRange StatisticsTimeRange `json:"time_range"`
	Snapshot  OrgOverviewSnapshot `json:"snapshot"`
	Window    OrgOverviewWindow   `json:"window"`
	Trend     OrgOverviewTrend    `json:"trend"`
}

// ClinicianStatisticsSubject 从业者摘要。
type ClinicianStatisticsSubject struct {
	ID            meta.ID  `json:"id"`
	OperatorID    *meta.ID `json:"operator_id,omitempty"`
	Name          string   `json:"name"`
	Department    string   `json:"department,omitempty"`
	Title         string   `json:"title,omitempty"`
	ClinicianType string   `json:"clinician_type"`
	IsActive      bool     `json:"is_active"`
}

// ClinicianStatisticsSnapshot 从业者快照。
type ClinicianStatisticsSnapshot struct {
	PrimaryTesteeCount      int64 `json:"primary_testee_count"`
	AttendingTesteeCount    int64 `json:"attending_testee_count"`
	CollaboratorTesteeCount int64 `json:"collaborator_testee_count"`
	TotalAccessibleTestees  int64 `json:"total_accessible_testees"`
	ActiveEntryCount        int64 `json:"active_entry_count"`
}

// ClinicianStatisticsWindow 从业者窗口指标。
type ClinicianStatisticsWindow struct {
	IntakeCount              int64 `json:"intake_count"`
	AssignedCount            int64 `json:"assigned_count"`
	CompletedAssessmentCount int64 `json:"completed_assessment_count"`
}

// ClinicianStatisticsFunnel 从业者入口漏斗。
type ClinicianStatisticsFunnel struct {
	CreatedCount    int64 `json:"created_count"`
	ResolvedCount   int64 `json:"resolved_count"`
	IntakeCount     int64 `json:"intake_count"`
	AssignedCount   int64 `json:"assigned_count"`
	AssessmentCount int64 `json:"assessment_count"`
}

// ClinicianStatistics 从业者统计。
type ClinicianStatistics struct {
	TimeRange StatisticsTimeRange         `json:"time_range"`
	Clinician ClinicianStatisticsSubject  `json:"clinician"`
	Snapshot  ClinicianStatisticsSnapshot `json:"snapshot"`
	Window    ClinicianStatisticsWindow   `json:"window"`
	Funnel    ClinicianStatisticsFunnel   `json:"funnel"`
}

// AssessmentEntryStatisticsMeta 入口元信息。
type AssessmentEntryStatisticsMeta struct {
	ID            meta.ID    `json:"id"`
	OrgID         int64      `json:"org_id"`
	ClinicianID   meta.ID    `json:"clinician_id"`
	Token         string     `json:"token"`
	TargetType    string     `json:"target_type"`
	TargetCode    string     `json:"target_code"`
	TargetVersion string     `json:"target_version,omitempty"`
	IsActive      bool       `json:"is_active"`
	CreatedAt     time.Time  `json:"created_at"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
	ClinicianName string     `json:"clinician_name,omitempty"`
}

// AssessmentEntryStatisticsCounts 入口漏斗计数。
type AssessmentEntryStatisticsCounts struct {
	ResolveCount    int64 `json:"resolve_count"`
	IntakeCount     int64 `json:"intake_count"`
	AssignedCount   int64 `json:"assigned_count"`
	AssessmentCount int64 `json:"assessment_count"`
}

// AssessmentEntryStatistics 单入口统计。
type AssessmentEntryStatistics struct {
	TimeRange      StatisticsTimeRange             `json:"time_range"`
	Entry          AssessmentEntryStatisticsMeta   `json:"entry"`
	Snapshot       AssessmentEntryStatisticsCounts `json:"snapshot"`
	Window         AssessmentEntryStatisticsCounts `json:"window"`
	LastResolvedAt *time.Time                      `json:"last_resolved_at,omitempty"`
	LastIntakeAt   *time.Time                      `json:"last_intake_at,omitempty"`
}

// AssessmentEntryStatisticsList 入口统计列表。
type AssessmentEntryStatisticsList struct {
	Items      []*AssessmentEntryStatistics `json:"items"`
	Total      int64                        `json:"total"`
	Page       int                          `json:"page"`
	PageSize   int                          `json:"page_size"`
	TotalPages int                          `json:"total_pages"`
}

// ClinicianStatisticsList 从业者统计列表。
type ClinicianStatisticsList struct {
	Items      []*ClinicianStatistics `json:"items"`
	Total      int64                  `json:"total"`
	Page       int                    `json:"page"`
	PageSize   int                    `json:"page_size"`
	TotalPages int                    `json:"total_pages"`
}

// ClinicianTesteeSummaryStatistics clinician 的受试者摘要统计。
type ClinicianTesteeSummaryStatistics struct {
	TimeRange               StatisticsTimeRange `json:"time_range"`
	TotalAccessibleTestees  int64               `json:"total_accessible_testees"`
	PrimaryTesteeCount      int64               `json:"primary_testee_count"`
	AttendingTesteeCount    int64               `json:"attending_testee_count"`
	CollaboratorTesteeCount int64               `json:"collaborator_testee_count"`
	KeyFocusTesteeCount     int64               `json:"key_focus_testee_count"`
	AssessedInWindowCount   int64               `json:"assessed_in_window_count"`
}

// QuestionnaireBatchStatisticsItem 内容最小统计项。
type QuestionnaireBatchStatisticsItem struct {
	Code             string  `json:"code"`
	TotalSubmissions int64   `json:"total_submissions"`
	TotalCompletions int64   `json:"total_completions"`
	CompletionRate   float64 `json:"completion_rate"`
}

// QuestionnaireBatchStatisticsResponse 内容最小统计批量响应。
type QuestionnaireBatchStatisticsResponse struct {
	Items []*QuestionnaireBatchStatisticsItem `json:"items"`
}

// TesteePeriodicTaskStatistics 周期任务统计项。
type TesteePeriodicTaskStatistics struct {
	Week         int        `json:"week"`
	Status       string     `json:"status"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
	PlannedAt    *time.Time `json:"planned_at,omitempty"`
	DueDate      *time.Time `json:"due_date,omitempty"`
	AssessmentID *string    `json:"assessment_id,omitempty"`
}

// TesteePeriodicProjectStatistics 周期项目统计项。
type TesteePeriodicProjectStatistics struct {
	ProjectID      string                         `json:"project_id"`
	ProjectName    string                         `json:"project_name"`
	ScaleName      string                         `json:"scale_name"`
	TotalWeeks     int                            `json:"total_weeks"`
	CompletedWeeks int                            `json:"completed_weeks"`
	CompletionRate float64                        `json:"completion_rate"`
	CurrentWeek    int                            `json:"current_week"`
	Tasks          []TesteePeriodicTaskStatistics `json:"tasks"`
	StartDate      *time.Time                     `json:"start_date,omitempty"`
	EndDate        *time.Time                     `json:"end_date,omitempty"`
}

// TesteePeriodicStatisticsResponse 受试者周期统计响应。
type TesteePeriodicStatisticsResponse struct {
	Projects       []TesteePeriodicProjectStatistics `json:"projects"`
	TotalProjects  int                               `json:"total_projects"`
	ActiveProjects int                               `json:"active_projects"`
}
