package eventpayload

import "time"

const (
	AIExplanationPromptEvaluationEvidenceVersionV2       = "v2"
	AIExplanationPromptEvaluationExecutionKindGeneration = "generation"
	AIExplanationPromptEvaluationExecutionKindSemantic   = "semantic"
	AIExplanationPromptEvaluationRequiredSlotsPerCase    = 5
	AIExplanationPromptEvaluationMaxExecutionsPerTarget  = 2
)

// AIExplanationRequestedData is the minimal durable wake-up fact for one
// manually requested AI explanation. Frozen input, Prompt and provider details
// stay in the Generation aggregate and never travel through MQ.
type AIExplanationRequestedData struct {
	OrgID          int64     `json:"org_id"`
	GenerationID   string    `json:"generation_id"`
	AssessmentID   string    `json:"assessment_id"`
	TesteeID       uint64    `json:"testee_id"`
	SourceReportID string    `json:"source_report_id"`
	Audience       string    `json:"audience"`
	RequestedAt    time.Time `json:"requested_at"`
}

// AIExplanationRetryRequestedData is the durable address of one explicitly
// authorized next attempt. The authorization reason and participant facts stay
// in Mongo; Worker receives only the facts needed to prove the wake-up.
type AIExplanationRetryRequestedData struct {
	OrgID           int64     `json:"org_id"`
	GenerationID    string    `json:"generation_id"`
	FailedRunID     string    `json:"failed_run_id"`
	AssessmentID    string    `json:"assessment_id"`
	TesteeID        uint64    `json:"testee_id"`
	SourceReportID  string    `json:"source_report_id"`
	Audience        string    `json:"audience"`
	ExpectedAttempt int       `json:"expected_attempt"`
	NextAttempt     int       `json:"next_attempt"`
	AttemptOrigin   string    `json:"attempt_origin"`
	ActionRequestID string    `json:"action_request_id"`
	RequestedAt     time.Time `json:"requested_at"`
}

// AIExplanationLeaseRecoveryRequestedData is a privacy-minimal wake-up for
// one exact expired Run lease. It resumes or safely terminalizes the same
// attempt; it never authorizes another Provider invocation attempt.
type AIExplanationLeaseRecoveryRequestedData struct {
	OrgID                  int64     `json:"org_id"`
	GenerationID           string    `json:"generation_id"`
	RunID                  string    `json:"run_id"`
	Attempt                int       `json:"attempt"`
	ExpectedLeaseExpiresAt time.Time `json:"expected_lease_expires_at"`
	InvocationPhase        string    `json:"invocation_phase"`
	RequestedAt            time.Time `json:"requested_at"`
}

// AIExplanationGeneratedData records the immutable successful lifecycle
// references. The generated content remains in the Artifact collection.
type AIExplanationGeneratedData struct {
	OrgID          int64     `json:"org_id"`
	GenerationID   string    `json:"generation_id"`
	RunID          string    `json:"run_id"`
	ArtifactID     string    `json:"artifact_id"`
	AssessmentID   string    `json:"assessment_id"`
	TesteeID       uint64    `json:"testee_id"`
	SourceReportID string    `json:"source_report_id"`
	Audience       string    `json:"audience"`
	GeneratedAt    time.Time `json:"generated_at"`
}

// AIExplanationFailedData records a safe terminal failure projection. It must
// never contain raw provider output, Prompt content or participant input.
type AIExplanationFailedData struct {
	OrgID          int64     `json:"org_id"`
	GenerationID   string    `json:"generation_id"`
	RunID          string    `json:"run_id"`
	AssessmentID   string    `json:"assessment_id"`
	TesteeID       uint64    `json:"testee_id"`
	SourceReportID string    `json:"source_report_id"`
	Audience       string    `json:"audience"`
	Attempt        int       `json:"attempt"`
	FailureKind    string    `json:"failure_kind"`
	FailureCode    string    `json:"failure_code"`
	Retryable      bool      `json:"retryable"`
	SafeReason     string    `json:"safe_reason"`
	FailedAt       time.Time `json:"failed_at"`
}

// AIExplanationPromptEvaluationStepRequestedData is a privacy-safe durable
// address for exactly one synthetic evaluation attempt. The immutable suite
// input and release identities stay in PromptEvaluationRun.
type AIExplanationPromptEvaluationStepRequestedData struct {
	OrgID            int64     `json:"org_id"`
	RunID            string    `json:"run_id"`
	CaseID           string    `json:"case_id"`
	Attempt          int       `json:"attempt"`
	RecheckID        string    `json:"recheck_id,omitempty"`
	EvidenceVersion  string    `json:"evidence_version,omitempty"`
	ExecutionKind    string    `json:"execution_kind,omitempty"`
	SlotOrdinal      int       `json:"slot_ordinal,omitempty"`
	CandidateID      string    `json:"candidate_id,omitempty"`
	ExecutionOrdinal int       `json:"execution_ordinal,omitempty"`
	RequestedBy      string    `json:"requested_by"`
	RequestedAt      time.Time `json:"requested_at"`
}
