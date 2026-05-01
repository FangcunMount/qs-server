package statistics

import (
	"database/sql/driver"
	"encoding/json"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"gorm.io/gorm"
)

const (
	StatisticsJourneySubjectOrg       = "org"
	StatisticsJourneySubjectClinician = "clinician"
	StatisticsJourneySubjectEntry     = "entry"

	StatisticsContentTypeQuestionnaire = "questionnaire"
	StatisticsContentTypeScale         = "scale"
)

// StatisticsJourneyDailyPO 统一承载机构、医生、入口维度的行为旅程日聚合。
type StatisticsJourneyDailyPO struct {
	ID          uint64         `gorm:"column:id;primaryKey"`
	OrgID       int64          `gorm:"column:org_id;not null;uniqueIndex:uniq_statistics_journey_daily,priority:1;index:idx_statistics_journey_org_date,priority:1"`
	SubjectType string         `gorm:"column:subject_type;size:32;not null;uniqueIndex:uniq_statistics_journey_daily,priority:2"`
	SubjectID   uint64         `gorm:"column:subject_id;not null;default:0;uniqueIndex:uniq_statistics_journey_daily,priority:3"`
	ClinicianID uint64         `gorm:"column:clinician_id;not null;default:0;index:idx_statistics_journey_clinician_date,priority:2"`
	EntryID     uint64         `gorm:"column:entry_id;not null;default:0;index:idx_statistics_journey_entry_date,priority:2"`
	StatDate    time.Time      `gorm:"column:stat_date;type:date;not null;uniqueIndex:uniq_statistics_journey_daily,priority:4;index:idx_statistics_journey_org_date,priority:2;index:idx_statistics_journey_clinician_date,priority:3;index:idx_statistics_journey_entry_date,priority:3"`
	CreatedAt   time.Time      `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt   time.Time      `gorm:"column:updated_at;autoUpdateTime"`
	DeletedAt   gorm.DeletedAt `gorm:"column:deleted_at;index"`

	EntryOpenedCount                 int64 `gorm:"column:entry_opened_count;not null;default:0"`
	IntakeConfirmedCount             int64 `gorm:"column:intake_confirmed_count;not null;default:0"`
	TesteeProfileCreatedCount        int64 `gorm:"column:testee_profile_created_count;not null;default:0"`
	CareRelationshipEstablishedCount int64 `gorm:"column:care_relationship_established_count;not null;default:0"`
	CareRelationshipTransferredCount int64 `gorm:"column:care_relationship_transferred_count;not null;default:0"`
	AnswerSheetSubmittedCount        int64 `gorm:"column:answersheet_submitted_count;not null;default:0"`
	AssessmentCreatedCount           int64 `gorm:"column:assessment_created_count;not null;default:0"`
	ReportGeneratedCount             int64 `gorm:"column:report_generated_count;not null;default:0"`
	EpisodeCompletedCount            int64 `gorm:"column:episode_completed_count;not null;default:0"`
	EpisodeFailedCount               int64 `gorm:"column:episode_failed_count;not null;default:0"`
	AssessmentFailedCount            int64 `gorm:"column:assessment_failed_count;not null;default:0"`

	AccessEntryOpenedCount                 int64 `gorm:"column:access_entry_opened_count;not null;default:0"`
	AccessIntakeConfirmedCount             int64 `gorm:"column:access_intake_confirmed_count;not null;default:0"`
	AccessTesteeCreatedCount               int64 `gorm:"column:access_testee_created_count;not null;default:0"`
	AccessCareRelationshipEstablishedCount int64 `gorm:"column:access_care_relationship_established_count;not null;default:0"`

	ServiceAnswerSheetSubmittedCount int64 `gorm:"column:service_answersheet_submitted_count;not null;default:0"`
	ServiceAssessmentCreatedCount    int64 `gorm:"column:service_assessment_created_count;not null;default:0"`
	ServiceReportGeneratedCount      int64 `gorm:"column:service_report_generated_count;not null;default:0"`
	ServiceAssessmentFailedCount     int64 `gorm:"column:service_assessment_failed_count;not null;default:0"`
}

func (StatisticsJourneyDailyPO) TableName() string { return "statistics_journey_daily" }

func (p *StatisticsJourneyDailyPO) BeforeCreate(_ *gorm.DB) error {
	if p.ID == 0 {
		p.ID = meta.New().Uint64()
	}
	return nil
}

// StatisticsContentDailyPO 承载问卷、量表等内容维度的日聚合。
type StatisticsContentDailyPO struct {
	ID          uint64         `gorm:"column:id;primaryKey"`
	OrgID       int64          `gorm:"column:org_id;not null;uniqueIndex:uniq_statistics_content_daily,priority:1;index:idx_statistics_content_org_date,priority:1"`
	ContentType string         `gorm:"column:content_type;size:50;not null;uniqueIndex:uniq_statistics_content_daily,priority:2"`
	ContentCode string         `gorm:"column:content_code;size:100;not null;uniqueIndex:uniq_statistics_content_daily,priority:3"`
	OriginType  string         `gorm:"column:origin_type;size:50;not null;default:'';uniqueIndex:uniq_statistics_content_daily,priority:4"`
	StatDate    time.Time      `gorm:"column:stat_date;type:date;not null;uniqueIndex:uniq_statistics_content_daily,priority:5;index:idx_statistics_content_org_date,priority:2"`
	CreatedAt   time.Time      `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt   time.Time      `gorm:"column:updated_at;autoUpdateTime"`
	DeletedAt   gorm.DeletedAt `gorm:"column:deleted_at;index"`

	SubmissionCount           int64 `gorm:"column:submission_count;not null;default:0"`
	CompletionCount           int64 `gorm:"column:completion_count;not null;default:0"`
	AnswerSheetSubmittedCount int64 `gorm:"column:answersheet_submitted_count;not null;default:0"`
	AssessmentCreatedCount    int64 `gorm:"column:assessment_created_count;not null;default:0"`
	ReportGeneratedCount      int64 `gorm:"column:report_generated_count;not null;default:0"`
	AssessmentFailedCount     int64 `gorm:"column:assessment_failed_count;not null;default:0"`
}

