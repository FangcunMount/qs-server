package statistics

import (
	"context"
	"time"
)

// EpisodeStatus 测评服务过程状态。
type EpisodeStatus string

const (
	EpisodeStatusActive    EpisodeStatus = "active"    // 进行中
	EpisodeStatusCompleted EpisodeStatus = "completed" // 已完成
	EpisodeStatusFailed    EpisodeStatus = "failed"    // 失败
)

// BehaviorEventName 行为足迹事件名。
type BehaviorEventName string

const (
	BehaviorEventEntryOpened                 BehaviorEventName = "entry_opened"                  // 入口打开
	BehaviorEventIntakeConfirmed             BehaviorEventName = "intake_confirmed"              // 接纳完成
	BehaviorEventTesteeProfileCreated        BehaviorEventName = "testee_profile_created"        // 创建受试者档案
	BehaviorEventCareRelationshipEstablished BehaviorEventName = "care_relationship_established" // 建立看护关系
	BehaviorEventCareRelationshipTransferred BehaviorEventName = "care_relationship_transferred" // 转移看护关系
	BehaviorEventAnswerSheetSubmitted        BehaviorEventName = "answersheet_submitted"         // 提交答卷
	BehaviorEventAssessmentCreated           BehaviorEventName = "assessment_created"            // 创建测评
	BehaviorEventReportGenerated             BehaviorEventName = "report_generated"              // 生成报告
)

// BehaviorFootprint 行为足迹。
type BehaviorFootprint struct {
	ID                string
	OrgID             int64
	SubjectType       string
	SubjectID         uint64
	ActorType         string
	ActorID           uint64
	EntryID           uint64
	ClinicianID       uint64
	SourceClinicianID uint64
	TesteeID          uint64
	AnswerSheetID     uint64
	AssessmentID      uint64
	ReportID          uint64
	EventName         BehaviorEventName
	OccurredAt        time.Time
	Properties        map[string]interface{}
}

// AssessmentEpisode 一次测评服务闭环。
type AssessmentEpisode struct {
	EpisodeID           uint64
	OrgID               int64
	EntryID             *uint64
	ClinicianID         *uint64
	TesteeID            uint64
	AnswerSheetID       uint64
	AssessmentID        *uint64
	ReportID            *uint64
	AttributedIntakeAt  *time.Time
	SubmittedAt         time.Time
	AssessmentCreatedAt *time.Time
	ReportGeneratedAt   *time.Time
	FailedAt            *time.Time
	Status              EpisodeStatus
	FailureReason       string
}

// BehaviorFootprintWriter 追加行为足迹。
type BehaviorFootprintWriter interface {
	AppendBehaviorFootprint(ctx context.Context, footprint *BehaviorFootprint) error
	FindLatestFootprintByEvent(ctx context.Context, orgID int64, testeeID uint64, eventName BehaviorEventName, occurredAt time.Time, window time.Duration) (*BehaviorFootprint, error)
}

// AssessmentEpisodeRepository 读写测评服务过程。
type AssessmentEpisodeRepository interface {
	SaveEpisode(ctx context.Context, episode *AssessmentEpisode) error
	FindEpisodeByAnswerSheetID(ctx context.Context, orgID int64, answerSheetID uint64) (*AssessmentEpisode, error)
	FindEpisodeByAssessmentID(ctx context.Context, orgID int64, assessmentID uint64) (*AssessmentEpisode, error)
}

// AnalyticsProjectionMutation 日级分析投影增量。
type AnalyticsProjectionMutation struct {
	OrgID       int64
	ClinicianID uint64
	EntryID     uint64
	StatDate    time.Time

	EntryOpenedCount                 int64
	IntakeConfirmedCount             int64
	TesteeProfileCreatedCount        int64
	CareRelationshipEstablishedCount int64
	CareRelationshipTransferredCount int64
	AnswerSheetSubmittedCount        int64
	AssessmentCreatedCount           int64
	ReportGeneratedCount             int64
	EpisodeCompletedCount            int64
	EpisodeFailedCount               int64
}

const (
	AnalyticsProjectorCheckpointStatusProcessing = "processing"
	AnalyticsProjectorCheckpointStatusCompleted  = "completed"
	AnalyticsProjectorCheckpointStatusPending    = "pending"
)

type AnalyticsPendingEvent struct {
	EventID      string
	EventType    string
	PayloadJSON  string
	AttemptCount int64
}

// AnalyticsProjectionRepository 维护分析投影。
type AnalyticsProjectionRepository interface {
	ApplyAnalyticsProjectionMutation(ctx context.Context, mutation AnalyticsProjectionMutation) error
	ApplyAnalyticsClinicianProjectionMutation(ctx context.Context, mutation AnalyticsProjectionMutation) error
	ApplyAnalyticsEntryProjectionMutation(ctx context.Context, mutation AnalyticsProjectionMutation) error
	ListEpisodesForAttribution(ctx context.Context, orgID int64, testeeID uint64, intakeAt time.Time, window time.Duration) ([]*AssessmentEpisode, error)
	TryBeginAnalyticsProjectorCheckpoint(ctx context.Context, eventID, eventType string) (string, error)
	MarkAnalyticsProjectorCheckpointStatus(ctx context.Context, eventID, status string) error
	UpsertAnalyticsPendingEvent(ctx context.Context, eventID, eventType, payload string, nextAttemptAt time.Time, lastError string) error
	ListDueAnalyticsPendingEvents(ctx context.Context, limit int, now time.Time) ([]*AnalyticsPendingEvent, error)
	RescheduleAnalyticsPendingEvent(ctx context.Context, eventID, lastError string, nextAttemptAt time.Time) error
	DeleteAnalyticsPendingEvent(ctx context.Context, eventID string) error
}
