package openai

import (
	"context"
	"encoding/json"
	"errors"
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

	provider := mustProvider(t, Config{Endpoint: server.URL, APIKey: "test-secret", HTTPClient: server.Client()})
	request := validRequest()
	response, err := provider.Generate(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if string(response.RawOutput) != `{"summary":"ok"}` {
		t.Fatalf("raw output = %s", response.RawOutput)
	}
	if response.Receipt.InvocationID != request.InvocationID || response.Receipt.RequestID != "resp_123" || response.Receipt.Provider != ProviderName || response.Receipt.Model != request.Route.ExecutionSpec.ResolvedModel {
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

func TestProviderMarksPostDispatchTimeoutResultUnknown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
	}))
	defer server.Close()
	provider := mustProvider(t, Config{Endpoint: server.URL, APIKey: "test-secret", HTTPClient: server.Client()})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := provider.Generate(ctx, validRequest())
	classified := requireProviderError(t, err)
	if classified.Kind != domainrun.FailureKindProviderTimeout || !classified.Retryable || !classified.ResultUnknown {
		t.Fatalf("classified error = %#v", classified)
	}
}

func TestProviderDoesNotMarkPreDispatchCancellationUnknown(t *testing.T) {
	provider := mustProvider(t, Config{Endpoint: "https://example.invalid/v1/responses", APIKey: "test-secret"})
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

func TestProviderRejectsOversizedResponse(t *testing.T) {
	provider := providerForResponseWithLimit(t, http.StatusOK, strings.Repeat("x", 65), 64)
	_, err := provider.Generate(context.Background(), validRequest())
	classified := requireProviderError(t, err)
	if classified.Code != "provider_response_too_large" || classified.ResultUnknown {
		t.Fatalf("classified error = %#v", classified)
	}
}

func TestProviderConfigurationRequiresCredential(t *testing.T) {
	if _, err := NewProvider(Config{}); err == nil {
		t.Fatal("expected missing API key rejection")
	}
	if _, err := NewProvider(Config{APIKey: "secret", Endpoint: "://bad"}); err == nil {
		t.Fatal("expected endpoint rejection")
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

func validRequest() appport.ProviderRequest {
	schema := []byte(`{"type":"object","additionalProperties":false,"properties":{"summary":{"type":"string"}},"required":["summary"]}`)
	return appport.ProviderRequest{
		InvocationID: "generation-9001-run-1",
		Route: appport.ProviderRoute{
			ExecutionSpec: aiexplanation.ProviderExecutionSpec{
				Route: "ai-explanation-primary", RouteRevision: "v1", ResolvedProvider: ProviderName,
				ResolvedModel: "gpt-test-2026-01-01", Fingerprint: aiexplanation.NewFingerprint([]byte("route")),
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

func providerForResponseWithLimit(t *testing.T, status int, body string, limit int64) *Provider {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return mustProvider(t, Config{Endpoint: server.URL, APIKey: "test-secret", HTTPClient: server.Client(), MaxResponseBytes: limit})
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
