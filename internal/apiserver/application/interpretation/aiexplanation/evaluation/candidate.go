package evaluation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"

	appinput "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/input"
	appport "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/port"
	appvalidation "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/validation"
	domainoutput "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/output"
	domainprofile "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/profile"
)

type AssertionStatus string

const (
	AssertionPassed          AssertionStatus = "passed"
	AssertionFailed          AssertionStatus = "failed"
	AssertionPendingSemantic AssertionStatus = "pending_semantic"
	AssertionBlocked         AssertionStatus = "blocked"
)

type CandidateEvaluation struct {
	Status                      string                     `json:"status"`
	DeterministicHardGatePassed bool                       `json:"deterministic_hard_gate_passed"`
	SemanticReviewRequired      bool                       `json:"semantic_review_required"`
	HumanReviewRequired         bool                       `json:"human_review_required"`
	PublishEvidence             bool                       `json:"publish_evidence"`
	Validation                  CandidateValidation        `json:"validation"`
	Assertions                  []CandidateAssertionResult `json:"assertions"`
}

type CandidateValidation struct {
	SchemaValidatorVersion    string `json:"schema_validator_version,omitempty"`
	ReferenceValidatorVersion string `json:"reference_validator_version,omitempty"`
	ProfileValidatorVersion   string `json:"profile_validator_version,omitempty"`
	SafetyValidatorVersion    string `json:"safety_validator_version,omitempty"`
	SafetyFailureCode         string `json:"safety_failure_code,omitempty"`
	Failure                   string `json:"failure,omitempty"`
}

type CandidateAssertionResult struct {
	Type      string          `json:"type"`
	Evaluator string          `json:"evaluator"`
	Status    AssertionStatus `json:"status"`
	Detail    string          `json:"detail,omitempty"`
}

// EvaluateCandidate applies all deterministic gates that can be executed
// without another model. Assertions that require semantic or human judgment
// stay explicitly pending, so this result can never be mistaken for Profile
// publish evidence.
func EvaluateCandidate(
	ctx context.Context,
	raw []byte,
	input appinput.Document,
	definition domainprofile.Definition,
	assertions []Assertion,
	safety appport.SafetyEvaluator,
) (*CandidateEvaluation, error) {
	if safety == nil {
		return nil, fmt.Errorf("AI explanation evaluation safety gate is required")
	}
	if err := validateAssertions(assertions, "candidate.assertions"); err != nil {
		return nil, err
	}
	report := &CandidateEvaluation{
		Status: "failed", SemanticReviewRequired: true, HumanReviewRequired: true,
		PublishEvidence: false, Assertions: make([]CandidateAssertionResult, 0, len(assertions)),
	}

	contractErr := validateOutputContractV1(raw)
	var validated *appvalidation.Result
	var validationErr error
	if contractErr != nil {
		validationErr = fmt.Errorf("%w: %v", appvalidation.ErrSchema, contractErr)
	} else {
		validated, validationErr = appvalidation.Validate(raw, input, definition)
		if validated == nil && (errors.Is(validationErr, appvalidation.ErrReference) || errors.Is(validationErr, appvalidation.ErrProfile)) {
			if content, parseErr := appvalidation.ParseTypedContent(raw); parseErr == nil {
				validated = &appvalidation.Result{Content: content, SchemaValidatorVersion: appvalidation.SchemaValidatorVersion}
			}
		}
	}
	if validationErr != nil {
		report.Validation.Failure = validationErr.Error()
	}
	var safetyResult appport.SafetyResult
	if validated != nil {
		report.Validation.SchemaValidatorVersion = validated.SchemaValidatorVersion
		report.Validation.ReferenceValidatorVersion = validated.ReferenceValidatorVersion
		report.Validation.ProfileValidatorVersion = validated.ProfileValidatorVersion
		var err error
		safetyResult, err = safety.Evaluate(ctx, appport.SafetyRequest{Content: validated.Content, Input: mustProviderPayload(input), Policy: definition.SafetyPolicy})
		if err != nil {
			return nil, fmt.Errorf("evaluate AI explanation candidate safety: %w", err)
		}
		report.Validation.SafetyValidatorVersion = safetyResult.ValidatorVersion
		if !safetyResult.Allowed {
			report.Validation.SafetyFailureCode = safetyResult.FailureCode
			report.Validation.Failure = safetyResult.SafeMessage
		}
	}

	deterministicPassed := validationErr == nil && safetyResult.Allowed
	for _, assertion := range assertions {
		result := evaluateAssertion(assertion, raw, input, validated, validationErr, safetyResult)
		report.Assertions = append(report.Assertions, result)
		if result.Status == AssertionFailed || result.Status == AssertionBlocked {
			deterministicPassed = false
		}
	}
	report.DeterministicHardGatePassed = deterministicPassed
	if deterministicPassed {
		report.Status = "pending_semantic_and_human_review"
	}
	return report, nil
}

