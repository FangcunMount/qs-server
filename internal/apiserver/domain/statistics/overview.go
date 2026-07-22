package statistics

// OrganizationOverview contains current organization resource counts and
// cumulative delivery counts used by the Statistics overview projection.
type OrganizationOverview struct {
	TesteeCount                int64 `json:"testee_count"`
	ClinicianCount             int64 `json:"clinician_count"`
	ActiveEntryCount           int64 `json:"active_entry_count"`
	AssessmentCount            int64 `json:"assessment_count"`
	ReportCount                int64 `json:"report_count"`
	ContentCount               int64 `json:"content_count"`
	AnswerSheetSubmissionCount int64 `json:"answer_sheet_submission_count"`
}

type AccessFunnelWindow struct {
	EntryOpenedCount                 int64 `json:"entry_opened_count"`
	IntakeConfirmedCount             int64 `json:"intake_confirmed_count"`
	TesteeCreatedCount               int64 `json:"testee_created_count"`
	CareRelationshipEstablishedCount int64 `json:"care_relationship_established_count"`
}

type AccessFunnelTrend struct {
	EntryOpened                 []DailyCount `json:"entry_opened"`
	IntakeConfirmed             []DailyCount `json:"intake_confirmed"`
	TesteeCreated               []DailyCount `json:"testee_created"`
	CareRelationshipEstablished []DailyCount `json:"care_relationship_established"`
}

type AccessFunnelStatistics struct {
	Window AccessFunnelWindow `json:"window"`
	Trend  AccessFunnelTrend  `json:"trend"`
}

type AssessmentServiceWindow struct {
	AnswerSheetSubmittedCount int64 `json:"answersheet_submitted_count"`
	AssessmentCreatedCount    int64 `json:"assessment_created_count"`
	ReportGeneratedCount      int64 `json:"report_generated_count"`
	AssessmentFailedCount     int64 `json:"assessment_failed_count"`
}

type AssessmentServiceTrend struct {
	AnswerSheetSubmitted []DailyCount `json:"answersheet_submitted"`
	AssessmentCreated    []DailyCount `json:"assessment_created"`
	ReportGenerated      []DailyCount `json:"report_generated"`
	AssessmentFailed     []DailyCount `json:"assessment_failed"`
}

type AssessmentServiceStatistics struct {
	Window AssessmentServiceWindow `json:"window"`
	Trend  AssessmentServiceTrend  `json:"trend"`
}

type DimensionAnalysisSummary struct {
	ClinicianCount int64 `json:"clinician_count"`
	EntryCount     int64 `json:"entry_count"`
	ContentCount   int64 `json:"content_count"`
}

type PlanTaskActivityWindow struct {
	TaskCreatedCount   int64 `json:"task_created_count"`
	TaskOpenedCount    int64 `json:"task_opened_count"`
	TaskCompletedCount int64 `json:"task_completed_count"`
	TaskExpiredCount   int64 `json:"task_expired_count"`
	EnrolledTestees    int64 `json:"enrolled_testees"`
	ActiveTestees      int64 `json:"active_testees"`
}

type PlanTaskActivityTrend struct {
	TaskCreated   []DailyCount `json:"task_created"`
	TaskOpened    []DailyCount `json:"task_opened"`
	TaskCompleted []DailyCount `json:"task_completed"`
	TaskExpired   []DailyCount `json:"task_expired"`
}

type PlanTaskActivityStatistics struct {
	Window PlanTaskActivityWindow `json:"window"`
	Trend  PlanTaskActivityTrend  `json:"trend"`
}

type PlanTaskFulfillmentWindow struct {
	PlannedTaskCount     int64   `json:"planned_task_count"`
	DueTaskCount         int64   `json:"due_task_count"`
	CompletedTaskCount   int64   `json:"completed_task_count"`
	OnTimeCompletedCount int64   `json:"on_time_completed_count"`
	OverdueTaskCount     int64   `json:"overdue_task_count"`
	CompletionRate       float64 `json:"completion_rate"`
	OnTimeCompletionRate float64 `json:"on_time_completion_rate"`
}

type PlanTaskFulfillmentTrend struct {
	Planned   []DailyCount `json:"planned"`
	Due       []DailyCount `json:"due"`
	Completed []DailyCount `json:"completed"`
	Overdue   []DailyCount `json:"overdue"`
}

type PlanTaskFulfillmentStatistics struct {
	Window PlanTaskFulfillmentWindow `json:"window"`
	Trend  PlanTaskFulfillmentTrend  `json:"trend"`
}

type PlanDomainStatistics struct {
	Activity    PlanTaskActivityStatistics    `json:"activity"`
	Fulfillment PlanTaskFulfillmentStatistics `json:"fulfillment"`
}
