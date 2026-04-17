package statistics

import (
	"time"

	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"gorm.io/gorm"
)

// BehaviorFootprintPO 行为足迹事件。
type BehaviorFootprintPO struct {
	ID                string         `gorm:"column:id;size:128;primaryKey"`
	OrgID             int64          `gorm:"column:org_id;not null;index:idx_bf_org_testee_event_del_time,priority:1;index:idx_bf_org_entry_event_del_time,priority:1;index:idx_bf_org_answersheet_event_del,priority:1;index:idx_bf_org_assessment_event_del,priority:1"`
	SubjectType       string         `gorm:"column:subject_type;size:64;not null"`
	SubjectID         uint64         `gorm:"column:subject_id;not null;default:0"`
	ActorType         string         `gorm:"column:actor_type;size:64;not null"`
	ActorID           uint64         `gorm:"column:actor_id;not null;default:0"`
	EntryID           uint64         `gorm:"column:entry_id;not null;default:0;index:idx_bf_org_entry_event_del_time,priority:2"`
	ClinicianID       uint64         `gorm:"column:clinician_id;not null;default:0"`
	SourceClinicianID uint64         `gorm:"column:source_clinician_id;not null;default:0"`
	TesteeID          uint64         `gorm:"column:testee_id;not null;default:0;index:idx_bf_org_testee_event_del_time,priority:2"`
	AnswerSheetID     uint64         `gorm:"column:answersheet_id;not null;default:0;index:idx_bf_org_answersheet_event_del,priority:2"`
	AssessmentID      uint64         `gorm:"column:assessment_id;not null;default:0;index:idx_bf_org_assessment_event_del,priority:2"`
	ReportID          uint64         `gorm:"column:report_id;not null;default:0"`
	EventName         string         `gorm:"column:event_name;size:64;not null;index:idx_bf_org_testee_event_del_time,priority:3;index:idx_bf_org_entry_event_del_time,priority:3;index:idx_bf_org_answersheet_event_del,priority:3;index:idx_bf_org_assessment_event_del,priority:3"`
	OccurredAt        time.Time      `gorm:"column:occurred_at;not null;index:idx_bf_org_testee_event_del_time,priority:5;index:idx_bf_org_entry_event_del_time,priority:5"`
	Properties        JSONField      `gorm:"column:properties_json;type:json"`
	CreatedAt         time.Time      `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt         time.Time      `gorm:"column:updated_at;autoUpdateTime"`
	DeletedAt         gorm.DeletedAt `gorm:"column:deleted_at;index:idx_bf_org_testee_event_del_time,priority:4;index:idx_bf_org_entry_event_del_time,priority:4;index:idx_bf_org_answersheet_event_del,priority:4;index:idx_bf_org_assessment_event_del,priority:4"`
}

func (BehaviorFootprintPO) TableName() string { return "behavior_footprint" }

func behaviorFootprintToDomain(po *BehaviorFootprintPO) *domainStatistics.BehaviorFootprint {
	if po == nil {
		return nil
	}
	return &domainStatistics.BehaviorFootprint{
		ID:                po.ID,
		OrgID:             po.OrgID,
		SubjectType:       po.SubjectType,
		SubjectID:         po.SubjectID,
		ActorType:         po.ActorType,
		ActorID:           po.ActorID,
		EntryID:           po.EntryID,
		ClinicianID:       po.ClinicianID,
		SourceClinicianID: po.SourceClinicianID,
		TesteeID:          po.TesteeID,
		AnswerSheetID:     po.AnswerSheetID,
		AssessmentID:      po.AssessmentID,
		ReportID:          po.ReportID,
		EventName:         domainStatistics.BehaviorEventName(po.EventName),
		OccurredAt:        po.OccurredAt,
		Properties:        map[string]interface{}(po.Properties),
	}
}

func behaviorFootprintFromDomain(footprint *domainStatistics.BehaviorFootprint) *BehaviorFootprintPO {
	if footprint == nil {
		return nil
	}
	return &BehaviorFootprintPO{
		ID:                footprint.ID,
		OrgID:             footprint.OrgID,
		SubjectType:       footprint.SubjectType,
		SubjectID:         footprint.SubjectID,
		ActorType:         footprint.ActorType,
		ActorID:           footprint.ActorID,
		EntryID:           footprint.EntryID,
		ClinicianID:       footprint.ClinicianID,
		SourceClinicianID: footprint.SourceClinicianID,
		TesteeID:          footprint.TesteeID,
		AnswerSheetID:     footprint.AnswerSheetID,
		AssessmentID:      footprint.AssessmentID,
		ReportID:          footprint.ReportID,
		EventName:         string(footprint.EventName),
		OccurredAt:        footprint.OccurredAt,
		Properties:        JSONField(footprint.Properties),
	}
}

// AssessmentEpisodePO 测评服务过程。
type AssessmentEpisodePO struct {
	EpisodeID           uint64         `gorm:"column:episode_id;primaryKey"`
	OrgID               int64          `gorm:"column:org_id;not null;index:idx_ae_org_testee_del_submitted,priority:1;index:idx_ae_org_assessment_del,priority:1"`
	EntryID             *uint64        `gorm:"column:entry_id"`
	ClinicianID         *uint64        `gorm:"column:clinician_id"`
	TesteeID            uint64         `gorm:"column:testee_id;not null;index:idx_ae_org_testee_del_submitted,priority:2"`
	AnswerSheetID       uint64         `gorm:"column:answersheet_id;not null;uniqueIndex:uk_assessment_episode_answersheet_id"`
	AssessmentID        *uint64        `gorm:"column:assessment_id;index:idx_ae_org_assessment_del,priority:2"`
	ReportID            *uint64        `gorm:"column:report_id"`
	AttributedIntakeAt  *time.Time     `gorm:"column:attributed_intake_at"`
	SubmittedAt         time.Time      `gorm:"column:submitted_at;not null;index:idx_ae_org_testee_del_submitted,priority:4"`
	AssessmentCreatedAt *time.Time     `gorm:"column:assessment_created_at"`
	ReportGeneratedAt   *time.Time     `gorm:"column:report_generated_at"`
	FailedAt            *time.Time     `gorm:"column:failed_at"`
	Status              string         `gorm:"column:status;size:32;not null"`
	FailureReason       string         `gorm:"column:failure_reason;type:text"`
	CreatedAt           time.Time      `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt           time.Time      `gorm:"column:updated_at;autoUpdateTime"`
	DeletedAt           gorm.DeletedAt `gorm:"column:deleted_at;index:idx_ae_org_testee_del_submitted,priority:3;index:idx_ae_org_assessment_del,priority:3"`
}

