package aiexplanation

import (
	"time"

	domainevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
	domainoutput "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/output"
	domainprofile "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/profile"
	base "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type AssociationPO struct {
	OrgID        int64  `bson:"org_id"`
	AssessmentID uint64 `bson:"assessment_id"`
	TesteeID     uint64 `bson:"testee_id"`
}

type ActorRefPO struct {
	Kind string `bson:"kind"`
	ID   string `bson:"id"`
}

type ProfileRefPO struct {
	ID          string `bson:"id"`
	Version     string `bson:"version"`
	Fingerprint string `bson:"fingerprint"`
}

type PromptRefPO struct {
	TemplateID  string `bson:"template_id"`
	Version     string `bson:"version"`
	Fingerprint string `bson:"fingerprint"`
	GitBlobSHA  string `bson:"git_blob_sha"`
}

type ExecutionSpecPO struct {
	Route            string `bson:"route"`
	RouteRevision    string `bson:"route_revision"`
	ResolvedProvider string `bson:"resolved_provider"`
	ResolvedModel    string `bson:"resolved_model"`
	Fingerprint      string `bson:"fingerprint"`
}

type ProviderReceiptPO struct {
	InvocationID string `bson:"invocation_id"`
	RequestID    string `bson:"request_id,omitempty"`
	Provider     string `bson:"provider"`
	Model        string `bson:"model"`
	InputTokens  int64  `bson:"input_tokens"`
	OutputTokens int64  `bson:"output_tokens"`
	LatencyNanos int64  `bson:"latency_nanos"`
}

type GenerationPO struct {
	base.BaseDocument `bson:",inline"`

	SourceReportID           uint64          `bson:"source_report_id"`
	Audience                 string          `bson:"audience"`
	Profile                  ProfileRefPO    `bson:"profile"`
	InputSchema              string          `bson:"input_schema"`
	InputJSON                []byte          `bson:"input_json"`
	InputFingerprint         string          `bson:"input_fingerprint"`
	ExecutionSpecFingerprint string          `bson:"execution_spec_fingerprint"`
	Association              AssociationPO   `bson:"association"`
	RequestedBy              ActorRefPO      `bson:"requested_by"`
	Prompt                   PromptRefPO     `bson:"prompt"`
	ExecutionSpec            ExecutionSpecPO `bson:"execution_spec"`
	Status                   string          `bson:"status"`
	LatestRunID              uint64          `bson:"latest_run_id,omitempty"`
	ArtifactID               uint64          `bson:"artifact_id,omitempty"`
	Version                  uint64          `bson:"version"`
	ExpiresAt                *time.Time      `bson:"expires_at,omitempty"`
	RetentionPolicyVersion   string          `bson:"retention_policy_version,omitempty"`
}

func (GenerationPO) CollectionName() string { return "ai_explanation_generations" }

type FailurePO struct {
	Kind        string `bson:"kind"`
	Code        string `bson:"code"`
	SafeMessage string `bson:"safe_message"`
	Retryable   bool   `bson:"retryable"`
}

type ClaimRecordPO struct {
	ReclaimedAt time.Time `bson:"reclaimed_at"`
	TraceID     string    `bson:"trace_id"`
}

type RetryAuthorizationPO struct {
	ExpectedAttempt           int       `bson:"expected_attempt"`
	NextAttempt               int       `bson:"next_attempt"`
	Origin                    string    `bson:"origin"`
	RequestID                 string    `bson:"request_id"`
	EventID                   string    `bson:"event_id"`
	Actor                     string    `bson:"actor"`
	Reason                    string    `bson:"reason"`
	AcceptedResultUnknownRisk bool      `bson:"accepted_result_unknown_risk"`
	AuthorizedAt              time.Time `bson:"authorized_at"`
}

type RecoveryWakeupPO struct {
	EventID                string    `bson:"event_id"`
	ExpectedLeaseExpiresAt time.Time `bson:"expected_lease_expires_at"`
	InvocationPhase        string    `bson:"invocation_phase"`
	RequestedAt            time.Time `bson:"requested_at"`
}

