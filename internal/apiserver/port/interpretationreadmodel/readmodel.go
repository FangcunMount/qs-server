// Package interpretationreadmodel defines the report query projection owned
// by Interpretation. It deliberately has no dependency on Evaluation read
// models: reports are Interpretation artifacts, not score projections.
package interpretationreadmodel

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var ErrReportNotFound = errors.New("interpretation report not found")

// CatalogDanglingSourceError identifies a catalog entry whose selected report
// body is missing. It is an internal consistency error, not a public message.
type CatalogDanglingSourceError struct {
	AssessmentID uint64
	SourceKind   string
	SourceID     uint64
}

func (e *CatalogDanglingSourceError) Error() string {
	return fmt.Sprintf("report catalog dangling source: assessment=%d source=%s/%d", e.AssessmentID, e.SourceKind, e.SourceID)
}

// CatalogSourceAssociationMismatchError identifies a catalog entry whose
// selected report body exists but disagrees on Assessment / Org / Testee
// association. It carries only identity and mismatched field names — never
// report body content.
type CatalogSourceAssociationMismatchError struct {
	AssessmentID     uint64
	SourceKind       string
	SourceID         uint64
	MismatchedFields []string
}

func (e *CatalogSourceAssociationMismatchError) Error() string {
	return fmt.Sprintf(
		"report catalog source association mismatch: assessment=%d source=%s/%d fields=%v",
		e.AssessmentID, e.SourceKind, e.SourceID, e.MismatchedFields,
	)
}

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
	DerivedScores  []ScoreValueRow
	Level          *ResultLevelRow
	NormReference  *NormReferenceRow
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
	AssessmentID        uint64
	ModelName           string
	ModelCode           string
	Model               ModelIdentityRow
	PrimaryScore        *ScoreValueRow
	Level               *ResultLevelRow
	TotalScore          float64
	RiskLevel           string
	Conclusion          string
	Dimensions          []ReportDimensionRow
	Suggestions         []ReportSuggestionRow
	ModelExtra          *ReportModelExtraRow
	PresentationProfile *PresentationProfileRow
	CreatedAt           time.Time
}

type CurrentReportMetadataStatus string

const (
	CurrentReportMetadataFound    CurrentReportMetadataStatus = "found"
	CurrentReportMetadataMissing  CurrentReportMetadataStatus = "missing"
	CurrentReportMetadataDangling CurrentReportMetadataStatus = "dangling"
	CurrentReportMetadataMismatch CurrentReportMetadataStatus = "mismatch"
)

type CurrentReportMetadata struct {
	AssessmentID     uint64
	Status           CurrentReportMetadataStatus
	CreatedAt        time.Time
	SourceKind       string
	SourceID         uint64
	MismatchedFields []string
}

type BatchReportMetadataReader interface {
	GetCurrentReportMetadataByAssessmentIDs(context.Context, []uint64) (map[uint64]CurrentReportMetadata, error)
}

type PresentationProfileRow struct {
	VisibleFactorCodes []string
	Source             string
}

type ModelIdentityRow struct {
	Kind         string
	Algorithm    string
	Code         string
	Version      string
	Title        string
	DecisionKind string
	// StaticOnly marks archived content that has no safe runtime identity.
	// It may be displayed but cannot be rebuilt or sent to a renderer.
	StaticOnly bool
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

type NormReferenceRow struct {
	ScoreKind    string
	Benchmark    float64
	TableVersion string
	FormVariant  string
	MinAgeMonths int
	MaxAgeMonths int
	Gender       string
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
	GetReportByAssessmentID(ctx context.Context, assessmentID uint64) (*ReportRow, error)
	ListReports(ctx context.Context, filter ReportFilter, page PageRequest) ([]ReportRow, int64, error)
}
