package workbench

import (
	"context"
	"time"
)

type QueueType string

const (
	QueueTypeHighRisk QueueType = "high_risk"
	QueueTypeFollowUp QueueType = "follow_up"
	QueueTypeKeyFocus QueueType = "key_focus"
)

type ScopeKind string

const (
	ScopeKindClinicianMe ScopeKind = "clinician_me"
	ScopeKindOrgAdmin    ScopeKind = "org_admin"
)

type Service interface {
	GetSummary(ctx context.Context, scope Scope) (*SummaryResult, error)
	ListQueue(ctx context.Context, dto ListQueueDTO) (*QueuePage, error)
}

type Scope struct {
	Kind           ScopeKind
	OrgID          int64
	OperatorUserID int64
	ClinicianID    *uint64
}

type ListQueueDTO struct {
	Scope
	QueueType QueueType
	Page      int
	PageSize  int
}

type SummaryResult struct {
	Counts QueueCounts
}

type QueueCounts struct {
	HighRisk int64
	FollowUp int64
	KeyFocus int64
}

type QueuePage struct {
	QueueType  QueueType
	Items      []QueueItem
	Total      int64
	Page       int
	PageSize   int
	TotalPages int
}

type QueueItem struct {
	Testee             Testee
	ReasonCode         string
	Reason             string
	ReasonAt           *time.Time
	RiskLevel          string
	Task               *TaskSummary
	PrimaryClinician   *ClinicianAssignment
	AssignedClinicians []ClinicianAssignment
	IsUnassigned       *bool
}

type Testee struct {
	ID               uint64
	OrgID            int64
	ProfileID        *uint64
	Name             string
	Gender           int8
	Birthday         *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
	Tags             []string
	Source           string
	IsKeyFocus       bool
	LastAssessmentAt *time.Time
	TotalAssessments int
	LastRiskLevel    string
}

type TaskSummary struct {
	TaskID    uint64
	PlanID    uint64
	Status    string
	PlannedAt time.Time
	OpenAt    *time.Time
	ExpireAt  *time.Time
	ScaleCode string
	EntryURL  string
}

type ClinicianAssignment struct {
	ID            uint64
	OrgID         int64
	OperatorID    *uint64
	Name          string
	Department    string
	Title         string
	ClinicianType string
	RelationType  string
	BoundAt       time.Time
}
