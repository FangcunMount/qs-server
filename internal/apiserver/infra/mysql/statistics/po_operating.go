package statistics

import (
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"gorm.io/gorm"
)

type AnalyticsAccessOrgDailyPO struct {
	ID                               uint64         `gorm:"column:id;primaryKey"`
	OrgID                            int64          `gorm:"column:org_id;not null;uniqueIndex:uniq_analytics_access_org_daily"`
	StatDate                         time.Time      `gorm:"column:stat_date;type:date;not null;uniqueIndex:uniq_analytics_access_org_daily"`
	EntryOpenedCount                 int64          `gorm:"column:entry_opened_count;not null;default:0"`
	IntakeConfirmedCount             int64          `gorm:"column:intake_confirmed_count;not null;default:0"`
	TesteeCreatedCount               int64          `gorm:"column:testee_created_count;not null;default:0"`
	CareRelationshipEstablishedCount int64          `gorm:"column:care_relationship_established_count;not null;default:0"`
	CreatedAt                        time.Time      `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt                        time.Time      `gorm:"column:updated_at;autoUpdateTime"`
	DeletedAt                        gorm.DeletedAt `gorm:"column:deleted_at;index"`
}

func (AnalyticsAccessOrgDailyPO) TableName() string { return "analytics_access_org_daily" }

func (p *AnalyticsAccessOrgDailyPO) BeforeCreate(_ *gorm.DB) error {
	if p.ID == 0 {
		p.ID = meta.New().Uint64()
	}
	return nil
}

type AnalyticsAccessClinicianDailyPO struct {
	ID                               uint64         `gorm:"column:id;primaryKey"`
	OrgID                            int64          `gorm:"column:org_id;not null;uniqueIndex:uniq_analytics_access_clinician_daily"`
	ClinicianID                      uint64         `gorm:"column:clinician_id;not null;uniqueIndex:uniq_analytics_access_clinician_daily"`
	StatDate                         time.Time      `gorm:"column:stat_date;type:date;not null;uniqueIndex:uniq_analytics_access_clinician_daily"`
	EntryOpenedCount                 int64          `gorm:"column:entry_opened_count;not null;default:0"`
	IntakeConfirmedCount             int64          `gorm:"column:intake_confirmed_count;not null;default:0"`
	TesteeCreatedCount               int64          `gorm:"column:testee_created_count;not null;default:0"`
	CareRelationshipEstablishedCount int64          `gorm:"column:care_relationship_established_count;not null;default:0"`
	CreatedAt                        time.Time      `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt                        time.Time      `gorm:"column:updated_at;autoUpdateTime"`
	DeletedAt                        gorm.DeletedAt `gorm:"column:deleted_at;index"`
}

func (AnalyticsAccessClinicianDailyPO) TableName() string {
	return "analytics_access_clinician_daily"
}

func (p *AnalyticsAccessClinicianDailyPO) BeforeCreate(_ *gorm.DB) error {
	if p.ID == 0 {
		p.ID = meta.New().Uint64()
	}
	return nil
}

