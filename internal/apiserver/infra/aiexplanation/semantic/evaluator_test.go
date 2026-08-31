package semantic

import (
	"context"
	"crypto/sha1" // #nosec G505 -- Git blob identity is defined as SHA-1.
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	interpretationschema "github.com/FangcunMount/qs-server/api/schema/interpretation"
	appevaluation "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/evaluation"
	appport "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/port"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
	domainevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
	domainoutput "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/output"
)

func TestEvaluatorUsesFrozenIndependentRouteAndMinimizedSyntheticPayload(t *testing.T) {
	provider := &providerStub{raw: validSemanticOutput(t)}
	evaluator, err := NewEvaluator(provider, semanticRoute())
	if err != nil {
		t.Fatal(err)
	}

	identity := evaluator.Identity()
	if identity.Version != EvaluatorVersionV1 || identity.Prompt.TemplateID != PromptTemplateIDV1 ||
		identity.Prompt.Version != PromptVersionV1 || identity.Prompt.Fingerprint != PromptFingerprintV1 ||
		identity.Prompt.GitBlobSHA != PromptGitBlobSHAV1 {
		t.Fatalf("semantic evaluator identity = %#v", identity)
	}
	if identity.OutputSchema.Version != aiexplanation.SemanticEvaluationOutputSchemaVersionV1 ||
		identity.OutputSchema.Fingerprint != aiexplanation.NewFingerprint(interpretationschema.AIExplanationSemanticEvaluationOutputV1()) ||
		identity.Provider != semanticRoute().ExecutionSpec || identity.Decoding.MaxOutputTokens != semanticRoute().MaxOutputTokens {
		t.Fatalf("semantic evaluator release identity = %#v", identity)
	}

	request := validSemanticRequest(t)
	outcome, err := evaluator.Evaluate(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if provider.calls != 1 || provider.request.InvocationID != request.InvocationID || provider.request.Route.ExecutionSpec != semanticRoute().ExecutionSpec {
		t.Fatalf("Provider call/request = %d/%#v", provider.calls, provider.request)
	}
	if provider.request.SystemMessage != systemMessageV1 || provider.request.TaskMessage != taskMessageV1 ||
		provider.request.DataPreamble != dataPreambleV1 || provider.request.OutputSchema.Version != aiexplanation.SemanticEvaluationOutputSchemaVersionV1 {
		t.Fatalf("semantic Provider messages/schema drifted: %#v", provider.request)
	}
	if outcome.Failure != nil || outcome.Result == nil || outcome.ProviderReceipt == nil || outcome.ProviderCallCount != 1 ||
		outcome.InvocationID != request.InvocationID || outcome.RawOutput == nil || outcome.NormalizedOutput == nil ||
		outcome.StartedAt.IsZero() || outcome.FinishedAt.Before(outcome.StartedAt) {
		t.Fatalf("semantic evaluation outcome = %#v", outcome)
	}
	result := outcome.Result
	if result.EvaluatorVersion != EvaluatorVersionV1 || outcome.ProviderReceipt.RequestID != "judge-request-1" ||
		result.Scores.Faithfulness != 5 || len(result.Decisions) != 1 ||
		result.Decisions[0].Type != request.Assertions[0].Type || result.Decisions[0].Status != domainevaluation.AssertionPassed {
		t.Fatalf("semantic evaluation result = %#v", result)
	}

	var payload map[string]any
	if err := json.Unmarshal(provider.request.DataJSON, &payload); err != nil {
		t.Fatal(err)
	}
	assertExactKeys(t, payload, []string{"schema_version", "suite_id", "case_id", "attempt", "assessment_input", "candidate_output", "assertions"})
	assessment := payload["assessment_input"].(map[string]any)
	assertExactKeys(t, assessment, []string{"context", "facts"})
	assertion := payload["assertions"].([]any)[0].(map[string]any)
	assertExactKeys(t, assertion, []string{"type", "scope", "ordinal", "hard", "parameters"})
	if assertion["type"] != request.Assertions[0].Type || assertion["scope"] != "case" || assertion["ordinal"] != float64(1) {
		t.Fatalf("semantic assertion wire identity = %#v", assertion)
	}
	for _, forbidden := range []string{"report_id", "outcome_id", "assessment_id", "testee_id", "user_id", "profile_id", "invocation_id", "request_id"} {
		if jsonContainsKey(payload, forbidden) {
			t.Fatalf("semantic Provider payload leaked forbidden key %q", forbidden)
		}
	}
}

func TestEvaluatorRejectsInvalidOutputContract(t *testing.T) {
	tests := map[string]string{
		"unknown field": strings.TrimSuffix(string(validSemanticOutput(t)), "}") + `,"extra":true}`,
		"score outside rubric": `{
          "schema_version":"ai-explanation-semantic-evaluation-output/v1",
          "scores":{"faithfulness":6,"cross_dimension_quality":5,"suggestion_actionability":5,"audience_clarity":5,"concision":5},
          "rationale":"invalid score",
          "decisions":[{"type":"not_parallel_dimension_summary","scope":"case","ordinal":1,"status":"passed","detail":"ok"}]
        }`,
		"unknown decision": `{
          "schema_version":"ai-explanation-semantic-evaluation-output/v1",
          "scores":{"faithfulness":5,"cross_dimension_quality":5,"suggestion_actionability":5,"audience_clarity":5,"concision":5},
          "rationale":"invalid decision",
          "decisions":[{"type":"not_parallel_dimension_summary","scope":"case","ordinal":1,"status":"pending_semantic","detail":"ok"}]
        }`,
	}
	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			provider := &providerStub{raw: []byte(raw)}
			evaluator, err := NewEvaluator(provider, semanticRoute())
			if err != nil {
				t.Fatal(err)
			}
			outcome, err := evaluator.Evaluate(context.Background(), validSemanticRequest(t))
			if err != nil || outcome.Failure == nil || outcome.Failure.Code != domainevaluation.SemanticOutputSchemaInvalid ||
				string(outcome.RawOutput) != raw || len(outcome.NormalizedOutput) == 0 {
				t.Fatalf("strict semantic output outcome = %#v, error = %v", outcome, err)
			}
		})
	}
}

