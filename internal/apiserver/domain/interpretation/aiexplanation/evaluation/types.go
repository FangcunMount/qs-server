// Package evaluation owns immutable release evidence for the AI explanation
// Prompt, Profile, schemas and provider route. Offline preflight results alone
// never satisfy this aggregate's publish gate.
package evaluation

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
)

const (
	RequiredGenerationCaseCount = 7
	RequiredRepetitionsPerCase  = 5
	RequiredGenerationAttempts  = RequiredGenerationCaseCount * RequiredRepetitionsPerCase
	MaxStoredOutputBytes        = 256 << 10
	MaxRecoveryRequests         = 20
)

type Status string

const (
	StatusCollecting     Status = "collecting"
	StatusAwaitingReview Status = "awaiting_review"
	StatusApproved       Status = "approved"
	StatusRejected       Status = "rejected"
	StatusCanceled       Status = "canceled"
)

func (s Status) IsValid() bool {
	switch s {
	case StatusCollecting, StatusAwaitingReview, StatusApproved, StatusRejected, StatusCanceled:
		return true
	default:
		return false
	}
}

func (s Status) IsTerminal() bool {
	return s == StatusApproved || s == StatusRejected || s == StatusCanceled
}

// AttemptExecution is the durable checkpoint written before a generation
// Provider call. Prepared checkpoints are safe to reclaim after their lease;
// dispatching checkpoints are not replayed automatically because the Provider
// may have accepted the request even when no receipt was persisted.
type AttemptExecutionPhase string

const (
	AttemptExecutionPrepared    AttemptExecutionPhase = "prepared"
	AttemptExecutionDispatching AttemptExecutionPhase = "dispatching"
)

type AttemptExecution struct {
	CaseID            string
	Attempt           int
	Owner             string
	InvocationID      string
	Phase             AttemptExecutionPhase
	ClaimedAt         time.Time
	LeaseExpiresAt    time.Time
	DispatchStartedAt *time.Time
}

type RecoveryRequest struct {
	ID          string
	CaseID      string
	Attempt     int
	Actor       string
	Reason      string
	RequestedAt time.Time
}

func (r RecoveryRequest) Validate() error {
	if strings.TrimSpace(r.ID) == "" || len(r.ID) > 256 || strings.TrimSpace(r.CaseID) == "" || r.Attempt < 1 ||
		strings.TrimSpace(r.Actor) == "" || len(r.Actor) > 256 || strings.TrimSpace(r.Reason) == "" || len(r.Reason) > 1000 || r.RequestedAt.IsZero() {
		return fmt.Errorf("AI explanation evaluation recovery audit is invalid")
	}
	return nil
}

func (e AttemptExecution) Validate() error {
	if strings.TrimSpace(e.CaseID) == "" || e.Attempt < 1 || strings.TrimSpace(e.Owner) == "" ||
		len(e.Owner) > 256 || strings.TrimSpace(e.InvocationID) == "" || len(e.InvocationID) > 256 ||
		e.ClaimedAt.IsZero() || !e.LeaseExpiresAt.After(e.ClaimedAt) {
		return fmt.Errorf("AI explanation evaluation attempt execution identity or lease is invalid")
	}
	switch e.Phase {
	case AttemptExecutionPrepared:
		if e.DispatchStartedAt != nil {
			return fmt.Errorf("prepared AI explanation evaluation attempt cannot have dispatch time")
		}
	case AttemptExecutionDispatching:
		if e.DispatchStartedAt == nil || e.DispatchStartedAt.Before(e.ClaimedAt) || e.DispatchStartedAt.After(e.LeaseExpiresAt) {
			return fmt.Errorf("dispatching AI explanation evaluation attempt requires a valid dispatch time")
		}
	default:
		return fmt.Errorf("AI explanation evaluation attempt execution phase is invalid")
	}
	return nil
}

func (e AttemptExecution) LeaseExpired(at time.Time) bool {
	return !at.IsZero() && !at.Before(e.LeaseExpiresAt)
}

type SuiteRef struct {
	ID          string
	Version     string
	Fingerprint aiexplanation.Fingerprint
	GitBlobSHA  string
}

