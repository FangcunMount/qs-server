package evaluation_test

import (
	"context"
	"crypto/sha1" // #nosec G505 -- Git blob identity is defined as SHA-1.
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	interpretationschema "github.com/FangcunMount/qs-server/api/schema/interpretation"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/evaluation"
	appport "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/port"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
)

func TestV1SuiteBytesMatchFrozenReleaseIdentity(t *testing.T) {
	raw := interpretationschema.AIExplanationPromptEvaluationCasesV1()
	if got := aiexplanation.NewFingerprint(raw); got != evaluation.SuiteFingerprintV1 {
		t.Fatalf("suite fingerprint = %s, want %s", got, evaluation.SuiteFingerprintV1)
	}
	gitBlobInput := append([]byte(fmt.Sprintf("blob %d\x00", len(raw))), raw...)
	gitBlobSum := sha1.Sum(gitBlobInput) // #nosec G401 -- Git blob identity is defined as SHA-1.
	if got := fmt.Sprintf("%x", gitBlobSum); got != evaluation.SuiteGitBlobSHAV1 {
		t.Fatalf("suite Git blob = %s, want %s", got, evaluation.SuiteGitBlobSHAV1)
	}
}

func TestV2SuiteBytesMatchFrozenReleaseIdentity(t *testing.T) {
	raw := interpretationschema.AIExplanationPromptEvaluationCasesV2()
	if got := aiexplanation.NewFingerprint(raw); got != evaluation.SuiteFingerprintV2 {
		t.Fatalf("suite fingerprint = %s, want %s", got, evaluation.SuiteFingerprintV2)
	}
	gitBlobInput := append([]byte(fmt.Sprintf("blob %d\x00", len(raw))), raw...)
	gitBlobSum := sha1.Sum(gitBlobInput) // #nosec G401 -- Git blob identity is defined as SHA-1.
	if got := fmt.Sprintf("%x", gitBlobSum); got != evaluation.SuiteGitBlobSHAV2 {
		t.Fatalf("suite Git blob = %s, want %s", got, evaluation.SuiteGitBlobSHAV2)
	}
	suite, err := evaluation.LoadFrozen(evaluation.SuiteIDV2, evaluation.SuiteVersionV2, evaluation.SuiteFingerprintV2)
	if err != nil {
		t.Fatal(err)
	}
	if suite.Prompt.Version != "v2" || suite.ProfileFixture.GenerationPolicy.PromptVersion != "v2" || suite.ProfileFixture.Version != "v2" {
		t.Fatalf("v2 suite identities = %#v %#v", suite.Prompt, suite.ProfileFixture)
	}
	if _, err := evaluation.LoadFrozen(evaluation.SuiteIDV1, evaluation.SuiteVersionV1, evaluation.SuiteFingerprintV2); !errors.Is(err, evaluation.ErrInvalidSuite) {
		t.Fatalf("mismatched frozen suite error = %v", err)
	}
}

func TestV3SuiteBytesMatchFrozenReleaseIdentity(t *testing.T) {
	raw := interpretationschema.AIExplanationPromptEvaluationCasesV3()
	if got := aiexplanation.NewFingerprint(raw); got != evaluation.SuiteFingerprintV3 {
		t.Fatalf("suite fingerprint = %s, want %s", got, evaluation.SuiteFingerprintV3)
	}
	gitBlobInput := append([]byte(fmt.Sprintf("blob %d\x00", len(raw))), raw...)
	gitBlobSum := sha1.Sum(gitBlobInput) // #nosec G401 -- Git blob identity is defined as SHA-1.
	if got := fmt.Sprintf("%x", gitBlobSum); got != evaluation.SuiteGitBlobSHAV3 {
		t.Fatalf("suite Git blob = %s, want %s", got, evaluation.SuiteGitBlobSHAV3)
	}
	suite, err := evaluation.LoadFrozen(evaluation.SuiteIDV3, evaluation.SuiteVersionV3, evaluation.SuiteFingerprintV3)
	if err != nil {
		t.Fatal(err)
	}
	if suite.Prompt.Version != "v3" || suite.ProfileFixture.GenerationPolicy.PromptVersion != "v3" || suite.ProfileFixture.Version != "v3" {
		t.Fatalf("v3 suite identities = %#v %#v", suite.Prompt, suite.ProfileFixture)
	}
	if _, err := evaluation.LoadFrozen(evaluation.SuiteIDV2, evaluation.SuiteVersionV2, evaluation.SuiteFingerprintV3); !errors.Is(err, evaluation.ErrInvalidSuite) {
		t.Fatalf("mismatched frozen suite error = %v", err)
	}
}

