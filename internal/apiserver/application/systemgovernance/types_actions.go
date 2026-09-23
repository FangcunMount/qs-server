package systemgovernance

import (
	"context"
	"errors"
	"time"
)

var ErrActionAuditInputConflict = errors.New("governance request ID already binds different action, actor, or input")
var ErrActionAuditPendingReconciliation = errors.New("governance replay is pending reconciliation")

const ActionAuditStatusPendingReconciliation = "pending_reconciliation"

// ActionDescriptor 描述governance 命令 exposed 到 operators。
type ActionDescriptor struct {
	ID                   string                 `json:"id"`
	Domain               Domain                 `json:"domain"`
	Label                string                 `json:"label"`
	RiskLevel            string                 `json:"risk_level"`
	Enabled              bool                   `json:"enabled"`
	Planned              bool                   `json:"planned"`
	RequiresConfirmation bool                   `json:"requires_confirmation"`
	InputSchema          map[string]interface{} `json:"input_schema,omitempty"`
}

// ActionsView 列出治理命令。
type ActionsView struct {
	GeneratedAt time.Time          `json:"generated_at"`
	Actions     []ActionDescriptor `json:"actions"`
}

// ActionRunRequest 是body 用于 POST /actions/:action_id/runs。
type ActionRunRequest struct {
	RequestID string                 `json:"request_id,omitempty"`
	Confirm   bool                   `json:"confirm"`
	Input     map[string]interface{} `json:"input,omitempty"`
}

// ActionRunResult 是结果 of executed governance 命令。
type ActionRunResult struct {
	RequestID  string                 `json:"request_id,omitempty"`
	ActionID   string                 `json:"action_id"`
	StartedAt  time.Time              `json:"started_at"`
	FinishedAt time.Time              `json:"finished_at"`
	Status     string                 `json:"status"`
	Message    string                 `json:"message,omitempty"`
	Result     map[string]interface{} `json:"result,omitempty"`
}

// PendingReplayAudit is an unresolved operator request. Input is the redacted
// original action input; operators must reuse it with the same request ID.
type PendingReplayAudit struct {
	RequestID   string                 `json:"request_id"`
	ActorUserID string                 `json:"actor_user_id"`
	Store       string                 `json:"store"`
	Input       map[string]interface{} `json:"input"`
	StartedAt   time.Time              `json:"started_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

type PendingReplayAuditPage struct {
	Items      []PendingReplayAudit `json:"items"`
	NextCursor string               `json:"next_cursor,omitempty"`
}

type PendingReplayAuditReader interface {
	ListPendingReplayAudits(context.Context, int64, string, int) (PendingReplayAuditPage, error)
}

// ActionAuditRecord is the persistence-neutral governance audit contract.
// Input must already be redacted before it crosses this port.
type ActionAuditRecord struct {
	RequestID      string
	ActionID       string
	OrgID          int64
	ActorUserID    uint64
	Component      string
	TargetInstance string
	Input          map[string]interface{}
	StartedAt      time.Time
	FinishedAt     time.Time
	Status         string
	Result         *ActionRunResult
	Error          *ActionAuditError
}

// ActionAuditError is the persistence-neutral error metadata required to
// replay a completed action with the same application error contract.
type ActionAuditError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type ActionAuditReplay struct {
	ActionID string            `json:"action_id,omitempty"`
	Result   *ActionRunResult  `json:"result,omitempty"`
	Error    *ActionAuditError `json:"error,omitempty"`
}

// ActionAuditStore atomically claims request IDs before execution and records
// their terminal result. A failed claim returns either the completed prior
// result or claimed=false while the first execution is still running.
type ActionAuditStore interface {
	Claim(context.Context, ActionAuditRecord) (existing *ActionAuditReplay, claimed bool, err error)
	Complete(context.Context, ActionAuditRecord) error
}

// RunningActionAuditReader returns the original unresolved record only when the
// retrying caller matches its action, actor, and complete redacted input.
// Absence is not authorization to execute the action again.
type RunningActionAuditReader interface {
	LoadRunning(context.Context, ActionAuditRecord) (ActionAuditRecord, bool, error)
}

// PendingActionAuditMarker records an uncertain replay without closing its
// identity. Only a matching durable authorization can later complete it.
type PendingActionAuditMarker interface {
	MarkPending(context.Context, ActionAuditRecord) error
}

// ActionAuditFallbackStore persists only terminal replay data when the primary
// audit store cannot finish a record. Implementations must not persist Input or
// actor credentials.
type ActionAuditFallbackStore interface {
	Load(context.Context, int64, string) (ActionAuditRecord, bool, error)
	Put(context.Context, ActionAuditRecord) error
	Delete(context.Context, int64, string) error
	List(context.Context, int) ([]ActionAuditRecord, error)
}

type ActionAuditRecoverer interface {
	Recover(context.Context, int) (int, error)
}