func (r SuiteRef) Validate() error {
	if strings.TrimSpace(r.ID) == "" || strings.TrimSpace(r.GitBlobSHA) == "" {
		return fmt.Errorf("AI explanation evaluation suite id and git blob sha are required")
	}
	if err := aiexplanation.ValidateVersion(r.Version); err != nil {
		return err
	}
	return r.Fingerprint.Validate()
}

type SchemaRef struct {
	Version     string
	Fingerprint aiexplanation.Fingerprint
}

func (r SchemaRef) Validate() error {
	if err := aiexplanation.ValidateVersion(r.Version); err != nil {
		return err
	}
	return r.Fingerprint.Validate()
}

// DecodingParameters records the actual non-secret provider request controls.
// Nil sampling fields mean the provider default was used.
type DecodingParameters struct {
	MaxOutputTokens int
	Temperature     *float64
	TopP            *float64
	Seed            *int64
	ReasoningEffort string
}

// SemanticEvaluatorSpec freezes the independent model-judge release used to
// resolve semantic assertions and score the rubric. It is deliberately
// separate from the generation Provider so a judge change invalidates release
// evidence even when the generation route is unchanged.
type SemanticEvaluatorSpec struct {
	Version      string
	Prompt       aiexplanation.PromptRef
	OutputSchema SchemaRef
	Provider     aiexplanation.ProviderExecutionSpec
	Decoding     DecodingParameters
}

func (s SemanticEvaluatorSpec) Validate() error {
	if err := aiexplanation.ValidateVersion(s.Version); err != nil {
		return err
	}
	if err := s.Prompt.Validate(); err != nil {
		return err
	}
	if err := s.OutputSchema.Validate(); err != nil {
		return err
	}
	if err := s.Provider.Validate(); err != nil {
		return err
	}
	return s.Decoding.Validate()
}

func (p DecodingParameters) Validate() error {
	if p.MaxOutputTokens < 1 {
		return fmt.Errorf("AI explanation evaluation max output tokens must be positive")
	}
	if p.Temperature != nil && (*p.Temperature < 0 || *p.Temperature > 2) {
		return fmt.Errorf("AI explanation evaluation temperature is invalid")
	}
	if p.TopP != nil && (*p.TopP <= 0 || *p.TopP > 1) {
		return fmt.Errorf("AI explanation evaluation top-p is invalid")
	}
	switch strings.TrimSpace(p.ReasoningEffort) {
	case "", "none", "minimal", "low", "medium", "high", "xhigh", "max":
	default:
		return fmt.Errorf("AI explanation evaluation reasoning effort is invalid")
	}
	return nil
}

type ReleaseIdentity struct {
	Suite                    SuiteRef
	Prompt                   aiexplanation.PromptRef
	Profile                  aiexplanation.ProfileRef
	InputSchema              SchemaRef
	OutputSchema             SchemaRef
	Provider                 aiexplanation.ProviderExecutionSpec
	Decoding                 DecodingParameters
	SemanticEvaluator        SemanticEvaluatorSpec
	GenerationCaseIDs        []string
	PreflightCaseID          string
	PreflightRejectionReason string
	RepetitionsPerCase       int
}

// Fingerprint is the semantic identity used by the persistence uniqueness
// gate. It covers every executable release input, including both generation
// and semantic-evaluator routes and decoding controls.
func (r ReleaseIdentity) Fingerprint() (aiexplanation.Fingerprint, error) {
	if err := r.Validate(); err != nil {
		return "", err
	}
	raw, err := json.Marshal(r)
	if err != nil {
		return "", fmt.Errorf("marshal AI explanation evaluation release identity: %w", err)
	}
	return aiexplanation.NewFingerprint(raw), nil
}

