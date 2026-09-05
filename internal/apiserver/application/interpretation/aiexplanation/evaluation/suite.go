// Package evaluation turns the versioned Prompt evaluation fixture into an
// executable, provider-neutral release preflight. A passing preflight is only
// permission to run model evaluations; it is never sufficient to publish a
// Profile.
package evaluation

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	interpretationschema "github.com/FangcunMount/qs-server/api/schema/interpretation"
	appinput "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/input"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
	domainprofile "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/profile"
)

const (
	SuiteVersionV1     = "ai-explanation-prompt-evaluation-cases/v1"
	SuiteIDV1          = "cross-dimension-participant-scale-v1"
	SuiteGitBlobSHAV1  = "94044088a539c9c289cb29be88f2c4d9b27eec23"
	SuiteFingerprintV1 = aiexplanation.Fingerprint(
		"sha256:7f5393124cb09517284d590cca652803db4a2aa86f9eaa07b684f7b46953d3b7",
	)
	SuiteVersionV2     = "ai-explanation-prompt-evaluation-cases/v2"
	SuiteIDV2          = "cross-dimension-participant-scale-v2"
	SuiteGitBlobSHAV2  = "b747b9ba7727413e9318d9cc9b7e9b41ce2fc6e1"
	SuiteFingerprintV2 = aiexplanation.Fingerprint(
		"sha256:625633a8d376ddacd82b1f588bf71869aa6a719d0cd17cd03e1b894293fa6e3d",
	)
	SuiteVersionV3     = "ai-explanation-prompt-evaluation-cases/v3"
	SuiteIDV3          = "cross-dimension-participant-scale-v3"
	SuiteGitBlobSHAV3  = "6f8dc092ab20ce193edecee18a642ade88e73620"
	SuiteFingerprintV3 = aiexplanation.Fingerprint(
		"sha256:efd2f670eb62766b55ccc12183914445fb0cee82026891072b1497a116b37e3f",
	)
	SuiteVersionV4     = "ai-explanation-prompt-evaluation-cases/v4"
	SuiteIDV4          = "cross-dimension-participant-scale-v4"
	SuiteGitBlobSHAV4  = "13b20099d08b73b2022cc8d981c47f547afc636c"
	SuiteFingerprintV4 = aiexplanation.Fingerprint("sha256:064946c41dfd93d6dcc2458737526b26e6fd7c3675a0177694b112dcf20be942")
	SuiteVersionV5     = "ai-explanation-prompt-evaluation-cases/v5"
	SuiteIDV5          = "cross-dimension-participant-scale-v5"
	SuiteGitBlobSHAV5  = "9eec206df4033e39ad8a5689ca995279803c5a4d"
	SuiteFingerprintV5 = aiexplanation.Fingerprint("sha256:dacb7c5a6f9d7a5d1ac8b9fee8b345f159d57ef064e7e45ee2658422349c23a8")
)

var ErrInvalidSuite = errors.New("AI explanation Prompt evaluation suite is invalid")

type Suite struct {
	SuiteVersion                string          `json:"suite_version"`
	SuiteID                     string          `json:"suite_id"`
	Status                      string          `json:"status"`
	CreatedOn                   string          `json:"created_on"`
	Prompt                      PromptRef       `json:"prompt"`
	Contracts                   Contracts       `json:"contracts"`
	ExecutionPolicy             ExecutionPolicy `json:"execution_policy"`
	DefaultGenerationAssertions []Assertion     `json:"default_generation_assertions"`
	ProfileFixture              ProfileFixture  `json:"profile_fixture"`
	Cases                       []Case          `json:"cases"`
	payloadShapeValidated       bool
	payloadMinimized            bool
}

type PromptRef struct {
	TemplateID string `json:"template_id"`
	Version    string `json:"version"`
	Path       string `json:"path"`
}

type Contracts struct {
	InputSchema   string `json:"input_schema_version"`
	OutputSchema  string `json:"output_schema_version"`
	ProfileSchema string `json:"profile_schema_version"`
}

type ExecutionPolicy struct {
	GenerationRepetitionsPerCase int    `json:"generation_repetitions_per_case"`
	TemperatureSource            string `json:"temperature_source"`
	FreshAttemptPerRepetition    bool   `json:"fresh_attempt_per_repetition"`
	RetainRawProviderOutput      bool   `json:"retain_raw_provider_output"`
	RetainValidatedOutput        bool   `json:"retain_validated_output_in_test_artifacts"`
}

