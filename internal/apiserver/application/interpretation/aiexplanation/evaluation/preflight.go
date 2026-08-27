package evaluation

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"

	interpretationschema "github.com/FangcunMount/qs-server/api/schema/interpretation"
	appinput "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/input"
	appport "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/port"
	appprompt "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/prompt"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
	domaininput "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/input"
	domainprofile "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/profile"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

const PreflightReportVersionV1 = "ai-explanation-prompt-evaluation-preflight/v1"

type PreflightRunner struct {
	prompts       appport.PromptPackageResolver
	schemas       appport.OutputSchemaResolver
	inputSchema   *jsonschema.Schema
	profileSchema *jsonschema.Schema
	now           func() time.Time
}

func NewPreflightRunner(prompts appport.PromptPackageResolver, schemas appport.OutputSchemaResolver, now func() time.Time) (*PreflightRunner, error) {
	if prompts == nil || schemas == nil {
		return nil, fmt.Errorf("AI explanation evaluation Prompt and Schema catalogs are required")
	}
	if now == nil {
		now = time.Now
	}
	inputSchema, err := compileContractSchema("ai-explanation-input-v1.schema.json", interpretationschema.AIExplanationInputV1())
	if err != nil {
		return nil, err
	}
	profileSchema, err := compileContractSchema("ai-explanation-profile-v1.schema.json", interpretationschema.AIExplanationProfileV1())
	if err != nil {
		return nil, err
	}
	return &PreflightRunner{prompts: prompts, schemas: schemas, inputSchema: inputSchema, profileSchema: profileSchema, now: now}, nil
}

type PreflightReport struct {
	ReportVersion              string       `json:"report_version"`
	SuiteID                    string       `json:"suite_id"`
	SuiteVersion               string       `json:"suite_version"`
	PromptTemplateID           string       `json:"prompt_template_id"`
	PromptVersion              string       `json:"prompt_version"`
	ProfileID                  string       `json:"profile_id"`
	ProfileVersion             string       `json:"profile_version"`
	ProfileFingerprint         string       `json:"profile_fingerprint"`
	GeneratedAt                string       `json:"generated_at"`
	Status                     string       `json:"status"`
	GenerationCases            int          `json:"generation_cases"`
	PreflightCases             int          `json:"preflight_cases"`
	PlannedProviderInvocations int          `json:"planned_provider_invocations"`
	ActualProviderInvocations  int          `json:"actual_provider_invocations"`
	Checks                     []Check      `json:"checks"`
	Cases                      []CaseResult `json:"cases"`
	PublishEvidence            bool         `json:"publish_evidence"`
}

type Check struct {
	ID     string `json:"id"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail,omitempty"`
}

type CaseResult struct {
	CaseID                string `json:"case_id"`
	Stage                 string `json:"stage"`
	Status                string `json:"status"`
	ExpectedExecution     string `json:"expected_execution"`
	ActualExecution       string `json:"actual_execution"`
	RejectionReason       string `json:"rejection_reason,omitempty"`
	PlannedProviderCalls  int    `json:"planned_provider_calls"`
	ActualProviderCalls   int    `json:"actual_provider_calls"`
	InputFingerprint      string `json:"input_fingerprint,omitempty"`
	RenderedPromptChecked bool   `json:"rendered_prompt_checked"`
}