type RunPO struct {
	base.BaseDocument `bson:",inline"`

	GenerationID           uint64                `bson:"generation_id"`
	Attempt                int                   `bson:"attempt"`
	Status                 string                `bson:"status"`
	Failure                *FailurePO            `bson:"failure,omitempty"`
	TraceID                string                `bson:"trace_id,omitempty"`
	StartedAt              *time.Time            `bson:"started_at,omitempty"`
	LeaseExpiresAt         *time.Time            `bson:"lease_expires_at,omitempty"`
	FinishedAt             *time.Time            `bson:"finished_at,omitempty"`
	Origin                 string                `bson:"origin"`
	InvocationID           string                `bson:"invocation_id,omitempty"`
	InvocationPhase        string                `bson:"invocation_phase"`
	DispatchStartedAt      *time.Time            `bson:"dispatch_started_at,omitempty"`
	Receipt                *ProviderReceiptPO    `bson:"receipt,omitempty"`
	RetryAuthorization     *RetryAuthorizationPO `bson:"retry_authorization,omitempty"`
	RecoveryWakeup         *RecoveryWakeupPO     `bson:"recovery_wakeup,omitempty"`
	ClaimHistory           []ClaimRecordPO       `bson:"claim_history,omitempty"`
	RecoveryCount          int                   `bson:"recovery_count"`
	LastReclaimedAt        *time.Time            `bson:"last_reclaimed_at,omitempty"`
	ExpiresAt              *time.Time            `bson:"expires_at,omitempty"`
	RetentionPolicyVersion string                `bson:"retention_policy_version,omitempty"`
}

func (RunPO) CollectionName() string { return "ai_explanation_runs" }

type SourceRefPO struct {
	ReportID             uint64        `bson:"report_id"`
	OutcomeID            uint64        `bson:"outcome_id"`
	Association          AssociationPO `bson:"association"`
	ReportType           string        `bson:"report_type"`
	TemplateVersion      string        `bson:"template_version"`
	ContentSchemaVersion string        `bson:"content_schema_version"`
	BuilderIdentity      string        `bson:"builder_identity"`
	ReportGeneratedAt    time.Time     `bson:"report_generated_at"`
}

type ValidationReceiptPO struct {
	SchemaValidatorVersion    string    `bson:"schema_validator_version"`
	ReferenceValidatorVersion string    `bson:"reference_validator_version"`
	ProfileValidatorVersion   string    `bson:"profile_validator_version"`
	SafetyValidatorVersion    string    `bson:"safety_validator_version"`
	ValidatedAt               time.Time `bson:"validated_at"`
}

type ArtifactPO struct {
	base.BaseDocument `bson:",inline"`

	GenerationID           uint64                `bson:"generation_id"`
	RunID                  uint64                `bson:"run_id"`
	Source                 SourceRefPO           `bson:"source"`
	Audience               string                `bson:"audience"`
	Profile                ProfileRefPO          `bson:"profile"`
	Prompt                 PromptRefPO           `bson:"prompt"`
	ExecutionSpec          ExecutionSpecPO       `bson:"execution_spec"`
	InputSchema            string                `bson:"input_schema"`
	InputFingerprint       string                `bson:"input_fingerprint"`
	OutputSchema           string                `bson:"output_schema"`
	SafetyPolicy           string                `bson:"safety_policy"`
	ProviderReceipt        ProviderReceiptPO     `bson:"provider_receipt"`
	Validation             ValidationReceiptPO   `bson:"validation"`
	Content                *domainoutput.Content `bson:"content"`
	GeneratedAt            time.Time             `bson:"generated_at"`
	ExpiresAt              *time.Time            `bson:"expires_at,omitempty"`
	RetentionPolicyVersion string                `bson:"retention_policy_version,omitempty"`
}

func (ArtifactPO) CollectionName() string { return "ai_explanation_artifacts" }

type ProfilePO struct {
	base.BaseDocument `bson:",inline"`

	Definition             domainprofile.Definition `bson:"definition"`
	Fingerprint            string                   `bson:"fingerprint"`
	SelectorSlotKey        string                   `bson:"selector_slot_key"`
	Status                 string                   `bson:"status"`
	CreatedBy              string                   `bson:"created_by,omitempty"`
	CreatedReason          string                   `bson:"created_reason,omitempty"`
	PublishedAt            *time.Time               `bson:"published_at,omitempty"`
	PublishedBy            string                   `bson:"published_by,omitempty"`
	PublishedReason        string                   `bson:"published_reason,omitempty"`
	PublishedEvidenceRunID uint64                   `bson:"published_evidence_run_id,omitempty"`
	DisabledAt             *time.Time               `bson:"disabled_at,omitempty"`
	DisabledBy             string                   `bson:"disabled_by,omitempty"`
	DisabledReason         string                   `bson:"disabled_reason,omitempty"`
}

