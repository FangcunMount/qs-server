package aiexplanation

import (
	"context"
	"errors"
)

var ErrDisabled = errors.New("AI explanation service is disabled")

type EvidenceRef struct {
	Kind string `json:"kind" enums:"dimension,overall_result,model_result,standard_suggestion"`
	Ref  string `json:"ref"`
}

type IntegratedInsight struct {
	Kind         string        `json:"kind" enums:"reinforcing_pattern,contrasting_pattern,combined_strength,combined_attention,context_dependent_pattern"`
	Title        string        `json:"title"`
	Content      string        `json:"content"`
	WhyItMatters string        `json:"why_it_matters"`
	EvidenceRefs []EvidenceRef `json:"evidence_refs"`
}

type Suggestion struct {
	Origin               string        `json:"origin" enums:"standard_derived,generated_low_risk"`
	Category             string        `json:"category"`
	Title                string        `json:"title"`
	Goal                 string        `json:"goal"`
	Actions              []string      `json:"actions"`
	Rationale            string        `json:"rationale"`
	EvidenceRefs         []EvidenceRef `json:"evidence_refs"`
	SourceSuggestionRefs []string      `json:"source_suggestion_refs"`
	Caution              string        `json:"caution,omitempty"`
}

type Content struct {
	SchemaVersion      string              `json:"schema_version" example:"ai-explanation-output/v1"`
	Summary            string              `json:"summary"`
	IntegratedInsights []IntegratedInsight `json:"integrated_insights"`
	Suggestions        []Suggestion        `json:"suggestions"`
	Limitations        []string            `json:"limitations"`
}

type Failure struct {
	Code        string `json:"code"`
	SafeMessage string `json:"safe_message"`
	Retryable   bool   `json:"retryable"`
}

type Output struct {
	Status         string   `json:"status" enums:"ready,not_ready,not_applicable,pending,generating,generated,failed"`
	ReasonCode     string   `json:"reason_code,omitempty"`
	GenerationID   string   `json:"generation_id,omitempty"`
	ArtifactID     string   `json:"artifact_id,omitempty"`
	SourceReportID string   `json:"source_report_id,omitempty"`
	SourceState    string   `json:"source_state" enums:"current,stale,unavailable,unknown"`
	Content        *Content `json:"content,omitempty"`
	Failure        *Failure `json:"failure,omitempty"`
	CreatedAt      string   `json:"created_at,omitempty"`
	UpdatedAt      string   `json:"updated_at,omitempty"`
}

type ExportSourceReceipt struct {
	AssessmentID         string `json:"assessment_id"`
	ReportID             string `json:"report_id"`
	OutcomeID            string `json:"outcome_id"`
	ReportType           string `json:"report_type"`
	TemplateVersion      string `json:"template_version"`
	ContentSchemaVersion string `json:"content_schema_version"`
	BuilderIdentity      string `json:"builder_identity"`
	ReportGeneratedAt    string `json:"report_generated_at"`
}

type ExportReleaseReceipt struct {
	ProfileID                 string `json:"profile_id"`
	ProfileVersion            string `json:"profile_version"`
	ProfileFingerprint        string `json:"profile_fingerprint"`
	PromptTemplateID          string `json:"prompt_template_id"`
	PromptVersion             string `json:"prompt_version"`
	PromptFingerprint         string `json:"prompt_fingerprint"`
	PromptGitBlobSHA          string `json:"prompt_git_blob_sha"`
	ProviderRoute             string `json:"provider_route"`
	ProviderRouteRevision     string `json:"provider_route_revision"`
	ResolvedProvider          string `json:"resolved_provider"`
	ResolvedModel             string `json:"resolved_model"`
	ExecutionSpecFingerprint  string `json:"execution_spec_fingerprint"`
	InputSchema               string `json:"input_schema"`
	OutputSchema              string `json:"output_schema"`
	SafetyPolicy              string `json:"safety_policy"`
	SchemaValidatorVersion    string `json:"schema_validator_version"`
	ReferenceValidatorVersion string `json:"reference_validator_version"`
	ProfileValidatorVersion   string `json:"profile_validator_version"`
	SafetyValidatorVersion    string `json:"safety_validator_version"`
	ValidatedAt               string `json:"validated_at"`
}

type ExportItem struct {
	GenerationID string               `json:"generation_id"`
	ArtifactID   string               `json:"artifact_id"`
	Source       ExportSourceReceipt  `json:"source"`
	Release      ExportReleaseReceipt `json:"release"`
	Content      Content              `json:"content"`
	GeneratedAt  string               `json:"generated_at"`
}

type ExportPage struct {
	SchemaVersion string       `json:"schema_version"`
	OrgID         uint64       `json:"org_id"`
	TesteeID      uint64       `json:"testee_id"`
	ExportedAt    string       `json:"exported_at"`
	SnapshotAt    string       `json:"snapshot_at"`
	Items         []ExportItem `json:"items"`
	NextCursor    string       `json:"next_cursor,omitempty"`
}

type Client interface {
	GetCapability(ctx context.Context, testeeID, assessmentID uint64, locale string, focusAreas []string) (*Output, error)
	Request(ctx context.Context, testeeID, assessmentID uint64, locale string, focusAreas []string) (*Output, error)
	Get(ctx context.Context, testeeID, assessmentID uint64, generationID string) (*Output, error)
	Export(ctx context.Context, testeeID uint64, pageSize int, cursor string) (*ExportPage, error)
}