type AnalyticsAccessEntryDailyPO struct {
	ID                               uint64         `gorm:"column:id;primaryKey"`
	OrgID                            int64          `gorm:"column:org_id;not null;uniqueIndex:uniq_analytics_access_entry_daily"`
	EntryID                          uint64         `gorm:"column:entry_id;not null;uniqueIndex:uniq_analytics_access_entry_daily"`
	ClinicianID                      uint64         `gorm:"column:clinician_id;not null;default:0"`
	StatDate                         time.Time      `gorm:"column:stat_date;type:date;not null;uniqueIndex:uniq_analytics_access_entry_daily"`
	EntryOpenedCount                 int64          `gorm:"column:entry_opened_count;not null;default:0"`
	IntakeConfirmedCount             int64          `gorm:"column:intake_confirmed_count;not null;default:0"`
	TesteeCreatedCount               int64          `gorm:"column:testee_created_count;not null;default:0"`
	CareRelationshipEstablishedCount int64          `gorm:"column:care_relationship_established_count;not null;default:0"`
	CreatedAt                        time.Time      `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt                        time.Time      `gorm:"column:updated_at;autoUpdateTime"`
	DeletedAt                        gorm.DeletedAt `gorm:"column:deleted_at;index"`
}

func (AnalyticsAccessEntryDailyPO) TableName() string { return "analytics_access_entry_daily" }

func (p *AnalyticsAccessEntryDailyPO) BeforeCreate(_ *gorm.DB) error {
	if p.ID == 0 {
		p.ID = meta.New().Uint64()
	}
	return nil
}

type AnalyticsAssessmentServiceOrgDailyPO struct {
	ID                        uint64         `gorm:"column:id;primaryKey"`
	OrgID                     int64          `gorm:"column:org_id;not null;uniqueIndex:uniq_analytics_assessment_service_org_daily"`
	StatDate                  time.Time      `gorm:"column:stat_date;type:date;not null;uniqueIndex:uniq_analytics_assessment_service_org_daily"`
	AnswerSheetSubmittedCount int64          `gorm:"column:answersheet_submitted_count;not null;default:0"`
	AssessmentCreatedCount    int64          `gorm:"column:assessment_created_count;not null;default:0"`
	ReportGeneratedCount      int64          `gorm:"column:report_generated_count;not null;default:0"`
	AssessmentFailedCount     int64          `gorm:"column:assessment_failed_count;not null;default:0"`
	CreatedAt                 time.Time      `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt                 time.Time      `gorm:"column:updated_at;autoUpdateTime"`
	DeletedAt                 gorm.DeletedAt `gorm:"column:deleted_at;index"`
}

func (AnalyticsAssessmentServiceOrgDailyPO) TableName() string {
	return "analytics_assessment_service_org_daily"
}

func (p *AnalyticsAssessmentServiceOrgDailyPO) BeforeCreate(_ *gorm.DB) error {
	if p.ID == 0 {
		p.ID = meta.New().Uint64()
	}
	return nil
}

type AnalyticsAssessmentServiceClinicianDailyPO struct {
	ID                        uint64         `gorm:"column:id;primaryKey"`
	OrgID                     int64          `gorm:"column:org_id;not null;uniqueIndex:uniq_analytics_assessment_service_clinician_daily"`
	ClinicianID               uint64         `gorm:"column:clinician_id;not null;uniqueIndex:uniq_analytics_assessment_service_clinician_daily"`
	StatDate                  time.Time      `gorm:"column:stat_date;type:date;not null;uniqueIndex:uniq_analytics_assessment_service_clinician_daily"`
	AnswerSheetSubmittedCount int64          `gorm:"column:answersheet_submitted_count;not null;default:0"`
	AssessmentCreatedCount    int64          `gorm:"column:assessment_created_count;not null;default:0"`
	ReportGeneratedCount      int64          `gorm:"column:report_generated_count;not null;default:0"`
	AssessmentFailedCount     int64          `gorm:"column:assessment_failed_count;not null;default:0"`
	CreatedAt                 time.Time      `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt                 time.Time      `gorm:"column:updated_at;autoUpdateTime"`
	DeletedAt                 gorm.DeletedAt `gorm:"column:deleted_at;index"`
}

func (AnalyticsAssessmentServiceClinicianDailyPO) TableName() string {
	return "analytics_assessment_service_clinician_daily"
}

func (p *AnalyticsAssessmentServiceClinicianDailyPO) BeforeCreate(_ *gorm.DB) error {
	if p.ID == 0 {
		p.ID = meta.New().Uint64()
	}
	return nil
}

type AnalyticsAssessmentServiceEntryDailyPO struct {
	ID                        uint64         `gorm:"column:id;primaryKey"`
	OrgID                     int64          `gorm:"column:org_id;not null;uniqueIndex:uniq_analytics_assessment_service_entry_daily"`
	EntryID                   uint64         `gorm:"column:entry_id;not null;uniqueIndex:uniq_analytics_assessment_service_entry_daily"`
	ClinicianID               uint64         `gorm:"column:clinician_id;not null;default:0"`
	StatDate                  time.Time      `gorm:"column:stat_date;type:date;not null;uniqueIndex:uniq_analytics_assessment_service_entry_daily"`
	AnswerSheetSubmittedCount int64          `gorm:"column:answersheet_submitted_count;not null;default:0"`
	AssessmentCreatedCount    int64          `gorm:"column:assessment_created_count;not null;default:0"`
	ReportGeneratedCount      int64          `gorm:"column:report_generated_count;not null;default:0"`
	AssessmentFailedCount     int64          `gorm:"column:assessment_failed_count;not null;default:0"`
	CreatedAt                 time.Time      `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt                 time.Time      `gorm:"column:updated_at;autoUpdateTime"`
	DeletedAt                 gorm.DeletedAt `gorm:"column:deleted_at;index"`
}

