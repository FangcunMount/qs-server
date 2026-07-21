package testee

import (
	"context"
	"time"
)

type Actor struct{ TesteeID uint64 }

type ListQuery struct {
	Page, PageSize                                                       int
	Status, ScaleCode, RiskLevel, ModelKind, ModelCode, DateFrom, DateTo string
	ModelKinds                                                           []string
}

type TrendQuery struct {
	FactorCode string
	Limit      int
}

type ModelIdentity struct {
	Kind, SubKind, Algorithm, Code, Version, Title, ProductChannel, AlgorithmFamily, DecisionKind string
}
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
	Model                                   ModelIdentity
	PrimaryScore                            *ScoreValue
	Level                                   *ResultLevel
	OriginType                              string
	OriginID                                *string
	Status                                  string
	SubmittedAt, FailedAt                   *time.Time
	FailureReason                           *string
}

type AssessmentList struct {
	Items                             []*Assessment
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

type Service interface {
	AuthorizeAssessment(context.Context, Actor, uint64) error
	GetAssessment(context.Context, Actor, uint64) (*Assessment, error)
	ListAssessments(context.Context, Actor, ListQuery) (*AssessmentList, error)
	GetScore(context.Context, Actor, uint64) (*Score, error)
	GetFactorTrend(context.Context, Actor, TrendQuery) (*FactorTrend, error)
	GetHighRiskFactors(context.Context, Actor, uint64) (*HighRiskFactors, error)
}
