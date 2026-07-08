package checkpoint

import (
	"time"
)

const (
	scopeEvaluationRun      = "evaluation_run"
	scopeAnalyticsProjector = "analytics_projector"
)

// RuntimeCheckpointPO persists unified runtime checkpoints.
type RuntimeCheckpointPO struct {
	ID               uint64     `gorm:"column:id;primaryKey"`
	Scope            string     `gorm:"column:scope;size:64;not null;uniqueIndex:uk_runtime_checkpoint_scope_resource_attempt,priority:1"`
	ResourceID       string     `gorm:"column:resource_id;size:128;not null;uniqueIndex:uk_runtime_checkpoint_scope_resource_attempt,priority:2"`
	AttemptNo        uint       `gorm:"column:attempt_no;not null;uniqueIndex:uk_runtime_checkpoint_scope_resource_attempt,priority:3"`
	AssessmentID     *uint64    `gorm:"column:assessment_id;index:idx_runtime_checkpoint_assessment_id"`
	EventType        *string    `gorm:"column:event_type;size:128"`
	Status           string     `gorm:"column:status;size:50;not null;index:idx_runtime_checkpoint_scope_status,priority:2"`
	StartedAt        time.Time  `gorm:"column:started_at;not null"`
	FinishedAt       *time.Time `gorm:"column:finished_at"`
	ErrorCode        *string    `gorm:"column:error_code;size:50"`
	ErrorMessage     *string    `gorm:"column:error_message;size:500"`
	Retryable        bool       `gorm:"column:retryable;not null;default:false"`
	TraceID          *string    `gorm:"column:trace_id;size:100"`
	InputSnapshotRef *string    `gorm:"column:input_snapshot_ref;size:200"`
	CreatedAt        time.Time  `gorm:"column:created_at"`
	UpdatedAt        time.Time  `gorm:"column:updated_at"`
	DeletedAt        *time.Time `gorm:"column:deleted_at;index:idx_runtime_checkpoint_deleted_at"`
}

func (RuntimeCheckpointPO) TableName() string {
	return "runtime_checkpoint"
}