func TestEvaluatorUsesEnvelopeNormalizedProviderOutput(t *testing.T) {
	validationOutput := validSemanticOutput(t)
	provider := &providerStub{
		raw:              []byte("```json\n" + string(validationOutput) + "\n```"),
		validationOutput: validationOutput,
	}
	evaluator, err := NewEvaluator(provider, semanticRoute())
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := evaluator.Evaluate(context.Background(), validSemanticRequest(t))
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Result == nil || string(outcome.RawOutput) != string(provider.raw) || string(outcome.NormalizedOutput) != string(validationOutput) {
		t.Fatalf("semantic envelope outcome = %#v", outcome)
	}
	result := outcome.Result
	if result.Scores.Faithfulness != 5 || len(result.Decisions) != 1 {
		t.Fatalf("semantic evaluation result = %#v", result)
	}
}

func TestEvaluatorRejectsMismatchedReceipt(t *testing.T) {
	provider := &providerStub{
		raw: validSemanticOutput(t),
		receipt: &aiexplanation.ProviderReceipt{
			InvocationID: "other-invocation", RequestID: "judge-request-1",
			Provider: "judge-provider", Model: "judge-model",
		},
	}
	evaluator, err := NewEvaluator(provider, semanticRoute())
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := evaluator.Evaluate(context.Background(), validSemanticRequest(t))
	if err != nil || outcome.Failure == nil || outcome.Failure.Code != domainevaluation.SemanticReceiptInvalid ||
		outcome.ProviderReceipt == nil || outcome.ProviderReceipt.InvocationID != "other-invocation" || len(outcome.RawOutput) == 0 {
		t.Fatalf("mismatched semantic Provider receipt outcome = %#v, error = %v", outcome, err)
	}
}

func TestEvaluatorClassifiesProviderAndMissingOutputFailures(t *testing.T) {
	tests := []struct {
		name          string
		provider      *providerStub
		code          string
		resultUnknown bool
		providerCode  string
	}{
		{
			name: "provider failure",
			provider: &providerStub{err: &appport.ProviderError{
				Code: "provider_timeout", SafeMessage: "provider timed out", Retryable: true,
			}},
			code: domainevaluation.SemanticProviderFailed, providerCode: "provider_timeout",
		},
		{
			name: "provider result unknown",
			provider: &providerStub{err: &appport.ProviderError{
				Code: "provider_result_unknown", SafeMessage: "provider result is unknown", ResultUnknown: true,
			}},
			code: domainevaluation.SemanticResultUnknown, resultUnknown: true, providerCode: "provider_result_unknown",
		},
		{
			name: "missing output", provider: &providerStub{}, code: domainevaluation.SemanticOutputMissingOrTooLarge,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			evaluator, err := NewEvaluator(test.provider, semanticRoute())
			if err != nil {
				t.Fatal(err)
			}
			outcome, err := evaluator.Evaluate(context.Background(), validSemanticRequest(t))
			if err != nil || outcome.Failure == nil || outcome.Failure.Code != test.code ||
				outcome.Failure.ResultUnknown != test.resultUnknown || outcome.ProviderFailureCode != test.providerCode ||
				outcome.ProviderCallCount != 1 || outcome.Result != nil {
				t.Fatalf("semantic failure outcome = %#v, error = %v", outcome, err)
			}
		})
	}
}