func (r ReleaseIdentity) Validate() error {
	if err := r.Suite.Validate(); err != nil {
		return err
	}
	if err := r.Prompt.Validate(); err != nil {
		return err
	}
	if err := r.Profile.Validate(); err != nil {
		return err
	}
	if err := r.InputSchema.Validate(); err != nil {
		return err
	}
	if err := r.OutputSchema.Validate(); err != nil {
		return err
	}
	if err := r.Provider.Validate(); err != nil {
		return err
	}
	if err := r.Decoding.Validate(); err != nil {
		return err
	}
	if err := r.SemanticEvaluator.Validate(); err != nil {
		return err
	}
	if r.InputSchema.Version != aiexplanation.InputSchemaVersionV1 || r.OutputSchema.Version != aiexplanation.OutputSchemaVersionV1 {
		return fmt.Errorf("AI explanation evaluation schema versions are invalid")
	}
	if r.RepetitionsPerCase != RequiredRepetitionsPerCase || len(r.GenerationCaseIDs) != RequiredGenerationCaseCount {
		return fmt.Errorf("AI explanation evaluation v1 requires seven cases and five repetitions")
	}
	seen := make(map[string]struct{}, len(r.GenerationCaseIDs))
	for _, caseID := range r.GenerationCaseIDs {
		caseID = strings.TrimSpace(caseID)
		if caseID == "" {
			return fmt.Errorf("AI explanation evaluation generation case id is required")
		}
		if _, exists := seen[caseID]; exists {
			return fmt.Errorf("AI explanation evaluation generation case id is duplicated")
		}
		seen[caseID] = struct{}{}
	}
	if strings.TrimSpace(r.PreflightCaseID) == "" || strings.TrimSpace(r.PreflightRejectionReason) == "" {
		return fmt.Errorf("AI explanation evaluation preflight identity is required")
	}
	if _, exists := seen[r.PreflightCaseID]; exists {
		return fmt.Errorf("AI explanation evaluation preflight case overlaps a generation case")
	}
	return nil
}

func (r ReleaseIdentity) IsGenerationCase(caseID string) bool {
	for _, expected := range r.GenerationCaseIDs {
		if expected == caseID {
			return true
		}
	}
	return false
}

type AttemptStage string

const (
	AttemptStageGeneration AttemptStage = "generation"
	AttemptStagePreflight  AttemptStage = "preflight"
)

type AssertionScope string

const (
	AssertionScopeDefault AssertionScope = "default"
	AssertionScopeCase    AssertionScope = "case"
)

type AssertionStatus string

const (
	AssertionPassed          AssertionStatus = "passed"
	AssertionFailed          AssertionStatus = "failed"
	AssertionPendingSemantic AssertionStatus = "pending_semantic"
	AssertionBlocked         AssertionStatus = "blocked"
)

type AssertionReceipt struct {
	Type  string
	Scope AssertionScope
	// Ordinal is the one-based occurrence of Type inside Scope. Evaluation
	// cases may intentionally contain the same assertion type more than once
	// with different parameters; those obligations must remain independent.
	Ordinal   int
	Hard      bool
	Evaluator string
	Status    AssertionStatus
	Detail    string
}

func (r AssertionReceipt) Validate() error {
	if strings.TrimSpace(r.Type) == "" || r.Ordinal < 1 || strings.TrimSpace(r.Evaluator) == "" {
		return fmt.Errorf("AI explanation evaluation assertion identity is required")
	}
	if r.Scope != AssertionScopeDefault && r.Scope != AssertionScopeCase {
		return fmt.Errorf("AI explanation evaluation assertion scope is invalid")
	}
	switch r.Status {
	case AssertionPassed, AssertionFailed, AssertionPendingSemantic, AssertionBlocked:
		return nil
	default:
		return fmt.Errorf("AI explanation evaluation assertion status is invalid")
	}
}

type SemanticScores struct {
	Faithfulness            int
	CrossDimensionQuality   int
	SuggestionActionability int
	AudienceClarity         int
	Concision               int
}

func (s SemanticScores) Validate() error {
	values := []int{s.Faithfulness, s.CrossDimensionQuality, s.SuggestionActionability, s.AudienceClarity, s.Concision}
	for _, value := range values {
		if value < 1 || value > 5 {
			return fmt.Errorf("AI explanation evaluation semantic score is outside 1..5")
		}
	}
	return nil
}