func (AnalyticsAssessmentServiceEntryDailyPO) TableName() string {
	return "analytics_assessment_service_entry_daily"
}

func (p *AnalyticsAssessmentServiceEntryDailyPO) BeforeCreate(_ *gorm.DB) error {
	if p.ID == 0 {
		p.ID = meta.New().Uint64()
	}
	return nil
}

type AnalyticsAssessmentServiceContentDailyPO struct {
	ID                        uint64         `gorm:"column:id;primaryKey"`
	OrgID                     int64          `gorm:"column:org_id;not null;uniqueIndex:uniq_analytics_assessment_service_content_daily"`
	ContentType               string         `gorm:"column:content_type;size:50;not null;uniqueIndex:uniq_analytics_assessment_service_content_daily"`
	ContentCode               string         `gorm:"column:content_code;size:100;not null;uniqueIndex:uniq_analytics_assessment_service_content_daily"`
	StatDate                  time.Time      `gorm:"column:stat_date;type:date;not null;uniqueIndex:uniq_analytics_assessment_service_content_daily"`
	AnswerSheetSubmittedCount int64          `gorm:"column:answersheet_submitted_count;not null;default:0"`
	AssessmentCreatedCount    int64          `gorm:"column:assessment_created_count;not null;default:0"`
	ReportGeneratedCount      int64          `gorm:"column:report_generated_count;not null;default:0"`
	AssessmentFailedCount     int64          `gorm:"column:assessment_failed_count;not null;default:0"`
	CreatedAt                 time.Time      `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt                 time.Time      `gorm:"column:updated_at;autoUpdateTime"`
	DeletedAt                 gorm.DeletedAt `gorm:"column:deleted_at;index"`
}

func (AnalyticsAssessmentServiceContentDailyPO) TableName() string {
	return "analytics_assessment_service_content_daily"
}

func (p *AnalyticsAssessmentServiceContentDailyPO) BeforeCreate(_ *gorm.DB) error {
	if p.ID == 0 {
		p.ID = meta.New().Uint64()
	}
	return nil
}

type AnalyticsPlanTaskDailyPO struct {
	ID                 uint64         `gorm:"column:id;primaryKey"`
	OrgID              int64          `gorm:"column:org_id;not null;uniqueIndex:uniq_analytics_plan_task_daily"`
	PlanID             uint64         `gorm:"column:plan_id;not null;uniqueIndex:uniq_analytics_plan_task_daily"`
	StatDate           time.Time      `gorm:"column:stat_date;type:date;not null;uniqueIndex:uniq_analytics_plan_task_daily"`
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

func (AnalyticsPlanTaskDailyPO) TableName() string { return "analytics_plan_task_daily" }

func (p *AnalyticsPlanTaskDailyPO) BeforeCreate(_ *gorm.DB) error {
	if p.ID == 0 {
		p.ID = meta.New().Uint64()
	}
	return nil
}
