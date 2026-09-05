package evaluation

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
)

const (
	EvaluationExecutionPolicySchemaVersionV1 = "ai-explanation-evaluation-execution-policy/v1"
	ReleaseGatePolicySchemaVersionV1         = "ai-explanation-release-gate-policy/v1"
)

var (
	policyIdentifierPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,127}$`)
	policyVersionPattern    = regexp.MustCompile(`^v[1-9][0-9]*$`)
)

type CandidateSelection string

const CandidateSelectionFirstContractConformant CandidateSelection = "first_contract_conformant_execution"

type FailureSelector struct {
	Stage FailureStage `json:"stage"`
	Code  string       `json:"code"`
}

func (s FailureSelector) Validate() error {
	if !s.Stage.IsValid() || !policyIdentifierPattern.MatchString(s.Code) {
		return fmt.Errorf("AI explanation evaluation failure selector is invalid")
	}
	return nil
}

func (s FailureSelector) key() string {
	return string(s.Stage) + "\x00" + s.Code
}

type EvaluationSlotPolicy struct {
	RequiredGenerationCases   int                `json:"required_generation_cases"`
	RequiredCandidatesPerCase int                `json:"required_candidates_per_case"`
	RequiredPreflightCases    int                `json:"required_preflight_cases"`
	CandidateSelection        CandidateSelection `json:"candidate_selection"`
}

type GenerationExecutionBudget struct {
	MaxExecutionsPerSlot int `json:"max_executions_per_slot"`
	MaxExecutionsPerRun  int `json:"max_executions_per_run"`
}

type SemanticExecutionBudget struct {
	MaxExecutionsPerCandidate int `json:"max_executions_per_candidate"`
	MaxExecutionsPerRun       int `json:"max_executions_per_run"`
}

type EvaluationRecoveryPolicy struct {
	AutoRetryableStageCodes                    []FailureSelector `json:"auto_retryable_stage_codes"`
	ManualRecoveryStageCodes                   []FailureSelector `json:"manual_recovery_stage_codes"`
	ResultUnknownRequiresManualAcknowledgement bool              `json:"result_unknown_requires_manual_acknowledgement"`
	QualityFailureReplacementAllowed           bool              `json:"quality_failure_replacement_allowed"`
	SemanticFailureRegeneratesCandidate        bool              `json:"semantic_failure_regenerates_candidate"`
}

func (p EvaluationRecoveryPolicy) AllowsAutomaticRetry(failure ClassifiedFailure) bool {
	if failure.Validate() != nil || !failure.Retryable {
		return false
	}
	selector := FailureSelector{Stage: failure.Stage, Code: failure.Code}
	for _, allowed := range p.AutoRetryableStageCodes {
		if allowed.key() == selector.key() {
			return true
		}
	}
	return false
}

// EvaluationExecutionPolicy freezes sample targets independently from the
// maximum number of external calls allowed to collect that evidence.
type EvaluationExecutionPolicy struct {
	SchemaVersion string                    `json:"schema_version"`
	PolicyID      string                    `json:"policy_id"`
	Version       string                    `json:"version"`
	SlotPolicy    EvaluationSlotPolicy      `json:"slot_policy"`
	Generation    GenerationExecutionBudget `json:"generation_budget"`
	Semantic      SemanticExecutionBudget   `json:"semantic_budget"`
	Recovery      EvaluationRecoveryPolicy  `json:"recovery_policy"`
}

func (p EvaluationExecutionPolicy) Validate() error {
	if p.SchemaVersion != EvaluationExecutionPolicySchemaVersionV1 ||
		!policyIdentifierPattern.MatchString(p.PolicyID) || !policyVersionPattern.MatchString(p.Version) {
		return fmt.Errorf("AI explanation evaluation execution policy identity is invalid")
	}
	if p.SlotPolicy.RequiredGenerationCases != RequiredGenerationCaseCount ||
		p.SlotPolicy.RequiredCandidatesPerCase != RequiredRepetitionsPerCase ||
		p.SlotPolicy.RequiredPreflightCases != 1 ||
		p.SlotPolicy.CandidateSelection != CandidateSelectionFirstContractConformant {
		return fmt.Errorf("AI explanation evaluation execution policy slot plan is invalid")
	}
	requiredCandidates := p.RequiredCandidateCount()
	if p.Generation.MaxExecutionsPerSlot != 2 ||
		p.Generation.MaxExecutionsPerRun != requiredCandidates*p.Generation.MaxExecutionsPerSlot {
		return fmt.Errorf("AI explanation evaluation generation execution budget is invalid")
	}
	if p.Semantic.MaxExecutionsPerCandidate != 2 ||
		p.Semantic.MaxExecutionsPerRun != requiredCandidates*p.Semantic.MaxExecutionsPerCandidate {
		return fmt.Errorf("AI explanation evaluation semantic execution budget is invalid")
	}
	if !p.Recovery.ResultUnknownRequiresManualAcknowledgement ||
		p.Recovery.QualityFailureReplacementAllowed || p.Recovery.SemanticFailureRegeneratesCandidate {
		return fmt.Errorf("AI explanation evaluation recovery invariants are invalid")
	}
	seen := make(map[string]string, len(p.Recovery.AutoRetryableStageCodes)+len(p.Recovery.ManualRecoveryStageCodes))
	for _, entry := range p.Recovery.AutoRetryableStageCodes {
		if err := entry.Validate(); err != nil {
			return err
		}
		if _, exists := seen[entry.key()]; exists {
			return fmt.Errorf("AI explanation evaluation recovery selector is duplicated")
		}
		seen[entry.key()] = "automatic"
	}
	for _, entry := range p.Recovery.ManualRecoveryStageCodes {
		if err := entry.Validate(); err != nil {
			return err
		}
		if _, exists := seen[entry.key()]; exists {
			return fmt.Errorf("AI explanation evaluation recovery selector is ambiguous")
		}
		seen[entry.key()] = "manual"
	}
	return nil
}

func (p EvaluationExecutionPolicy) RequiredCandidateCount() int {
	return p.SlotPolicy.RequiredGenerationCases * p.SlotPolicy.RequiredCandidatesPerCase
}

func (p EvaluationExecutionPolicy) WorstCaseProviderCalls() int {
	return p.Generation.MaxExecutionsPerRun + p.Semantic.MaxExecutionsPerRun
}

func (p EvaluationExecutionPolicy) Fingerprint() (aiexplanation.Fingerprint, error) {
	if err := p.Validate(); err != nil {
		return "", err
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return "", fmt.Errorf("marshal AI explanation evaluation execution policy: %w", err)
	}
	return aiexplanation.NewFingerprint(raw), nil
}

func (p EvaluationExecutionPolicy) Clone() EvaluationExecutionPolicy {
	cloned := p
	cloned.Recovery.AutoRetryableStageCodes = append([]FailureSelector(nil), p.Recovery.AutoRetryableStageCodes...)
	cloned.Recovery.ManualRecoveryStageCodes = append([]FailureSelector(nil), p.Recovery.ManualRecoveryStageCodes...)
	return cloned
}

// CurrentEvaluationExecutionPolicy returns the versioned policy used for new
// v2 runs. The complete value is copied into each Run and fingerprinted; later
// code changes therefore cannot rewrite historical budgets or recovery rules.
func CurrentEvaluationExecutionPolicy() EvaluationExecutionPolicy {
	return EvaluationExecutionPolicy{
		SchemaVersion: EvaluationExecutionPolicySchemaVersionV1,
		PolicyID:      "release-evaluation-bounded-recovery",
		Version:       "v2",
		SlotPolicy: EvaluationSlotPolicy{
			RequiredGenerationCases: RequiredGenerationCaseCount, RequiredCandidatesPerCase: RequiredRepetitionsPerCase,
			RequiredPreflightCases: 1, CandidateSelection: CandidateSelectionFirstContractConformant,
		},
		Generation: GenerationExecutionBudget{MaxExecutionsPerSlot: 2, MaxExecutionsPerRun: 2 * RequiredGenerationAttempts},
		Semantic:   SemanticExecutionBudget{MaxExecutionsPerCandidate: 2, MaxExecutionsPerRun: 2 * RequiredGenerationAttempts},
		Recovery: EvaluationRecoveryPolicy{
			AutoRetryableStageCodes: []FailureSelector{
				{Stage: FailureStageGenerationExecution, Code: "provider_rate_limited"},
				{Stage: FailureStageSemanticEvaluation, Code: SemanticProviderRateLimited},
				{Stage: FailureStageSemanticEvaluation, Code: SemanticProviderNoMessage},
				{Stage: FailureStageSemanticEvaluation, Code: SemanticOutputSchemaInvalid},
			},
			ManualRecoveryStageCodes: []FailureSelector{
				{Stage: FailureStageGenerationExecution, Code: "provider_result_unknown"},
				{Stage: FailureStageSemanticEvaluation, Code: SemanticResultUnknown},
			},
			ResultUnknownRequiresManualAcknowledgement: true,
		},
	}
}

type ReleaseIdentityComponent string

const (
	ReleaseComponentSuite                ReleaseIdentityComponent = "suite"
	ReleaseComponentPrompt               ReleaseIdentityComponent = "prompt"
	ReleaseComponentProfile              ReleaseIdentityComponent = "profile"
	ReleaseComponentInputSchema          ReleaseIdentityComponent = "input_schema"
	ReleaseComponentOutputSchema         ReleaseIdentityComponent = "output_schema"
	ReleaseComponentGenerationRoute      ReleaseIdentityComponent = "generation_route"
	ReleaseComponentSemanticPrompt       ReleaseIdentityComponent = "semantic_prompt"
	ReleaseComponentSemanticOutputSchema ReleaseIdentityComponent = "semantic_output_schema"
	ReleaseComponentSemanticRoute        ReleaseIdentityComponent = "semantic_route"
	ReleaseComponentExecutionPolicy      ReleaseIdentityComponent = "execution_policy"
)

var requiredReleaseIdentityComponents = []ReleaseIdentityComponent{
	ReleaseComponentSuite,
	ReleaseComponentPrompt,
	ReleaseComponentProfile,
	ReleaseComponentInputSchema,
	ReleaseComponentOutputSchema,
	ReleaseComponentGenerationRoute,
	ReleaseComponentSemanticPrompt,
	ReleaseComponentSemanticOutputSchema,
	ReleaseComponentSemanticRoute,
	ReleaseComponentExecutionPolicy,
}

type ReleaseIdentityGatePolicy struct {
	RequiredComponents      []ReleaseIdentityComponent `json:"required_components"`
	RequireFingerprintMatch bool                       `json:"require_fingerprint_match"`
}

type SampleCompletenessGatePolicy struct {
	RequiredGenerationCases              int  `json:"required_generation_cases"`
	RequiredCandidatesPerCase            int  `json:"required_candidates_per_case"`
	RequiredCandidateCount               int  `json:"required_candidate_count"`
	RequiredSemanticReceiptsPerCandidate int  `json:"required_semantic_receipts_per_candidate"`
	RejectUnresolvedResultUnknown        bool `json:"reject_unresolved_result_unknown"`
	RejectBudgetOverrun                  bool `json:"reject_budget_overrun"`
}

type ExecutionReliabilityGatePolicy struct {
	MinInfrastructureSuccessRate                    float64 `json:"min_infrastructure_success_rate"`
	MinGenerationContractConformanceRate            float64 `json:"min_generation_contract_conformance_rate"`
	MinSemanticExecutionSuccessRate                 float64 `json:"min_semantic_execution_success_rate"`
	InfrastructureDenominator                       string  `json:"infrastructure_denominator"`
	GenerationContractDenominator                   string  `json:"generation_contract_denominator"`
	SemanticExecutionDenominator                    string  `json:"semantic_execution_denominator"`
	IncludeResultUnknownInInfrastructureDenominator bool    `json:"include_result_unknown_in_infrastructure_denominator"`
}

type SemanticScoreThresholds struct {
	Faithfulness            float64 `json:"faithfulness"`
	CrossDimensionQuality   float64 `json:"cross_dimension_quality"`
	SuggestionActionability float64 `json:"suggestion_actionability"`
	AudienceClarity         float64 `json:"audience_clarity"`
	Concision               float64 `json:"concision"`
}

type CandidateQualityGatePolicy struct {
	MinAssertionPassesPerCase          int                     `json:"min_assertion_passes_per_case"`
	MinAssertionPassesOverall          int                     `json:"min_assertion_passes_overall"`
	MinimumSemanticScores              SemanticScoreThresholds `json:"minimum_semantic_scores"`
	MinimumSemanticAverages            SemanticScoreThresholds `json:"minimum_semantic_averages"`
	HardAssertionFailureRejectsRelease bool                    `json:"hard_assertion_failure_rejects_release"`
	QualityFailureReplacementAllowed   bool                    `json:"quality_failure_replacement_allowed"`
}

type HumanAccountabilityGatePolicy struct {
	RequiredRoles                        []ReviewRole `json:"required_roles"`
	RequiredReviewsPerCandidate          int          `json:"required_reviews_per_candidate"`
	RequiredReviewCount                  int          `json:"required_review_count"`
	RequireDistinctReviewersPerCandidate bool         `json:"require_distinct_reviewers_per_candidate"`
	RequireReason                        bool         `json:"require_reason"`
	AnyRejectionRejectsRelease           bool         `json:"any_rejection_rejects_release"`
}

// ReleaseGatePolicy freezes every G1-G5 threshold and denominator. In
// particular, result_unknown remains in the infrastructure denominator.
type ReleaseGatePolicy struct {
	SchemaVersion        string                         `json:"schema_version"`
	PolicyID             string                         `json:"policy_id"`
	Version              string                         `json:"version"`
	ReleaseIdentity      ReleaseIdentityGatePolicy      `json:"release_identity"`
	SampleCompleteness   SampleCompletenessGatePolicy   `json:"sample_completeness"`
	ExecutionReliability ExecutionReliabilityGatePolicy `json:"execution_reliability"`
	CandidateQuality     CandidateQualityGatePolicy     `json:"candidate_quality"`
	HumanAccountability  HumanAccountabilityGatePolicy  `json:"human_accountability"`
	ApprovalRule         string                         `json:"approval_rule"`
}

func (p ReleaseGatePolicy) Validate() error {
	if p.SchemaVersion != ReleaseGatePolicySchemaVersionV1 ||
		!policyIdentifierPattern.MatchString(p.PolicyID) || (p.Version != "v1" && p.Version != "v2") ||
		p.ApprovalRule != "all_gates_must_pass" {
		return fmt.Errorf("AI explanation release gate policy identity is invalid")
	}
	if !p.ReleaseIdentity.RequireFingerprintMatch || !sameReleaseIdentityComponents(p.ReleaseIdentity.RequiredComponents) {
		return fmt.Errorf("AI explanation release identity gate policy is invalid")
	}
	completeness := p.SampleCompleteness
	if completeness.RequiredGenerationCases != RequiredGenerationCaseCount ||
		completeness.RequiredCandidatesPerCase != RequiredRepetitionsPerCase ||
		completeness.RequiredCandidateCount != RequiredGenerationAttempts ||
		completeness.RequiredSemanticReceiptsPerCandidate != 1 ||
		!completeness.RejectUnresolvedResultUnknown || !completeness.RejectBudgetOverrun {
		return fmt.Errorf("AI explanation sample completeness gate policy is invalid")
	}
	reliability := p.ExecutionReliability
	if !validRate(reliability.MinInfrastructureSuccessRate) ||
		!validRate(reliability.MinGenerationContractConformanceRate) ||
		!validRate(reliability.MinSemanticExecutionSuccessRate) ||
		reliability.InfrastructureDenominator != "dispatched_provider_executions" ||
		reliability.GenerationContractDenominator != "definite_output_generation_executions" ||
		reliability.SemanticExecutionDenominator != "dispatched_semantic_executions" ||
		!reliability.IncludeResultUnknownInInfrastructureDenominator {
		return fmt.Errorf("AI explanation execution reliability gate policy is invalid")
	}
	quality := p.CandidateQuality
	if quality.MinAssertionPassesPerCase != 4 || quality.MinAssertionPassesOverall != 32 ||
		!quality.HardAssertionFailureRejectsRelease || quality.QualityFailureReplacementAllowed ||
		!sameSemanticThresholds(quality.MinimumSemanticScores, SemanticScoreThresholds{4, 3, 3, 3, 3}) ||
		!sameSemanticThresholds(quality.MinimumSemanticAverages, SemanticScoreThresholds{4.5, 4, 4, 4, 4}) {
		return fmt.Errorf("AI explanation candidate quality gate policy is invalid")
	}
	human := p.HumanAccountability
	if !sameReviewRoles(human.RequiredRoles) || human.RequiredReviewsPerCandidate != 2 ||
		human.RequiredReviewCount != 2*RequiredGenerationAttempts ||
		!human.RequireDistinctReviewersPerCandidate || !human.RequireReason || !human.AnyRejectionRejectsRelease {
		return fmt.Errorf("AI explanation human accountability gate policy is invalid")
	}
	return nil
}

func (p ReleaseGatePolicy) Fingerprint() (aiexplanation.Fingerprint, error) {
	if err := p.Validate(); err != nil {
		return "", err
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return "", fmt.Errorf("marshal AI explanation release gate policy: %w", err)
	}
	return aiexplanation.NewFingerprint(raw), nil
}

func (p ReleaseGatePolicy) Clone() ReleaseGatePolicy {
	cloned := p
	cloned.ReleaseIdentity.RequiredComponents = append([]ReleaseIdentityComponent(nil), p.ReleaseIdentity.RequiredComponents...)
	cloned.HumanAccountability.RequiredRoles = append([]ReviewRole(nil), p.HumanAccountability.RequiredRoles...)
	return cloned
}

// CurrentReleaseGatePolicy is frozen into every new v2 Run alongside the
// execution policy. It intentionally keeps all external-call failures in the
// reliability denominator even when a bounded recovery later succeeds. Version
// v2 corrects infrastructure success and applies the frozen case-pass thresholds;
// historical Runs retain v1 semantics when their gate result is revalidated.
func CurrentReleaseGatePolicy() ReleaseGatePolicy {
	return ReleaseGatePolicy{
		SchemaVersion: ReleaseGatePolicySchemaVersionV1,
		PolicyID:      "release-gates",
		Version:       "v2",
		ReleaseIdentity: ReleaseIdentityGatePolicy{
			RequiredComponents: append([]ReleaseIdentityComponent(nil), requiredReleaseIdentityComponents...), RequireFingerprintMatch: true,
		},
		SampleCompleteness: SampleCompletenessGatePolicy{
			RequiredGenerationCases: RequiredGenerationCaseCount, RequiredCandidatesPerCase: RequiredRepetitionsPerCase,
			RequiredCandidateCount: RequiredGenerationAttempts, RequiredSemanticReceiptsPerCandidate: 1,
			RejectUnresolvedResultUnknown: true, RejectBudgetOverrun: true,
		},
		ExecutionReliability: ExecutionReliabilityGatePolicy{
			MinInfrastructureSuccessRate: 0.98, MinGenerationContractConformanceRate: 0.95, MinSemanticExecutionSuccessRate: 0.98,
			InfrastructureDenominator:                       "dispatched_provider_executions",
			GenerationContractDenominator:                   "definite_output_generation_executions",
			SemanticExecutionDenominator:                    "dispatched_semantic_executions",
			IncludeResultUnknownInInfrastructureDenominator: true,
		},
		CandidateQuality: CandidateQualityGatePolicy{
			MinAssertionPassesPerCase: 4, MinAssertionPassesOverall: 32,
			MinimumSemanticScores:              SemanticScoreThresholds{Faithfulness: 4, CrossDimensionQuality: 3, SuggestionActionability: 3, AudienceClarity: 3, Concision: 3},
			MinimumSemanticAverages:            SemanticScoreThresholds{Faithfulness: 4.5, CrossDimensionQuality: 4, SuggestionActionability: 4, AudienceClarity: 4, Concision: 4},
			HardAssertionFailureRejectsRelease: true,
		},
		HumanAccountability: HumanAccountabilityGatePolicy{
			RequiredRoles:               append([]ReviewRole(nil), ReviewRoleAssessmentSemantics, ReviewRoleSafetyProduct),
			RequiredReviewsPerCandidate: 2, RequiredReviewCount: 2 * RequiredGenerationAttempts,
			RequireDistinctReviewersPerCandidate: true, RequireReason: true, AnyRejectionRejectsRelease: true,
		},
		ApprovalRule: "all_gates_must_pass",
	}
}

func sameReleaseIdentityComponents(actual []ReleaseIdentityComponent) bool {
	if len(actual) != len(requiredReleaseIdentityComponents) {
		return false
	}
	seen := make(map[ReleaseIdentityComponent]struct{}, len(actual))
	for _, component := range actual {
		seen[component] = struct{}{}
	}
	if len(seen) != len(requiredReleaseIdentityComponents) {
		return false
	}
	for _, component := range requiredReleaseIdentityComponents {
		if _, exists := seen[component]; !exists {
			return false
		}
	}
	return true
}

func sameReviewRoles(actual []ReviewRole) bool {
	if len(actual) != 2 {
		return false
	}
	seen := map[ReviewRole]bool{}
	for _, role := range actual {
		seen[role] = true
	}
	return seen[ReviewRoleAssessmentSemantics] && seen[ReviewRoleSafetyProduct]
}

func sameSemanticThresholds(actual, expected SemanticScoreThresholds) bool {
	return actual == expected
}

func validRate(value float64) bool {
	return value >= 0 && value <= 1
}

func validateEvidenceText(value string, max int) bool {
	value = strings.TrimSpace(value)
	return value != "" && len(value) <= max && !strings.ContainsAny(value, "<>")
}
