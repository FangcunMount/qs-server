package scalereadmodel

import (
	"context"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// PageRequest describes a read-model page request.
type PageRequest struct {
	Page     int
	PageSize int
}

// ScaleFilter contains typed filters for scale list queries.
type ScaleFilter struct {
	Status   string
	Title    string
	Category string
}

// IsEmpty reports whether no optional scale filters were supplied.
func (f ScaleFilter) IsEmpty() bool {
	return f.Status == "" && f.Title == "" && f.Category == ""
}

// ScaleSummaryRow is a transport-neutral scale list row.
type ScaleSummaryRow struct {
	Code              string
	Title             string
	Description       string
	Category          string
	Stages            []string
	ApplicableAges    []string
	Reporters         []string
	Tags              []string
	QuestionnaireCode string
	QuestionCount     int32
	Status            string
	CreatedBy         meta.ID
	CreatedAt         time.Time
	UpdatedBy         meta.ID
	UpdatedAt         time.Time
}

// ScaleReader exposes scale read-model queries.
type ScaleReader interface {
	ListScales(ctx context.Context, filter ScaleFilter, page PageRequest) ([]ScaleSummaryRow, error)
	CountScales(ctx context.Context, filter ScaleFilter) (int64, error)
}

// ScaleFactorReader exposes factor read-model queries for future factor-only screens.
type ScaleFactorReader interface {
	ListFactors(ctx context.Context, scaleCode string) ([]ScaleFactorRow, error)
}

// ScaleFactorRow is a transport-neutral factor read row.
type ScaleFactorRow struct {
	Code            string
	Title           string
	FactorType      string
	IsTotalScore    bool
	IsShow          bool
	QuestionCodes   []string
	ScoringStrategy string
	ScoringParams   map[string]interface{}
	MaxScore        *float64
}
