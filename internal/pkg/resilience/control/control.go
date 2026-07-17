// Package control defines the transport-neutral control contract for
// process-owned resilience subsystems. It deliberately does not implement data
// plane rate limiting, queues, backpressure, or leases.
package control

import (
	"context"
	"errors"
	"strconv"
	"time"
)

func ScopedRequestID(orgID int64, requestID string) string {
	return strconv.FormatInt(orgID, 10) + ":" + requestID
}

var (
	ErrVersionConflict = errors.New("resilience control version conflict")
	ErrUnavailable     = errors.New("resilience control unavailable")
	ErrInvalidState    = errors.New("resilience control invalid state")
	ErrInvalidArgument = errors.New("resilience control invalid argument")
)

type Target struct {
	Component  string `json:"component"`
	InstanceID string `json:"instance_id,omitempty"`
}

type VersionedState struct {
	Version   uint64      `json:"version"`
	Payload   []byte      `json:"payload"`
	UpdatedAt time.Time   `json:"updated_at"`
	ExpiresAt time.Time   `json:"expires_at,omitempty"`
	Actor     ActionActor `json:"actor"`
}

type ActionActor struct {
	OrgID  int64  `json:"org_id"`
	UserID uint64 `json:"user_id"`
}

type StateStore interface {
	Load(context.Context, string) (VersionedState, bool, error)
	CompareAndSwap(context.Context, string, uint64, VersionedState, time.Duration) (VersionedState, error)
	Delete(context.Context, string, uint64) error
}

// InstanceHeartbeater is implemented by control-plane adapters that can
// publish process liveness without exposing their storage technology.
type InstanceHeartbeater interface {
	Heartbeat(context.Context, InstanceIdentity, time.Duration) error
}

// StateSignalWatcher provides a best-effort wake-up channel. Consumers must
// still reconcile periodically because Pub/Sub delivery is not durable.
type StateSignalWatcher interface {
	WatchStateSignals(context.Context) (<-chan string, error)
}

type Command struct {
	RequestID string      `json:"request_id"`
	ActionID  string      `json:"action_id"`
	Target    Target      `json:"target"`
	Payload   []byte      `json:"payload"`
	Actor     ActionActor `json:"actor"`
	IssuedAt  time.Time   `json:"issued_at"`
	ExpiresAt time.Time   `json:"expires_at"`
}

type CommandStatus string

const (
	CommandStatusOK      CommandStatus = "ok"
	CommandStatusNoop    CommandStatus = "noop"
	CommandStatusPartial CommandStatus = "partial"
	CommandStatusTimeout CommandStatus = "timeout"
	CommandStatusFailed  CommandStatus = "failed"
)

type CommandResult struct {
	RequestID  string        `json:"request_id"`
	OrgID      int64         `json:"org_id"`
	ActionID   string        `json:"action_id"`
	Component  string        `json:"component"`
	InstanceID string        `json:"instance_id"`
	Status     CommandStatus `json:"status"`
	Message    string        `json:"message,omitempty"`
	Payload    []byte        `json:"payload,omitempty"`
	FinishedAt time.Time     `json:"finished_at"`
}

type CommandExecutor interface {
	Execute(context.Context, Command) (CommandResult, error)
}

type CommandStore interface {
	PublishCommand(context.Context, Command, time.Duration) error
	ListCommands(context.Context, string, string) ([]Command, error)
	Claim(context.Context, string, string, time.Duration) (bool, error)
	PutCommandResult(context.Context, CommandResult, time.Duration) error
	ListCommandResults(context.Context, int64, string) ([]CommandResult, error)
	ListInstances(context.Context, string) ([]InstanceIdentity, error)
}

type QueueState string

const (
	QueueStateActive   QueueState = "active"
	QueueStateDraining QueueState = "draining"
	QueueStatePaused   QueueState = "paused"
)

type DrainOptions struct {
	Timeout time.Duration
}

type DrainResult struct {
	State      QueueState `json:"state"`
	Version    uint64     `json:"version"`
	Depth      int        `json:"depth"`
	InFlight   int        `json:"in_flight"`
	FinishedAt time.Time  `json:"finished_at"`
}

type QueueController interface {
	Drain(context.Context, DrainOptions) (DrainResult, error)
	Resume(context.Context) error
}

type RatePolicy struct {
	RatePerSecond float64 `json:"rate_per_second"`
	Burst         int     `json:"burst"`
}

func (p RatePolicy) Valid() bool { return p.RatePerSecond > 0 && p.Burst > 0 }

type RateLimitChange struct {
	Mode            string     `json:"mode"`
	Component       string     `json:"component"`
	Budget          string     `json:"budget"`
	ExpectedVersion uint64     `json:"expected_version"`
	Global          RatePolicy `json:"global"`
	User            RatePolicy `json:"user"`
	TTLSeconds      int        `json:"ttl_seconds"`
}

type RateLimitChangeResult struct {
	Status    CommandStatus `json:"status"`
	Component string        `json:"component"`
	Budget    string        `json:"budget"`
	Version   uint64        `json:"version"`
	Source    string        `json:"source"`
	ExpiresAt time.Time     `json:"expires_at,omitempty"`
}

type QueueChange struct {
	RequestID      string     `json:"-"`
	Component      string     `json:"component"`
	Queue          string     `json:"queue"`
	Target         string     `json:"target"`
	DesiredState   QueueState `json:"desired_state"`
	TimeoutSeconds int        `json:"timeout_seconds"`
}

type QueueChangeResult struct {
	Status    CommandStatus   `json:"status"`
	Component string          `json:"component"`
	Queue     string          `json:"queue"`
	State     QueueState      `json:"state"`
	Version   uint64          `json:"version"`
	Instances []CommandResult `json:"instances,omitempty"`
}

type LeaderChange struct {
	RequestID       string `json:"-"`
	Component       string `json:"component"`
	InstanceID      string `json:"instance_id"`
	Workload        string `json:"workload"`
	CooldownSeconds int    `json:"cooldown_seconds"`
	TimeoutSeconds  int    `json:"timeout_seconds"`
}

type Governor interface {
	TuneRateLimit(context.Context, ActionActor, RateLimitChange) (RateLimitChangeResult, error)
	SetQueueState(context.Context, ActionActor, QueueChange) (QueueChangeResult, error)
	RelinquishLeader(context.Context, ActionActor, LeaderChange) (any, error)
}