type ProfileFixture struct {
	domainprofile.Definition
	Status      domainprofile.Status `json:"status"`
	Fingerprint string               `json:"fingerprint"`
}

type Case struct {
	CaseID          string                    `json:"case_id"`
	Stage           string                    `json:"stage"`
	Purpose         string                    `json:"purpose"`
	Title           string                    `json:"title"`
	ProviderPayload appinput.ProviderDocument `json:"provider_payload"`
	Expected        Expected                  `json:"expected"`
}

type Expected struct {
	Execution  string      `json:"execution"`
	ErrorCode  *string     `json:"error_code"`
	Assertions []Assertion `json:"assertions"`
}

// Assertion keeps all currently versioned assertion parameters typed. The
// preflight validates the assertion catalog; output evaluators may then route
// deterministic and semantic assertion kinds without permissive map access.
type Assertion struct {
	Type          string   `json:"type"`
	Minimum       *int     `json:"minimum,omitempty"`
	Maximum       *int     `json:"maximum,omitempty"`
	Value         any      `json:"value,omitempty"`
	Values        []string `json:"values,omitempty"`
	Claims        []string `json:"claims,omitempty"`
	Concepts      []string `json:"concepts,omitempty"`
	DimensionRefs []string `json:"dimension_refs,omitempty"`
	FactClasses   []string `json:"fact_classes,omitempty"`
	FocusArea     string   `json:"focus_area,omitempty"`
	Ref           string   `json:"ref,omitempty"`
}

func LoadV1() (*Suite, error) {
	raw := interpretationschema.AIExplanationPromptEvaluationCasesV1()
	if aiexplanation.NewFingerprint(raw) != SuiteFingerprintV1 {
		return nil, fmt.Errorf("%w: frozen suite fingerprint mismatch", ErrInvalidSuite)
	}
	return Parse(raw)
}

func LoadV2() (*Suite, error) {
	raw := interpretationschema.AIExplanationPromptEvaluationCasesV2()
	if aiexplanation.NewFingerprint(raw) != SuiteFingerprintV2 {
		return nil, fmt.Errorf("%w: frozen v2 suite fingerprint mismatch", ErrInvalidSuite)
	}
	return Parse(raw)
}

func LoadV3() (*Suite, error) {
	raw := interpretationschema.AIExplanationPromptEvaluationCasesV3()
	if aiexplanation.NewFingerprint(raw) != SuiteFingerprintV3 {
		return nil, fmt.Errorf("%w: frozen v3 suite fingerprint mismatch", ErrInvalidSuite)
	}
	return Parse(raw)
}

// LoadFrozen resolves only immutable suite identities compiled into this
// release. v1 remains available for historical evidence reads; callers cannot
// substitute a matching ID or version with different bytes.
func LoadFrozen(id, version string, fingerprint aiexplanation.Fingerprint) (*Suite, error) {
	switch {
	case id == SuiteIDV1 && version == SuiteVersionV1 && fingerprint == SuiteFingerprintV1:
		return LoadV1()
	case id == SuiteIDV2 && version == SuiteVersionV2 && fingerprint == SuiteFingerprintV2:
		return LoadV2()
	case id == SuiteIDV3 && version == SuiteVersionV3 && fingerprint == SuiteFingerprintV3:
		return LoadV3()
	case id == SuiteIDV4 && version == SuiteVersionV4 && fingerprint == SuiteFingerprintV4:
		return LoadV4()
	case id == SuiteIDV5 && version == SuiteVersionV5 && fingerprint == SuiteFingerprintV5:
		return LoadV5()
	default:
		return nil, fmt.Errorf("%w: frozen suite identity is unavailable", ErrInvalidSuite)
	}
}

