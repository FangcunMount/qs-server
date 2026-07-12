package evaluationreadmodel

import (
	"context"
	"time"
)

type PageRequest struct {
	Page     int
	PageSize int
}

func (p PageRequest) Offset() int {
	page := p.Page
	if page < 1 {
		page = 1
	}
	return (page - 1) * p.Limit()
}

func (p PageRequest) Limit() int {
	pageSize := p.PageSize
	if pageSize < 1 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return pageSize
}

type AssessmentFilter struct {
	OrgID                 int64
	TesteeID              *uint64
	AccessibleTesteeIDs   []uint64
	RestrictToAccessScope bool
	Statuses              []string
	ScaleCode             string
	RiskLevel             string
	ModelKind             string
	ModelAlgorithm        string
	ModelCode             string
	DateFrom              *time.Time
	DateTo                *time.Time
}

type AssessmentRow struct {
	ID                       uint64
	OrgID                    int64
	TesteeID                 uint64
	QuestionnaireCode        string
	QuestionnaireVersion     string
	AnswerSheetID            uint64
	EvaluationModelKind      *string
	EvaluationModelSubKind   *string
	EvaluationModelAlgorithm *string
	EvaluationModelCode      *string
	EvaluationModelVersion   *string
	EvaluationModelTitle     *string
	PrimaryScoreKind         *string
	PrimaryScoreValue        *float64
	PrimaryScoreLabel        *string
	PrimaryScoreMax          *float64
	LevelCode                *string
	LevelLabel               *string
	Severity                 *string
	OriginType               string
	OriginID                 *string
	Status                   string
	TotalScore               *float64
	RiskLevel                *string
	SubmittedAt              *time.Time
	EvaluatedAt              *time.Time
	FailedAt                 *time.Time
	FailureReason            *string
}

type AssessmentReader interface {
	GetAssessment(ctx context.Context, id uint64) (*AssessmentRow, error)
	GetAssessmentByAnswerSheetID(ctx context.Context, answerSheetID uint64) (*AssessmentRow, error)
	ListAssessments(ctx context.Context, filter AssessmentFilter, page PageRequest) ([]AssessmentRow, int64, error)
}

type ScoreFactorRow struct {
	FactorCode   string
	FactorName   string
	RawScore     float64
	RiskLevel    string
	IsTotalScore bool
}

type ScoreRow struct {
	AssessmentID uint64
	TotalScore   float64
	RiskLevel    string
	FactorScores []ScoreFactorRow
}

type FactorTrendFilter struct {
	TesteeID   uint64
	FactorCode string
	Limit      int
}

// ScoreProjectionReader reads the mutable assessment_score query projection.
// It is suitable for analytical views such as trends, never as the canonical
// source for one Evaluation outcome.
type ScoreProjectionReader interface {
	GetScoreByAssessmentID(ctx context.Context, assessmentID uint64) (*ScoreRow, error)
	ListFactorTrend(ctx context.Context, filter FactorTrendFilter) ([]ScoreRow, error)
}
