package checkpoint

import (
	"time"
)

const (
	scopeEvaluationRun = "evaluation_run"
)

// RuntimeCheckpointPO persists unified runtime checkpoints.
type RuntimeCheckpointPO struct {
	ID                 uint64     `gorm:"column:id;primaryKey"`
	Scope              string     `gorm:"column:scope;size:64;not null;uniqueIndex:uk_runtime_checkpoint_scope_resource_attempt,priority:1"`
	ResourceID         string     `gorm:"column:resource_id;size:128;not null;uniqueIndex:uk_runtime_checkpoint_scope_resource_attempt,priority:2"`
	AttemptNo          uint       `gorm:"column:attempt_no;not null;uniqueIndex:uk_runtime_checkpoint_scope_resource_attempt,priority:3"`
	AssessmentID       *uint64    `gorm:"column:assessment_id;index:idx_runtime_checkpoint_assessment_id"`
	EventType          *string    `gorm:"column:event_type;size:128"`
	Status             string     `gorm:"column:status;size:50;not null;index:idx_runtime_checkpoint_scope_status,priority:2"`
	StartedAt          time.Time  `gorm:"column:started_at;not null"`
	FinishedAt         *time.Time `gorm:"column:finished_at"`
	ErrorCode          *string    `gorm:"column:error_code;size:50"`
	ErrorMessage       *string    `gorm:"column:error_message;size:500"`
	Retryable          bool       `gorm:"column:retryable;not null;default:false"`
	AttemptOrigin      *string    `gorm:"column:attempt_origin;size:32"`
	RetryDisposition   *string    `gorm:"column:retry_disposition;size:32;index:idx_runtime_checkpoint_retry_due,priority:3"`
	NextAttemptAt      *time.Time `gorm:"column:next_attempt_at;index:idx_runtime_checkpoint_retry_due,priority:4"`
	PolicyMaxAttempts  *uint      `gorm:"column:policy_max_attempts"`
	RetryPolicyVersion *string    `gorm:"column:retry_policy_version;size:64"`
	RetryEventID       *string    `gorm:"column:retry_event_id;size:64"`
	ActionRequestID    *string    `gorm:"column:action_request_id;size:64"`
	TraceID            *string    `gorm:"column:trace_id;size:100"`
	InputSnapshotRef   *string    `gorm:"column:input_snapshot_ref;size:200"`
	ClaimToken         *string    `gorm:"column:claim_token;size:100;index:idx_runtime_checkpoint_claim"`
	LeaseExpiresAt     *time.Time `gorm:"column:lease_expires_at;index:idx_runtime_checkpoint_claim"`
	CreatedAt          time.Time  `gorm:"column:created_at"`
	UpdatedAt          time.Time  `gorm:"column:updated_at"`
	DeletedAt          *time.Time `gorm:"column:deleted_at;index:idx_runtime_checkpoint_deleted_at"`
}

func (RuntimeCheckpointPO) TableName() string {
	return "runtime_checkpoint"
}