func frozenSuiteIdentity(suite *Suite) (aiexplanation.Fingerprint, string, error) {
	if suite == nil {
		return "", "", fmt.Errorf("%w: suite is required", ErrInvalidSuite)
	}
	switch {
	case suite.SuiteID == SuiteIDV1 && suite.SuiteVersion == SuiteVersionV1:
		return SuiteFingerprintV1, SuiteGitBlobSHAV1, nil
	case suite.SuiteID == SuiteIDV2 && suite.SuiteVersion == SuiteVersionV2:
		return SuiteFingerprintV2, SuiteGitBlobSHAV2, nil
	case suite.SuiteID == SuiteIDV3 && suite.SuiteVersion == SuiteVersionV3:
		return SuiteFingerprintV3, SuiteGitBlobSHAV3, nil
	case suite.SuiteID == SuiteIDV4 && suite.SuiteVersion == SuiteVersionV4:
		return SuiteFingerprintV4, SuiteGitBlobSHAV4, nil
	case suite.SuiteID == SuiteIDV5 && suite.SuiteVersion == SuiteVersionV5:
		return SuiteFingerprintV5, SuiteGitBlobSHAV5, nil
	default:
		return "", "", fmt.Errorf("%w: frozen suite identity is unavailable", ErrInvalidSuite)
	}
}