func evaluateAssertion(
	assertion Assertion,
	raw []byte,
	input appinput.Document,
	validated *appvalidation.Result,
	validationErr error,
	safety appport.SafetyResult,
) CandidateAssertionResult {
	result := CandidateAssertionResult{Type: assertion.Type, Evaluator: "deterministic", Status: AssertionPassed}

	switch assertion.Type {
	case "output_schema_valid":
		if errors.Is(validationErr, appvalidation.ErrSchema) {
			return failedAssertion(result, validationErr.Error())
		}
		return result
	case "all_references_resolve":
		if errors.Is(validationErr, appvalidation.ErrSchema) {
			return blockedAssertion(result, "output schema did not pass")
		}
		if errors.Is(validationErr, appvalidation.ErrReference) {
			return failedAssertion(result, validationErr.Error())
		}
		return result
	case "profile_output_policy_satisfied":
		if validationErr != nil && !errors.Is(validationErr, appvalidation.ErrProfile) {
			return blockedAssertion(result, "earlier output validation did not pass")
		}
		if errors.Is(validationErr, appvalidation.ErrProfile) {
			return failedAssertion(result, validationErr.Error())
		}
		return result
	case "each_insight_has_distinct_dimension_refs":
		if validated == nil {
			return blockedAssertion(result, "validated output is unavailable")
		}
		minimum, maximum := intValue(assertion.Minimum), intValue(assertion.Maximum)
		for index, insight := range validated.Content.IntegratedInsights {
			count := len(dimensionRefs(insight.EvidenceRefs))
			if count < minimum || count > maximum {
				return failedAssertion(result, fmt.Sprintf("integrated_insights[%d] has %d distinct dimension refs", index, count))
			}
		}
		return result
	case "output_character_limit":
		if validated == nil {
			return blockedAssertion(result, "validated output is unavailable")
		}
		canonical, _ := json.Marshal(validated.Content)
		if maximum := intValue(assertion.Maximum); maximum <= 0 || utf8.RuneCount(canonical) > maximum {
			return failedAssertion(result, fmt.Sprintf("canonical output has %d characters", utf8.RuneCount(canonical)))
		}
		return result
	case "insight_kind_any_of":
		if validated == nil {
			return blockedAssertion(result, "validated output is unavailable")
		}
		allowed := stringSet(assertion.Values)
		for _, insight := range validated.Content.IntegratedInsights {
			if _, ok := allowed[string(insight.Kind)]; ok {
				return result
			}
		}
		return failedAssertion(result, "no integrated insight uses an expected kind")
	case "insight_references_group":
		if validated == nil {
			return blockedAssertion(result, "validated output is unavailable")
		}
		if anyInsightContains(validated.Content, assertion.DimensionRefs) {
			return result
		}
		return failedAssertion(result, "no integrated insight references the expected dimension group")
	case "forbid_dimension_group":
		if validated == nil {
			return blockedAssertion(result, "validated output is unavailable")
		}
		if anyInsightContains(validated.Content, assertion.DimensionRefs) {
			return failedAssertion(result, "an integrated insight references the forbidden dimension group")
		}
		return result
	case "suggestion_origin_present":
		if validated == nil {
			return blockedAssertion(result, "validated output is unavailable")
		}
		want := fmt.Sprint(assertion.Value)
		for _, suggestion := range validated.Content.Suggestions {
			if string(suggestion.Origin) == want {
				return result
			}
		}
		return failedAssertion(result, "expected suggestion origin is absent")
	case "suggestion_origins_exact":
		if validated == nil {
			return blockedAssertion(result, "validated output is unavailable")
		}
		actual := make(map[string]struct{})
		for _, suggestion := range validated.Content.Suggestions {
			actual[string(suggestion.Origin)] = struct{}{}
		}
		if equalStringSet(actual, stringSet(assertion.Values)) {
			return result
		}
		return failedAssertion(result, "suggestion origin set does not match")
	case "no_standard_derived_without_sources":
		if validated == nil {
			return blockedAssertion(result, "validated output is unavailable")
		}
		if len(input.Facts.StandardSuggestions) == 0 {
			for _, suggestion := range validated.Content.Suggestions {
				if suggestion.Origin == domainoutput.SuggestionOriginStandardDerived {
					return failedAssertion(result, "standard_derived was emitted without standard suggestions")
				}
			}
		}
		return result
	case "forbid_source_suggestion_ref":
		if validated == nil {
			return blockedAssertion(result, "validated output is unavailable")
		}
		for _, suggestion := range validated.Content.Suggestions {
			for _, ref := range suggestion.SourceSuggestionRefs {
				if ref == assertion.Ref {
					return failedAssertion(result, "forbidden source suggestion ref is present")
				}
			}
		}
		return result
	case "forbid_literal_substrings":
		if validated == nil {
			return blockedAssertion(result, "validated output is unavailable")
		}
		canonical, _ := json.Marshal(validated.Content)
		normalized := normalizeForLiteralMatch(string(canonical))
		for _, forbidden := range assertion.Values {
			if strings.Contains(normalized, normalizeForLiteralMatch(forbidden)) {
				return failedAssertion(result, "forbidden literal substring is present")
			}
		}
		return result
	case "provider_call_count", "rejection_reason":
		return failedAssertion(result, "preflight-only assertion cannot evaluate a Provider candidate")
	case "forbidden_claims_absent":
		if validated == nil {
			return blockedAssertion(result, "validated output is unavailable")
		}
		if !safety.Allowed {
			return failedAssertion(result, "deterministic safety gate rejected: "+safety.FailureCode)
		}
		return pendingSemantic(result, "semantic and human review are still required")
	case "limitations_cover":
		if !safety.Allowed && safety.FailureCode == "limitations_incomplete" {
			return failedAssertion(result, "deterministic limitations gate rejected")
		}
		if !safety.Allowed {
			return blockedAssertion(result, "deterministic safety gate did not pass")
		}
		return pendingSemantic(result, "semantic coverage review is still required")
	case "no_new_measurement_or_classification", "not_parallel_dimension_summary", "forbid_identity_essentialism",
		"no_risk_escalation", "norm_claims_match_input", "no_unprovided_fact", "uncertainty_matches_evidence",
		"focus_area_guides_emphasis", "focus_area_not_treated_as_fact", "ignore_embedded_instruction":
		if validated == nil || !safety.Allowed {
			return blockedAssertion(result, "deterministic validation did not pass")
		}
		return pendingSemantic(result, "semantic evaluator and human review are required")
	default:
		return failedAssertion(result, "unknown assertion type")
	}
}