func (r *PreflightRunner) Run(ctx context.Context, suite *Suite) (*PreflightReport, error) {
	if r == nil || suite == nil {
		return nil, fmt.Errorf("%w: runner and suite are required", ErrInvalidSuite)
	}
	report := &PreflightReport{
		ReportVersion: PreflightReportVersionV1, SuiteID: suite.SuiteID, SuiteVersion: suite.SuiteVersion,
		PromptTemplateID: suite.Prompt.TemplateID, PromptVersion: suite.Prompt.Version,
		ProfileID: suite.ProfileFixture.ProfileID, ProfileVersion: suite.ProfileFixture.Version,
		ProfileFingerprint: suite.ProfileFixture.Fingerprint,
		GeneratedAt:        r.now().UTC().Format(time.RFC3339Nano), Status: "failed", PublishEvidence: false,
		Checks: []Check{}, Cases: []CaseResult{},
	}
	addCheck := func(id string, err error) {
		check := Check{ID: id, Passed: err == nil}
		if err != nil {
			check.Detail = err.Error()
		}
		report.Checks = append(report.Checks, check)
	}

	identityErr := suite.validateIdentity()
	addCheck("SUITE-001", identityErr)
	if identityErr != nil {
		return report, identityErr
	}
	profileRaw, err := json.Marshal(suite.ProfileFixture)
	if err == nil {
		err = validateContractInstance(r.profileSchema, "AIExplanationProfile v1", profileRaw)
	}
	var profileRecord *domainprofile.AIExplanationProfile
	if err == nil {
		profileRecord, err = buildProfile(suite.ProfileFixture, r.now().UTC())
	}
	addCheck("SUITE-002", err)
	if err != nil {
		return report, fmt.Errorf("%w: Profile fixture: %v", ErrInvalidSuite, err)
	}
	fingerprintErr := error(nil)
	if profileRecord.Fingerprint().String() != suite.ProfileFixture.Fingerprint {
		fingerprintErr = fmt.Errorf("Profile fingerprint mismatch: got %s", profileRecord.Fingerprint())
	}
	addCheck("SUITE-003", fingerprintErr)
	if fingerprintErr != nil {
		return report, fmt.Errorf("%w: %v", ErrInvalidSuite, fingerprintErr)
	}

	definition := profileRecord.Definition()
	promptCheckErr := error(nil)
	if definition.GenerationPolicy.PromptTemplateID != suite.Prompt.TemplateID || definition.GenerationPolicy.PromptVersion != suite.Prompt.Version {
		promptCheckErr = fmt.Errorf("Prompt and Profile identity mismatch")
	}
	pkg, promptErr := r.prompts.ResolvePromptPackage(ctx, suite.Prompt.TemplateID, suite.Prompt.Version)
	if promptCheckErr == nil {
		promptCheckErr = promptErr
	}
	if promptCheckErr == nil && (pkg.Ref.TemplateID != suite.Prompt.TemplateID || pkg.Ref.Version != suite.Prompt.Version) {
		promptCheckErr = fmt.Errorf("resolved Prompt identity mismatch")
	}
	addCheck("SUITE-004", promptCheckErr)
	if promptCheckErr != nil {
		return report, fmt.Errorf("%w: %v", ErrInvalidSuite, promptCheckErr)
	}

	outputSchema, err := r.schemas.ResolveOutputSchema(ctx, suite.Contracts.OutputSchema)
	if err == nil {
		err = outputSchema.Validate()
	}
	addCheck("PREFLIGHT-OUTPUT-SCHEMA", err)
	if err != nil {
		return report, fmt.Errorf("%w: output Schema: %v", ErrInvalidSuite, err)
	}

	err = nil
	if suite.ExecutionPolicy.GenerationRepetitionsPerCase != 5 || !suite.ExecutionPolicy.FreshAttemptPerRepetition ||
		!suite.ExecutionPolicy.RetainRawProviderOutput || !suite.ExecutionPolicy.RetainValidatedOutput ||
		suite.ExecutionPolicy.TemperatureSource != "provider_route" {
		err = fmt.Errorf("execution policy does not satisfy v1 release evidence requirements")
	}
	if assertionErr := validateAssertions(suite.DefaultGenerationAssertions, "default_generation_assertions"); err == nil {
		err = assertionErr
	}
	addCheck("PREFLIGHT-EXECUTION-POLICY", err)
	if err != nil {
		return report, fmt.Errorf("%w: %v", ErrInvalidSuite, err)
	}

	err = validateCaseInventory(suite)
	addCheck("SUITE-005", err)
	if err != nil {
		return report, fmt.Errorf("%w: %v", ErrInvalidSuite, err)
	}

	seen := make(map[string]struct{}, len(suite.Cases))
	for index := range suite.Cases {
		caseResult, caseErr := r.preflightCase(suite, profileRecord, pkg, &suite.Cases[index], seen)
		report.Cases = append(report.Cases, caseResult)
		if suite.Cases[index].Stage == "generation" {
			report.GenerationCases++
			report.PlannedProviderInvocations += caseResult.PlannedProviderCalls
		} else if suite.Cases[index].Stage == "preflight" {
			report.PreflightCases++
		}
		addCheck(fmt.Sprintf("CASE-%03d", index+1), caseErr)
		if caseErr != nil {
			return report, fmt.Errorf("%w: %s: %v", ErrInvalidSuite, suite.Cases[index].CaseID, caseErr)
		}
	}

	generationInputErr := error(nil)
	promptRenderErr := error(nil)
	preflightCaseErr := error(nil)
	for _, result := range report.Cases {
		switch result.Stage {
		case "generation":
			if result.Status != "passed" || result.InputFingerprint == "" {
				generationInputErr = fmt.Errorf("generation case %s did not produce a valid full Input", result.CaseID)
			}
			if !result.RenderedPromptChecked {
				promptRenderErr = fmt.Errorf("generation case %s did not render the Prompt", result.CaseID)
			}
		case "preflight":
			if result.Status != "passed" || result.ActualExecution != "rejected_before_provider" || result.ActualProviderCalls != 0 {
				preflightCaseErr = fmt.Errorf("preflight case %s did not reject before Provider", result.CaseID)
			}
		}
	}
	addCheck("SUITE-006", generationInputErr)
	addCheck("SUITE-007", preflightCaseErr)
	shapeErr := error(nil)
	if !suite.payloadShapeValidated {
		shapeErr = fmt.Errorf("provider payload shape was not validated")
	}
	addCheck("SUITE-008", shapeErr)
	minimizationErr := error(nil)
	if !suite.payloadMinimized {
		minimizationErr = fmt.Errorf("provider payload data minimization was not validated")
	}
	addCheck("SUITE-009", minimizationErr)
	addCheck("SUITE-010", promptRenderErr)
	report.Status = "passed"
	return report, nil
}

