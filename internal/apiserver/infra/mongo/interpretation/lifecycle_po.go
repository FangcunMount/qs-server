package interpretation

import (
	"time"

	base "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
)

// ReportGenerationPO stores the report-generation aggregate.
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

	GenerationID       uint64                   `bson:"generation_id"`
	Attempt            int                      `bson:"attempt"`
	Status             string                   `bson:"status"`
	Failure            *InterpretationFailurePO `bson:"failure,omitempty"`
	TraceID            string                   `bson:"trace_id,omitempty"`
	StartedAt          *time.Time               `bson:"started_at,omitempty"`
	LeaseExpiresAt     *time.Time               `bson:"lease_expires_at,omitempty"`
	FinishedAt         *time.Time               `bson:"finished_at,omitempty"`
	AttemptOrigin      string                   `bson:"attempt_origin,omitempty"`
	RetryDisposition   string                   `bson:"retry_disposition,omitempty"`
	NextAttemptAt      *time.Time               `bson:"next_attempt_at,omitempty"`
	PolicyMaxAttempts  int                      `bson:"policy_max_attempts,omitempty"`
	RetryPolicyVersion string                   `bson:"retry_policy_version,omitempty"`
	RetryEventID       string                   `bson:"retry_event_id,omitempty"`
	ActionRequestID    string                   `bson:"action_request_id,omitempty"`
	RecoveryCount      int                      `bson:"recovery_count,omitempty"`
	LastReclaimedAt    *time.Time               `bson:"last_reclaimed_at,omitempty"`
	ClaimHistory       []ClaimHistoryPO         `bson:"claim_history,omitempty"`
}

type ClaimHistoryPO struct {
	ReclaimedAt time.Time `bson:"reclaimed_at"`
	TraceID     string    `bson:"trace_id"`
}

func (InterpretationRunPO) CollectionName() string { return "interpretation_runs" }

// InterpretReportPO stores immutable successful reports.
type InterpretReportPO struct {
	base.BaseDocument `bson:",inline"`

	GenerationID         uint64    `bson:"generation_id"`
	OutcomeID            uint64    `bson:"outcome_id"`
	InterpretationRunID  uint64    `bson:"interpretation_run_id"`
	ReportType           string    `bson:"report_type"`
	TemplateVersion      string    `bson:"template_version"`
	BuilderIdentity      string    `bson:"builder_identity,omitempty"`
	ContentSchemaVersion string    `bson:"content_schema_version,omitempty"`
	GeneratedAt          time.Time `bson:"generated_at"`

	// Frozen Outcome correlation doubles as the query envelope. It is a value
	// snapshot, not an Assessment aggregate reference.
	OrgID        int64  `bson:"org_id"`
	AssessmentID uint64 `bson:"assessment_id"`
	TesteeID     uint64 `bson:"testee_id"`

	ScaleName           string                 `bson:"scale_name,omitempty"`
	ScaleCode           string                 `bson:"scale_code,omitempty"`
	Model               *ModelIdentityPO       `bson:"model,omitempty"`
	PrimaryScore        *ScoreValuePO          `bson:"primary_score,omitempty"`
	Level               *ResultLevelPO         `bson:"level,omitempty"`
	TotalScore          float64                `bson:"total_score"`
	RiskLevel           string                 `bson:"risk_level,omitempty"`
	Conclusion          string                 `bson:"conclusion,omitempty"`
	Dimensions          []DimensionInterpretPO `bson:"dimensions,omitempty"`
	Suggestions         []SuggestionPO         `bson:"suggestions,omitempty"`
	ModelExtra          *ModelExtraPO          `bson:"model_extra,omitempty"`
	PresentationProfile *PresentationProfilePO `bson:"presentation_profile,omitempty"`
}

func (InterpretReportPO) CollectionName() string { return "interpret_report_artifacts" }

const (
	ReportCatalogSourceArtifact = "artifact"
	ReportCatalogSourceArchive  = "archive"
)

// ReportCatalogPO is a compact, assessment-level query index. Report content
// remains in interpret_report_artifacts or archived_reports.
type ReportCatalogPO struct {
	AssessmentID uint64    `bson:"assessment_id"`
	OrgID        int64     `bson:"org_id"`
	TesteeID     uint64    `bson:"testee_id"`
	OutcomeID    uint64    `bson:"outcome_id,omitempty"`
	GenerationID uint64    `bson:"generation_id,omitempty"`
	SourceKind   string    `bson:"source_kind"`
	SourceID     uint64    `bson:"source_id"`
	ModelCode    string    `bson:"model_code,omitempty"`
	RiskLevel    string    `bson:"risk_level,omitempty"`
	SortAt       time.Time `bson:"sort_at"`
	SortReportID uint64    `bson:"sort_report_id"`
	UpdatedAt    time.Time `bson:"updated_at"`
}

func (ReportCatalogPO) CollectionName() string { return "report_query_catalog" }