func TestV1PreflightPlansThirtyFiveCallsWithoutCallingProvider(t *testing.T) {
	suite, err := evaluation.LoadV1()
	if err != nil {
		t.Fatal(err)
	}
	fixed := time.Date(2026, 8, 27, 1, 2, 3, 0, time.UTC)
	runner, err := evaluation.NewPreflightRunner(promptResolverStub{}, schemaResolverStub{}, func() time.Time { return fixed })
	if err != nil {
		t.Fatal(err)
	}
	report, err := runner.Run(context.Background(), suite)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "passed" || report.GenerationCases != 7 || report.PreflightCases != 1 || report.PlannedProviderInvocations != 35 || report.ActualProviderInvocations != 0 {
		t.Fatalf("unexpected preflight summary: %#v", report)
	}
	if report.PublishEvidence {
		t.Fatal("offline preflight must never be publish evidence")
	}
	if got := report.Cases[7]; got.ActualExecution != "rejected_before_provider" || got.RejectionReason != "insufficient_eligible_dimensions" || got.ActualProviderCalls != 0 {
		t.Fatalf("preflight rejection = %#v", got)
	}
	for _, result := range report.Cases[:7] {
		if result.ActualExecution != "ready_for_provider" || result.PlannedProviderCalls != 5 || !result.RenderedPromptChecked || result.InputFingerprint == "" {
			t.Fatalf("generation case preflight = %#v", result)
		}
	}
}

func TestV2PreflightPlansThirtyFiveCallsWithoutCallingProvider(t *testing.T) {
	suite, err := evaluation.LoadV2()
	if err != nil {
		t.Fatal(err)
	}
	runner, err := evaluation.NewPreflightRunner(promptResolverStub{}, schemaResolverStub{}, func() time.Time {
		return time.Date(2026, 9, 1, 1, 2, 3, 0, time.UTC)
	})
	if err != nil {
		t.Fatal(err)
	}
	report, err := runner.Run(context.Background(), suite)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "passed" || report.SuiteVersion != evaluation.SuiteVersionV2 || report.PromptVersion != "v2" ||
		report.GenerationCases != 7 || report.PreflightCases != 1 || report.PlannedProviderInvocations != 35 || report.ActualProviderInvocations != 0 {
		t.Fatalf("unexpected v2 preflight summary: %#v", report)
	}
}

func TestV3PreflightPlansThirtyFiveCallsWithoutCallingProvider(t *testing.T) {
	suite, err := evaluation.LoadV3()
	if err != nil {
		t.Fatal(err)
	}
	runner, err := evaluation.NewPreflightRunner(promptResolverStub{}, schemaResolverStub{}, func() time.Time {
		return time.Date(2026, 9, 1, 1, 2, 3, 0, time.UTC)
	})
	if err != nil {
		t.Fatal(err)
	}
	report, err := runner.Run(context.Background(), suite)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "passed" || report.SuiteVersion != evaluation.SuiteVersionV3 || report.PromptVersion != "v3" ||
		report.GenerationCases != 7 || report.PreflightCases != 1 || report.PlannedProviderInvocations != 35 || report.ActualProviderInvocations != 0 {
		t.Fatalf("unexpected v3 preflight summary: %#v", report)
	}
}