func TestEvaluatorRejectsInvalidAssertionInventoryBeforeProvider(t *testing.T) {
	provider := &providerStub{raw: validSemanticOutput(t)}
	evaluator, err := NewEvaluator(provider, semanticRoute())
	if err != nil {
		t.Fatal(err)
	}

	request := validSemanticRequest(t)
	request.Assertions = append(request.Assertions, request.Assertions[0])
	if _, err := evaluator.Evaluate(context.Background(), request); err == nil || provider.calls != 0 {
		t.Fatalf("duplicate assertion error/calls = %v/%d", err, provider.calls)
	}

	request = validSemanticRequest(t)
	request.Assertions[0].Parameters.Type = "different_assertion"
	if _, err := evaluator.Evaluate(context.Background(), request); err == nil || provider.calls != 0 {
		t.Fatalf("mismatched assertion error/calls = %v/%d", err, provider.calls)
	}
}

func TestExecutablePromptMatchesNormativeMarkdownAndFrozenHashes(t *testing.T) {
	path := filepath.Join("..", "..", "..", "..", "..", "api", "schema", "interpretation", "ai-explanation-semantic-evaluator-prompt-v1.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	blocks := regexp.MustCompile("(?s)```text\\n(.*?)\\n```").FindAllStringSubmatch(strings.ReplaceAll(string(raw), "\r\n", "\n"), -1)
	if len(blocks) < 3 {
		t.Fatalf("normative semantic evaluator text blocks = %d", len(blocks))
	}
	if blocks[0][1] != systemMessageV1 || blocks[1][1] != taskMessageV1 {
		t.Fatal("executable semantic evaluator system/task messages drifted from normative Markdown")
	}
	dataBlock := strings.TrimSuffix(blocks[2][1], "\n\n{{semantic_evaluation_payload_json}}")
	if dataBlock != dataPreambleV1 {
		t.Fatal("executable semantic evaluator data preamble drifted from normative Markdown")
	}
	sha256Sum := sha256.Sum256(raw)
	if got := "sha256:" + hex.EncodeToString(sha256Sum[:]); got != PromptFingerprintV1.String() {
		t.Fatalf("semantic evaluator Prompt fingerprint = %s, want %s", got, PromptFingerprintV1)
	}
	gitBlobInput := append([]byte(fmt.Sprintf("blob %d\x00", len(raw))), raw...)
	gitBlobSum := sha1.Sum(gitBlobInput) // #nosec G401 -- Git blob identity is defined as SHA-1.
	if got := hex.EncodeToString(gitBlobSum[:]); got != PromptGitBlobSHAV1 {
		t.Fatalf("semantic evaluator Prompt Git blob = %s, want %s", got, PromptGitBlobSHAV1)
	}
}

type providerStub struct {
	calls            int
	request          appport.ProviderRequest
	raw              []byte
	validationOutput []byte
	receipt          *aiexplanation.ProviderReceipt
	err              error
}

func (p *providerStub) Generate(_ context.Context, request appport.ProviderRequest) (*appport.ProviderResponse, error) {
	p.calls++
	p.request = request
	if p.err != nil {
		return nil, p.err
	}
	receipt := aiexplanation.ProviderReceipt{
		InvocationID: request.InvocationID, RequestID: "judge-request-1",
		Provider: request.Route.ExecutionSpec.ResolvedProvider, Model: request.Route.ExecutionSpec.ResolvedModel,
		InputTokens: 100, OutputTokens: 50, Latency: 10 * time.Millisecond,
	}
	if p.receipt != nil {
		receipt = *p.receipt
	}
	return &appport.ProviderResponse{
		RawOutput: append([]byte(nil), p.raw...), ValidationOutput: append([]byte(nil), p.validationOutput...), Receipt: receipt,
	}, nil
}

func semanticRoute() appport.ProviderRoute {
	return appport.ProviderRoute{
		ExecutionSpec: aiexplanation.ProviderExecutionSpec{
			Route: "semantic_judge_v1", RouteRevision: "v1", ResolvedProvider: "judge-provider", ResolvedModel: "judge-model",
			Fingerprint: aiexplanation.NewFingerprint([]byte("independent-semantic-route-v1")),
		},
		Capabilities: appport.ProviderCapabilities{StructuredOutput: true}, Timeout: 20 * time.Second, MaxOutputTokens: 2500,
	}
}

func validSemanticRequest(t *testing.T) appevaluation.SemanticEvaluationRequest {
	t.Helper()
	suite, err := appevaluation.LoadV1()
	if err != nil {
		t.Fatal(err)
	}
	var payload any
	for _, candidate := range suite.Cases {
		if candidate.Expected.Execution == "call_provider" {
			payload = candidate.ProviderPayload
			break
		}
	}
	inputJSON, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	outputJSON, err := json.Marshal(validCandidate())
	if err != nil {
		t.Fatal(err)
	}
	assertion := appevaluation.Assertion{Type: "not_parallel_dimension_summary"}
	return appevaluation.SemanticEvaluationRequest{
		InvocationID: "ai-semantic-eval:9001:PROMPT-EVAL-001:1", SuiteID: suite.SuiteID,
		CaseID: "PROMPT-EVAL-001", Attempt: 1, InputJSON: inputJSON, OutputJSON: outputJSON,
		Assertions: []appevaluation.SemanticAssertion{{
			Type: assertion.Type, Scope: domainevaluation.AssertionScopeCase, Ordinal: 1, Parameters: assertion,
		}},
	}
}

func validCandidate() domainoutput.Content {
	refs := []domainoutput.EvidenceRef{
		{Kind: domainoutput.EvidenceKindDimension, Ref: "dimension:one"},
		{Kind: domainoutput.EvidenceKindDimension, Ref: "dimension:two"},
	}
	return domainoutput.Content{
		SchemaVersion: aiexplanation.OutputSchemaVersionV1,
		Summary:       "两个维度可以结合观察。",
		IntegratedInsights: []domainoutput.IntegratedInsight{{
			Kind: domainoutput.InsightKindContextDependent, Title: "组合观察", Content: "两个维度在部分情境中可能共同变化。",
			WhyItMatters: "结合观察有助于理解本次结果。", EvidenceRefs: refs,
		}},
		Suggestions: []domainoutput.Suggestion{{
			Origin: domainoutput.SuggestionOriginGeneratedLowRisk, Category: "daily_practice", Title: "简短记录", Goal: "观察共同变化",
			Actions: []string{"选择一个日常情境做简短记录"}, Rationale: "该步骤与两个维度相关。", EvidenceRefs: refs,
		}},
		Limitations: []string{"仅基于本次测评，不构成诊断或确定性判断。"},
	}
}

func validSemanticOutput(t *testing.T) []byte {
	t.Helper()
	raw, err := json.Marshal(outputDocument{
		SchemaVersion: aiexplanation.SemanticEvaluationOutputSchemaVersionV1,
		Scores: outputScores{
			Faithfulness: 5, CrossDimensionQuality: 5, SuggestionActionability: 4, AudienceClarity: 5, Concision: 4,
		},
		Rationale: "候选内容忠实、清晰并包含跨维度关系。",
		Decisions: []outputDecision{{
			Type: "not_parallel_dimension_summary", Scope: "case", Ordinal: 1, Status: "passed", Detail: "摘要表达了维度关系。",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func assertExactKeys(t *testing.T, value map[string]any, want []string) {
	t.Helper()
	if len(value) != len(want) {
		t.Fatalf("JSON keys = %v, want %v", mapKeys(value), want)
	}
	for _, key := range want {
		if _, exists := value[key]; !exists {
			t.Fatalf("JSON keys = %v, missing %q", mapKeys(value), key)
		}
	}
}

func mapKeys(value map[string]any) []string {
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	return keys
}

func jsonContainsKey(value any, target string) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if key == target || jsonContainsKey(child, target) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if jsonContainsKey(child, target) {
				return true
			}
		}
	}
	return false
}