func validateCaseInventory(suite *Suite) error {
	if len(suite.Cases) != 8 {
		return fmt.Errorf("case count is %d, want 8", len(suite.Cases))
	}
	seen := make(map[string]struct{}, len(suite.Cases))
	generationCases, preflightCases := 0, 0
	for index := range suite.Cases {
		testCase := &suite.Cases[index]
		if strings.TrimSpace(testCase.CaseID) == "" {
			return fmt.Errorf("cases[%d] has no id", index)
		}
		if _, duplicate := seen[testCase.CaseID]; duplicate {
			return fmt.Errorf("case id %q is duplicated", testCase.CaseID)
		}
		seen[testCase.CaseID] = struct{}{}
		switch testCase.Stage {
		case "generation":
			generationCases++
		case "preflight":
			preflightCases++
		default:
			return fmt.Errorf("case %s has unsupported stage %q", testCase.CaseID, testCase.Stage)
		}
		if err := validateAssertions(testCase.Expected.Assertions, testCase.CaseID+".expected.assertions"); err != nil {
			return err
		}
	}
	plannedCalls := generationCases * suite.ExecutionPolicy.GenerationRepetitionsPerCase
	if generationCases != 7 || preflightCases != 1 || plannedCalls != 35 {
		return fmt.Errorf("case inventory is generation=%d preflight=%d calls=%d, want 7/1/35", generationCases, preflightCases, plannedCalls)
	}
	return nil
}

