package assessmententry

import (
	"context"
	"time"
)

// AssessmentEntryService 测评入口服务。
type AssessmentEntryService interface {
	Create(ctx context.Context, dto CreateAssessmentEntryDTO) (*AssessmentEntryResult, error)
	GetByID(ctx context.Context, entryID uint64) (*AssessmentEntryResult, error)
	Deactivate(ctx context.Context, entryID uint64) (*AssessmentEntryResult, error)
	Reactivate(ctx context.Context, entryID uint64) (*AssessmentEntryResult, error)
	ListByClinician(ctx context.Context, dto ListAssessmentEntryDTO) (*AssessmentEntryListResult, error)
	Resolve(ctx context.Context, token string) (*ResolvedAssessmentEntryResult, error)
	Intake(ctx context.Context, token string, dto IntakeByAssessmentEntryDTO) (*AssessmentEntryIntakeResult, error)
}

// ResolveLogWriter 写入入口解析日志。
type ResolveLogWriter interface {
	LogResolve(ctx context.Context, orgID int64, clinicianID, entryID uint64, resolvedAt time.Time) error
}

// IntakeLogWriter 写入入口 intake 成功日志。
type IntakeLogWriter interface {
	LogIntake(ctx context.Context, orgID int64, clinicianID, entryID, testeeID uint64, intakeAt time.Time, testeeCreated, assignmentCreated bool) error
}

// BehaviorEventStager 将 assessment entry 行为事件可靠暂存到 outbox。
type BehaviorEventStager interface {
	StageEntryOpened(ctx context.Context, orgID int64, clinicianID, entryID uint64, occurredAt time.Time) error
	StageIntakeConfirmed(ctx context.Context, orgID int64, clinicianID, entryID, testeeID uint64, occurredAt time.Time) error
	StageTesteeProfileCreated(ctx context.Context, orgID int64, clinicianID, entryID, testeeID uint64, occurredAt time.Time) error
	StageCareRelationshipEstablished(ctx context.Context, orgID int64, clinicianID, entryID, testeeID uint64, occurredAt time.Time) error
}

// CreateAssessmentEntryDTO 创建测评入口。
type CreateAssessmentEntryDTO struct {
	OrgID         int64
	ClinicianID   uint64
	TargetType    string
	TargetCode    string
	TargetVersion string
	ExpiresAt     *time.Time
}

// ListAssessmentEntryDTO 查询测评入口列表。
type ListAssessmentEntryDTO struct {
	OrgID       int64
	ClinicianID uint64
	Offset      int
	Limit       int
}

// AssessmentEntryResult 测评入口结果。
type AssessmentEntryResult struct {
	ID            uint64
	OrgID         int64
	ClinicianID   uint64
	Token         string
	TargetType    string
	TargetCode    string
	TargetVersion string
	IsActive      bool
	ExpiresAt     *time.Time
}

// AssessmentEntryListResult 测评入口列表结果。
type AssessmentEntryListResult struct {
	Items      []*AssessmentEntryResult
	TotalCount int64
	Offset     int
	Limit      int
}

// ClinicianSummaryResult 从业者摘要。
type ClinicianSummaryResult struct {
	ID            uint64
	OperatorID    *uint64
	Name          string
	Department    string
	Title         string
	ClinicianType string
}

// TesteeSummaryResult 受试者摘要。
type TesteeSummaryResult struct {
	ID         uint64
	OrgID      int64
	ProfileID  *uint64
	Name       string
	Gender     int8
	Birthday   *time.Time
	Age        int
	Tags       []string
	Source     string
	IsKeyFocus bool
}

// RelationSummaryResult 关系摘要。
type RelationSummaryResult struct {
	ID           uint64
	OrgID        int64
	ClinicianID  uint64
	TesteeID     uint64
	RelationType string
	SourceType   string
	SourceID     *uint64
	IsActive     bool
	BoundAt      time.Time
	UnboundAt    *time.Time
}

// ResolvedAssessmentEntryResult 入口解析结果。
type ResolvedAssessmentEntryResult struct {
	Entry     *AssessmentEntryResult
	Clinician *ClinicianSummaryResult
}

// IntakeByAssessmentEntryDTO 扫码 intake 请求。
type IntakeByAssessmentEntryDTO struct {
	ProfileID *uint64
	Name      string
	Gender    int8
	Birthday  *time.Time
}

// AssessmentEntryIntakeResult 扫码 intake 结果。
type AssessmentEntryIntakeResult struct {
	Entry      *AssessmentEntryResult
	Clinician  *ClinicianSummaryResult
	Testee     *TesteeSummaryResult
	Relation   *RelationSummaryResult
	Assignment *RelationSummaryResult
}