func (StatisticsContentDailyPO) TableName() string { return "statistics_content_daily" }

func (p *StatisticsContentDailyPO) BeforeCreate(_ *gorm.DB) error {
	if p.ID == 0 {
		p.ID = meta.New().Uint64()
	}
	return nil
}

// StatisticsPlanDailyPO 承载计划任务日聚合。
type StatisticsPlanDailyPO struct {
	ID                 uint64         `gorm:"column:id;primaryKey"`
	OrgID              int64          `gorm:"column:org_id;not null;uniqueIndex:uniq_statistics_plan_daily,priority:1;index:idx_statistics_plan_org_date,priority:1"`
	PlanID             uint64         `gorm:"column:plan_id;not null;uniqueIndex:uniq_statistics_plan_daily,priority:2"`
	StatDate           time.Time      `gorm:"column:stat_date;type:date;not null;uniqueIndex:uniq_statistics_plan_daily,priority:3;index:idx_statistics_plan_org_date,priority:2"`
	TaskCreatedCount   int64          `gorm:"column:task_created_count;not null;default:0"`
	TaskOpenedCount    int64          `gorm:"column:task_opened_count;not null;default:0"`
	TaskCompletedCount int64          `gorm:"column:task_completed_count;not null;default:0"`
	TaskExpiredCount   int64          `gorm:"column:task_expired_count;not null;default:0"`
	EnrolledTestees    int64          `gorm:"column:enrolled_testees;not null;default:0"`
	ActiveTestees      int64          `gorm:"column:active_testees;not null;default:0"`
	CreatedAt          time.Time      `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt          time.Time      `gorm:"column:updated_at;autoUpdateTime"`
	DeletedAt          gorm.DeletedAt `gorm:"column:deleted_at;index"`
}

func (StatisticsPlanDailyPO) TableName() string { return "statistics_plan_daily" }

func (p *StatisticsPlanDailyPO) BeforeCreate(_ *gorm.DB) error {
	if p.ID == 0 {
		p.ID = meta.New().Uint64()
	}
	return nil
}

// StatisticsOrgSnapshotPO 承载机构级总览快照。
type StatisticsOrgSnapshotPO struct {
	ID                      uint64         `gorm:"column:id;primaryKey"`
	OrgID                   int64          `gorm:"column:org_id;not null;uniqueIndex:uniq_statistics_org_snapshot_org"`
	TesteeCount             int64          `gorm:"column:testee_count;not null;default:0"`
	ClinicianCount          int64          `gorm:"column:clinician_count;not null;default:0"`
	ActiveEntryCount        int64          `gorm:"column:active_entry_count;not null;default:0"`
	AssessmentCount         int64          `gorm:"column:assessment_count;not null;default:0"`
	ReportCount             int64          `gorm:"column:report_count;not null;default:0"`
	DimensionClinicianCount int64          `gorm:"column:dimension_clinician_count;not null;default:0"`
	DimensionEntryCount     int64          `gorm:"column:dimension_entry_count;not null;default:0"`
	DimensionContentCount   int64          `gorm:"column:dimension_content_count;not null;default:0"`
	SnapshotAt              time.Time      `gorm:"column:snapshot_at;not null"`
	CreatedAt               time.Time      `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt               time.Time      `gorm:"column:updated_at;autoUpdateTime"`
	DeletedAt               gorm.DeletedAt `gorm:"column:deleted_at;index"`
}