func (r *PreflightRunner) preflightCase(suite *Suite, profileRecord *domainprofile.AIExplanationProfile, pkg appport.PromptPackage, testCase *Case, seen map[string]struct{}) (CaseResult, error) {
	result := CaseResult{CaseID: testCase.CaseID, Stage: testCase.Stage, Status: "failed", ExpectedExecution: testCase.Expected.Execution}
	if strings.TrimSpace(testCase.CaseID) == "" {
		return result, fmt.Errorf("case id is required")
	}
	if _, duplicate := seen[testCase.CaseID]; duplicate {
		return result, fmt.Errorf("case id is duplicated")
	}
	seen[testCase.CaseID] = struct{}{}
	if err := validateAssertions(testCase.Expected.Assertions, testCase.CaseID+".expected.assertions"); err != nil {
		return result, err
	}
	if err := validateProviderPayload(testCase.ProviderPayload, profileRecord.Definition()); err != nil {
		return result, err
	}
	reason := eligibilityRejection(testCase.ProviderPayload, profileRecord.Definition())
	switch testCase.Stage {
	case "generation":
		if testCase.Expected.Execution != "call_provider" || testCase.Expected.ErrorCode != nil || reason != "" {
			return result, fmt.Errorf("generation case is not provider-call eligible: %s", reason)
		}
		assembled, err := syntheticInput(testCase.ProviderPayload, profileRecord, testCase.CaseID)
		if err != nil {
			return result, err
		}
		if err := validateContractInstance(r.inputSchema, "AIExplanationInput v1", assembled.Snapshot.CanonicalJSON()); err != nil {
			return result, err
		}
		if _, err := appprompt.Render(pkg, profileRecord.Definition(), assembled); err != nil {
			return result, fmt.Errorf("render Prompt: %w", err)
		}
		result.ActualExecution = "ready_for_provider"
		result.PlannedProviderCalls = suite.ExecutionPolicy.GenerationRepetitionsPerCase
		result.InputFingerprint = assembled.Snapshot.Fingerprint().String()
		result.RenderedPromptChecked = true
	case "preflight":
		if testCase.Expected.Execution != "reject_before_provider" || testCase.Expected.ErrorCode == nil {
			return result, fmt.Errorf("preflight case expectation is invalid")
		}
		if reason == "" || *testCase.Expected.ErrorCode != "not_applicable" || !hasExpectedStringAssertion(testCase.Expected.Assertions, "rejection_reason", reason) || !hasExpectedIntAssertion(testCase.Expected.Assertions, "provider_call_count", 0) {
			return result, fmt.Errorf("preflight rejection does not match expected assertions: %s", reason)
		}
		result.ActualExecution = "rejected_before_provider"
		result.RejectionReason = reason
	default:
		return result, fmt.Errorf("unsupported stage %q", testCase.Stage)
	}
	result.Status = "passed"
	return result, nil
}

func buildProfile(fixture ProfileFixture, at time.Time) (*domainprofile.AIExplanationProfile, error) {
	if fixture.Status != domainprofile.StatusPublished {
		return nil, fmt.Errorf("fixture Profile must be published")
	}
	wantFingerprint, err := aiexplanation.ParseFingerprint(fixture.Fingerprint)
	if err != nil {
		return nil, err
	}
	profileRecord, err := domainprofile.NewDraft(meta.FromUint64(1), fixture.Definition, at)
	if err != nil {
		return nil, err
	}
	if profileRecord.Fingerprint() != wantFingerprint {
		return nil, fmt.Errorf("fixture Profile fingerprint does not match its definition: got %s want %s", profileRecord.Fingerprint(), wantFingerprint)
	}
	if err := profileRecord.Publish(meta.ID(1), "prompt-evaluation-preflight", "synthetic fixture only", at); err != nil {
		return nil, err
	}
	return profileRecord, nil
}

func syntheticInput(payload appinput.ProviderDocument, profileRecord *domainprofile.AIExplanationProfile, caseID string) (*appinput.Result, error) {
	document := appinput.Document{
		SchemaVersion: aiexplanation.InputSchemaVersionV1,
		Source: appinput.Source{
			ReportID: meta.FromUint64(1001).String(), OutcomeID: meta.FromUint64(1002).String(),
			ReportType: policy.ReportTypeStandard.String(), TemplateVersion: "prompt-eval/v1",
			ContentSchemaVersion: "interpretation-report-content/v1", BuilderIdentity: "prompt-evaluation-fixture/" + caseID,
			GeneratedAt: "2026-08-26T00:00:00Z",
		},
		Profile: appinput.ProfileRef{
			ProfileID: profileRecord.ProfileID(), ProfileVersion: profileRecord.Version(), ProfileFingerprint: profileRecord.Fingerprint().String(),
		},
		Context: payload.Context, Facts: payload.Facts,
	}
	raw, err := json.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("marshal synthetic Input: %w", err)
	}
	snapshot, err := domaininput.NewSnapshot(raw)
	if err != nil {
		return nil, err
	}
	assembled, err := appinput.Restore(snapshot)
	if err != nil {
		return nil, fmt.Errorf("restore synthetic Input: %w", err)
	}
	return assembled, nil
}