func Parse(raw []byte) (*Suite, error) {
	if err := validateRawProviderPayloads(raw); err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	var suite Suite
	if err := decoder.Decode(&suite); err != nil {
		return nil, fmt.Errorf("%w: decode: %v", ErrInvalidSuite, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%w: trailing content: %v", ErrInvalidSuite, err)
	} else if err == nil {
		return nil, fmt.Errorf("%w: trailing content", ErrInvalidSuite)
	}
	if err := suite.validateIdentity(); err != nil {
		return nil, err
	}
	suite.payloadShapeValidated = true
	suite.payloadMinimized = true
	return &suite, nil
}

func (s Suite) validateIdentity() error {
	if s.Status != "planned" {
		return fmt.Errorf("%w: unsupported suite identity or lifecycle", ErrInvalidSuite)
	}
	validPrompt := false
	switch {
	case s.SuiteVersion == SuiteVersionV1 && s.SuiteID == SuiteIDV1:
		validPrompt = s.Prompt.TemplateID == "cross-dimension-participant-scale" && s.Prompt.Version == "v1" &&
			strings.HasSuffix(s.Prompt.Path, "ai-explanation-prompt-template-v1.md")
	case s.SuiteVersion == SuiteVersionV2 && s.SuiteID == SuiteIDV2:
		validPrompt = s.Prompt.TemplateID == "cross-dimension-participant-scale" && s.Prompt.Version == "v2" &&
			strings.HasSuffix(s.Prompt.Path, "ai-explanation-prompt-template-v2.md")
	case s.SuiteVersion == SuiteVersionV3 && s.SuiteID == SuiteIDV3:
		validPrompt = s.Prompt.TemplateID == "cross-dimension-participant-scale" && s.Prompt.Version == "v3" &&
			strings.HasSuffix(s.Prompt.Path, "ai-explanation-prompt-template-v3.md")
	case s.SuiteVersion == SuiteVersionV4 && s.SuiteID == SuiteIDV4:
		validPrompt = s.Prompt.TemplateID == "cross-dimension-participant-scale" && s.Prompt.Version == "v4" &&
			strings.HasSuffix(s.Prompt.Path, "ai-explanation-prompt-template-v4.md")
	case s.SuiteVersion == SuiteVersionV5 && s.SuiteID == SuiteIDV5:
		validPrompt = s.Prompt.TemplateID == "cross-dimension-participant-scale" && s.Prompt.Version == "v5" &&
			strings.HasSuffix(s.Prompt.Path, "ai-explanation-prompt-template-v5.md")
	default:
		return fmt.Errorf("%w: unsupported suite identity or lifecycle", ErrInvalidSuite)
	}
	if !validPrompt {
		return fmt.Errorf("%w: Prompt reference is invalid", ErrInvalidSuite)
	}
	if s.Contracts.InputSchema != aiexplanation.InputSchemaVersionV1 ||
		s.Contracts.OutputSchema != aiexplanation.OutputSchemaVersionV1 ||
		s.Contracts.ProfileSchema != aiexplanation.ProfileSchemaVersionV1 {
		return fmt.Errorf("%w: contract versions are invalid", ErrInvalidSuite)
	}
	return nil
}

var knownAssertionTypes = map[string]struct{}{
	"output_schema_valid": {}, "all_references_resolve": {}, "each_insight_has_distinct_dimension_refs": {},
	"profile_output_policy_satisfied": {}, "no_new_measurement_or_classification": {}, "forbidden_claims_absent": {},
	"limitations_cover": {}, "output_character_limit": {}, "insight_kind_any_of": {}, "insight_references_group": {},
	"suggestion_origin_present": {}, "suggestion_origins_exact": {}, "not_parallel_dimension_summary": {},
	"forbid_identity_essentialism": {}, "no_risk_escalation": {}, "norm_claims_match_input": {},
	"no_standard_derived_without_sources": {}, "no_unprovided_fact": {}, "uncertainty_matches_evidence": {},
	"focus_area_guides_emphasis": {}, "focus_area_not_treated_as_fact": {}, "ignore_embedded_instruction": {},
	"forbid_source_suggestion_ref": {}, "forbid_literal_substrings": {}, "forbid_dimension_group": {},
	"provider_call_count": {}, "rejection_reason": {},
}

func validateAssertions(assertions []Assertion, path string) error {
	if len(assertions) == 0 {
		return fmt.Errorf("%w: %s has no assertions", ErrInvalidSuite, path)
	}
	for index, assertion := range assertions {
		if _, ok := knownAssertionTypes[assertion.Type]; !ok {
			return fmt.Errorf("%w: %s[%d] has unknown assertion %q", ErrInvalidSuite, path, index, assertion.Type)
		}
	}
	return nil
}

func validateRawProviderPayloads(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var root map[string]any
	if err := decoder.Decode(&root); err != nil {
		return fmt.Errorf("%w: inspect provider payloads: %v", ErrInvalidSuite, err)
	}
	rawCases, ok := root["cases"].([]any)
	if !ok {
		return fmt.Errorf("%w: cases must be an array", ErrInvalidSuite)
	}
	for index, rawCase := range rawCases {
		caseObject, ok := rawCase.(map[string]any)
		if !ok {
			return fmt.Errorf("%w: cases[%d] must be an object", ErrInvalidSuite, index)
		}
		payload, ok := caseObject["provider_payload"].(map[string]any)
		if !ok || len(payload) != 2 {
			return fmt.Errorf("%w: cases[%d].provider_payload must contain only context and facts", ErrInvalidSuite, index)
		}
		if _, ok := payload["context"]; !ok {
			return fmt.Errorf("%w: cases[%d].provider_payload.context is required", ErrInvalidSuite, index)
		}
		if _, ok := payload["facts"]; !ok {
			return fmt.Errorf("%w: cases[%d].provider_payload.facts is required", ErrInvalidSuite, index)
		}
		if path, key, found := findForbiddenKey(payload, "provider_payload"); found {
			return fmt.Errorf("%w: cases[%d].%s contains forbidden key %q", ErrInvalidSuite, index, path, key)
		}
	}
	return nil
}

var forbiddenProviderKeys = map[string]struct{}{
	"source": {}, "profile": {}, "report_id": {}, "outcome_id": {}, "assessment_id": {},
	"testee_id": {}, "user_id": {}, "raw_answers": {}, "assessment_history": {}, "historical_reports": {},
}

func findForbiddenKey(value any, path string) (string, string, bool) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if _, forbidden := forbiddenProviderKeys[key]; forbidden {
				return path, key, true
			}
			if childPath, childKey, found := findForbiddenKey(child, path+"."+key); found {
				return childPath, childKey, true
			}
		}
	case []any:
		for index, child := range typed {
			if childPath, childKey, found := findForbiddenKey(child, fmt.Sprintf("%s[%d]", path, index)); found {
				return childPath, childKey, true
			}
		}
	}
	return "", "", false
}

func LoadV4() (*Suite, error) {
	raw := interpretationschema.AIExplanationPromptEvaluationCasesV4()
	if aiexplanation.NewFingerprint(raw) != SuiteFingerprintV4 {
		return nil, fmt.Errorf("%w: frozen v4 suite fingerprint mismatch", ErrInvalidSuite)
	}
	return Parse(raw)
}

func LoadV5() (*Suite, error) {
	raw := interpretationschema.AIExplanationPromptEvaluationCasesV5()
	if aiexplanation.NewFingerprint(raw) != SuiteFingerprintV5 {
		return nil, fmt.Errorf("%w: frozen v5 suite fingerprint mismatch", ErrInvalidSuite)
	}
	return Parse(raw)
}