func TestParseRejectsUnknownAssertionType(t *testing.T) {
	raw := interpretationschema.AIExplanationPromptEvaluationCasesV1()
	raw = []byte(strings.Replace(string(raw), `"type": "output_schema_valid"`, `"type": "silently_ignore_me"`, 1))
	suite, err := evaluation.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	runner, err := evaluation.NewPreflightRunner(promptResolverStub{}, schemaResolverStub{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	report, err := runner.Run(context.Background(), suite)
	if !errors.Is(err, evaluation.ErrInvalidSuite) || report == nil || report.Status != "failed" {
		t.Fatalf("run error/report = %v %#v", err, report)
	}
}

func TestParseRejectsUnknownSuiteField(t *testing.T) {
	raw := interpretationschema.AIExplanationPromptEvaluationCasesV1()
	var object map[string]any
	if err := json.Unmarshal(raw, &object); err != nil {
		t.Fatal(err)
	}
	object["future_field"] = true
	mutated, err := json.Marshal(object)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := evaluation.Parse(mutated); !errors.Is(err, evaluation.ErrInvalidSuite) {
		t.Fatalf("parse error = %v", err)
	}
}

func TestParseRejectsBusinessIdentityInsideProviderPayload(t *testing.T) {
	raw := interpretationschema.AIExplanationPromptEvaluationCasesV1()
	var object map[string]any
	if err := json.Unmarshal(raw, &object); err != nil {
		t.Fatal(err)
	}
	cases := object["cases"].([]any)
	first := cases[0].(map[string]any)
	payload := first["provider_payload"].(map[string]any)
	facts := payload["facts"].(map[string]any)
	facts["testee_id"] = "synthetic-but-forbidden"
	mutated, err := json.Marshal(object)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := evaluation.Parse(mutated); !errors.Is(err, evaluation.ErrInvalidSuite) || !strings.Contains(err.Error(), "testee_id") {
		t.Fatalf("parse error = %v", err)
	}
}

func TestPreflightUsesInputJSONSchemaForGenerationFixtures(t *testing.T) {
	suite, err := evaluation.LoadV1()
	if err != nil {
		t.Fatal(err)
	}
	suite.Cases[0].ProviderPayload.Facts.Dimensions[0].HierarchyLevel = -1
	runner, err := evaluation.NewPreflightRunner(promptResolverStub{}, schemaResolverStub{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	report, err := runner.Run(context.Background(), suite)
	if !errors.Is(err, evaluation.ErrInvalidSuite) || report == nil || !strings.Contains(err.Error(), "hierarchy_level") {
		t.Fatalf("run error/report = %v %#v", err, report)
	}
}

type promptResolverStub struct{}

func (promptResolverStub) ResolvePromptPackage(_ context.Context, templateID, version string) (appport.PromptPackage, error) {
	return appport.PromptPackage{
		Ref: aiexplanation.PromptRef{
			TemplateID: templateID, Version: version,
			Fingerprint: aiexplanation.NewFingerprint([]byte("test Prompt")), GitBlobSHA: "test-blob",
		},
		SystemMessage: "system", TaskTemplate: "task", DataPreamble: "data",
	}, nil
}

type schemaResolverStub struct{}

func (schemaResolverStub) ResolveOutputSchema(_ context.Context, version string) (appport.StructuredOutputSchema, error) {
	raw := []byte(`{"type":"object"}`)
	return appport.StructuredOutputSchema{Version: version, Name: "test", JSON: raw, Fingerprint: aiexplanation.NewFingerprint(raw)}, nil
}

func TestV4SuiteBytesMatchFrozenReleaseIdentity(t *testing.T) {
	raw := interpretationschema.AIExplanationPromptEvaluationCasesV4()
	if got := aiexplanation.NewFingerprint(raw); got != evaluation.SuiteFingerprintV4 {
		t.Fatalf("suite fingerprint = %s, want %s", got, evaluation.SuiteFingerprintV4)
	}
	gitBlobInput := append([]byte(fmt.Sprintf("blob %d\x00", len(raw))), raw...)
	gitBlobSum := sha1.Sum(gitBlobInput) // #nosec G401 -- Git blob identity is defined as SHA-1.
	if got := fmt.Sprintf("%x", gitBlobSum); got != evaluation.SuiteGitBlobSHAV4 {
		t.Fatalf("suite Git blob = %s, want %s", got, evaluation.SuiteGitBlobSHAV4)
	}
	suite, err := evaluation.LoadFrozen(evaluation.SuiteIDV4, evaluation.SuiteVersionV4, evaluation.SuiteFingerprintV4)
	if err != nil {
		t.Fatal(err)
	}
	if suite.Prompt.Version != "v4" || suite.ProfileFixture.GenerationPolicy.PromptVersion != "v4" || suite.ProfileFixture.Version != "v4" {
		t.Fatalf("v4 suite identities = %#v %#v", suite.Prompt, suite.ProfileFixture)
	}
	if _, err := evaluation.LoadFrozen(evaluation.SuiteIDV2, evaluation.SuiteVersionV2, evaluation.SuiteFingerprintV4); !errors.Is(err, evaluation.ErrInvalidSuite) {
		t.Fatalf("mismatched frozen suite error = %v", err)
	}
}

func TestV4PreflightPlansThirtyFiveCallsWithoutCallingProvider(t *testing.T) {
	suite, err := evaluation.LoadV4()
	if err != nil {
		t.Fatal(err)
	}
	runner, err := evaluation.NewPreflightRunner(promptResolverStub{}, schemaResolverStub{}, func() time.Time {
		return time.Date(2026, 9, 1, 1, 2, 3, 0, time.UTC)
	})
	if err != nil {
		t.Fatal(err)
	}
	report, err := runner.Run(context.Background(), suite)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "passed" || report.SuiteVersion != evaluation.SuiteVersionV4 || report.PromptVersion != "v4" ||
		report.GenerationCases != 7 || report.PreflightCases != 1 || report.PlannedProviderInvocations != 35 || report.ActualProviderInvocations != 0 {
		t.Fatalf("unexpected v4 preflight summary: %#v", report)
	}
}

// The new prompt must be judged against the same examples and assertions.
func TestV4PreservesV3EvaluationCasesAndPolicy(t *testing.T) {
	old, err := evaluation.LoadV3()
	if err != nil {
		t.Fatal(err)
	}
	current, err := evaluation.LoadV4()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(old.Cases, current.Cases) || !reflect.DeepEqual(old.DefaultGenerationAssertions, current.DefaultGenerationAssertions) || !reflect.DeepEqual(old.ExecutionPolicy, current.ExecutionPolicy) {
		t.Fatal("v4 changed evaluation examples, assertions or execution requirements")
	}
	definition := current.ProfileFixture.Definition
	definition.Version = old.ProfileFixture.Version
	definition.GenerationPolicy.PromptVersion = old.ProfileFixture.GenerationPolicy.PromptVersion
	if !reflect.DeepEqual(definition, old.ProfileFixture.Definition) {
		t.Fatal("v4 changed Profile policy beyond version and prompt binding")
	}
}

func TestV5SuiteBytesMatchFrozenReleaseIdentity(t *testing.T) {
	raw := interpretationschema.AIExplanationPromptEvaluationCasesV5()
	if got := aiexplanation.NewFingerprint(raw); got != evaluation.SuiteFingerprintV5 {
		t.Fatalf("suite fingerprint = %s, want %s", got, evaluation.SuiteFingerprintV5)
	}
	gitBlobInput := append([]byte(fmt.Sprintf("blob %d\x00", len(raw))), raw...)
	gitBlobSum := sha1.Sum(gitBlobInput) // #nosec G401 -- Git blob identity is defined as SHA-1.
	if got := fmt.Sprintf("%x", gitBlobSum); got != evaluation.SuiteGitBlobSHAV5 {
		t.Fatalf("suite Git blob = %s, want %s", got, evaluation.SuiteGitBlobSHAV5)
	}
	suite, err := evaluation.LoadFrozen(evaluation.SuiteIDV5, evaluation.SuiteVersionV5, evaluation.SuiteFingerprintV5)
	if err != nil {
		t.Fatal(err)
	}
	if suite.Prompt.Version != "v5" || suite.ProfileFixture.GenerationPolicy.PromptVersion != "v5" || suite.ProfileFixture.Version != "v5" {
		t.Fatalf("v5 suite identities = %#v %#v", suite.Prompt, suite.ProfileFixture)
	}
	if _, err := evaluation.LoadFrozen(evaluation.SuiteIDV2, evaluation.SuiteVersionV2, evaluation.SuiteFingerprintV5); !errors.Is(err, evaluation.ErrInvalidSuite) {
		t.Fatalf("mismatched frozen suite error = %v", err)
	}
}

func TestV5PreflightPlansThirtyFiveCallsWithoutCallingProvider(t *testing.T) {
	suite, err := evaluation.LoadV5()
	if err != nil {
		t.Fatal(err)
	}
	runner, err := evaluation.NewPreflightRunner(promptResolverStub{}, schemaResolverStub{}, func() time.Time {
		return time.Date(2026, 9, 1, 1, 2, 3, 0, time.UTC)
	})
	if err != nil {
		t.Fatal(err)
	}
	report, err := runner.Run(context.Background(), suite)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "passed" || report.SuiteVersion != evaluation.SuiteVersionV5 || report.PromptVersion != "v5" ||
		report.GenerationCases != 7 || report.PreflightCases != 1 || report.PlannedProviderInvocations != 35 || report.ActualProviderInvocations != 0 {
		t.Fatalf("unexpected v5 preflight summary: %#v", report)
	}
}

func TestV5PreservesV4EvaluationCasesAndPolicy(t *testing.T) {
	old, err := evaluation.LoadV4()
	if err != nil {
		t.Fatal(err)
	}
	current, err := evaluation.LoadV5()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(old.Cases, current.Cases) || !reflect.DeepEqual(old.DefaultGenerationAssertions, current.DefaultGenerationAssertions) || !reflect.DeepEqual(old.ExecutionPolicy, current.ExecutionPolicy) {
		t.Fatal("v5 changed evaluation examples, assertions or execution requirements")
	}
	definition := current.ProfileFixture.Definition
	definition.Version = old.ProfileFixture.Version
	definition.GenerationPolicy.PromptVersion = old.ProfileFixture.GenerationPolicy.PromptVersion
	if !reflect.DeepEqual(definition, old.ProfileFixture.Definition) {
		t.Fatal("v5 changed Profile policy beyond version and prompt binding")
	}
}