func (ProfilePO) CollectionName() string { return "ai_explanation_profiles" }

type EvaluationSuiteRefPO struct {
	ID          string `bson:"id"`
	Version     string `bson:"version"`
	Fingerprint string `bson:"fingerprint"`
	GitBlobSHA  string `bson:"git_blob_sha"`
}

type EvaluationSchemaRefPO struct {
	Version     string `bson:"version"`
	Fingerprint string `bson:"fingerprint"`
}

type EvaluationDecodingPO struct {
	MaxOutputTokens int      `bson:"max_output_tokens"`
	Temperature     *float64 `bson:"temperature,omitempty"`
	TopP            *float64 `bson:"top_p,omitempty"`
	Seed            *int64   `bson:"seed,omitempty"`
	ReasoningEffort string   `bson:"reasoning_effort,omitempty"`
}

type EvaluationSemanticEvaluatorSpecPO struct {
	Version      string                `bson:"version"`
	Prompt       PromptRefPO           `bson:"prompt"`
	OutputSchema EvaluationSchemaRefPO `bson:"output_schema"`
	Provider     ExecutionSpecPO       `bson:"provider"`
	Decoding     EvaluationDecodingPO  `bson:"decoding"`
}

type EvaluationReleasePO struct {
	Suite                    EvaluationSuiteRefPO              `bson:"suite"`
	Prompt                   PromptRefPO                       `bson:"prompt"`
	Profile                  ProfileRefPO                      `bson:"profile"`
	InputSchema              EvaluationSchemaRefPO             `bson:"input_schema"`
	OutputSchema             EvaluationSchemaRefPO             `bson:"output_schema"`
	Provider                 ExecutionSpecPO                   `bson:"provider"`
	Decoding                 EvaluationDecodingPO              `bson:"decoding"`
	SemanticEvaluator        EvaluationSemanticEvaluatorSpecPO `bson:"semantic_evaluator"`
	GenerationCaseIDs        []string                          `bson:"generation_case_ids"`
	PreflightCaseID          string                            `bson:"preflight_case_id"`
	PreflightRejectionReason string                            `bson:"preflight_rejection_reason"`
	RepetitionsPerCase       int                               `bson:"repetitions_per_case"`
}

type EvaluationAssertionPO struct {
	Type      string `bson:"type"`
	Scope     string `bson:"scope"`
	Ordinal   int    `bson:"ordinal"`
	Hard      bool   `bson:"hard"`
	Evaluator string `bson:"evaluator"`
	Status    string `bson:"status"`
	Detail    string `bson:"detail,omitempty"`
}

type EvaluationSemanticScoresPO struct {
	Faithfulness            int `bson:"faithfulness"`
	CrossDimensionQuality   int `bson:"cross_dimension_quality"`
	SuggestionActionability int `bson:"suggestion_actionability"`
	AudienceClarity         int `bson:"audience_clarity"`
	Concision               int `bson:"concision"`
}

type EvaluationSemanticPO struct {
	EvaluatorVersion string                     `bson:"evaluator_version"`
	ProviderReceipt  ProviderReceiptPO          `bson:"provider_receipt"`
	Scores           EvaluationSemanticScoresPO `bson:"scores"`
	Rationale        string                     `bson:"rationale"`
}

type EvaluationAttemptFailurePO struct {
	Stage         string `bson:"stage"`
	Code          string `bson:"code"`
	SafeMessage   string `bson:"safe_message"`
	Retryable     bool   `bson:"retryable"`
	ResultUnknown bool   `bson:"result_unknown"`
}

type EvaluationSemanticExecutionPO struct {
	InvocationID        string                      `bson:"invocation_id"`
	EvaluatorVersion    string                      `bson:"evaluator_version"`
	StartedAt           time.Time                   `bson:"started_at"`
	FinishedAt          time.Time                   `bson:"finished_at"`
	ProviderCallCount   int                         `bson:"provider_call_count"`
	ProviderReceipt     *ProviderReceiptPO          `bson:"provider_receipt,omitempty"`
	RawOutput           []byte                      `bson:"raw_output,omitempty"`
	NormalizedOutput    []byte                      `bson:"normalized_output,omitempty"`
	ProviderFailureCode string                      `bson:"provider_failure_code,omitempty"`
	Failure             *EvaluationAttemptFailurePO `bson:"failure,omitempty"`
}

