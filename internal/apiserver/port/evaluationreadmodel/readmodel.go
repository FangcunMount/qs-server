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
	DateFrom              *time.Time
	DateTo                *time.Time
}

type AssessmentRow struct {
	ID                   uint64
	OrgID                int64
	TesteeID             uint64
	QuestionnaireCode    string
	QuestionnaireVersion string
	AnswerSheetID        uint64
	MedicalScaleID       *uint64
	MedicalScaleCode     *string
	MedicalScaleName     *string
	OriginType           string
	OriginID             *string
	Status               string
	TotalScore           *float64
	RiskLevel            *string
	SubmittedAt          *time.Time
	InterpretedAt        *time.Time
	FailedAt             *time.Time
	FailureReason        *string
}

type LatestRiskFilter struct {
	OrgID     int64
	TesteeIDs []uint64
}

type LatestRiskQueueFilter struct {
	OrgID               int64
	TesteeIDs           []uint64
	RestrictToTesteeIDs bool
	RiskLevels          []string
}

type LatestRiskRow struct {
	AssessmentID uint64
	OrgID        int64
	TesteeID     uint64
	RiskLevel    string
	OccurredAt   time.Time
}

type LatestRiskPage struct {
	Items    []LatestRiskRow
	Total    int64
	Page     int
	PageSize int
}

type AssessmentReader interface {
	GetAssessment(ctx context.Context, id uint64) (*AssessmentRow, error)
	GetAssessmentByAnswerSheetID(ctx context.Context, answerSheetID uint64) (*AssessmentRow, error)
	ListAssessments(ctx context.Context, filter AssessmentFilter, page PageRequest) ([]AssessmentRow, int64, error)
}

type LatestRiskReader interface {
	ListLatestRisksByTesteeIDs(ctx context.Context, filter LatestRiskFilter) ([]LatestRiskRow, error)
	ListLatestRiskQueue(ctx context.Context, filter LatestRiskQueueFilter, page PageRequest) (LatestRiskPage, error)
}

type ScoreFactorRow struct {
	FactorCode   string
	FactorName   string
	RawScore     float64
	RiskLevel    string
	Conclusion   string
	Suggestion   string
	IsTotalScore bool
}

type ScoreRow struct {
	AssessmentID     uint64
	TotalScore       float64
	RiskLevel        string
	FactorScores     []ScoreFactorRow
	MedicalScaleID   *uint64
	MedicalScaleCode *string
}

type FactorTrendFilter struct {
	TesteeID   uint64
	FactorCode string
	Limit      int
}

type ScoreReader interface {
	GetScoreByAssessmentID(ctx context.Context, assessmentID uint64) (*ScoreRow, error)
	ListFactorTrend(ctx context.Context, filter FactorTrendFilter) ([]ScoreRow, error)
}

type ReportFilter struct {
	TesteeID     *uint64
	TesteeIDs    []uint64
	HighRiskOnly bool
	ScaleCode    string
	RiskLevel    *string
}

type ReportDimensionRow struct {
	FactorCode  string
	FactorName  string
	RawScore    float64
	MaxScore    *float64
	RiskLevel   string
	Description string
	Suggestion  string
}

type ReportSuggestionRow struct {
	Category   string
	Content    string
	FactorCode *string
}

type ReportRow struct {
	AssessmentID uint64
	ScaleName    string
	ScaleCode    string
	TotalScore   float64
	RiskLevel    string
	Conclusion   string
	Dimensions   []ReportDimensionRow
	Suggestions  []ReportSuggestionRow
	CreatedAt    time.Time
}

type ReportReader interface {
	GetReportByID(ctx context.Context, reportID uint64) (*ReportRow, error)
	GetReportByAssessmentID(ctx context.Context, assessmentID uint64) (*ReportRow, error)
	ListReports(ctx context.Context, filter ReportFilter, page PageRequest) ([]ReportRow, int64, error)
}