type SemanticReceipt struct {
	EvaluatorVersion string
	ProviderReceipt  aiexplanation.ProviderReceipt
	Scores           SemanticScores
	Rationale        string
}

func (r SemanticReceipt) Validate() error {
	if strings.TrimSpace(r.EvaluatorVersion) == "" || strings.TrimSpace(r.ProviderReceipt.RequestID) == "" || strings.TrimSpace(r.Rationale) == "" {
		return fmt.Errorf("AI explanation evaluation semantic evaluator and rationale are required")
	}
	if err := r.ProviderReceipt.Validate(); err != nil {
		return err
	}
	return r.Scores.Validate()
}

type AttemptFailure struct {
	Stage         string
	Code          string
	SafeMessage   string
	Retryable     bool
	ResultUnknown bool
}

func (f AttemptFailure) Validate() error {
	if strings.TrimSpace(f.Stage) == "" || len(f.Stage) > 64 || strings.TrimSpace(f.Code) == "" || len(f.Code) > 128 || strings.TrimSpace(f.SafeMessage) == "" || len(f.SafeMessage) > 1000 {
		return fmt.Errorf("AI explanation evaluation attempt failure is invalid")
	}
	return nil
}

type AttemptRecord struct {
	CaseID            string
	Attempt           int
	Stage             AttemptStage
	StartedAt         time.Time
	FinishedAt        time.Time
	ProviderCallCount int
	ProviderReceipt   *aiexplanation.ProviderReceipt
	RawOutput         []byte
	NormalizedOutput  []byte
	OutputFingerprint aiexplanation.Fingerprint
	RejectionReason   string
	Failure           *AttemptFailure
	Assertions        []AssertionReceipt
	Semantic          *SemanticReceipt
}

func (a AttemptRecord) Validate() error {
	if strings.TrimSpace(a.CaseID) == "" || a.Attempt < 1 || a.StartedAt.IsZero() || a.FinishedAt.Before(a.StartedAt) {
		return fmt.Errorf("AI explanation evaluation attempt identity or timing is invalid")
	}
	if len(a.Assertions) == 0 {
		return fmt.Errorf("AI explanation evaluation attempt assertions are required")
	}
	seen := make(map[string]struct{}, len(a.Assertions))
	for _, assertion := range a.Assertions {
		if err := assertion.Validate(); err != nil {
			return err
		}
		key := assertion.Type + "\x00" + string(assertion.Scope) + "\x00" + fmt.Sprint(assertion.Ordinal) + "\x00" + assertion.Evaluator
		if _, exists := seen[key]; exists {
			return fmt.Errorf("AI explanation evaluation assertion receipt is duplicated")
		}
		seen[key] = struct{}{}
	}
	switch a.Stage {
	case AttemptStageGeneration:
		if a.ProviderCallCount < 0 || a.ProviderCallCount > 1 || len(a.RawOutput) > MaxStoredOutputBytes || len(a.NormalizedOutput) > MaxStoredOutputBytes || strings.TrimSpace(a.RejectionReason) != "" {
			return fmt.Errorf("AI explanation generation attempt evidence is invalid")
		}
		if a.ProviderReceipt != nil {
			if a.ProviderCallCount != 1 {
				return fmt.Errorf("AI explanation provider receipt requires one provider call")
			}
			if err := a.ProviderReceipt.Validate(); err != nil {
				return err
			}
		}
		if a.Failure == nil {
			if a.ProviderCallCount != 1 || a.ProviderReceipt == nil || len(a.RawOutput) == 0 {
				return fmt.Errorf("successful AI explanation generation attempt evidence is incomplete")
			}
		} else if err := a.Failure.Validate(); err != nil {
			return err
		}
		if len(a.NormalizedOutput) > 0 {
			if !json.Valid(a.NormalizedOutput) || a.OutputFingerprint != aiexplanation.NewFingerprint(a.NormalizedOutput) {
				return fmt.Errorf("AI explanation normalized output evidence is invalid")
			}
		} else if a.OutputFingerprint != "" {
			return fmt.Errorf("AI explanation output fingerprint requires normalized output")
		}
		if a.Semantic != nil {
			return a.Semantic.Validate()
		}
		return nil
	case AttemptStagePreflight:
		if a.Attempt != 1 || a.ProviderCallCount != 0 || a.ProviderReceipt != nil || len(a.RawOutput) != 0 || len(a.NormalizedOutput) != 0 || a.OutputFingerprint != "" || a.Failure != nil || a.Semantic != nil || strings.TrimSpace(a.RejectionReason) == "" {
			return fmt.Errorf("AI explanation preflight attempt evidence is invalid")
		}
		return nil
	default:
		return fmt.Errorf("AI explanation evaluation attempt stage is invalid")
	}
}

