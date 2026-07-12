// Package interpretationreadmodel defines the report query projection owned
// by Interpretation. It deliberately has no dependency on Evaluation read
// models: reports are Interpretation artifacts, not score projections.
package interpretationreadmodel

import (
	"context"
	"errors"
	"time"
)

var ErrReportNotFound = errors.New("interpretation report not found")

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

type ReportFilter struct {
	OrgID        *int64
	TesteeID     *uint64
	TesteeIDs    []uint64
	HighRiskOnly bool
	ModelCode    string
	RiskLevel    *string
}

type ReportDimensionRow struct {
	FactorCode     string
	FactorName     string
	RawScore       float64
	MaxScore       *float64
	RiskLevel      string
	Role           string
	ParentCode     string
	HierarchyLevel int
	SortOrder      int
	Description    string
	Suggestion     string
}

type ReportSuggestionRow struct {
	Category   string
	Content    string
	FactorCode *string
}

type ReportRow struct {
	AssessmentID uint64
	ModelName    string
	ModelCode    string
	Model        ModelIdentityRow
	PrimaryScore *ScoreValueRow
	Level        *ResultLevelRow
	TotalScore   float64
	RiskLevel    string
	Conclusion   string
	Dimensions   []ReportDimensionRow
	Suggestions  []ReportSuggestionRow
	ModelExtra   *ReportModelExtraRow
	CreatedAt    time.Time
}

type ModelIdentityRow struct {
	Kind            string
	SubKind         string
	Algorithm       string
	Code            string
	Version         string
	Title           string
	ProductChannel  string
	AlgorithmFamily string
}

type ScoreValueRow struct {
	Kind  string
	Value float64
	Label string
	Max   *float64
}

type ResultLevelRow struct {
	Code     string
	Label    string
	Severity string
}

type ReportModelExtraRow struct {
	Kind           string
	TypeCode       string
	TypeName       string
	OneLiner       string
	ImageURL       string
	MatchPercent   float64
	IsSpecial      bool
	SpecialTrigger string
	Commentary     string
	Rarity         *ReportModelRarityRow
}

type ReportModelRarityRow struct {
	Percent float64
	Label   string
	OneInX  int
}

type ReportReader interface {
	GetReportByID(ctx context.Context, reportID uint64) (*ReportRow, error)
	GetReportByAssessmentID(ctx context.Context, assessmentID uint64) (*ReportRow, error)
	ListReports(ctx context.Context, filter ReportFilter, page PageRequest) ([]ReportRow, int64, error)
}
