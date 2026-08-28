package responsesapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	appport "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/port"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
	domainrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/run"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestProviderSendsOneStatelessStructuredResponsesRequest(t *testing.T) {
	var captured map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer test-secret" {
			t.Errorf("request method/auth = %s/%q", r.Method, r.Header.Get("Authorization"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("content type = %q", r.Header.Get("Content-Type"))
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
          "id":"resp_123","status":"completed","model":"gpt-test-2026-01-01",
          "output":[{"type":"reasoning"},{"type":"message","content":[{"type":"output_text","text":"{\"summary\":\"ok\"}"}]}],
          "usage":{"input_tokens":21,"output_tokens":8}
        }`))
	}))
	defer server.Close()

	provider := mustProvider(t, Config{Provider: ProviderOpenAI, Endpoint: server.URL, APIKey: "test-secret", HTTPClient: server.Client()})
	request := validRequest()
	response, err := provider.Generate(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if string(response.RawOutput) != `{"summary":"ok"}` {
		t.Fatalf("raw output = %s", response.RawOutput)
	}
	if response.Receipt.InvocationID != request.InvocationID || response.Receipt.RequestID != "resp_123" || response.Receipt.Provider != ProviderOpenAI || response.Receipt.Model != request.Route.ExecutionSpec.ResolvedModel {
		t.Fatalf("receipt = %#v", response.Receipt)
	}
	if response.Receipt.InputTokens != 21 || response.Receipt.OutputTokens != 8 || response.Receipt.Latency < 0 {
		t.Fatalf("receipt usage = %#v", response.Receipt)
	}

	if captured["model"] != request.Route.ExecutionSpec.ResolvedModel || captured["instructions"] != request.SystemMessage || captured["store"] != false {
		t.Fatalf("request header fields = %#v", captured)
	}
	if _, exists := captured["tools"]; exists {
		t.Fatal("one-shot AI explanation request must not enable tools")
	}
	if _, exists := captured["metadata"]; exists {
		t.Fatal("internal invocation identity must not be sent as metadata")
	}
	if strings.Contains(mustJSON(t, captured), request.InvocationID) {
		t.Fatal("internal invocation identity leaked into provider body")
	}
	input := captured["input"].([]any)
	if len(input) != 2 || input[0].(map[string]any)["role"] != "developer" || input[1].(map[string]any)["role"] != "user" {
		t.Fatalf("input messages = %#v", input)
	}
	developerText := input[0].(map[string]any)["content"].([]any)[0].(map[string]any)["text"]
	dataText := input[1].(map[string]any)["content"].([]any)[0].(map[string]any)["text"]
	if developerText != request.TaskMessage || !strings.HasPrefix(dataText.(string), request.DataPreamble+"\n\n") || !strings.HasSuffix(dataText.(string), string(request.DataJSON)) {
		t.Fatalf("separated messages = %#v", input)
	}
	format := captured["text"].(map[string]any)["format"].(map[string]any)
	if format["type"] != "json_schema" || format["strict"] != true || format["name"] != "AIExplanationOutput_v1" {
		t.Fatalf("structured format = %#v", format)
	}
	if format["schema"].(map[string]any)["additionalProperties"] != false {
		t.Fatalf("structured schema = %#v", format["schema"])
	}
}

func TestDeepSeekProviderUsesResponsesSchemaWithoutOpenAIStrictExtension(t *testing.T) {
	var captured map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(`{
          "id":"ds_resp_123","status":"completed","model":"deepseek-v4-flash",
          "output":[{"type":"reasoning","content":[{"type":"reasoning_text","text":"not persisted"}]},{"type":"message","content":[{"type":"output_text","text":"{\"summary\":\"ok\"}"}]}],
          "usage":{"input_tokens":34,"output_tokens":13}
        }`))
	}))
	defer server.Close()

	provider := mustProvider(t, Config{
		Provider: ProviderDeepSeek, Endpoint: server.URL, APIKey: "test-deepseek-secret", HTTPClient: server.Client(),
	})
	request := validRequestForProvider(ProviderDeepSeek, "deepseek-v4-flash")
	response, err := provider.Generate(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if response.Receipt.Provider != ProviderDeepSeek || response.Receipt.Model != "deepseek-v4-flash" ||
		response.Receipt.InputTokens != 34 || response.Receipt.OutputTokens != 13 {
		t.Fatalf("DeepSeek receipt = %#v", response.Receipt)
	}
	format := captured["text"].(map[string]any)["format"].(map[string]any)
	if format["type"] != "json_schema" || format["name"] != "AIExplanationOutput_v1" || format["schema"] == nil {
		t.Fatalf("DeepSeek structured format = %#v", format)
	}
	if _, exists := format["strict"]; exists {
		t.Fatalf("DeepSeek request must not send the OpenAI-only strict extension: %#v", format)
	}
	if _, exists := captured["store"]; exists {
		t.Fatalf("DeepSeek request must omit the unsupported store parameter: %#v", captured)
	}
}

func TestDeepSeekProviderConcatenatesOneMessageOutputTextParts(t *testing.T) {
	provider := providerForProviderResponse(t, ProviderDeepSeek, http.StatusOK, `{
      "id":"ds_resp_parts","status":"completed","model":"deepseek-v4-flash",
      "output":[
        {"type":"reasoning","content":[{"type":"reasoning_text","text":"not persisted"}]},
        {"type":"message","content":[
          {"type":"output_text","text":"{\"summary\":"},
          {"type":"output_text","text":"\"ok\"}"}
        ]}
      ],
      "usage":{"input_tokens":34,"output_tokens":13}
    }`)

	response, err := provider.Generate(context.Background(), validRequestForProvider(ProviderDeepSeek, "deepseek-v4-flash"))
	if err != nil {
		t.Fatal(err)
	}
	if string(response.RawOutput) != `{"summary":"ok"}` {
		t.Fatalf("concatenated raw output = %q", response.RawOutput)
	}
}

func TestProviderClassifiesDocumentedIncompleteReasons(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		reason string
		kind   domainrun.FailureKind
		code   string
	}{
		{name: "output token limit", reason: "max_output_tokens", kind: domainrun.FailureKindProviderTransport, code: "provider_output_token_limit"},
		{name: "content filter", reason: "content_filter", kind: domainrun.FailureKindProviderRefusal, code: "provider_refusal"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			provider := providerForProviderResponse(t, ProviderDeepSeek, http.StatusOK, fmt.Sprintf(`{
              "id":"ds_incomplete","status":"incomplete","model":"deepseek-v4-flash",
              "incomplete_details":{"reason":%q},"output":[]
            }`, testCase.reason))
			_, err := provider.Generate(context.Background(), validRequestForProvider(ProviderDeepSeek, "deepseek-v4-flash"))
			classified := requireProviderError(t, err)
			if classified.Kind != testCase.kind || classified.Code != testCase.code || classified.Retryable || classified.ResultUnknown {
				t.Fatalf("classified incomplete response = %#v", classified)
			}
		})
	}
}

func TestProviderRecordsIncompleteResponseUsageAndSafeShape(t *testing.T) {
	shapeBefore := testutil.ToFloat64(providerResponseShapesTotal.WithLabelValues(
		providerPurposeGeneration,
		providerResponseStatusIncompleteTokenLimit,
		providerResponseShapeSingleMessageOutputText,
	))
	inputBefore := testutil.ToFloat64(providerResponseTokensTotal.WithLabelValues(
		providerPurposeGeneration,
		providerResponseStatusIncompleteTokenLimit,
		"input",
	))
	outputBefore := testutil.ToFloat64(providerResponseTokensTotal.WithLabelValues(
		providerPurposeGeneration,
		providerResponseStatusIncompleteTokenLimit,
		"output",
	))

	provider := providerForProviderResponse(t, ProviderDeepSeek, http.StatusOK, `{
      "id":"ds_incomplete_usage","status":"incomplete","model":"deepseek-v4-flash",
      "incomplete_details":{"reason":"max_output_tokens"},
      "output":[{"type":"message","content":[{"type":"output_text","text":"partial provider output must not escape"}]}],
      "usage":{"input_tokens":321,"output_tokens":8000}
    }`)
	_, err := provider.Generate(context.Background(), validRequestForProvider(ProviderDeepSeek, "deepseek-v4-flash"))
	classified := requireProviderError(t, err)
	if classified.Code != "provider_output_token_limit" || classified.SafeMessage != "Provider 输出达到 token 上限，未形成完整结构化结果" {
		t.Fatalf("classified incomplete response = %#v", classified)
	}
	if strings.Contains(err.Error(), "partial provider output") {
		t.Fatal("partial provider output leaked through provider error")
	}
	if delta := testutil.ToFloat64(providerResponseShapesTotal.WithLabelValues(
		providerPurposeGeneration,
		providerResponseStatusIncompleteTokenLimit,
		providerResponseShapeSingleMessageOutputText,
	)) - shapeBefore; delta != 1 {
		t.Fatalf("incomplete shape metric delta = %v", delta)
	}
	if delta := testutil.ToFloat64(providerResponseTokensTotal.WithLabelValues(
		providerPurposeGeneration,
		providerResponseStatusIncompleteTokenLimit,
		"input",
	)) - inputBefore; delta != 321 {
		t.Fatalf("incomplete input token metric delta = %v", delta)
	}
	if delta := testutil.ToFloat64(providerResponseTokensTotal.WithLabelValues(
		providerPurposeGeneration,
		providerResponseStatusIncompleteTokenLimit,
		"output",
	)) - outputBefore; delta != 8000 {
		t.Fatalf("incomplete output token metric delta = %v", delta)
	}
}

func TestProviderExplainsInvalidOutputCardinalityWithoutLeakingContent(t *testing.T) {
	for _, testCase := range []struct {
		name        string
		output      string
		expectedMsg string
		shape       string
	}{
		{
			name:        "no message",
			output:      `[{"type":"reasoning","content":[{"type":"reasoning_text","text":"sensitive reasoning"}]}]`,
			expectedMsg: "Provider 未返回结构化消息",
			shape:       providerResponseShapeNoMessage,
		},
		{
			name: "multiple messages",
			output: `[
              {"type":"message","content":[{"type":"output_text","text":"first sensitive output"}]},
              {"type":"message","content":[{"type":"output_text","text":"second sensitive output"}]}
            ]`,
			expectedMsg: "Provider 返回了多个结构化消息",
			shape:       providerResponseShapeMultipleMessages,
		},
		{
			name:        "message without output text",
			output:      `[{"type":"message","content":[{"type":"reasoning_text","text":"sensitive reasoning"}]}]`,
			expectedMsg: "Provider 结构化消息缺少输出文本",
			shape:       providerResponseShapeSingleMessageNoText,
		},
		{
			name:        "message with empty output text",
			output:      `[{"type":"message","content":[{"type":"output_text","text":"  "}]}]`,
			expectedMsg: "Provider 结构化消息的输出文本为空",
			shape:       providerResponseShapeSingleMessageEmptyText,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			shapeBefore := testutil.ToFloat64(providerResponseShapesTotal.WithLabelValues(
				providerPurposeGeneration,
				providerResponseStatusCompleted,
				testCase.shape,
			))
			provider := providerForResponse(t, http.StatusOK, fmt.Sprintf(`{
              "id":"resp_invalid_shape","status":"completed","model":"gpt-test-2026-01-01",
              "output":%s
            }`, testCase.output))
			_, err := provider.Generate(context.Background(), validRequest())
			classified := requireProviderError(t, err)
			if classified.Code != "provider_output_cardinality_invalid" || classified.SafeMessage != testCase.expectedMsg {
				t.Fatalf("classified invalid output = %#v", classified)
			}
			if strings.Contains(err.Error(), "sensitive") {
				t.Fatal("provider-generated output leaked through cardinality error")
			}
			if delta := testutil.ToFloat64(providerResponseShapesTotal.WithLabelValues(
				providerPurposeGeneration,
				providerResponseStatusCompleted,
				testCase.shape,
			)) - shapeBefore; delta != 1 {
				t.Fatalf("response shape metric delta = %v", delta)
			}
		})
	}
}

func TestProviderClassifiesRefusalWithoutReturningRawRefusal(t *testing.T) {
	provider := providerForResponse(t, http.StatusOK, `{
      "id":"resp_refusal","status":"completed","model":"gpt-test-2026-01-01",
      "output":[{"type":"message","content":[{"type":"refusal","refusal":"sensitive provider text"}]}]
    }`)
	_, err := provider.Generate(context.Background(), validRequest())
	classified := requireProviderError(t, err)
	if classified.Kind != domainrun.FailureKindProviderRefusal || classified.Code != "provider_refusal" || classified.Retryable || classified.ResultUnknown {
		t.Fatalf("classified error = %#v", classified)
	}
	if strings.Contains(err.Error(), "sensitive provider text") {
		t.Fatal("raw refusal leaked through provider error")
	}
}

func TestProviderClassifiesRateLimitFromStatusOnly(t *testing.T) {
	provider := providerForResponse(t, http.StatusTooManyRequests, `{"error":{"message":"sensitive request echo"}}`)
	_, err := provider.Generate(context.Background(), validRequest())
	classified := requireProviderError(t, err)
	if classified.Kind != domainrun.FailureKindProviderRateLimit || !classified.Retryable || classified.ResultUnknown {
		t.Fatalf("classified error = %#v", classified)
	}
	if strings.Contains(err.Error(), "sensitive request echo") {
		t.Fatal("provider HTTP body leaked through error")
	}
}

func TestDeepSeekProviderClassifiesDocumentedHTTPStatuses(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		status    int
		kind      domainrun.FailureKind
		code      string
		retryable bool
	}{
		{name: "insufficient balance", status: http.StatusPaymentRequired, kind: domainrun.FailureKindProviderTransport, code: "provider_request_rejected"},
		{name: "rate limited", status: http.StatusTooManyRequests, kind: domainrun.FailureKindProviderRateLimit, code: "provider_rate_limited", retryable: true},
		{name: "service unavailable", status: http.StatusServiceUnavailable, kind: domainrun.FailureKindProviderTransport, code: "provider_server_error", retryable: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			provider := providerForProviderResponse(t, ProviderDeepSeek, testCase.status, `{"error":{"message":"must not escape"}}`)
			_, err := provider.Generate(context.Background(), validRequestForProvider(ProviderDeepSeek, "deepseek-test-model"))
			classified := requireProviderError(t, err)
			if classified.Kind != testCase.kind || classified.Code != testCase.code || classified.Retryable != testCase.retryable || classified.ResultUnknown {
				t.Fatalf("classified DeepSeek error = %#v", classified)
			}
			if strings.Contains(err.Error(), "must not escape") {
				t.Fatal("DeepSeek HTTP response body leaked through provider error")
			}
		})
	}
}

func TestProviderMarksPostDispatchTimeoutResultUnknown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
	}))
	defer server.Close()
	provider := mustProvider(t, Config{Provider: ProviderOpenAI, Endpoint: server.URL, APIKey: "test-secret", HTTPClient: server.Client()})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := provider.Generate(ctx, validRequest())
	classified := requireProviderError(t, err)
	if classified.Kind != domainrun.FailureKindProviderTimeout || !classified.Retryable || !classified.ResultUnknown {
		t.Fatalf("classified error = %#v", classified)
	}
}

func TestProviderClassifiesResponseBodyDeadlineAsUnknownTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		<-r.Context().Done()
	}))
	defer server.Close()

	provider := mustProvider(t, Config{Provider: ProviderOpenAI, Endpoint: server.URL, APIKey: "test-secret", HTTPClient: server.Client()})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := provider.Generate(ctx, validRequest())
	classified := requireProviderError(t, err)
	if classified.Kind != domainrun.FailureKindProviderTimeout || classified.Code != "provider_timeout" || !classified.Retryable || !classified.ResultUnknown {
		t.Fatalf("classified body deadline = %#v", classified)
	}
}

func TestProviderMarksGenericResponseBodyReadFailureResultUnknown(t *testing.T) {
	client := &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       readFailureBody{},
		}, nil
	})}
	provider := mustProvider(t, Config{Provider: ProviderOpenAI, Endpoint: "https://provider.example/responses", APIKey: "test-secret", HTTPClient: client})
	_, err := provider.Generate(context.Background(), validRequest())
	classified := requireProviderError(t, err)
	if classified.Kind != domainrun.FailureKindProviderTransport || classified.Code != "provider_response_read_failed" || !classified.Retryable || !classified.ResultUnknown {
		t.Fatalf("classified body read failure = %#v", classified)
	}
	if strings.Contains(err.Error(), "sensitive response read detail") {
		t.Fatal("raw response read failure leaked through provider error")
	}
}

func TestProviderDoesNotMarkPreDispatchCancellationUnknown(t *testing.T) {
	provider := mustProvider(t, Config{Provider: ProviderOpenAI, Endpoint: "https://example.invalid/v1/responses", APIKey: "test-secret"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := provider.Generate(ctx, validRequest())
	classified := requireProviderError(t, err)
	if classified.ResultUnknown || classified.Retryable {
		t.Fatalf("classified error = %#v", classified)
	}
}

func TestProviderRejectsMismatchedResolvedModel(t *testing.T) {
	provider := providerForResponse(t, http.StatusOK, `{
      "id":"resp_123","status":"completed","model":"different-model",
      "output":[{"type":"message","content":[{"type":"output_text","text":"{}"}]}]
    }`)
	_, err := provider.Generate(context.Background(), validRequest())
	classified := requireProviderError(t, err)
	if classified.Code != "provider_model_mismatch" || classified.Retryable {
		t.Fatalf("classified error = %#v", classified)
	}
}

func TestProviderRejectsRouteForDifferentProviderBeforeDispatch(t *testing.T) {
	provider := mustProvider(t, Config{Provider: ProviderDeepSeek, APIKey: "secret"})
	_, err := provider.Generate(context.Background(), validRequest())
	classified := requireProviderError(t, err)
	if classified.Code != "provider_request_invalid" || classified.ResultUnknown {
		t.Fatalf("mismatched Provider error = %#v", classified)
	}
}

func TestProviderRejectsOversizedResponse(t *testing.T) {
	provider := providerForResponseWithLimit(t, http.StatusOK, strings.Repeat("x", 65), 64)
	_, err := provider.Generate(context.Background(), validRequest())
	classified := requireProviderError(t, err)
	if classified.Code != "provider_response_too_large" || classified.ResultUnknown {
		t.Fatalf("classified error = %#v", classified)
	}
}

func TestProviderConfigurationRequiresCredential(t *testing.T) {
	if _, err := NewProvider(Config{Provider: ProviderOpenAI}); err == nil {
		t.Fatal("expected missing API key rejection")
	}
	if _, err := NewProvider(Config{Provider: ProviderDeepSeek, APIKey: "secret", Endpoint: "://bad"}); err == nil {
		t.Fatal("expected endpoint rejection")
	}
	if _, err := NewProvider(Config{Provider: "unsupported", APIKey: "secret"}); err == nil {
		t.Fatal("expected unsupported Provider rejection")
	}
	openAI := mustProvider(t, Config{Provider: ProviderOpenAI, APIKey: "secret"})
	deepSeek := mustProvider(t, Config{Provider: ProviderDeepSeek, APIKey: "secret"})
	if openAI.endpoint != openAIDefaultEndpoint || deepSeek.endpoint != deepSeekDefaultEndpoint {
		t.Fatalf("default endpoints = %q/%q", openAI.endpoint, deepSeek.endpoint)
	}
}

func TestProviderMetricsExposeOnlyBoundedPurposeResultsAndTokenDirections(t *testing.T) {
	successBefore := testutil.ToFloat64(providerRequestsTotal.WithLabelValues(providerPurposeGeneration, providerResultSuccess))
	inputBefore := testutil.ToFloat64(providerTokensTotal.WithLabelValues(providerPurposeGeneration, "input"))
	outputBefore := testutil.ToFloat64(providerTokensTotal.WithLabelValues(providerPurposeGeneration, "output"))

	provider := providerForResponse(t, http.StatusOK, `{
      "id":"resp_metrics","status":"completed","model":"gpt-test-2026-01-01",
      "output":[{"type":"message","content":[{"type":"output_text","text":"{\"summary\":\"ok\"}"}]}],
      "usage":{"input_tokens":13,"output_tokens":5}
    }`)
	if _, err := provider.Generate(context.Background(), validRequest()); err != nil {
		t.Fatal(err)
	}
	if delta := testutil.ToFloat64(providerRequestsTotal.WithLabelValues(providerPurposeGeneration, providerResultSuccess)) - successBefore; delta != 1 {
		t.Fatalf("success metric delta = %v", delta)
	}
	if delta := testutil.ToFloat64(providerTokensTotal.WithLabelValues(providerPurposeGeneration, "input")) - inputBefore; delta != 13 {
		t.Fatalf("input token metric delta = %v", delta)
	}
	if delta := testutil.ToFloat64(providerTokensTotal.WithLabelValues(providerPurposeGeneration, "output")) - outputBefore; delta != 5 {
		t.Fatalf("output token metric delta = %v", delta)
	}

	unknownBefore := testutil.ToFloat64(providerRequestsTotal.WithLabelValues(providerPurposeUnknown, providerResultRequestRejected))
	invalid := validRequest()
	invalid.OutputSchema.Version = "untrusted-dynamic-version"
	invalid.DataJSON = []byte("not-json")
	if _, err := provider.Generate(context.Background(), invalid); err == nil {
		t.Fatal("invalid request error = nil")
	}
	if delta := testutil.ToFloat64(providerRequestsTotal.WithLabelValues(providerPurposeUnknown, providerResultRequestRejected)) - unknownBefore; delta != 1 {
		t.Fatalf("unknown-purpose rejection metric delta = %v", delta)
	}

	resultUnknown := providerMetricResult(&appport.ProviderError{
		Kind: domainrun.FailureKindProviderTimeout, Code: "provider_timeout", ResultUnknown: true,
	})
	if resultUnknown != providerResultUnknown {
		t.Fatalf("result-unknown classification = %q", resultUnknown)
	}
}

func TestProviderFailureMetricsExposeOnlyReviewedCodes(t *testing.T) {
	known := &appport.ProviderError{
		Kind: domainrun.FailureKindProviderTransport, Code: "provider_response_read_failed", ResultUnknown: true,
	}
	knownBefore := testutil.ToFloat64(providerFailuresTotal.WithLabelValues(
		providerPurposeGeneration, providerResultUnknown, "provider_response_read_failed",
	))
	observeProviderInvocation(aiexplanation.OutputSchemaVersionV1, time.Second, nil, known)
	if delta := testutil.ToFloat64(providerFailuresTotal.WithLabelValues(
		providerPurposeGeneration, providerResultUnknown, "provider_response_read_failed",
	)) - knownBefore; delta != 1 {
		t.Fatalf("known failure metric delta = %v", delta)
	}

	unknown := &appport.ProviderError{Kind: domainrun.FailureKindProviderTransport, Code: "remote_dynamic_code"}
	otherBefore := testutil.ToFloat64(providerFailuresTotal.WithLabelValues(
		providerPurposeSemanticEvaluator, providerResultError, providerFailureCodeOther,
	))
	observeProviderInvocation(aiexplanation.SemanticEvaluationOutputSchemaVersionV1, time.Second, nil, unknown)
	if delta := testutil.ToFloat64(providerFailuresTotal.WithLabelValues(
		providerPurposeSemanticEvaluator, providerResultError, providerFailureCodeOther,
	)) - otherBefore; delta != 1 {
		t.Fatalf("unreviewed failure metric delta = %v", delta)
	}
}

func validRequest() appport.ProviderRequest {
	return validRequestForProvider(ProviderOpenAI, "gpt-test-2026-01-01")
}

func validRequestForProvider(providerName, model string) appport.ProviderRequest {
	schema := []byte(`{"type":"object","additionalProperties":false,"properties":{"summary":{"type":"string"}},"required":["summary"]}`)
	return appport.ProviderRequest{
		InvocationID: "generation-9001-run-1",
		Route: appport.ProviderRoute{
			ExecutionSpec: aiexplanation.ProviderExecutionSpec{
				Route: "ai-explanation-primary", RouteRevision: "v1", ResolvedProvider: providerName,
				ResolvedModel: model, Fingerprint: aiexplanation.NewFingerprint([]byte("route")),
			},
			Capabilities: appport.ProviderCapabilities{StructuredOutput: true}, Timeout: time.Second, MaxOutputTokens: 1200,
		},
		SystemMessage: "system constraints", TaskMessage: "task constraints", DataPreamble: "data boundary",
		DataJSON: []byte(`{"context":{"locale":"zh-CN"},"facts":{"dimensions":[]}}`),
		OutputSchema: appport.StructuredOutputSchema{
			Version: "ai-explanation-output/v1", Name: "AIExplanationOutput v1", JSON: schema,
			Fingerprint: aiexplanation.NewFingerprint(schema),
		},
	}
}

func providerForResponse(t *testing.T, status int, body string) *Provider {
	t.Helper()
	return providerForResponseWithLimit(t, status, body, 4096)
}

func providerForProviderResponse(t *testing.T, providerName string, status int, body string) *Provider {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return mustProvider(t, Config{Provider: providerName, Endpoint: server.URL, APIKey: "test-secret", HTTPClient: server.Client(), MaxResponseBytes: 4096})
}

func providerForResponseWithLimit(t *testing.T, status int, body string, limit int64) *Provider {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return mustProvider(t, Config{Provider: ProviderOpenAI, Endpoint: server.URL, APIKey: "test-secret", HTTPClient: server.Client(), MaxResponseBytes: limit})
}

func mustProvider(t *testing.T, config Config) *Provider {
	t.Helper()
	provider, err := NewProvider(config)
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

func requireProviderError(t *testing.T, err error) *appport.ProviderError {
	t.Helper()
	var classified *appport.ProviderError
	if !errors.As(err, &classified) || classified == nil {
		t.Fatalf("error = %T %v", err, err)
	}
	return classified
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (fn roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

type readFailureBody struct{}

func (readFailureBody) Read([]byte) (int, error) {
	return 0, fmt.Errorf("sensitive response read detail")
}

func (readFailureBody) Close() error { return nil }

var _ io.ReadCloser = readFailureBody{}
