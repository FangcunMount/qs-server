package interpretation

import (
	"time"

	base "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
)

// ReportGenerationPO is kept in a distinct collection so legacy
// interpret_reports documents remain readable throughout the migration.
type ReportGenerationPO struct {
	base.BaseDocument `bson:",inline"`

	OutcomeID       uint64 `bson:"outcome_id"`
	ReportType      string `bson:"report_type"`
	TemplateVersion string `bson:"template_version"`
	Status          string `bson:"status"`
	LatestRunID     uint64 `bson:"latest_run_id,omitempty"`
	ReportID        uint64 `bson:"report_id,omitempty"`
	Version         uint64 `bson:"version"`
}

func (ReportGenerationPO) CollectionName() string { return "report_generations" }

type InterpretationFailurePO struct {
	Kind        string `bson:"kind"`
	Code        string `bson:"code"`
	SafeMessage string `bson:"safe_message"`
	Retryable   bool   `bson:"retryable"`
}

type InterpretationRunPO struct {
	base.BaseDocument `bson:",inline"`

	GenerationID uint64                    `bson:"generation_id"`
	Attempt      int                       `bson:"attempt"`
	Status       string                    `bson:"status"`
	Failure      *InterpretationFailurePO `bson:"failure,omitempty"`
	TraceID      string                    `bson:"trace_id,omitempty"`
	StartedAt    *time.Time                `bson:"started_at,omitempty"`
	FinishedAt   *time.Time                `bson:"finished_at,omitempty"`
}

func (InterpretationRunPO) CollectionName() string { return "interpretation_runs" }

// InterpretReportArtifactPO is the new immutable artifact collection. The
// old interpret_reports collection remains untouched until backfill and cutover.
type InterpretReportArtifactPO struct {
	base.BaseDocument `bson:",inline"`

	GenerationID        uint64 `bson:"generation_id"`
	OutcomeID           uint64 `bson:"outcome_id"`
	InterpretationRunID uint64 `bson:"interpretation_run_id"`
	ReportType          string `bson:"report_type"`
	TemplateVersion     string `bson:"template_version"`
	GeneratedAt         time.Time `bson:"generated_at"`

	// Frozen Outcome correlation doubles as the query envelope. It is a value
	// snapshot, not an Assessment aggregate reference.
	OrgID        int64  `bson:"org_id"`
	AssessmentID uint64 `bson:"assessment_id"`
	TesteeID     uint64 `bson:"testee_id"`

	ScaleName string `bson:"scale_name,omitempty"`
	ScaleCode string `bson:"scale_code,omitempty"`
	Model        *ModelIdentityPO `bson:"model,omitempty"`
	PrimaryScore *ScoreValuePO    `bson:"primary_score,omitempty"`
	Level        *ResultLevelPO   `bson:"level,omitempty"`
	TotalScore float64 `bson:"total_score"`
	RiskLevel string `bson:"risk_level,omitempty"`
	Conclusion string `bson:"conclusion,omitempty"`
	Dimensions  []DimensionInterpretPO `bson:"dimensions,omitempty"`
	Suggestions []SuggestionPO `bson:"suggestions,omitempty"`
	ModelExtra  *ModelExtraPO `bson:"model_extra,omitempty"`
}

func (InterpretReportArtifactPO) CollectionName() string { return "interpret_report_artifacts" }