func (StatisticsOrgSnapshotPO) TableName() string { return "statistics_org_snapshot" }

func (p *StatisticsOrgSnapshotPO) BeforeCreate(_ *gorm.DB) error {
	if p.ID == 0 {
		p.ID = meta.New().Uint64()
	}
	return nil
}

// AssessmentEntryResolveLogPO 入口解析日志。
type AssessmentEntryResolveLogPO struct {
	ID          uint64         `gorm:"column:id;primaryKey"`
	OrgID       int64          `gorm:"column:org_id;not null;index:idx_entry_resolve_org_entry_time,priority:1"`
	ClinicianID uint64         `gorm:"column:clinician_id;not null;index:idx_entry_resolve_clinician_time,priority:1"`
	EntryID     uint64         `gorm:"column:entry_id;not null;index:idx_entry_resolve_org_entry_time,priority:2"`
	ResolvedAt  time.Time      `gorm:"column:resolved_at;not null;index:idx_entry_resolve_org_entry_time,priority:3;index:idx_entry_resolve_clinician_time,priority:2"`
	CreatedAt   time.Time      `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt   time.Time      `gorm:"column:updated_at;autoUpdateTime"`
	DeletedAt   gorm.DeletedAt `gorm:"column:deleted_at;index"`
}

// TableName 指定表名。
func (AssessmentEntryResolveLogPO) TableName() string {
	return "assessment_entry_resolve_log"
}

// BeforeCreate GORM hook。
func (p *AssessmentEntryResolveLogPO) BeforeCreate(_ *gorm.DB) error {
	if p.ID == 0 {
		p.ID = meta.New().Uint64()
	}
	return nil
}

// AssessmentEntryIntakeLogPO 入口 intake 成功日志。
type AssessmentEntryIntakeLogPO struct {
	ID                uint64         `gorm:"column:id;primaryKey"`
	OrgID             int64          `gorm:"column:org_id;not null;index:idx_entry_intake_org_entry_time,priority:1;index:idx_entry_intake_org_testee_time,priority:1"`
	ClinicianID       uint64         `gorm:"column:clinician_id;not null;index:idx_entry_intake_clinician_time,priority:1"`
	EntryID           uint64         `gorm:"column:entry_id;not null;index:idx_entry_intake_org_entry_time,priority:2"`
	TesteeID          uint64         `gorm:"column:testee_id;not null;index:idx_entry_intake_org_testee_time,priority:2"`
	TesteeCreated     bool           `gorm:"column:testee_created;not null;default:false"`
	AssignmentCreated bool           `gorm:"column:assignment_created;not null;default:false"`
	IntakeAt          time.Time      `gorm:"column:intake_at;not null;index:idx_entry_intake_org_entry_time,priority:3;index:idx_entry_intake_clinician_time,priority:2;index:idx_entry_intake_org_testee_time,priority:3"`
	CreatedAt         time.Time      `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt         time.Time      `gorm:"column:updated_at;autoUpdateTime"`
	DeletedAt         gorm.DeletedAt `gorm:"column:deleted_at;index"`
}

// TableName 指定表名。
func (AssessmentEntryIntakeLogPO) TableName() string {
	return "assessment_entry_intake_log"
}

// BeforeCreate GORM hook。
func (p *AssessmentEntryIntakeLogPO) BeforeCreate(_ *gorm.DB) error {
	if p.ID == 0 {
		p.ID = meta.New().Uint64()
	}
	return nil
}

// ==================== JSONField 辅助类型 ====================

// JSONField JSON字段类型
type JSONField map[string]interface{}

// Value 实现 driver.Valuer 接口
func (j JSONField) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

// Scan 实现 sql.Scanner 接口
func (j *JSONField) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}

	bytes, ok := value.([]byte)
	if !ok {
		return json.Unmarshal([]byte(value.(string)), j)
	}

	return json.Unmarshal(bytes, j)
}
