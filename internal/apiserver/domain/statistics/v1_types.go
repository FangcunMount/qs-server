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
// 注意：
// - EntryCreatedCount 属于供给侧资源指标，表示这段时间新发布了多少入口，不属于 C 端旅程节点。
// - 其余 entry/intake/assigned/assessment 字段属于接入行为和测评服务过程指标。
type OrgOverviewWindow struct {
	NewTestees               int64 `json:"new_testees"`                // 入口旅程里新建的受试者人数
	EntryCreatedCount        int64 `json:"entry_created_count"`        // 供给侧：新建入口数
	EntryResolvedCount       int64 `json:"entry_resolved_count"`       // 旅程侧：被用户成功打开的入口次数
	EntryIntakeCount         int64 `json:"entry_intake_count"`         // 旅程侧：完成 intake 的次数
	RelationAssignedCount    int64 `json:"relation_assigned_count"`    // 旅程侧：建立照护关系的唯一受试者数
	AssessmentCreatedCount   int64 `json:"assessment_created_count"`   // 旅程侧：结果就绪后形成测评的次数
	AssessmentCompletedCount int64 `json:"assessment_completed_count"` // 旅程侧：结果就绪的次数
}

// OrgOverviewTrend 机构概览趋势。
type OrgOverviewTrend struct {
	Assessments []DailyCount `json:"assessments"`
	Intakes     []DailyCount `json:"intakes"`
	Assignments []DailyCount `json:"assignments"`
}

// OrganizationOverview 机构资源总览。
type OrganizationOverview struct {
	TesteeCount      int64 `json:"testee_count"`
	ClinicianCount   int64 `json:"clinician_count"`
	ActiveEntryCount int64 `json:"active_entry_count"`
	AssessmentCount  int64 `json:"assessment_count"`
	ReportCount      int64 `json:"report_count"`
}

// AccessFunnelWindow 接入漏斗窗口指标。
type AccessFunnelWindow struct {
	EntryOpenedCount                 int64 `json:"entry_opened_count"`
	IntakeConfirmedCount             int64 `json:"intake_confirmed_count"`
	TesteeCreatedCount               int64 `json:"testee_created_count"`
	CareRelationshipEstablishedCount int64 `json:"care_relationship_established_count"`
}

// AccessFunnelTrend 接入漏斗趋势。
type AccessFunnelTrend struct {
	EntryOpened                 []DailyCount `json:"entry_opened"`
	IntakeConfirmed             []DailyCount `json:"intake_confirmed"`
	TesteeCreated               []DailyCount `json:"testee_created"`
	CareRelationshipEstablished []DailyCount `json:"care_relationship_established"`
}

// AccessFunnelStatistics 接入漏斗统计域。
type AccessFunnelStatistics struct {
	Window AccessFunnelWindow `json:"window"`
	Trend  AccessFunnelTrend  `json:"trend"`
}

// AssessmentServiceWindow 测评服务交付窗口指标。
type AssessmentServiceWindow struct {
	AnswerSheetSubmittedCount int64 `json:"answersheet_submitted_count"`
	AssessmentCreatedCount    int64 `json:"assessment_created_count"`
	ReportGeneratedCount      int64 `json:"report_generated_count"`
	AssessmentFailedCount     int64 `json:"assessment_failed_count"`
}

// AssessmentServiceTrend 测评服务交付趋势。
type AssessmentServiceTrend struct {
	AnswerSheetSubmitted []DailyCount `json:"answersheet_submitted"`
	AssessmentCreated    []DailyCount `json:"assessment_created"`
	ReportGenerated      []DailyCount `json:"report_generated"`
	AssessmentFailed     []DailyCount `json:"assessment_failed"`
}

// AssessmentServiceStatistics 测评服务交付统计域。
type AssessmentServiceStatistics struct {
	Window AssessmentServiceWindow `json:"window"`
	Trend  AssessmentServiceTrend  `json:"trend"`
}

// DimensionAnalysisSummary 维度分析入口摘要。
type DimensionAnalysisSummary struct {
	ClinicianCount int64 `json:"clinician_count"`
	EntryCount     int64 `json:"entry_count"`
	ContentCount   int64 `json:"content_count"`
}

// PlanTaskWindow 计划任务窗口指标。
type PlanTaskWindow struct {
	TaskCreatedCount   int64 `json:"task_created_count"`
	TaskOpenedCount    int64 `json:"task_opened_count"`
	TaskCompletedCount int64 `json:"task_completed_count"`
	TaskExpiredCount   int64 `json:"task_expired_count"`
	EnrolledTestees    int64 `json:"enrolled_testees"`
	ActiveTestees      int64 `json:"active_testees"`
}

// PlanTaskTrend 计划任务趋势。
type PlanTaskTrend struct {
	TaskCreated   []DailyCount `json:"task_created"`
	TaskOpened    []DailyCount `json:"task_opened"`
	TaskCompleted []DailyCount `json:"task_completed"`
	TaskExpired   []DailyCount `json:"task_expired"`
}

// PlanDomainStatistics 计划统计域。
type PlanDomainStatistics struct {
	Window PlanTaskWindow `json:"window"`
	Trend  PlanTaskTrend  `json:"trend"`
}

// StatisticsOverview 机构统计概览。
type StatisticsOverview struct {
	OrgID                int64                       `json:"org_id"`
	TimeRange            StatisticsTimeRange         `json:"time_range"`
	OrganizationOverview OrganizationOverview        `json:"organization_overview"`
	AccessFunnel         AccessFunnelStatistics      `json:"access_funnel"`
	AssessmentService    AssessmentServiceStatistics `json:"assessment_service"`
	DimensionAnalysis    DimensionAnalysisSummary    `json:"dimension_analysis"`
	Plan                 PlanDomainStatistics        `json:"plan"`
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
// 这里全部是接入行为和测评服务过程指标，不含供给侧入口创建量。
type ClinicianStatisticsWindow struct {
	IntakeCount              int64 `json:"intake_count"`
	AssignedCount            int64 `json:"assigned_count"`
	CompletedAssessmentCount int64 `json:"completed_assessment_count"`
}

// ClinicianStatisticsFunnel 从业者入口漏斗。
// CreatedCount 是供给侧资源量，其余节点是接入行为和测评服务过程量。
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

// AssessmentEntryStatisticsCounts 单入口旅程计数。
// 这里不再表达供给侧“入口已发布”，只表达用户打开入口之后的旅程。
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