type EvaluationAttemptPO struct {
	CaseID            string                         `bson:"case_id"`
	Attempt           int                            `bson:"attempt"`
	Stage             string                         `bson:"stage"`
	StartedAt         time.Time                      `bson:"started_at"`
	FinishedAt        time.Time                      `bson:"finished_at"`
	ProviderCallCount int                            `bson:"provider_call_count"`
	ProviderReceipt   *ProviderReceiptPO             `bson:"provider_receipt,omitempty"`
	RawOutput         []byte                         `bson:"raw_output,omitempty"`
	NormalizedOutput  []byte                         `bson:"normalized_output,omitempty"`
	OutputFingerprint string                         `bson:"output_fingerprint,omitempty"`
	RejectionReason   string                         `bson:"rejection_reason,omitempty"`
	Failure           *EvaluationAttemptFailurePO    `bson:"failure,omitempty"`
	Assertions        []EvaluationAssertionPO        `bson:"assertions"`
	Semantic          *EvaluationSemanticPO          `bson:"semantic,omitempty"`
	SemanticExecution *EvaluationSemanticExecutionPO `bson:"semantic_execution,omitempty"`
}

type EvaluationHumanReviewPO struct {
	CaseID     string    `bson:"case_id"`
	Attempt    int       `bson:"attempt"`
	Role       string    `bson:"role"`
	Reviewer   string    `bson:"reviewer"`
	Decision   string    `bson:"decision"`
	ReviewedAt time.Time `bson:"reviewed_at"`
	Reason     string    `bson:"reason"`
}

type EvaluationAttemptExecutionPO struct {
	CaseID            string     `bson:"case_id"`
	Attempt           int        `bson:"attempt"`
	Owner             string     `bson:"owner"`
	InvocationID      string     `bson:"invocation_id"`
	Phase             string     `bson:"phase"`
	ClaimedAt         time.Time  `bson:"claimed_at"`
	LeaseExpiresAt    time.Time  `bson:"lease_expires_at"`
	DispatchStartedAt *time.Time `bson:"dispatch_started_at,omitempty"`
}

type EvaluationRecoveryRequestPO struct {
	ID          string    `bson:"id"`
	CaseID      string    `bson:"case_id"`
	Attempt     int       `bson:"attempt"`
	Actor       string    `bson:"actor"`
	Reason      string    `bson:"reason"`
	RequestedAt time.Time `bson:"requested_at"`
}

type EvaluationGateReasonPO struct {
	Code    string `bson:"code"`
	CaseID  string `bson:"case_id,omitempty"`
	Attempt int    `bson:"attempt,omitempty"`
	Detail  string `bson:"detail"`
}

type EvaluationQualityMetricsPO struct {
	GenerationAttempts     int     `bson:"generation_attempts"`
	CaseAssertionPasses    int     `bson:"case_assertion_passes"`
	FaithfulnessAverage    float64 `bson:"faithfulness_average"`
	CrossDimensionAverage  float64 `bson:"cross_dimension_average"`
	ActionabilityAverage   float64 `bson:"actionability_average"`
	AudienceClarityAverage float64 `bson:"audience_clarity_average"`
	ConcisionAverage       float64 `bson:"concision_average"`
	HumanReviews           int     `bson:"human_reviews"`
}

type EvaluationGatePO struct {
	Passed  bool                       `bson:"passed"`
	Reasons []EvaluationGateReasonPO   `bson:"reasons"`
	Metrics EvaluationQualityMetricsPO `bson:"metrics"`
}

type PromptEvaluationRunPO struct {
	base.BaseDocument `bson:",inline"`

	Release                EvaluationReleasePO           `bson:"release"`
	ActiveReleaseKey       string                        `bson:"active_release_key,omitempty"`
	ActiveExecutionOrgKey  string                        `bson:"active_execution_org_key,omitempty"`
	Status                 string                        `bson:"status"`
	Version                int64                         `bson:"version"`
	Attempts               []EvaluationAttemptPO         `bson:"attempts"`
	Reviews                []EvaluationHumanReviewPO     `bson:"reviews"`
	Execution              *EvaluationAttemptExecutionPO `bson:"execution,omitempty"`
	Recoveries             []EvaluationRecoveryRequestPO `bson:"recoveries,omitempty"`
	RequestedOrgID         int64                         `bson:"requested_org_id,omitempty"`
	RequestedBy            string                        `bson:"requested_by,omitempty"`
	RequestReason          string                        `bson:"request_reason,omitempty"`
	ClosedAt               *time.Time                    `bson:"closed_at,omitempty"`
	FinalizedAt            *time.Time                    `bson:"finalized_at,omitempty"`
	FinalizedBy            string                        `bson:"finalized_by,omitempty"`
	FinalReason            string                        `bson:"final_reason,omitempty"`
	Gate                   *EvaluationGatePO             `bson:"gate,omitempty"`
	CanceledAt             *time.Time                    `bson:"canceled_at,omitempty"`
	CanceledBy             string                        `bson:"canceled_by,omitempty"`
	CancelReason           string                        `bson:"cancel_reason,omitempty"`
	ExpiresAt              *time.Time                    `bson:"expires_at,omitempty"`
	RetentionPolicyVersion string                        `bson:"retention_policy_version,omitempty"`
}