type ReviewRole string

const (
	ReviewRoleAssessmentSemantics ReviewRole = "assessment_semantics"
	ReviewRoleSafetyProduct       ReviewRole = "safety_product"
)

type ReviewDecision string

const (
	ReviewDecisionApprove ReviewDecision = "approve"
	ReviewDecisionReject  ReviewDecision = "reject"
)

type HumanReview struct {
	CaseID     string
	Attempt    int
	Role       ReviewRole
	Reviewer   string
	Decision   ReviewDecision
	ReviewedAt time.Time
	Reason     string
}

func (r HumanReview) Validate() error {
	if strings.TrimSpace(r.CaseID) == "" || r.Attempt < 1 || strings.TrimSpace(r.Reviewer) == "" || r.ReviewedAt.IsZero() || strings.TrimSpace(r.Reason) == "" {
		return fmt.Errorf("AI explanation human review audit is required")
	}
	if r.Role != ReviewRoleAssessmentSemantics && r.Role != ReviewRoleSafetyProduct {
		return fmt.Errorf("AI explanation human review role is invalid")
	}
	if r.Decision != ReviewDecisionApprove && r.Decision != ReviewDecisionReject {
		return fmt.Errorf("AI explanation human review decision is invalid")
	}
	return nil
}

type GateReason struct {
	Code    string
	CaseID  string
	Attempt int
	Detail  string
}

type QualityMetrics struct {
	GenerationAttempts     int
	CaseAssertionPasses    int
	FaithfulnessAverage    float64
	CrossDimensionAverage  float64
	ActionabilityAverage   float64
	AudienceClarityAverage float64
	ConcisionAverage       float64
	HumanReviews           int
}

type GateResult struct {
	Passed  bool
	Reasons []GateReason
	Metrics QualityMetrics
}

func cloneRelease(value ReleaseIdentity) ReleaseIdentity {
	cloned := value
	cloned.GenerationCaseIDs = append([]string(nil), value.GenerationCaseIDs...)
	cloned.Decoding = cloneDecoding(value.Decoding)
	cloned.SemanticEvaluator.Decoding = cloneDecoding(value.SemanticEvaluator.Decoding)
	return cloned
}

func cloneDecoding(value DecodingParameters) DecodingParameters {
	cloned := value
	if value.Temperature != nil {
		copy := *value.Temperature
		cloned.Temperature = &copy
	}
	if value.TopP != nil {
		copy := *value.TopP
		cloned.TopP = &copy
	}
	if value.Seed != nil {
		copy := *value.Seed
		cloned.Seed = &copy
	}
	return cloned
}

func cloneAttempt(value AttemptRecord) AttemptRecord {
	cloned := value
	cloned.RawOutput = append([]byte(nil), value.RawOutput...)
	cloned.NormalizedOutput = append([]byte(nil), value.NormalizedOutput...)
	cloned.Assertions = append([]AssertionReceipt(nil), value.Assertions...)
	if value.ProviderReceipt != nil {
		copy := *value.ProviderReceipt
		cloned.ProviderReceipt = &copy
	}
	if value.Semantic != nil {
		copy := *value.Semantic
		cloned.Semantic = &copy
	}
	if value.Failure != nil {
		copy := *value.Failure
		cloned.Failure = &copy
	}
	return cloned
}

func cloneGate(value *GateResult) *GateResult {
	if value == nil {
		return nil
	}
	cloned := *value
	cloned.Reasons = append([]GateReason(nil), value.Reasons...)
	return &cloned
}
