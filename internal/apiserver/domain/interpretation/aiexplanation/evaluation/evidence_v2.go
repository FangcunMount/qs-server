package evaluation

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

const PromptEvaluationEvidenceSchemaVersionV2 = "prompt-evaluation-evidence/v2"

var evidenceEntityIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9:._/-]{0,127}$`)

type EvidenceStatus string

const (
	EvidenceStatusRequested      EvidenceStatus = "requested"
	EvidenceStatusCollecting     EvidenceStatus = "collecting"
	EvidenceStatusBlocked        EvidenceStatus = "blocked"
	EvidenceStatusAwaitingReview EvidenceStatus = "awaiting_review"
	EvidenceStatusApproved       EvidenceStatus = "approved"
	EvidenceStatusRejected       EvidenceStatus = "rejected"
	EvidenceStatusCanceled       EvidenceStatus = "canceled"
)

func (s EvidenceStatus) IsValid() bool {
	switch s {
	case EvidenceStatusRequested,
		EvidenceStatusCollecting,
		EvidenceStatusBlocked,
		EvidenceStatusAwaitingReview,
		EvidenceStatusApproved,
		EvidenceStatusRejected,
		EvidenceStatusCanceled:
		return true
	default:
		return false
	}
}

func (s EvidenceStatus) IsTerminal() bool {
	return s == EvidenceStatusApproved || s == EvidenceStatusRejected || s == EvidenceStatusCanceled
}

type FrozenContractRef struct {
	ID          string                    `json:"id"`
	Version     string                    `json:"version"`
	Fingerprint aiexplanation.Fingerprint `json:"fingerprint"`
}

func (r FrozenContractRef) Validate() error {
	if !evidenceEntityIDPattern.MatchString(r.ID) {
		return fmt.Errorf("AI explanation frozen contract id is invalid")
	}
	if err := aiexplanation.ValidateVersion(r.Version); err != nil {
		return err
	}
	return r.Fingerprint.Validate()
}

type EvidenceReleaseIdentity struct {
	Fingerprint          aiexplanation.Fingerprint
	Suite                FrozenContractRef
	Prompt               FrozenContractRef
	Profile              FrozenContractRef
	InputSchema          FrozenContractRef
	OutputSchema         FrozenContractRef
	GenerationRoute      FrozenContractRef
	SemanticPrompt       FrozenContractRef
	SemanticOutputSchema FrozenContractRef
	SemanticRoute        FrozenContractRef
	ExecutionPolicy      FrozenContractRef
	GatePolicy           FrozenContractRef
}

func (r EvidenceReleaseIdentity) Validate(executionPolicy EvaluationExecutionPolicy, gatePolicy ReleaseGatePolicy) error {
	refs := []FrozenContractRef{
		r.Suite, r.Prompt, r.Profile, r.InputSchema, r.OutputSchema, r.GenerationRoute,
		r.SemanticPrompt, r.SemanticOutputSchema, r.SemanticRoute, r.ExecutionPolicy, r.GatePolicy,
	}
	for _, ref := range refs {
		if err := ref.Validate(); err != nil {
			return err
		}
	}
	expectedFingerprint, err := r.ExpectedFingerprint()
	if err != nil {
		return err
	}
	if r.Fingerprint != expectedFingerprint {
		return fmt.Errorf("AI explanation evaluation release identity fingerprint is invalid")
	}
	executionFingerprint, err := executionPolicy.Fingerprint()
	if err != nil {
		return err
	}
	gateFingerprint, err := gatePolicy.Fingerprint()
	if err != nil {
		return err
	}
	if r.ExecutionPolicy.ID != executionPolicy.PolicyID || r.ExecutionPolicy.Version != executionPolicy.Version || r.ExecutionPolicy.Fingerprint != executionFingerprint ||
		r.GatePolicy.ID != gatePolicy.PolicyID || r.GatePolicy.Version != gatePolicy.Version || r.GatePolicy.Fingerprint != gateFingerprint {
		return fmt.Errorf("AI explanation evaluation policy references do not match the frozen documents")
	}
	return nil
}

func (r EvidenceReleaseIdentity) ExpectedFingerprint() (aiexplanation.Fingerprint, error) {
	components := map[string]FrozenContractRef{
		"suite": r.Suite, "prompt": r.Prompt, "profile": r.Profile,
		"input_schema": r.InputSchema, "output_schema": r.OutputSchema, "generation_route": r.GenerationRoute,
		"semantic_prompt": r.SemanticPrompt, "semantic_output_schema": r.SemanticOutputSchema, "semantic_route": r.SemanticRoute,
		"execution_policy": r.ExecutionPolicy, "gate_policy": r.GatePolicy,
	}
	for _, ref := range components {
		if err := ref.Validate(); err != nil {
			return "", err
		}
	}
	raw, err := json.Marshal(components)
	if err != nil {
		return "", fmt.Errorf("marshal AI explanation evaluation release identity: %w", err)
	}
	return aiexplanation.NewFingerprint(raw), nil
}

type CandidateSlotStatus string

const (
	CandidateSlotPending  CandidateSlotStatus = "pending"
	CandidateSlotAccepted CandidateSlotStatus = "accepted"
	CandidateSlotBlocked  CandidateSlotStatus = "blocked"
)

func (s CandidateSlotStatus) IsValid() bool {
	return s == CandidateSlotPending || s == CandidateSlotAccepted || s == CandidateSlotBlocked
}

type ExecutionStatus string

const (
	ExecutionStatusPrepared      ExecutionStatus = "prepared"
	ExecutionStatusDispatching   ExecutionStatus = "dispatching"
	ExecutionStatusSucceeded     ExecutionStatus = "succeeded"
	ExecutionStatusFailed        ExecutionStatus = "failed"
	ExecutionStatusResultUnknown ExecutionStatus = "result_unknown"
)

func (s ExecutionStatus) IsValid() bool {
	switch s {
	case ExecutionStatusPrepared, ExecutionStatusDispatching, ExecutionStatusSucceeded, ExecutionStatusFailed, ExecutionStatusResultUnknown:
		return true
	default:
		return false
	}
}

type PreflightEvidenceStatus string

const (
	PreflightEvidencePending PreflightEvidenceStatus = "pending"
	PreflightEvidencePassed  PreflightEvidenceStatus = "passed"
	PreflightEvidenceFailed  PreflightEvidenceStatus = "failed"
)

type PreflightCaseEvidence struct {
	CaseID            string
	Status            PreflightEvidenceStatus
	EvaluatedAt       *time.Time
	ProviderCallCount int
	RejectionReason   string
	Assertions        []AssertionReceipt
}

func (e PreflightCaseEvidence) Validate() error {
	if !evidenceEntityIDPattern.MatchString(e.CaseID) || e.ProviderCallCount != 0 {
		return fmt.Errorf("AI explanation preflight evidence identity is invalid")
	}
	switch e.Status {
	case PreflightEvidencePending:
		if e.EvaluatedAt != nil || e.RejectionReason != "" || len(e.Assertions) != 0 {
			return fmt.Errorf("pending AI explanation preflight contains terminal evidence")
		}
		return nil
	case PreflightEvidencePassed, PreflightEvidenceFailed:
		if e.EvaluatedAt == nil || !policyIdentifierPattern.MatchString(e.RejectionReason) || len(e.Assertions) == 0 || len(e.Assertions) > 16 {
			return fmt.Errorf("terminal AI explanation preflight evidence is incomplete")
		}
		passed := map[string]bool{"provider_call_count": false, "rejection_reason": false}
		for _, assertion := range e.Assertions {
			if err := assertion.Validate(); err != nil {
				return err
			}
			if _, required := passed[assertion.Type]; required && assertion.Status == AssertionPassed {
				passed[assertion.Type] = true
			}
		}
		if e.Status == PreflightEvidencePassed && (!passed["provider_call_count"] || !passed["rejection_reason"]) {
			return fmt.Errorf("passed AI explanation preflight is missing required assertions")
		}
		return nil
	default:
		return fmt.Errorf("AI explanation preflight evidence status is invalid")
	}
}

type CandidateSlot struct {
	CaseID                 string
	Ordinal                int
	Status                 CandidateSlotStatus
	GenerationExecutionIDs []string
	Candidate              *Candidate
}

func (s CandidateSlot) key() string {
	return s.CaseID + "\x00" + fmt.Sprint(s.Ordinal)
}

type Candidate struct {
	ID                          string
	GenerationExecutionID       string
	NormalizedOutputFingerprint aiexplanation.Fingerprint
	AcceptedAt                  time.Time
	Assertions                  []AssertionReceipt
	SemanticExecutionIDs        []string
	AcceptedSemanticExecutionID string
	ReviewReady                 bool
}

func (c Candidate) Validate() error {
	if !evidenceEntityIDPattern.MatchString(c.ID) || !evidenceEntityIDPattern.MatchString(c.GenerationExecutionID) || c.AcceptedAt.IsZero() || len(c.Assertions) == 0 {
		return fmt.Errorf("AI explanation evaluation candidate identity is invalid")
	}
	if err := c.NormalizedOutputFingerprint.Validate(); err != nil {
		return err
	}
	for _, assertion := range c.Assertions {
		if err := assertion.Validate(); err != nil {
			return err
		}
	}
	if err := validateUniqueEntityIDs(c.SemanticExecutionIDs); err != nil {
		return err
	}
	if c.ReviewReady != (c.AcceptedSemanticExecutionID != "") {
		return fmt.Errorf("AI explanation candidate review readiness is inconsistent")
	}
	if c.AcceptedSemanticExecutionID != "" && !containsString(c.SemanticExecutionIDs, c.AcceptedSemanticExecutionID) {
		return fmt.Errorf("AI explanation accepted semantic execution is not attached to the candidate")
	}
	return nil
}

type CandidateGenerationExecution struct {
	ID                          string
	CaseID                      string
	SlotOrdinal                 int
	ExecutionOrdinal            int
	InvocationID                string
	Status                      ExecutionStatus
	StartedAt                   time.Time
	FinishedAt                  *time.Time
	ProviderCallCount           int
	ProviderReceipt             *aiexplanation.ProviderReceipt
	RawOutput                   []byte
	NormalizedOutput            []byte
	NormalizedOutputFingerprint aiexplanation.Fingerprint
	Failure                     *ClassifiedFailure
}

func (e CandidateGenerationExecution) Validate() error {
	if !evidenceEntityIDPattern.MatchString(e.ID) || !evidenceEntityIDPattern.MatchString(e.CaseID) ||
		!evidenceEntityIDPattern.MatchString(e.InvocationID) || e.SlotOrdinal < 1 || e.SlotOrdinal > RequiredRepetitionsPerCase ||
		e.ExecutionOrdinal < 1 || e.ExecutionOrdinal > 2 || !e.Status.IsValid() || e.StartedAt.IsZero() ||
		e.ProviderCallCount < 0 || e.ProviderCallCount > 1 || len(e.RawOutput) > MaxStoredOutputBytes || len(e.NormalizedOutput) > MaxStoredOutputBytes {
		return fmt.Errorf("AI explanation candidate generation execution is invalid")
	}
	if e.FinishedAt != nil && e.FinishedAt.Before(e.StartedAt) {
		return fmt.Errorf("AI explanation candidate generation execution timing is invalid")
	}
	if e.ProviderReceipt != nil {
		if e.ProviderCallCount != 1 {
			return fmt.Errorf("AI explanation candidate generation receipt requires one Provider call")
		}
		if err := e.ProviderReceipt.Validate(); err != nil {
			return err
		}
	}
	if err := validateNormalizedEvidence(e.NormalizedOutput, e.NormalizedOutputFingerprint); err != nil {
		return err
	}
	switch e.Status {
	case ExecutionStatusPrepared:
		if e.FinishedAt != nil || e.ProviderCallCount != 0 || e.ProviderReceipt != nil || len(e.RawOutput) != 0 || len(e.NormalizedOutput) != 0 || e.Failure != nil {
			return fmt.Errorf("prepared AI explanation generation execution contains terminal evidence")
		}
	case ExecutionStatusDispatching:
		if e.FinishedAt != nil || e.ProviderCallCount != 1 || e.Failure != nil {
			return fmt.Errorf("dispatching AI explanation generation execution is invalid")
		}
	case ExecutionStatusSucceeded:
		if e.FinishedAt == nil || e.ProviderCallCount != 1 || e.ProviderReceipt == nil || len(e.RawOutput) == 0 || len(e.NormalizedOutput) == 0 || e.Failure != nil {
			return fmt.Errorf("successful AI explanation generation execution evidence is incomplete")
		}
		if err := validateV2ProviderReceipt(*e.ProviderReceipt, e.InvocationID); err != nil {
			return err
		}
	case ExecutionStatusFailed, ExecutionStatusResultUnknown:
		if e.FinishedAt == nil || e.Failure == nil {
			return fmt.Errorf("failed AI explanation generation execution evidence is incomplete")
		}
		if err := e.Failure.Validate(); err != nil {
			return err
		}
		if (e.Status == ExecutionStatusResultUnknown) != e.Failure.ResultUnknown || e.Failure.CandidateExists() {
			return fmt.Errorf("AI explanation generation execution failure classification is inconsistent")
		}
	}
	return nil
}

type SemanticDecision struct {
	Type    string
	Scope   AssertionScope
	Ordinal int
	Status  AssertionStatus
	Detail  string
}

func (d SemanticDecision) Validate() error {
	if !policyIdentifierPattern.MatchString(d.Type) || d.Ordinal < 1 || d.Ordinal > 64 ||
		(d.Scope != AssertionScopeDefault && d.Scope != AssertionScopeCase) ||
		(d.Status != AssertionPassed && d.Status != AssertionFailed) || !validateEvidenceText(d.Detail, 2000) {
		return fmt.Errorf("AI explanation semantic decision is invalid")
	}
	return nil
}

type SemanticEvaluationResult struct {
	EvaluatorVersion  string
	Scores            SemanticScores
	Rationale         string
	Decisions         []SemanticDecision
	OutputFingerprint aiexplanation.Fingerprint
}

func (r SemanticEvaluationResult) Validate() error {
	if err := aiexplanation.ValidateVersion(r.EvaluatorVersion); err != nil {
		return err
	}
	if err := r.Scores.Validate(); err != nil {
		return err
	}
	if !validateEvidenceText(r.Rationale, 4000) || len(r.Decisions) == 0 || len(r.Decisions) > 64 {
		return fmt.Errorf("AI explanation semantic evaluation result is invalid")
	}
	for _, decision := range r.Decisions {
		if err := decision.Validate(); err != nil {
			return err
		}
	}
	return r.OutputFingerprint.Validate()
}

type SemanticEvaluationExecution struct {
	ID                string
	CandidateID       string
	ExecutionOrdinal  int
	InvocationID      string
	Status            ExecutionStatus
	StartedAt         time.Time
	FinishedAt        *time.Time
	ProviderCallCount int
	ProviderReceipt   *aiexplanation.ProviderReceipt
	RawOutput         []byte
	NormalizedOutput  []byte
	Result            *SemanticEvaluationResult
	Failure           *ClassifiedFailure
}

func (e SemanticEvaluationExecution) Validate() error {
	if !evidenceEntityIDPattern.MatchString(e.ID) || !evidenceEntityIDPattern.MatchString(e.CandidateID) ||
		!evidenceEntityIDPattern.MatchString(e.InvocationID) || e.ExecutionOrdinal < 1 || e.ExecutionOrdinal > 2 ||
		!e.Status.IsValid() || e.StartedAt.IsZero() || e.ProviderCallCount < 0 || e.ProviderCallCount > 1 ||
		len(e.RawOutput) > MaxStoredOutputBytes || len(e.NormalizedOutput) > MaxStoredOutputBytes {
		return fmt.Errorf("AI explanation semantic evaluation execution is invalid")
	}
	if e.FinishedAt != nil && e.FinishedAt.Before(e.StartedAt) {
		return fmt.Errorf("AI explanation semantic evaluation execution timing is invalid")
	}
	if e.ProviderReceipt != nil {
		if e.ProviderCallCount != 1 {
			return fmt.Errorf("AI explanation semantic evaluation receipt requires one Provider call")
		}
		if err := e.ProviderReceipt.Validate(); err != nil {
			return err
		}
	}
	switch e.Status {
	case ExecutionStatusPrepared:
		if e.FinishedAt != nil || e.ProviderCallCount != 0 || e.ProviderReceipt != nil || len(e.RawOutput) != 0 || len(e.NormalizedOutput) != 0 || e.Result != nil || e.Failure != nil {
			return fmt.Errorf("prepared AI explanation semantic execution contains terminal evidence")
		}
	case ExecutionStatusDispatching:
		if e.FinishedAt != nil || e.ProviderCallCount != 1 || e.Result != nil || e.Failure != nil {
			return fmt.Errorf("dispatching AI explanation semantic execution is invalid")
		}
	case ExecutionStatusSucceeded:
		if e.FinishedAt == nil || e.ProviderCallCount != 1 || e.ProviderReceipt == nil || len(e.RawOutput) == 0 || len(e.NormalizedOutput) == 0 || e.Result == nil || e.Failure != nil {
			return fmt.Errorf("successful AI explanation semantic execution evidence is incomplete")
		}
		if !json.Valid(e.NormalizedOutput) || e.Result.OutputFingerprint != aiexplanation.NewFingerprint(e.NormalizedOutput) {
			return fmt.Errorf("AI explanation semantic normalized output evidence is invalid")
		}
		if err := validateV2ProviderReceipt(*e.ProviderReceipt, e.InvocationID); err != nil {
			return err
		}
		if err := e.Result.Validate(); err != nil {
			return err
		}
	case ExecutionStatusFailed, ExecutionStatusResultUnknown:
		if e.FinishedAt == nil || e.Result != nil || e.Failure == nil {
			return fmt.Errorf("failed AI explanation semantic execution evidence is incomplete")
		}
		if err := e.Failure.Validate(); err != nil {
			return err
		}
		if (e.Status == ExecutionStatusResultUnknown) != e.Failure.ResultUnknown ||
			!e.Failure.ResultUnknown && e.Failure.Kind != FailureKindSemanticExecution && e.Failure.Kind != FailureKindInfrastructureExecution && e.Failure.Kind != FailureKindProviderProtocol {
			return fmt.Errorf("AI explanation semantic execution failure classification is inconsistent")
		}
	}
	return nil
}

type CandidateHumanReview struct {
	CandidateID string
	Role        ReviewRole
	Reviewer    string
	Decision    ReviewDecision
	ReviewedAt  time.Time
	Reason      string
}

func (r CandidateHumanReview) Validate() error {
	if !evidenceEntityIDPattern.MatchString(r.CandidateID) || !evidenceEntityIDPattern.MatchString(r.Reviewer) ||
		r.ReviewedAt.IsZero() || !validateEvidenceText(r.Reason, 1000) ||
		(r.Role != ReviewRoleAssessmentSemantics && r.Role != ReviewRoleSafetyProduct) ||
		(r.Decision != ReviewDecisionApprove && r.Decision != ReviewDecisionReject) {
		return fmt.Errorf("AI explanation candidate human review is invalid")
	}
	return nil
}

type EvidenceGateResult struct {
	EvaluatedAt time.Time
	Passed      bool
	GatePasses  map[string]bool
	Metrics     []EvidenceGateMetric
	Reasons     []EvidenceGateReason
}

type EvidenceGateMetric struct {
	Name        string
	Numerator   int
	Denominator int
	Value       float64
	Threshold   float64
}

type EvidenceGateReason struct {
	Gate         string
	Code         string
	Detail       string
	EvidenceRefs []string
}

func (g EvidenceGateResult) Validate() error {
	if g.EvaluatedAt.IsZero() || len(g.GatePasses) != 5 || len(g.Metrics) > 64 || len(g.Reasons) > 256 {
		return fmt.Errorf("AI explanation evaluation G1-G5 result is invalid")
	}
	allPassed := true
	for _, gate := range []string{"G1", "G2", "G3", "G4", "G5"} {
		passed, exists := g.GatePasses[gate]
		if !exists {
			return fmt.Errorf("AI explanation evaluation gate decision is missing")
		}
		allPassed = allPassed && passed
	}
	if g.Passed != allPassed {
		return fmt.Errorf("AI explanation evaluation aggregate gate decision is inconsistent")
	}
	for _, metric := range g.Metrics {
		if !policyIdentifierPattern.MatchString(metric.Name) || metric.Numerator < 0 || metric.Denominator < 0 || !validRate(metric.Value) || !validRate(metric.Threshold) || metric.Numerator > metric.Denominator {
			return fmt.Errorf("AI explanation evaluation gate metric is invalid")
		}
	}
	for _, reason := range g.Reasons {
		if !isEvidenceGate(reason.Gate) || !policyIdentifierPattern.MatchString(reason.Code) || !validateEvidenceText(reason.Detail, 2000) || len(reason.EvidenceRefs) > 32 {
			return fmt.Errorf("AI explanation evaluation gate reason is invalid")
		}
		if err := validateUniqueEntityIDs(reason.EvidenceRefs); err != nil {
			return err
		}
	}
	return nil
}

func isEvidenceGate(value string) bool {
	return value == "G1" || value == "G2" || value == "G3" || value == "G4" || value == "G5"
}

type EvidenceRunAudit struct {
	OrganizationID int64
	RequestedBy    string
	RequestReason  string
	CreatedAt      time.Time
	ClosedAt       *time.Time
	FinalizedAt    *time.Time
	CanceledAt     *time.Time
}

type ResultUnknownResolutionDecision string

const (
	ResultUnknownAuthorizeReplacement ResultUnknownResolutionDecision = "authorize_replacement"
	ResultUnknownCancelRun            ResultUnknownResolutionDecision = "cancel_run"
)

type ResultUnknownResolution struct {
	ExecutionID                          string
	Decision                             ResultUnknownResolutionDecision
	Actor                                string
	Reason                               string
	AcknowledgedDuplicateCallAndCostRisk bool
	ResolvedAt                           time.Time
}

func (r ResultUnknownResolution) Validate() error {
	if !evidenceEntityIDPattern.MatchString(r.ExecutionID) ||
		(r.Decision != ResultUnknownAuthorizeReplacement && r.Decision != ResultUnknownCancelRun) ||
		!evidenceEntityIDPattern.MatchString(r.Actor) || !validateEvidenceText(r.Reason, 1000) ||
		!r.AcknowledgedDuplicateCallAndCostRisk || r.ResolvedAt.IsZero() {
		return fmt.Errorf("AI explanation result-unknown resolution audit is invalid")
	}
	return nil
}

type EvidenceStateTransition struct {
	Reason         string `json:"reason,omitempty" bson:"reason,omitempty"`
	From           *EvidenceStatus
	To             EvidenceStatus
	CauseCode      string
	Actor          string
	TransitionedAt time.Time
	EvidenceRefs   []string
}

func (t EvidenceStateTransition) Validate() error {
	if len(t.Reason) > 1000 || !t.To.IsValid() || t.From != nil && !t.From.IsValid() || !policyIdentifierPattern.MatchString(t.CauseCode) ||
		!evidenceEntityIDPattern.MatchString(t.Actor) || t.TransitionedAt.IsZero() || len(t.EvidenceRefs) > 32 {
		return fmt.Errorf("AI explanation evaluation state transition is invalid")
	}
	return validateUniqueEntityIDs(t.EvidenceRefs)
}

func isAllowedEvidenceTransition(from *EvidenceStatus, to EvidenceStatus) bool {
	if from == nil {
		return to == EvidenceStatusRequested
	}
	switch *from {
	case EvidenceStatusRequested:
		return to == EvidenceStatusCollecting || to == EvidenceStatusCanceled
	case EvidenceStatusCollecting:
		return to == EvidenceStatusBlocked || to == EvidenceStatusAwaitingReview || to == EvidenceStatusCanceled
	case EvidenceStatusBlocked:
		return to == EvidenceStatusCollecting || to == EvidenceStatusCanceled
	case EvidenceStatusAwaitingReview:
		return to == EvidenceStatusApproved || to == EvidenceStatusRejected || to == EvidenceStatusCanceled
	default:
		return false
	}
}

func (a EvidenceRunAudit) Validate(status EvidenceStatus) error {
	if a.OrganizationID <= 0 || !evidenceEntityIDPattern.MatchString(a.RequestedBy) || !validateEvidenceText(a.RequestReason, 1000) || a.CreatedAt.IsZero() {
		return fmt.Errorf("AI explanation evaluation v2 request audit is invalid")
	}
	for _, timestamp := range []*time.Time{a.ClosedAt, a.FinalizedAt, a.CanceledAt} {
		if timestamp != nil && timestamp.Before(a.CreatedAt) {
			return fmt.Errorf("AI explanation evaluation v2 audit timing is invalid")
		}
	}
	switch status {
	case EvidenceStatusAwaitingReview:
		if a.ClosedAt == nil || a.FinalizedAt != nil || a.CanceledAt != nil {
			return fmt.Errorf("AI explanation evaluation awaiting-review audit is invalid")
		}
	case EvidenceStatusApproved, EvidenceStatusRejected:
		if a.ClosedAt == nil || a.FinalizedAt == nil || a.FinalizedAt.Before(*a.ClosedAt) || a.CanceledAt != nil {
			return fmt.Errorf("AI explanation evaluation finalized audit is invalid")
		}
	case EvidenceStatusCanceled:
		if a.CanceledAt == nil || a.FinalizedAt != nil {
			return fmt.Errorf("AI explanation evaluation cancellation audit is invalid")
		}
	}
	return nil
}

// PromptEvaluationEvidenceV2 is the target evidence model. It can coexist
// with legacy AttemptRecord data; no v1 record is silently reinterpreted as a
// candidate slot or a semantic execution.
type PromptEvaluationEvidenceV2 struct {
	SchemaVersion                string
	RunID                        meta.ID
	Release                      EvidenceReleaseIdentity
	ExecutionPolicy              EvaluationExecutionPolicy
	GatePolicy                   ReleaseGatePolicy
	Status                       EvidenceStatus
	PreflightEvidence            []PreflightCaseEvidence
	Slots                        []CandidateSlot
	GenerationExecutions         []CandidateGenerationExecution
	SemanticExecutions           []SemanticEvaluationExecution
	HumanReviews                 []CandidateHumanReview
	UnresolvedResultUnknownCount int
	ResultUnknownResolutions     []ResultUnknownResolution
	StateTransitions             []EvidenceStateTransition
	GateResult                   *EvidenceGateResult
	Audit                        EvidenceRunAudit
	version                      int64
	execution                    *EvidenceExecutionCheckpoint
}

func NewPromptEvaluationEvidenceV2(
	runID meta.ID,
	release EvidenceReleaseIdentity,
	executionPolicy EvaluationExecutionPolicy,
	gatePolicy ReleaseGatePolicy,
	generationCaseIDs []string,
	preflightCaseID string,
	organizationID int64,
	requestedBy string,
	requestReason string,
	createdAt time.Time,
) (*PromptEvaluationEvidenceV2, error) {
	if len(generationCaseIDs) != executionPolicy.SlotPolicy.RequiredGenerationCases {
		return nil, fmt.Errorf("AI explanation evaluation generation case plan is invalid")
	}
	seen := make(map[string]struct{}, len(generationCaseIDs))
	slots := make([]CandidateSlot, 0, executionPolicy.RequiredCandidateCount())
	for _, caseID := range generationCaseIDs {
		if !evidenceEntityIDPattern.MatchString(caseID) {
			return nil, fmt.Errorf("AI explanation evaluation generation case id is invalid")
		}
		if _, exists := seen[caseID]; exists {
			return nil, fmt.Errorf("AI explanation evaluation generation case id is duplicated")
		}
		seen[caseID] = struct{}{}
		for ordinal := 1; ordinal <= executionPolicy.SlotPolicy.RequiredCandidatesPerCase; ordinal++ {
			slots = append(slots, CandidateSlot{CaseID: caseID, Ordinal: ordinal, Status: CandidateSlotPending})
		}
	}
	evidence := &PromptEvaluationEvidenceV2{
		SchemaVersion:   PromptEvaluationEvidenceSchemaVersionV2,
		RunID:           runID,
		version:         1,
		Release:         release,
		ExecutionPolicy: executionPolicy.Clone(),
		GatePolicy:      gatePolicy.Clone(),
		Status:          EvidenceStatusRequested,
		PreflightEvidence: []PreflightCaseEvidence{{
			CaseID: preflightCaseID, Status: PreflightEvidencePending,
		}},
		Slots: slots,
		StateTransitions: []EvidenceStateTransition{{
			To: EvidenceStatusRequested, CauseCode: "evaluation_requested", Actor: requestedBy, TransitionedAt: createdAt,
		}},
		Audit: EvidenceRunAudit{
			OrganizationID: organizationID, RequestedBy: requestedBy, RequestReason: requestReason, CreatedAt: createdAt,
		},
	}
	if err := evidence.Validate(); err != nil {
		return nil, err
	}
	return evidence, nil
}

func (e *PromptEvaluationEvidenceV2) Transition(to EvidenceStatus, causeCode, actor string, evidenceRefs []string, at time.Time) error {
	if e == nil || e.Status.IsTerminal() || !to.IsValid() || at.IsZero() {
		return fmt.Errorf("AI explanation evaluation state transition is invalid")
	}
	from := e.Status
	transition := EvidenceStateTransition{
		From: &from, To: to, CauseCode: causeCode, Actor: actor, TransitionedAt: at,
		EvidenceRefs: append([]string(nil), evidenceRefs...),
	}
	if err := transition.Validate(); err != nil || !isAllowedEvidenceTransition(transition.From, transition.To) {
		return fmt.Errorf("AI explanation evaluation state transition is invalid")
	}
	cloned := e.Clone()
	cloned.Status = to
	cloned.StateTransitions = append(cloned.StateTransitions, transition)
	cloned.version++
	if to != EvidenceStatusCollecting && cloned.execution != nil {
		return fmt.Errorf("AI explanation evaluation cannot leave collecting with an active execution")
	}
	switch to {
	case EvidenceStatusAwaitingReview:
		cloned.Audit.ClosedAt = copyTime(at)
	case EvidenceStatusApproved, EvidenceStatusRejected:
		cloned.Audit.FinalizedAt = copyTime(at)
	case EvidenceStatusCanceled:
		cloned.Audit.CanceledAt = copyTime(at)
	}
	if err := cloned.Validate(); err != nil {
		return err
	}
	*e = cloned
	return nil
}

func (e *PromptEvaluationEvidenceV2) Finalize(actor, causeCode string, at time.Time) error {
	if e == nil || e.Status != EvidenceStatusAwaitingReview {
		return fmt.Errorf("AI explanation evaluation awaiting-review evidence is required")
	}
	gate, err := e.EvaluateGate(at)
	if err != nil {
		return err
	}
	cloned := e.Clone()
	cloned.GateResult = &gate
	target := EvidenceStatusRejected
	if gate.Passed {
		target = EvidenceStatusApproved
	}
	if err := cloned.Transition(target, causeCode, actor, nil, at); err != nil {
		return err
	}
	*e = cloned
	return nil
}

func (e PromptEvaluationEvidenceV2) EvaluateGate(at time.Time) (EvidenceGateResult, error) {
	if e.Status != EvidenceStatusAwaitingReview || at.IsZero() {
		return EvidenceGateResult{}, fmt.Errorf("AI explanation evaluation awaiting-review evidence is required")
	}
	if err := e.Validate(); err != nil {
		return EvidenceGateResult{}, err
	}
	for _, review := range e.HumanReviews {
		if review.ReviewedAt.After(at) {
			return EvidenceGateResult{}, fmt.Errorf("AI explanation evaluation gate cannot predate a human review")
		}
	}
	return e.evaluateGateUnchecked(at), nil
}

func (e PromptEvaluationEvidenceV2) Validate() error {
	if e.SchemaVersion != PromptEvaluationEvidenceSchemaVersionV2 || e.RunID.IsZero() || !e.Status.IsValid() || e.version < 1 {
		return fmt.Errorf("AI explanation Prompt evaluation evidence v2 identity is invalid")
	}
	if err := e.ExecutionPolicy.Validate(); err != nil {
		return err
	}
	if err := e.GatePolicy.Validate(); err != nil {
		return err
	}
	if err := e.Release.Validate(e.ExecutionPolicy, e.GatePolicy); err != nil {
		return err
	}
	if err := e.Audit.Validate(e.Status); err != nil {
		return err
	}
	if err := e.validateExecutionCheckpoint(); err != nil {
		return err
	}
	if len(e.PreflightEvidence) != e.ExecutionPolicy.SlotPolicy.RequiredPreflightCases {
		return fmt.Errorf("AI explanation preflight evidence inventory is invalid")
	}
	for _, preflight := range e.PreflightEvidence {
		if err := preflight.Validate(); err != nil {
			return err
		}
		if preflight.EvaluatedAt != nil && preflight.EvaluatedAt.Before(e.Audit.CreatedAt) {
			return fmt.Errorf("AI explanation preflight evidence predates the run")
		}
	}
	if err := validateEvidenceStateTransitions(e.StateTransitions, e.Status, e.Audit.CreatedAt); err != nil {
		return err
	}
	if len(e.Slots) != e.ExecutionPolicy.RequiredCandidateCount() || len(e.GenerationExecutions) > e.ExecutionPolicy.Generation.MaxExecutionsPerRun || len(e.SemanticExecutions) > e.ExecutionPolicy.Semantic.MaxExecutionsPerRun {
		return fmt.Errorf("AI explanation Prompt evaluation evidence exceeds or misses its frozen plan")
	}
	generationByID, generationBySlot, err := validateGenerationExecutions(e.GenerationExecutions, e.ExecutionPolicy)
	if err != nil {
		return err
	}
	semanticByID, semanticByCandidate, err := validateSemanticExecutions(e.SemanticExecutions, e.ExecutionPolicy)
	if err != nil {
		return err
	}
	candidates, reviewReady, err := validateCandidateSlots(e.Slots, generationByID, generationBySlot, semanticByID, semanticByCandidate, e.ExecutionPolicy)
	if err != nil {
		return err
	}
	for _, slot := range e.Slots {
		if slot.CaseID == e.PreflightEvidence[0].CaseID {
			return fmt.Errorf("AI explanation preflight case overlaps a generation case")
		}
	}
	if err := validateCandidateReviews(e.HumanReviews, candidates, reviewReady); err != nil {
		return err
	}
	if len(e.HumanReviews) > 0 {
		if (e.Status != EvidenceStatusAwaitingReview && e.Status != EvidenceStatusApproved && e.Status != EvidenceStatusRejected && e.Status != EvidenceStatusCanceled) || e.Audit.ClosedAt == nil {
			return fmt.Errorf("AI explanation candidate review requires a closed evidence inventory")
		}
		for _, review := range e.HumanReviews {
			if e.Audit.CanceledAt != nil && review.ReviewedAt.After(*e.Audit.CanceledAt) {
				return fmt.Errorf("AI explanation candidate review postdates cancellation")
			}
			if review.ReviewedAt.Before(*e.Audit.ClosedAt) {
				return fmt.Errorf("AI explanation candidate review predates evidence collection closure")
			}
		}
	}
	unknownExecutions := make(map[string]time.Time)
	for _, execution := range e.GenerationExecutions {
		if execution.Status == ExecutionStatusResultUnknown {
			unknownExecutions[execution.ID] = *execution.FinishedAt
		}
	}
	for _, execution := range e.SemanticExecutions {
		if execution.Status == ExecutionStatusResultUnknown {
			unknownExecutions[execution.ID] = *execution.FinishedAt
		}
	}
	resolvedUnknown, err := validateResultUnknownResolutions(e.ResultUnknownResolutions, unknownExecutions)
	if err != nil {
		return err
	}
	unresolvedUnknown := len(unknownExecutions) - resolvedUnknown
	if e.UnresolvedResultUnknownCount != unresolvedUnknown {
		return fmt.Errorf("AI explanation unresolved result-unknown count is inconsistent")
	}
	if unresolvedUnknown > 0 && e.Status != EvidenceStatusBlocked && e.Status != EvidenceStatusCanceled {
		return fmt.Errorf("AI explanation unresolved result-unknown execution must block the evaluation")
	}
	preflightComplete := len(e.PreflightEvidence) == 1 && e.PreflightEvidence[0].Status == PreflightEvidencePassed
	complete := preflightComplete && len(candidates) == e.ExecutionPolicy.RequiredCandidateCount() && len(reviewReady) == len(candidates) && unresolvedUnknown == 0
	if e.Status == EvidenceStatusAwaitingReview && !complete {
		return fmt.Errorf("AI explanation evaluation cannot await review before candidate evidence is complete")
	}
	if e.Status == EvidenceStatusApproved || e.Status == EvidenceStatusRejected {
		if !complete || len(e.HumanReviews) != e.GatePolicy.HumanAccountability.RequiredReviewCount || e.GateResult == nil {
			return fmt.Errorf("AI explanation terminal evaluation evidence is incomplete")
		}
		if err := e.GateResult.Validate(); err != nil {
			return err
		}
		for _, review := range e.HumanReviews {
			if review.ReviewedAt.After(e.GateResult.EvaluatedAt) {
				return fmt.Errorf("AI explanation evaluation gate predates a human review")
			}
		}
		expectedGate := e.evaluateGateUnchecked(e.GateResult.EvaluatedAt)
		if !reflect.DeepEqual(*e.GateResult, expectedGate) {
			return fmt.Errorf("AI explanation evaluation gate result is not derived from its frozen evidence")
		}
		if (e.Status == EvidenceStatusApproved) != e.GateResult.Passed {
			return fmt.Errorf("AI explanation evaluation terminal state contradicts its gate result")
		}
	} else if e.GateResult != nil {
		return fmt.Errorf("AI explanation non-final evaluation cannot contain a final gate result")
	}
	return nil
}

func (e PromptEvaluationEvidenceV2) evaluateGateUnchecked(at time.Time) EvidenceGateResult {
	result := EvidenceGateResult{
		EvaluatedAt: at,
		GatePasses:  map[string]bool{"G1": true, "G2": true, "G3": true, "G4": true, "G5": true},
	}
	addReason := func(gate, code, detail string, refs ...string) {
		result.GatePasses[gate] = false
		result.Reasons = append(result.Reasons, EvidenceGateReason{Gate: gate, Code: code, Detail: detail, EvidenceRefs: append([]string(nil), refs...)})
	}

	dispatched, infrastructureSucceeded := 0, 0
	definiteGenerationOutput, conformantGenerationOutput := 0, 0
	for _, execution := range e.GenerationExecutions {
		if execution.ProviderCallCount == 1 {
			dispatched++
			if e.GatePolicy.countsInfrastructureSuccess(execution.Status, execution.InvocationID, execution.ProviderReceipt, execution.Failure) {
				infrastructureSucceeded++
			}
		}
		if len(execution.RawOutput) > 0 && execution.Status != ExecutionStatusResultUnknown && execution.Status != ExecutionStatusDispatching {
			definiteGenerationOutput++
			if execution.Status == ExecutionStatusSucceeded {
				conformantGenerationOutput++
			}
		}
	}
	semanticDispatched, semanticSucceeded := 0, 0
	for _, execution := range e.SemanticExecutions {
		if execution.ProviderCallCount == 1 {
			dispatched++
			semanticDispatched++
			if e.GatePolicy.countsInfrastructureSuccess(execution.Status, execution.InvocationID, execution.ProviderReceipt, execution.Failure) {
				infrastructureSucceeded++
			}
		}
		if execution.Status == ExecutionStatusSucceeded {
			semanticSucceeded++
		}
	}
	infrastructureRate := evidenceRate(infrastructureSucceeded, dispatched)
	generationConformanceRate := evidenceRate(conformantGenerationOutput, definiteGenerationOutput)
	semanticSuccessRate := evidenceRate(semanticSucceeded, semanticDispatched)
	reliability := e.GatePolicy.ExecutionReliability
	result.Metrics = append(result.Metrics,
		EvidenceGateMetric{Name: "infrastructure_success_rate", Numerator: infrastructureSucceeded, Denominator: dispatched, Value: infrastructureRate, Threshold: reliability.MinInfrastructureSuccessRate},
		EvidenceGateMetric{Name: "generation_contract_conformance_rate", Numerator: conformantGenerationOutput, Denominator: definiteGenerationOutput, Value: generationConformanceRate, Threshold: reliability.MinGenerationContractConformanceRate},
		EvidenceGateMetric{Name: "semantic_execution_success_rate", Numerator: semanticSucceeded, Denominator: semanticDispatched, Value: semanticSuccessRate, Threshold: reliability.MinSemanticExecutionSuccessRate},
	)
	if infrastructureRate < reliability.MinInfrastructureSuccessRate {
		addReason("G3", "infrastructure_success_rate_below_threshold", "Provider execution reliability is below the frozen threshold")
	}
	if generationConformanceRate < reliability.MinGenerationContractConformanceRate {
		addReason("G3", "generation_contract_conformance_rate_below_threshold", "Generation contract conformance is below the frozen threshold")
	}
	if semanticSuccessRate < reliability.MinSemanticExecutionSuccessRate {
		addReason("G3", "semantic_execution_success_rate_below_threshold", "Semantic execution reliability is below the frozen threshold")
	}

	casePasses := make(map[string]int, e.ExecutionPolicy.SlotPolicy.RequiredGenerationCases)
	overallPasses := 0
	semanticCount := 0
	var scoreTotals SemanticScoreThresholds
	semanticByID := make(map[string]SemanticEvaluationExecution, len(e.SemanticExecutions))
	for _, execution := range e.SemanticExecutions {
		semanticByID[execution.ID] = execution
	}
	minimumScores := e.GatePolicy.CandidateQuality.MinimumSemanticScores
	for _, slot := range e.Slots {
		if e.GatePolicy.Version == "v2" {
			if _, exists := casePasses[slot.CaseID]; !exists {
				casePasses[slot.CaseID] = 0
			}
		}
		candidate := slot.Candidate
		casePresent, casePassed, hardPassed := evaluateCandidateAssertions(candidate.Assertions)
		if !casePresent || !casePassed {
			if !casePresent || e.GatePolicy.Version == "v1" {
				addReason("G4", "candidate_case_assertion_failed", "Candidate did not pass all case-scoped assertions", candidate.ID)
			}
		} else {
			casePasses[slot.CaseID]++
			overallPasses++
		}
		if !hardPassed {
			addReason("G4", "candidate_hard_assertion_failed", "Candidate did not pass all hard assertions", candidate.ID)
		}
		semantic := semanticByID[candidate.AcceptedSemanticExecutionID]
		if semantic.Result == nil {
			addReason("G4", "candidate_semantic_result_missing", "Candidate has no accepted semantic result", candidate.ID)
			continue
		}
		semanticCount++
		scores := semantic.Result.Scores
		scoreTotals.Faithfulness += float64(scores.Faithfulness)
		scoreTotals.CrossDimensionQuality += float64(scores.CrossDimensionQuality)
		scoreTotals.SuggestionActionability += float64(scores.SuggestionActionability)
		scoreTotals.AudienceClarity += float64(scores.AudienceClarity)
		scoreTotals.Concision += float64(scores.Concision)
		if float64(scores.Faithfulness) < minimumScores.Faithfulness ||
			float64(scores.CrossDimensionQuality) < minimumScores.CrossDimensionQuality ||
			float64(scores.SuggestionActionability) < minimumScores.SuggestionActionability ||
			float64(scores.AudienceClarity) < minimumScores.AudienceClarity ||
			float64(scores.Concision) < minimumScores.Concision {
			addReason("G4", "candidate_semantic_score_below_minimum", "Candidate semantic score is below the frozen per-output minimum", candidate.ID)
		}
	}
	caseIDs := make([]string, 0, len(casePasses))
	for caseID := range casePasses {
		caseIDs = append(caseIDs, caseID)
	}
	sort.Strings(caseIDs)
	for _, caseID := range caseIDs {
		passes := casePasses[caseID]
		if passes < e.GatePolicy.CandidateQuality.MinAssertionPassesPerCase {
			addReason("G4", "case_assertion_stability_failed", "Case has too few passing candidates", caseID)
		}
	}
	if overallPasses < e.GatePolicy.CandidateQuality.MinAssertionPassesOverall {
		addReason("G4", "case_assertion_overall_failed", "Too few candidates passed case assertions")
	}
	if semanticCount != e.ExecutionPolicy.RequiredCandidateCount() || !semanticAveragesMeet(scoreTotals, semanticCount, e.GatePolicy.CandidateQuality.MinimumSemanticAverages) {
		addReason("G4", "semantic_average_below_threshold", "Semantic score averages do not meet the frozen release threshold")
	}

	reviewsByCandidate := make(map[string]map[ReviewRole]CandidateHumanReview, len(e.Slots))
	for _, review := range e.HumanReviews {
		if reviewsByCandidate[review.CandidateID] == nil {
			reviewsByCandidate[review.CandidateID] = make(map[ReviewRole]CandidateHumanReview, 2)
		}
		reviewsByCandidate[review.CandidateID][review.Role] = review
		if review.Decision == ReviewDecisionReject {
			addReason("G5", "human_review_rejected", "A required human reviewer rejected the candidate", review.CandidateID)
		}
	}
	for _, slot := range e.Slots {
		candidateID := slot.Candidate.ID
		roles := reviewsByCandidate[candidateID]
		for _, role := range e.GatePolicy.HumanAccountability.RequiredRoles {
			if _, exists := roles[role]; !exists {
				addReason("G5", "human_review_incomplete", "Candidate is missing a required human review role", candidateID)
			}
		}
	}
	if len(e.HumanReviews) != e.GatePolicy.HumanAccountability.RequiredReviewCount {
		addReason("G5", "human_review_count_incomplete", "Human review evidence does not meet the frozen count")
	}

	result.Passed = true
	for _, gate := range []string{"G1", "G2", "G3", "G4", "G5"} {
		result.Passed = result.Passed && result.GatePasses[gate]
	}
	return result
}

func (p ReleaseGatePolicy) countsInfrastructureSuccess(status ExecutionStatus, invocationID string, receipt *aiexplanation.ProviderReceipt, failure *ClassifiedFailure) bool {
	// Preserve the original definite-terminal calculation for frozen v1 Runs.
	if p.Version == "v1" {
		return status == ExecutionStatusSucceeded || status == ExecutionStatusFailed
	}
	if status == ExecutionStatusSucceeded {
		return true // Execution validation requires a matching Provider receipt.
	}
	if status != ExecutionStatusFailed || failure == nil || receipt == nil ||
		receipt.Validate() != nil || validateV2ProviderReceipt(*receipt, invocationID) != nil {
		return false
	}
	// A completed Provider response may fail output validation. Count that
	// response here; its failure remains in the separate contract/semantic gate.
	switch failure.Kind {
	case FailureKindOutputContractConformance:
		return true
	case FailureKindSemanticExecution:
		switch failure.Code {
		case SemanticOutputMissingOrTooLarge, SemanticOutputSchemaInvalid,
			SemanticOutputDecodeInvalid, SemanticDecisionContractInvalid:
			return true
		}
	}
	return false
}

func evaluateCandidateAssertions(assertions []AssertionReceipt) (casePresent, casePassed, hardPassed bool) {
	type group struct {
		scope  AssertionScope
		hard   bool
		passed bool
		failed bool
	}
	groups := make(map[string]*group, len(assertions))
	for _, assertion := range assertions {
		key := string(assertion.Scope) + "\x00" + assertion.Type + "\x00" + fmt.Sprint(assertion.Ordinal)
		value := groups[key]
		if value == nil {
			value = &group{scope: assertion.Scope}
			groups[key] = value
		}
		value.hard = value.hard || assertion.Hard || assertion.Scope == AssertionScopeDefault
		value.passed = value.passed || assertion.Status == AssertionPassed
		value.failed = value.failed || assertion.Status == AssertionFailed || assertion.Status == AssertionBlocked
	}
	casePassed, hardPassed = true, true
	for _, value := range groups {
		effectivePassed := value.passed && !value.failed
		if value.scope == AssertionScopeCase {
			casePresent = true
			casePassed = casePassed && effectivePassed
		}
		if value.hard {
			hardPassed = hardPassed && effectivePassed
		}
	}
	return casePresent, casePassed, hardPassed
}

func semanticAveragesMeet(totals SemanticScoreThresholds, count int, minimum SemanticScoreThresholds) bool {
	if count == 0 {
		return false
	}
	denominator := float64(count)
	return totals.Faithfulness/denominator >= minimum.Faithfulness &&
		totals.CrossDimensionQuality/denominator >= minimum.CrossDimensionQuality &&
		totals.SuggestionActionability/denominator >= minimum.SuggestionActionability &&
		totals.AudienceClarity/denominator >= minimum.AudienceClarity &&
		totals.Concision/denominator >= minimum.Concision
}

func evidenceRate(numerator, denominator int) float64 {
	if denominator == 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}

func validateGenerationExecutions(values []CandidateGenerationExecution, policy EvaluationExecutionPolicy) (map[string]CandidateGenerationExecution, map[string][]CandidateGenerationExecution, error) {
	byID := make(map[string]CandidateGenerationExecution, len(values))
	bySlot := make(map[string][]CandidateGenerationExecution)
	invocations := make(map[string]struct{}, len(values))
	for _, value := range values {
		if err := value.Validate(); err != nil {
			return nil, nil, err
		}
		if value.Status == ExecutionStatusPrepared || value.Status == ExecutionStatusDispatching {
			return nil, nil, fmt.Errorf("AI explanation generation execution inventory must contain terminal evidence only")
		}
		if _, exists := byID[value.ID]; exists {
			return nil, nil, fmt.Errorf("AI explanation generation execution id is duplicated")
		}
		if _, exists := invocations[value.InvocationID]; exists {
			return nil, nil, fmt.Errorf("AI explanation generation invocation id is duplicated")
		}
		byID[value.ID] = value
		invocations[value.InvocationID] = struct{}{}
		key := value.CaseID + "\x00" + fmt.Sprint(value.SlotOrdinal)
		bySlot[key] = append(bySlot[key], value)
		if len(bySlot[key]) > policy.Generation.MaxExecutionsPerSlot {
			return nil, nil, fmt.Errorf("AI explanation generation execution slot budget is exceeded")
		}
	}
	for key := range bySlot {
		sort.Slice(bySlot[key], func(i, j int) bool { return bySlot[key][i].ExecutionOrdinal < bySlot[key][j].ExecutionOrdinal })
		for index, execution := range bySlot[key] {
			if execution.ExecutionOrdinal != index+1 {
				return nil, nil, fmt.Errorf("AI explanation generation execution ordinals are not contiguous")
			}
		}
	}
	return byID, bySlot, nil
}

func validateSemanticExecutions(values []SemanticEvaluationExecution, policy EvaluationExecutionPolicy) (map[string]SemanticEvaluationExecution, map[string][]SemanticEvaluationExecution, error) {
	byID := make(map[string]SemanticEvaluationExecution, len(values))
	byCandidate := make(map[string][]SemanticEvaluationExecution)
	invocations := make(map[string]struct{}, len(values))
	for _, value := range values {
		if err := value.Validate(); err != nil {
			return nil, nil, err
		}
		if value.Status == ExecutionStatusPrepared || value.Status == ExecutionStatusDispatching {
			return nil, nil, fmt.Errorf("AI explanation semantic execution inventory must contain terminal evidence only")
		}
		if _, exists := byID[value.ID]; exists {
			return nil, nil, fmt.Errorf("AI explanation semantic execution id is duplicated")
		}
		if _, exists := invocations[value.InvocationID]; exists {
			return nil, nil, fmt.Errorf("AI explanation semantic invocation id is duplicated")
		}
		byID[value.ID] = value
		invocations[value.InvocationID] = struct{}{}
		byCandidate[value.CandidateID] = append(byCandidate[value.CandidateID], value)
		if len(byCandidate[value.CandidateID]) > policy.Semantic.MaxExecutionsPerCandidate {
			return nil, nil, fmt.Errorf("AI explanation semantic execution candidate budget is exceeded")
		}
	}
	for candidateID := range byCandidate {
		sort.Slice(byCandidate[candidateID], func(i, j int) bool {
			return byCandidate[candidateID][i].ExecutionOrdinal < byCandidate[candidateID][j].ExecutionOrdinal
		})
		for index, execution := range byCandidate[candidateID] {
			if execution.ExecutionOrdinal != index+1 {
				return nil, nil, fmt.Errorf("AI explanation semantic execution ordinals are not contiguous")
			}
		}
	}
	return byID, byCandidate, nil
}

func validateCandidateSlots(slots []CandidateSlot, generationByID map[string]CandidateGenerationExecution, generationBySlot map[string][]CandidateGenerationExecution, semanticByID map[string]SemanticEvaluationExecution, semanticByCandidate map[string][]SemanticEvaluationExecution, policy EvaluationExecutionPolicy) (map[string]Candidate, map[string]Candidate, error) {
	seenSlots := make(map[string]struct{}, len(slots))
	caseSlots := make(map[string]int)
	candidates := make(map[string]Candidate)
	reviewReady := make(map[string]Candidate)
	attachedGeneration := make(map[string]struct{}, len(generationByID))
	attachedSemantic := make(map[string]struct{}, len(semanticByID))
	for _, slot := range slots {
		if !evidenceEntityIDPattern.MatchString(slot.CaseID) || slot.Ordinal < 1 || slot.Ordinal > policy.SlotPolicy.RequiredCandidatesPerCase || !slot.Status.IsValid() {
			return nil, nil, fmt.Errorf("AI explanation candidate slot identity is invalid")
		}
		key := slot.key()
		if _, exists := seenSlots[key]; exists {
			return nil, nil, fmt.Errorf("AI explanation candidate slot is duplicated")
		}
		seenSlots[key] = struct{}{}
		caseSlots[slot.CaseID]++
		if err := validateUniqueEntityIDs(slot.GenerationExecutionIDs); err != nil {
			return nil, nil, err
		}
		executions := generationBySlot[key]
		if len(executions) != len(slot.GenerationExecutionIDs) {
			return nil, nil, fmt.Errorf("AI explanation candidate slot generation execution inventory is inconsistent")
		}
		for index, executionID := range slot.GenerationExecutionIDs {
			if executions[index].ID != executionID {
				return nil, nil, fmt.Errorf("AI explanation candidate slot generation execution order is inconsistent")
			}
			if _, exists := generationByID[executionID]; !exists {
				return nil, nil, fmt.Errorf("AI explanation candidate slot references an unknown generation execution")
			}
			attachedGeneration[executionID] = struct{}{}
		}
		firstSucceeded := ""
		for index, execution := range executions {
			if execution.Status == ExecutionStatusSucceeded {
				if index != len(executions)-1 {
					return nil, nil, fmt.Errorf("AI explanation generation cannot continue after a candidate is accepted")
				}
				firstSucceeded = execution.ID
				break
			}
		}
		if slot.Status == CandidateSlotAccepted {
			if slot.Candidate == nil || firstSucceeded == "" || slot.Candidate.GenerationExecutionID != firstSucceeded {
				return nil, nil, fmt.Errorf("AI explanation candidate slot must accept its first contract-conformant execution")
			}
			candidate := *slot.Candidate
			if err := candidate.Validate(); err != nil {
				return nil, nil, err
			}
			if _, exists := candidates[candidate.ID]; exists {
				return nil, nil, fmt.Errorf("AI explanation candidate id is duplicated")
			}
			generation := generationByID[candidate.GenerationExecutionID]
			if generation.FinishedAt == nil || candidate.AcceptedAt.Before(*generation.FinishedAt) || candidate.NormalizedOutputFingerprint != generation.NormalizedOutputFingerprint {
				return nil, nil, fmt.Errorf("AI explanation candidate output does not match its generation execution")
			}
			semanticExecutions := semanticByCandidate[candidate.ID]
			if len(semanticExecutions) != len(candidate.SemanticExecutionIDs) {
				return nil, nil, fmt.Errorf("AI explanation candidate semantic execution inventory is inconsistent")
			}
			firstSemanticSuccess := ""
			for index, semanticID := range candidate.SemanticExecutionIDs {
				execution, exists := semanticByID[semanticID]
				if !exists || execution.CandidateID != candidate.ID || semanticExecutions[index].ID != semanticID || execution.StartedAt.Before(candidate.AcceptedAt) {
					return nil, nil, fmt.Errorf("AI explanation candidate references an invalid semantic execution")
				}
				if firstSemanticSuccess == "" && execution.Status == ExecutionStatusSucceeded {
					if index != len(candidate.SemanticExecutionIDs)-1 {
						return nil, nil, fmt.Errorf("AI explanation semantic evaluation cannot continue after a receipt is accepted")
					}
					firstSemanticSuccess = semanticID
				}
				attachedSemantic[semanticID] = struct{}{}
			}
			if candidate.AcceptedSemanticExecutionID != firstSemanticSuccess {
				return nil, nil, fmt.Errorf("AI explanation candidate must accept its first complete semantic execution")
			}
			candidates[candidate.ID] = candidate
			if candidate.ReviewReady {
				reviewReady[candidate.ID] = candidate
			}
		} else if slot.Candidate != nil || firstSucceeded != "" {
			return nil, nil, fmt.Errorf("AI explanation unaccepted slot cannot hide an eligible candidate")
		}
	}
	if len(caseSlots) != policy.SlotPolicy.RequiredGenerationCases {
		return nil, nil, fmt.Errorf("AI explanation candidate slot case inventory is invalid")
	}
	for _, count := range caseSlots {
		if count != policy.SlotPolicy.RequiredCandidatesPerCase {
			return nil, nil, fmt.Errorf("AI explanation candidate slot distribution is invalid")
		}
	}
	if len(attachedGeneration) != len(generationByID) || len(attachedSemantic) != len(semanticByID) {
		return nil, nil, fmt.Errorf("AI explanation execution evidence is not attached to a frozen candidate slot")
	}
	return candidates, reviewReady, nil
}

func validateCandidateReviews(values []CandidateHumanReview, candidates, reviewReady map[string]Candidate) error {
	seenRole := make(map[string]struct{}, len(values))
	seenReviewer := make(map[string]struct{}, len(values))
	for _, value := range values {
		if err := value.Validate(); err != nil {
			return err
		}
		candidate, exists := candidates[value.CandidateID]
		if !exists {
			return fmt.Errorf("AI explanation human review references an unknown candidate")
		}
		if _, exists := reviewReady[value.CandidateID]; !exists {
			return fmt.Errorf("AI explanation human review cannot target a candidate without complete semantic evidence")
		}
		if value.ReviewedAt.Before(candidate.AcceptedAt) {
			return fmt.Errorf("AI explanation human review predates its candidate")
		}
		roleKey := value.CandidateID + "\x00" + string(value.Role)
		reviewerKey := value.CandidateID + "\x00" + value.Reviewer
		if _, exists := seenRole[roleKey]; exists {
			return fmt.Errorf("AI explanation candidate review role is duplicated")
		}
		if _, exists := seenReviewer[reviewerKey]; exists {
			return fmt.Errorf("AI explanation candidate reviewer cannot fill both roles")
		}
		seenRole[roleKey] = struct{}{}
		seenReviewer[reviewerKey] = struct{}{}
	}
	return nil
}

func validateNormalizedEvidence(raw []byte, fingerprint aiexplanation.Fingerprint) error {
	if len(raw) == 0 {
		if fingerprint != "" {
			return fmt.Errorf("AI explanation output fingerprint requires normalized output")
		}
		return nil
	}
	if !json.Valid(raw) || fingerprint != aiexplanation.NewFingerprint(raw) {
		return fmt.Errorf("AI explanation normalized output evidence is invalid")
	}
	return nil
}

func validateV2ProviderReceipt(receipt aiexplanation.ProviderReceipt, invocationID string) error {
	if receipt.InvocationID != invocationID || !evidenceEntityIDPattern.MatchString(receipt.RequestID) ||
		!policyIdentifierPattern.MatchString(receipt.Provider) || len(receipt.Model) > 128 {
		return fmt.Errorf("AI explanation evaluation Provider receipt identity is invalid")
	}
	return nil
}

func validateUniqueEntityIDs(values []string) error {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if !evidenceEntityIDPattern.MatchString(value) {
			return fmt.Errorf("AI explanation evidence reference is invalid")
		}
		if _, exists := seen[value]; exists {
			return fmt.Errorf("AI explanation evidence reference is duplicated")
		}
		seen[value] = struct{}{}
	}
	return nil
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func cloneCandidate(value *Candidate) *Candidate {
	if value == nil {
		return nil
	}
	cloned := *value
	cloned.Assertions = append([]AssertionReceipt(nil), value.Assertions...)
	cloned.SemanticExecutionIDs = append([]string(nil), value.SemanticExecutionIDs...)
	return &cloned
}

func (e PromptEvaluationEvidenceV2) Clone() PromptEvaluationEvidenceV2 {
	cloned := e
	cloned.execution = cloneEvidenceExecutionCheckpoint(e.execution)
	cloned.ExecutionPolicy = e.ExecutionPolicy.Clone()
	cloned.GatePolicy = e.GatePolicy.Clone()
	cloned.PreflightEvidence = append([]PreflightCaseEvidence(nil), e.PreflightEvidence...)
	for index := range cloned.PreflightEvidence {
		cloned.PreflightEvidence[index].Assertions = append([]AssertionReceipt(nil), e.PreflightEvidence[index].Assertions...)
		cloned.PreflightEvidence[index].EvaluatedAt = copyTimePtr(e.PreflightEvidence[index].EvaluatedAt)
	}
	cloned.Slots = make([]CandidateSlot, len(e.Slots))
	for index, slot := range e.Slots {
		cloned.Slots[index] = slot
		cloned.Slots[index].GenerationExecutionIDs = append([]string(nil), slot.GenerationExecutionIDs...)
		cloned.Slots[index].Candidate = cloneCandidate(slot.Candidate)
	}
	cloned.GenerationExecutions = append([]CandidateGenerationExecution(nil), e.GenerationExecutions...)
	for index := range cloned.GenerationExecutions {
		cloned.GenerationExecutions[index].RawOutput = append([]byte(nil), e.GenerationExecutions[index].RawOutput...)
		cloned.GenerationExecutions[index].NormalizedOutput = append([]byte(nil), e.GenerationExecutions[index].NormalizedOutput...)
		if e.GenerationExecutions[index].Failure != nil {
			failure := e.GenerationExecutions[index].Failure.Clone()
			cloned.GenerationExecutions[index].Failure = &failure
		}
	}
	cloned.SemanticExecutions = append([]SemanticEvaluationExecution(nil), e.SemanticExecutions...)
	for index := range cloned.SemanticExecutions {
		cloned.SemanticExecutions[index].RawOutput = append([]byte(nil), e.SemanticExecutions[index].RawOutput...)
		cloned.SemanticExecutions[index].NormalizedOutput = append([]byte(nil), e.SemanticExecutions[index].NormalizedOutput...)
		if e.SemanticExecutions[index].Failure != nil {
			failure := e.SemanticExecutions[index].Failure.Clone()
			cloned.SemanticExecutions[index].Failure = &failure
		}
		if e.SemanticExecutions[index].Result != nil {
			result := *e.SemanticExecutions[index].Result
			result.Decisions = append([]SemanticDecision(nil), e.SemanticExecutions[index].Result.Decisions...)
			cloned.SemanticExecutions[index].Result = &result
		}
	}
	cloned.HumanReviews = append([]CandidateHumanReview(nil), e.HumanReviews...)
	cloned.ResultUnknownResolutions = append([]ResultUnknownResolution(nil), e.ResultUnknownResolutions...)
	cloned.StateTransitions = append([]EvidenceStateTransition(nil), e.StateTransitions...)
	for index := range cloned.StateTransitions {
		cloned.StateTransitions[index].EvidenceRefs = append([]string(nil), e.StateTransitions[index].EvidenceRefs...)
		if e.StateTransitions[index].From != nil {
			from := *e.StateTransitions[index].From
			cloned.StateTransitions[index].From = &from
		}
	}
	if e.GateResult != nil {
		gate := *e.GateResult
		gate.GatePasses = make(map[string]bool, len(e.GateResult.GatePasses))
		for key, value := range e.GateResult.GatePasses {
			gate.GatePasses[key] = value
		}
		gate.Metrics = append([]EvidenceGateMetric(nil), e.GateResult.Metrics...)
		gate.Reasons = append([]EvidenceGateReason(nil), e.GateResult.Reasons...)
		for index := range gate.Reasons {
			gate.Reasons[index].EvidenceRefs = append([]string(nil), e.GateResult.Reasons[index].EvidenceRefs...)
		}
		cloned.GateResult = &gate
	}
	cloned.Audit.ClosedAt = copyTimePtr(e.Audit.ClosedAt)
	cloned.Audit.FinalizedAt = copyTimePtr(e.Audit.FinalizedAt)
	cloned.Audit.CanceledAt = copyTimePtr(e.Audit.CanceledAt)
	return cloned
}

func validateResultUnknownResolutions(values []ResultUnknownResolution, unknownExecutions map[string]time.Time) (int, error) {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if err := value.Validate(); err != nil {
			return 0, err
		}
		finishedAt, exists := unknownExecutions[value.ExecutionID]
		if !exists || value.ResolvedAt.Before(finishedAt) {
			return 0, fmt.Errorf("AI explanation result-unknown resolution does not match an unknown execution")
		}
		if _, exists := seen[value.ExecutionID]; exists {
			return 0, fmt.Errorf("AI explanation result-unknown execution is resolved more than once")
		}
		seen[value.ExecutionID] = struct{}{}
	}
	return len(seen), nil
}

func validateEvidenceStateTransitions(values []EvidenceStateTransition, status EvidenceStatus, createdAt time.Time) error {
	if len(values) == 0 || len(values) > 512 {
		return fmt.Errorf("AI explanation evaluation state transition history is required")
	}
	var previous *EvidenceStatus
	var previousAt time.Time
	for index, value := range values {
		if err := value.Validate(); err != nil {
			return err
		}
		if index == 0 {
			if value.From != nil || value.To != EvidenceStatusRequested || value.TransitionedAt.Before(createdAt) {
				return fmt.Errorf("AI explanation evaluation initial state transition is invalid")
			}
		} else if value.From == nil || previous == nil || *value.From != *previous || value.TransitionedAt.Before(previousAt) {
			return fmt.Errorf("AI explanation evaluation state transition history is not contiguous")
		}
		if !isAllowedEvidenceTransition(value.From, value.To) {
			return fmt.Errorf("AI explanation evaluation state transition is not allowed")
		}
		to := value.To
		previous = &to
		previousAt = value.TransitionedAt
	}
	if previous == nil || *previous != status {
		return fmt.Errorf("AI explanation evaluation state does not match its transition history")
	}
	return nil
}