func (PromptEvaluationRunPO) CollectionName() string {
	return "ai_explanation_prompt_evaluations"
}

type EvaluationV2ExecutionCheckpointPO struct {
	ID                string     `bson:"id"`
	Kind              string     `bson:"kind"`
	CaseID            string     `bson:"case_id"`
	SlotOrdinal       int        `bson:"slot_ordinal"`
	CandidateID       string     `bson:"candidate_id,omitempty"`
	ExecutionOrdinal  int        `bson:"execution_ordinal"`
	Owner             string     `bson:"owner"`
	InvocationID      string     `bson:"invocation_id"`
	Phase             string     `bson:"phase"`
	ClaimedAt         time.Time  `bson:"claimed_at"`
	LeaseExpiresAt    time.Time  `bson:"lease_expires_at"`
	DispatchStartedAt *time.Time `bson:"dispatch_started_at,omitempty"`
}

const PromptEvaluationEvidenceVersionV2 = "v2"

// PromptEvaluationEvidenceV2PO deliberately remains in the existing Run
// collection. evidence_version is the fail-closed discriminator; v1 readers
// exclude documents that contain it.
type PromptEvaluationEvidenceV2PO struct {
	base.BaseDocument `bson:",inline"`

	EvidenceVersion              string                                          `bson:"evidence_version"`
	SchemaVersion                string                                          `bson:"schema_version"`
	Release                      domainevaluation.EvidenceReleaseIdentity        `bson:"release"`
	ReleaseFingerprint           string                                          `bson:"release_fingerprint"`
	ExecutionPolicy              domainevaluation.EvaluationExecutionPolicy      `bson:"execution_policy"`
	GatePolicy                   domainevaluation.ReleaseGatePolicy              `bson:"gate_policy"`
	Status                       string                                          `bson:"status"`
	Version                      int64                                           `bson:"version"`
	PreflightEvidence            []domainevaluation.PreflightCaseEvidence        `bson:"preflight_evidence"`
	Slots                        []domainevaluation.CandidateSlot                `bson:"slots"`
	GenerationExecutions         []domainevaluation.CandidateGenerationExecution `bson:"generation_executions"`
	SemanticExecutions           []domainevaluation.SemanticEvaluationExecution  `bson:"semantic_executions"`
	HumanReviews                 []domainevaluation.CandidateHumanReview         `bson:"human_reviews"`
	UnresolvedResultUnknownCount int                                             `bson:"unresolved_result_unknown_count"`
	ResultUnknownResolutions     []domainevaluation.ResultUnknownResolution      `bson:"result_unknown_resolutions"`
	StateTransitions             []domainevaluation.EvidenceStateTransition      `bson:"state_transitions"`
	GateResult                   *domainevaluation.EvidenceGateResult            `bson:"gate_result,omitempty"`
	Audit                        domainevaluation.EvidenceRunAudit               `bson:"audit"`
	Execution                    *EvaluationV2ExecutionCheckpointPO              `bson:"execution,omitempty"`
	ActiveReleaseKey             string                                          `bson:"active_release_key,omitempty"`
	ActiveExecutionOrgKey        string                                          `bson:"active_execution_org_key,omitempty"`
	RequestedOrgID               int64                                           `bson:"requested_org_id"`
	ClosedAt                     *time.Time                                      `bson:"closed_at,omitempty"`
	FinalizedAt                  *time.Time                                      `bson:"finalized_at,omitempty"`
	CanceledAt                   *time.Time                                      `bson:"canceled_at,omitempty"`
	ExpiresAt                    *time.Time                                      `bson:"expires_at,omitempty"`
	RetentionPolicyVersion       string                                          `bson:"retention_policy_version,omitempty"`
}