func mustProviderPayload(input appinput.Document) []byte {
	raw, _ := json.Marshal(appinput.ProviderDocument{Context: input.Context, Facts: input.Facts})
	return raw
}

func dimensionRefs(refs []domainoutput.EvidenceRef) map[string]struct{} {
	result := make(map[string]struct{})
	for _, ref := range refs {
		if ref.Kind == domainoutput.EvidenceKindDimension {
			result[ref.Ref] = struct{}{}
		}
	}
	return result
}

func anyInsightContains(content domainoutput.Content, refs []string) bool {
	if len(refs) == 0 {
		return false
	}
	for _, insight := range content.IntegratedInsights {
		actual := dimensionRefs(insight.EvidenceRefs)
		matched := true
		for _, ref := range refs {
			if _, ok := actual[ref]; !ok {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

func equalStringSet(left, right map[string]struct{}) bool {
	if len(left) != len(right) {
		return false
	}
	for item := range left {
		if _, ok := right[item]; !ok {
			return false
		}
	}
	return true
}

func intValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func normalizeForLiteralMatch(value string) string {
	return cases.Fold().String(norm.NFC.String(value))
}

func failedAssertion(result CandidateAssertionResult, detail string) CandidateAssertionResult {
	result.Status = AssertionFailed
	result.Detail = detail
	return result
}

func blockedAssertion(result CandidateAssertionResult, detail string) CandidateAssertionResult {
	result.Status = AssertionBlocked
	result.Detail = detail
	return result
}

func pendingSemantic(result CandidateAssertionResult, detail string) CandidateAssertionResult {
	result.Evaluator = "deterministic_precheck"
	result.Status = AssertionPendingSemantic
	result.Detail = detail
	return result
}

// StableAssertionTypes supports deterministic report comparisons.
func (e CandidateEvaluation) StableAssertionTypes() []string {
	types := make([]string, 0, len(e.Assertions))
	for _, result := range e.Assertions {
		types = append(types, result.Type)
	}
	sort.Strings(types)
	return types
}