func (AssessmentEpisodePO) TableName() string { return "assessment_episode" }

func (p *AssessmentEpisodePO) BeforeCreate(_ *gorm.DB) error {
	if p.EpisodeID == 0 {
		p.EpisodeID = uint64(meta.New())
	}
	return nil
}

func assessmentEpisodeToDomain(po *AssessmentEpisodePO) *domainStatistics.AssessmentEpisode {
	if po == nil {
		return nil
	}
	return &domainStatistics.AssessmentEpisode{
		EpisodeID:           po.EpisodeID,
		OrgID:               po.OrgID,
		EntryID:             po.EntryID,
		ClinicianID:         po.ClinicianID,
		TesteeID:            po.TesteeID,
		AnswerSheetID:       po.AnswerSheetID,
		AssessmentID:        po.AssessmentID,
		ReportID:            po.ReportID,
		AttributedIntakeAt:  po.AttributedIntakeAt,
		SubmittedAt:         po.SubmittedAt,
		AssessmentCreatedAt: po.AssessmentCreatedAt,
		ReportGeneratedAt:   po.ReportGeneratedAt,
		FailedAt:            po.FailedAt,
		Status:              domainStatistics.EpisodeStatus(po.Status),
		FailureReason:       po.FailureReason,
	}
}

func assessmentEpisodeFromDomain(e *domainStatistics.AssessmentEpisode) *AssessmentEpisodePO {
	if e == nil {
		return nil
	}
	return &AssessmentEpisodePO{
		EpisodeID:           e.EpisodeID,
		OrgID:               e.OrgID,
		EntryID:             e.EntryID,
		ClinicianID:         e.ClinicianID,
		TesteeID:            e.TesteeID,
		AnswerSheetID:       e.AnswerSheetID,
		AssessmentID:        e.AssessmentID,
		ReportID:            e.ReportID,
		AttributedIntakeAt:  e.AttributedIntakeAt,
		SubmittedAt:         e.SubmittedAt,
		AssessmentCreatedAt: e.AssessmentCreatedAt,
		ReportGeneratedAt:   e.ReportGeneratedAt,
		FailedAt:            e.FailedAt,
		Status:              string(e.Status),
		FailureReason:       e.FailureReason,
	}
}

