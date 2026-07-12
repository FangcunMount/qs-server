package operator

import (
	"context"
	"time"
)

type AccessScope struct {
	IsAdmin     bool
	ClinicianID *uint64
}

type AccessChecker interface {
	ResolveAccessScope(context.Context, int64, int64) (*AccessScope, error)
	ValidateTesteeAccess(context.Context, int64, int64, uint64) error
	ListAccessibleTesteeIDs(context.Context, int64, int64) ([]uint64, error)
}

type ListQuery struct {
	Page, PageSize        int
	TesteeID              *uint64
	Status                string
	AccessibleTesteeIDs   []uint64
	RestrictToAccessScope bool
}

type TesteeListScope struct {
	TesteeID            uint64
	AccessibleTesteeIDs []uint64
	Restricted          bool
}

type TrendQuery struct {
	TesteeID   uint64
	FactorCode string
	Limit      int
}

type ModelIdentity struct{ Kind, SubKind, Algorithm, Code, Version, Title, ProductChannel, AlgorithmFamily string }
type ScoreValue struct {
	Kind  string
	Value float64
	Label string
	Max   *float64
}
type ResultLevel struct{ Code, Label, Severity string }

type Assessment struct {
	ID, OrgID, TesteeID, AnswerSheetID      uint64
	QuestionnaireCode, QuestionnaireVersion string
	ModelKind, ModelSubKind, ModelAlgorithm *string
	ModelCode, ModelVersion, ModelTitle     *string
	OriginType                              string
	OriginID                                *string
	Status                                  string
	TotalScore                              *float64
	RiskLevel                               *string
	SubmittedAt, EvaluatedAt, FailedAt      *time.Time
	FailureReason                           *string
}

type AssessmentList struct {
	Items                             []*Assessment
	Total, Page, PageSize, TotalPages int
}

type OutcomeAssessment struct {
	ID, OrgID, TesteeID, AnswerSheetID      uint64
	QuestionnaireCode, QuestionnaireVersion string
	Model                                   ModelIdentity
	PrimaryScore                            *ScoreValue
	Level                                   *ResultLevel
	OriginType                              string
	OriginID                                *string
	Status                                  string
	SubmittedAt, FailedAt                   *time.Time
	FailureReason                           *string
}

type OutcomeAssessmentList struct {
	Items                             []*OutcomeAssessment
	Total, Page, PageSize, TotalPages int
}

type FactorScore struct {
	FactorCode, FactorName string
	RawScore               float64
	MaxScore               *float64
	RiskLevel              string
	IsTotalScore           bool
}
type Score struct {
	AssessmentID uint64
	TotalScore   float64
	RiskLevel    string
	FactorScores []FactorScore
}
type TrendPoint struct {
	AssessmentID uint64
	RawScore     float64
	RiskLevel    string
}
type FactorTrend struct {
	TesteeID               uint64
	FactorCode, FactorName string
	DataPoints             []TrendPoint
}
type HighRiskFactors struct {
	AssessmentID    uint64
	HasHighRisk     bool
	HighRiskFactors []FactorScore
	NeedsUrgentCare bool
}

type Run struct {
	RunID            string
	AssessmentID     uint64
	AttemptNo        int
	Status           string
	Retryable        bool
	ErrorCode        string
	ErrorMessage     string
	StartedAt        time.Time
	FinishedAt       *time.Time
	TraceID          string
	InputSnapshotRef string
}
type RunList struct{ Items []*Run }
type RetryableFailedRun struct {
	Run
	OrgID int64
}
type RetryableFailedRunList struct {
	Items      []*RetryableFailedRun
	NextCursor uint64
}

type QueryService interface {
	ValidateTesteeAccess(context.Context, Actor, uint64) error
	ScopeTesteeList(context.Context, Actor, uint64) (TesteeListScope, error)
	GetAssessment(context.Context, Actor, uint64) (*Assessment, error)
	ListAssessments(context.Context, Actor, ListQuery) (*AssessmentList, error)
	GetAssessmentOutcome(context.Context, Actor, uint64) (*OutcomeAssessment, error)
	ListAssessmentsOutcome(context.Context, Actor, ListQuery) (*OutcomeAssessmentList, error)
	GetScores(context.Context, Actor, uint64) (*Score, error)
	GetHighRiskFactors(context.Context, Actor, uint64) (*HighRiskFactors, error)
	GetFactorTrend(context.Context, Actor, TrendQuery) (*FactorTrend, error)
	ListAssessmentRuns(context.Context, Actor, uint64, int) (*RunList, error)
	GetLatestAssessmentRun(context.Context, Actor, uint64) (*Run, error)
	ListRetryableFailedRuns(context.Context, Actor, int, uint64) (*RetryableFailedRunList, error)
}
