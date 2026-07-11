package interpretation

import (
	"context"
	"time"
)

// ListReportsDTO describes a report list query and its optional access scope.
type ListReportsDTO struct {
	TesteeID              uint64
	Page                  int
	PageSize              int
	AccessibleTesteeIDs   []uint64
	RestrictToAccessScope bool
}

type ModelIdentityResult struct {
	Kind            string `json:"kind"`
	SubKind         string `json:"sub_kind,omitempty"`
	Algorithm       string `json:"algorithm,omitempty"`
	Code            string `json:"code"`
	Version         string `json:"version,omitempty"`
	Title           string `json:"title,omitempty"`
	ProductChannel  string `json:"product_channel,omitempty"`
	AlgorithmFamily string `json:"algorithm_family,omitempty"`
}

type ScoreValueResult struct {
	Kind  string   `json:"kind"`
	Value float64  `json:"value"`
	Label string   `json:"label,omitempty"`
	Max   *float64 `json:"max,omitempty"`
}

type ResultLevelResult struct {
	Code     string `json:"code"`
	Label    string `json:"label"`
	Severity string `json:"severity,omitempty"`
}

type ReportResult struct {
	AssessmentID uint64
	ModelName    string
	ModelCode    string
	TotalScore   float64
	RiskLevel    string
	Conclusion   string
	Dimensions   []DimensionResult
	Suggestions  []SuggestionDTO
	CreatedAt    time.Time
	ModelExtra   *ModelExtraResult
}

type ReportListResult struct {
	Items      []*ReportResult
	Total      int
	Page       int
	PageSize   int
	TotalPages int
}

type ReportOutcomeResult struct {
	AssessmentID uint64              `json:"assessment_id"`
	Model        ModelIdentityResult `json:"model"`
	PrimaryScore *ScoreValueResult   `json:"primary_score,omitempty"`
	Level        *ResultLevelResult  `json:"level,omitempty"`
	Conclusion   string              `json:"conclusion"`
	Dimensions   []DimensionResult   `json:"dimensions"`
	Suggestions  []SuggestionDTO     `json:"suggestions"`
	ModelExtra   *ModelExtraResult   `json:"model_extra,omitempty"`
	CreatedAt    time.Time           `json:"created_at"`
}

type ReportOutcomeListResult struct {
	Items      []*ReportOutcomeResult `json:"items"`
	Total      int                    `json:"total"`
	Page       int                    `json:"page"`
	PageSize   int                    `json:"page_size"`
	TotalPages int                    `json:"total_pages"`
}

type ModelExtraResult struct {
	Kind           string             `json:"kind,omitempty"`
	TypeCode       string             `json:"type_code,omitempty"`
	TypeName       string             `json:"type_name,omitempty"`
	OneLiner       string             `json:"one_liner,omitempty"`
	ImageURL       string             `json:"image_url,omitempty"`
	MatchPercent   float64            `json:"match_percent,omitempty"`
	IsSpecial      bool               `json:"is_special,omitempty"`
	SpecialTrigger string             `json:"special_trigger,omitempty"`
	Commentary     string             `json:"commentary,omitempty"`
	Rarity         *ModelRarityResult `json:"rarity,omitempty"`
}

type ModelRarityResult struct {
	Percent float64 `json:"percent,omitempty"`
	Label   string  `json:"label,omitempty"`
	OneInX  int     `json:"one_in_x,omitempty"`
}

type DimensionResult struct {
	FactorCode     string
	FactorName     string
	RawScore       float64
	MaxScore       *float64
	RiskLevel      string
	Role           string `json:"role,omitempty"`
	ParentCode     string `json:"parent_code,omitempty"`
	HierarchyLevel int    `json:"hierarchy_level,omitempty"`
	SortOrder      int    `json:"sort_order,omitempty"`
	Description    string
	Suggestion     string
}

type SuggestionDTO struct {
	Category   string
	Content    string
	FactorCode *string
}

// ReportQueryService serves report viewers from Interpretation-owned projections.
type ReportQueryService interface {
	GetByAssessmentID(ctx context.Context, assessmentID uint64) (*ReportResult, error)
	ListByTesteeID(ctx context.Context, dto ListReportsDTO) (*ReportListResult, error)
	GetOutcomeByAssessmentID(ctx context.Context, assessmentID uint64) (*ReportOutcomeResult, error)
	ListOutcomeByTesteeID(ctx context.Context, dto ListReportsDTO) (*ReportOutcomeListResult, error)
}