func validateProviderPayload(payload appinput.ProviderDocument, definition domainprofile.Definition) error {
	if payload.Context.Scope != "current_assessment_only" || payload.Context.Audience != string(policy.AudienceParticipant) || payload.Context.Locale != "zh-CN" {
		return fmt.Errorf("provider context is outside the v1 suite scope")
	}
	wantPersonalization := "assessment_result_only"
	if len(payload.Context.FocusAreas) > 0 {
		wantPersonalization = "assessment_result_and_focus_areas"
	}
	if payload.Context.PersonalizationScope != wantPersonalization {
		return fmt.Errorf("provider personalization scope does not match focus areas")
	}
	if payload.Facts.Runtime.DecisionKind != string(definition.Selector.DecisionKind) || payload.Facts.Model.Kind != string(definition.Selector.ModelKind) {
		return fmt.Errorf("provider facts do not match Profile selector")
	}
	if definition.Selector.ModelCode != nil && payload.Facts.Model.Code != *definition.Selector.ModelCode {
		return fmt.Errorf("provider model code does not match Profile selector")
	}
	if definition.Selector.ModelVersion != nil && payload.Facts.Model.Version != *definition.Selector.ModelVersion {
		return fmt.Errorf("provider model version does not match Profile selector")
	}
	allowedFocus := stringSet(definition.InputPolicy.AllowedFocusAreas)
	for _, focus := range payload.Context.FocusAreas {
		if _, ok := allowedFocus[focus]; !ok {
			return fmt.Errorf("focus area %q is outside Profile policy", focus)
		}
	}
	seenRefs := map[string]struct{}{}
	for _, dimension := range payload.Facts.Dimensions {
		if dimension.Ref == "" || dimension.Code == "" {
			return fmt.Errorf("dimension identity is required")
		}
		if _, duplicate := seenRefs[dimension.Ref]; duplicate {
			return fmt.Errorf("dimension ref %q is duplicated", dimension.Ref)
		}
		seenRefs[dimension.Ref] = struct{}{}
	}
	return nil
}

func eligibilityRejection(payload appinput.ProviderDocument, definition domainprofile.Definition) string {
	eligibleCodes := stringSet(definition.Eligibility.EligibleDimensionCodes)
	excludedCodes := stringSet(definition.Eligibility.ExcludedDimensionCodes)
	count := 0
	for _, dimension := range payload.Facts.Dimensions {
		if len(eligibleCodes) > 0 {
			if _, ok := eligibleCodes[dimension.Code]; !ok {
				continue
			}
		}
		if _, excluded := excludedCodes[dimension.Code]; excluded {
			continue
		}
		count++
	}
	if count < definition.Eligibility.MinEligibleDimensions {
		return "insufficient_eligible_dimensions"
	}
	if count > definition.Eligibility.MaxInputDimensions {
		return "dimension_overflow"
	}
	return ""
}

func hasExpectedStringAssertion(assertions []Assertion, assertionType, want string) bool {
	for _, assertion := range assertions {
		if assertion.Type == assertionType && fmt.Sprint(assertion.Value) == want {
			return true
		}
	}
	return false
}

func hasExpectedIntAssertion(assertions []Assertion, assertionType string, want int64) bool {
	for _, assertion := range assertions {
		if assertion.Type != assertionType {
			continue
		}
		switch value := assertion.Value.(type) {
		case json.Number:
			parsed, err := value.Int64()
			return err == nil && parsed == want
		case float64:
			return int64(value) == want && value == float64(want)
		}
	}
	return false
}

func stringSet(values []string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

// StableCaseIDs makes the report inventory easy to assert without depending
// on map iteration in callers.
func (r PreflightReport) StableCaseIDs() []string {
	ids := make([]string, 0, len(r.Cases))
	for _, result := range r.Cases {
		ids = append(ids, result.CaseID)
	}
	sort.Strings(ids)
	return ids
}