const (
	AnalyticsProjectorCheckpointStatusProcessing = "processing"
	AnalyticsProjectorCheckpointStatusCompleted  = "completed"
	AnalyticsProjectorCheckpointStatusPending    = "pending"
)

type AnalyticsProjectorCheckpointPO struct {
	EventID   string         `gorm:"column:event_id;size:128;primaryKey"`
	EventType string         `gorm:"column:event_type;size:128;not null"`
	Status    string         `gorm:"column:status;size:32;not null;index"`
	CreatedAt time.Time      `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt time.Time      `gorm:"column:updated_at;autoUpdateTime"`
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at;index"`
}

func (AnalyticsProjectorCheckpointPO) TableName() string { return "analytics_projector_checkpoint" }

type AnalyticsPendingEventPO struct {
	EventID       string         `gorm:"column:event_id;size:128;primaryKey"`
	EventType     string         `gorm:"column:event_type;size:128;not null;index"`
	PayloadJSON   string         `gorm:"column:payload_json;type:longtext;not null"`
	NextAttemptAt time.Time      `gorm:"column:next_attempt_at;not null;index"`
	AttemptCount  int64          `gorm:"column:attempt_count;not null;default:0"`
	LastError     string         `gorm:"column:last_error;type:text"`
	CreatedAt     time.Time      `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt     time.Time      `gorm:"column:updated_at;autoUpdateTime"`
	DeletedAt     gorm.DeletedAt `gorm:"column:deleted_at;index"`
}

func (AnalyticsPendingEventPO) TableName() string { return "analytics_pending_event" }

type AnalyticsProjectionOrgDailyPO struct {
	ID                               uint64         `gorm:"column:id;primaryKey"`
	OrgID                            int64          `gorm:"column:org_id;not null;uniqueIndex:uniq_analytics_projection_org_daily"`
	StatDate                         time.Time      `gorm:"column:stat_date;type:date;not null;uniqueIndex:uniq_analytics_projection_org_daily"`
	EntryOpenedCount                 int64          `gorm:"column:entry_opened_count;not null;default:0"`
	IntakeConfirmedCount             int64          `gorm:"column:intake_confirmed_count;not null;default:0"`
	TesteeProfileCreatedCount        int64          `gorm:"column:testee_profile_created_count;not null;default:0"`
	CareRelationshipEstablishedCount int64          `gorm:"column:care_relationship_established_count;not null;default:0"`
	CareRelationshipTransferredCount int64          `gorm:"column:care_relationship_transferred_count;not null;default:0"`
	AnswerSheetSubmittedCount        int64          `gorm:"column:answersheet_submitted_count;not null;default:0"`
	AssessmentCreatedCount           int64          `gorm:"column:assessment_created_count;not null;default:0"`
	ReportGeneratedCount             int64          `gorm:"column:report_generated_count;not null;default:0"`
	EpisodeCompletedCount            int64          `gorm:"column:episode_completed_count;not null;default:0"`
	EpisodeFailedCount               int64          `gorm:"column:episode_failed_count;not null;default:0"`
	CreatedAt                        time.Time      `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt                        time.Time      `gorm:"column:updated_at;autoUpdateTime"`
	DeletedAt                        gorm.DeletedAt `gorm:"column:deleted_at;index"`
}

func (AnalyticsProjectionOrgDailyPO) TableName() string { return "analytics_projection_org_daily" }

func (p *AnalyticsProjectionOrgDailyPO) BeforeCreate(_ *gorm.DB) error {
	if p.ID == 0 {
		p.ID = uint64(meta.New())
	}
	return nil
}

