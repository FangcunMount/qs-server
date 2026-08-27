package evaluation_test

import (
	"context"
	"crypto/sha1" // #nosec G505 -- Git blob identity is defined as SHA-1.
	"encoding/json"
	"errors"
	"fmt"
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