func (PromptEvaluationEvidenceV2PO) CollectionName() string {
	return (PromptEvaluationRunPO{}).CollectionName()
}

type PromptEvaluationRecheckPO struct {
	base.BaseDocument `bson:",inline"`

	SourceRunID            meta.ID                       `bson:"source_run_id"`
	SourceCaseID           string                        `bson:"source_case_id"`
	SourceAttempt          int                           `bson:"source_attempt"`
	ActiveSourceKey        string                        `bson:"active_source_key,omitempty"`
	Release                EvaluationReleasePO           `bson:"release"`
	Status                 string                        `bson:"status"`
	Version                int64                         `bson:"version"`
	Execution              *EvaluationAttemptExecutionPO `bson:"execution,omitempty"`
	Result                 *EvaluationAttemptPO          `bson:"result,omitempty"`
	RequestedOrgID         int64                         `bson:"requested_org_id"`
	RequestedBy            string                        `bson:"requested_by"`
	Reason                 string                        `bson:"reason"`
	FinishedAt             *time.Time                    `bson:"finished_at,omitempty"`
	ExpiresAt              *time.Time                    `bson:"expires_at,omitempty"`
	RetentionPolicyVersion string                        `bson:"retention_policy_version,omitempty"`
}

func (PromptEvaluationRecheckPO) CollectionName() string {
	return "ai_explanation_prompt_evaluation_rechecks"
}

type PromptEvaluationBudgetReservationPO struct {
	RunID               meta.ID   `bson:"run_id"`
	RequestedBy         string    `bson:"requested_by"`
	ProviderInvocations int       `bson:"provider_invocations"`
	ReservedAt          time.Time `bson:"reserved_at"`
}

type PromptEvaluationDailyBudgetPO struct {
	base.BaseDocument `bson:",inline"`

	OrgID                       int64                                 `bson:"org_id"`
	BudgetDay                   time.Time                             `bson:"budget_day"`
	ReservedProviderInvocations int                                   `bson:"reserved_provider_invocations"`
	Reservations                []PromptEvaluationBudgetReservationPO `bson:"reservations"`
	ExpiresAt                   *time.Time                            `bson:"expires_at,omitempty"`
	RetentionPolicyVersion      string                                `bson:"retention_policy_version,omitempty"`
}

func (PromptEvaluationDailyBudgetPO) CollectionName() string {
	return "ai_explanation_prompt_evaluation_daily_budgets"
}

type ParticipantBudgetReservationPO struct {
	ReservationID       string    `bson:"reservation_id"`
	GenerationID        meta.ID   `bson:"generation_id"`
	Attempt             int       `bson:"attempt"`
	Origin              string    `bson:"origin"`
	UserID              string    `bson:"user_id"`
	AssessmentID        meta.ID   `bson:"assessment_id"`
	ProviderInvocations int       `bson:"provider_invocations"`
	ReservedAt          time.Time `bson:"reserved_at"`
}

type ParticipantDailyBudgetPO struct {
	base.BaseDocument `bson:",inline"`

	OrgID                       int64                            `bson:"org_id"`
	BudgetDay                   time.Time                        `bson:"budget_day"`
	ReservedProviderInvocations int                              `bson:"reserved_provider_invocations"`
	RedactedProviderInvocations int                              `bson:"redacted_provider_invocations,omitempty"`
	Reservations                []ParticipantBudgetReservationPO `bson:"reservations"`
	ExpiresAt                   *time.Time                       `bson:"expires_at,omitempty"`
	RetentionPolicyVersion      string                           `bson:"retention_policy_version,omitempty"`
}

func (ParticipantDailyBudgetPO) CollectionName() string {
	return "ai_explanation_participant_daily_budgets"
}

type ParticipantActiveCapacityReservationPO struct {
	GenerationID meta.ID   `bson:"generation_id"`
	RunID        meta.ID   `bson:"run_id"`
	UserID       string    `bson:"user_id"`
	AssessmentID meta.ID   `bson:"assessment_id"`
	AcquiredAt   time.Time `bson:"acquired_at"`
}

type ParticipantActiveCapacityPO struct {
	base.BaseDocument `bson:",inline"`

	OrgID            int64                                    `bson:"org_id"`
	ActiveExecutions int                                      `bson:"active_executions"`
	Reservations     []ParticipantActiveCapacityReservationPO `bson:"reservations"`
}

func (ParticipantActiveCapacityPO) CollectionName() string {
	return "ai_explanation_participant_active_capacity"
}