type AnalyticsProjectionClinicianDailyPO struct {
	ID                               uint64         `gorm:"column:id;primaryKey"`
	OrgID                            int64          `gorm:"column:org_id;not null;uniqueIndex:uniq_analytics_projection_clinician_daily"`
	ClinicianID                      uint64         `gorm:"column:clinician_id;not null;uniqueIndex:uniq_analytics_projection_clinician_daily"`
	StatDate                         time.Time      `gorm:"column:stat_date;type:date;not null;uniqueIndex:uniq_analytics_projection_clinician_daily"`
	EntryOpenedCount                 int64          `gorm:"column:entry_opened_count;not null;default:0"`
	IntakeConfirmedCount             int64          `gorm:"column:intake_confirmed_count;not null;default:0"`
	TesteeProfileCreatedCount        int64          `gorm:"column:testee_profile_created_count;not null;default:0"`
	CareRelationshipEstablishedCount int64          `gorm:"column:care_relationship_established_count;not null;default:0"`
	CareRelationshipTransferredCount int64          `gorm:"column:care_relationship_transferred_count;not null;default:0"`
	AnswerSheetSubmittedCount        int64          `gorm:"column:answersheet_submitted_count;not null;default:0"`
	AssessmentCreatedCount           int64          `gorm:"column:assessment_created_count;not null;default:0"`
	ReportGeneratedCount             int64          `gorm:"column:report_generated_count;not null;default:0"`
	EpisodeCompletedCount            int64          `gorm:"column:episode_completed_count;not null;default:0"`
	EpisodeFailedCount               int64          `gorm:"column:episode_failed_count;not null;default:0"`
	CreatedAt                        time.Time      `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt                        time.Time      `gorm:"column:updated_at;autoUpdateTime"`
	DeletedAt                        gorm.DeletedAt `gorm:"column:deleted_at;index"`
}

func (AnalyticsProjectionClinicianDailyPO) TableName() string {
	return "analytics_projection_clinician_daily"
}

func (p *AnalyticsProjectionClinicianDailyPO) BeforeCreate(_ *gorm.DB) error {
	if p.ID == 0 {
		p.ID = uint64(meta.New())
	}
	return nil
}

type AnalyticsProjectionEntryDailyPO struct {
	ID                               uint64         `gorm:"column:id;primaryKey"`
	OrgID                            int64          `gorm:"column:org_id;not null;uniqueIndex:uniq_analytics_projection_entry_daily"`
	EntryID                          uint64         `gorm:"column:entry_id;not null;uniqueIndex:uniq_analytics_projection_entry_daily"`
	ClinicianID                      uint64         `gorm:"column:clinician_id;not null;default:0"`
	StatDate                         time.Time      `gorm:"column:stat_date;type:date;not null;uniqueIndex:uniq_analytics_projection_entry_daily"`
	EntryOpenedCount                 int64          `gorm:"column:entry_opened_count;not null;default:0"`
	IntakeConfirmedCount             int64          `gorm:"column:intake_confirmed_count;not null;default:0"`
	TesteeProfileCreatedCount        int64          `gorm:"column:testee_profile_created_count;not null;default:0"`
	CareRelationshipEstablishedCount int64          `gorm:"column:care_relationship_established_count;not null;default:0"`
	CareRelationshipTransferredCount int64          `gorm:"column:care_relationship_transferred_count;not null;default:0"`
	AnswerSheetSubmittedCount        int64          `gorm:"column:answersheet_submitted_count;not null;default:0"`
	AssessmentCreatedCount           int64          `gorm:"column:assessment_created_count;not null;default:0"`
	ReportGeneratedCount             int64          `gorm:"column:report_generated_count;not null;default:0"`
	EpisodeCompletedCount            int64          `gorm:"column:episode_completed_count;not null;default:0"`
	EpisodeFailedCount               int64          `gorm:"column:episode_failed_count;not null;default:0"`
	CreatedAt                        time.Time      `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt                        time.Time      `gorm:"column:updated_at;autoUpdateTime"`
	DeletedAt                        gorm.DeletedAt `gorm:"column:deleted_at;index"`
}

func (AnalyticsProjectionEntryDailyPO) TableName() string { return "analytics_projection_entry_daily" }

func (p *AnalyticsProjectionEntryDailyPO) BeforeCreate(_ *gorm.DB) error {
	if p.ID == 0 {
		p.ID = uint64(meta.New())
	}
	return nil
}
