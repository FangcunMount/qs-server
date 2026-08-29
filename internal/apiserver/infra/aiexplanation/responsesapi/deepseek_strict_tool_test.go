package responsesapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	interpretationschema "github.com/FangcunMount/qs-server/api/schema/interpretation"
	appport "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/port"
)

func TestDeepSeekStrictToolProviderForcesOneSchemaConstrainedFunctionCall(t *testing.T) {
	var captured deepSeekChatRequest
	var capturedRaw map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/beta/chat/completions" {
			t.Errorf("request path = %q", request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer test-secret" {
			t.Errorf("authorization header = %q", request.Header.Get("Authorization"))
		}
		if err := json.NewDecoder(request.Body).Decode(&capturedRaw); err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(capturedRaw)
		_ = json.Unmarshal(raw, &captured)
		_, _ = w.Write([]byte(`{
          "id":"chatcmpl-strict-1","model":"deepseek-v4-pro",
          "choices":[{"finish_reason":"tool_calls","message":{"tool_calls":[{
            "type":"function","function":{"name":"AIExplanationOutput_v1","arguments":"{\"summary\":\"ok\"}"}
          }]}}],
          "usage":{"prompt_tokens":34,"completion_tokens":13,"completion_tokens_details":{"reasoning_tokens":3}}
        }`))
	}))
	defer server.Close()

	provider := mustProvider(t, Config{
		Provider: ProviderDeepSeek, Protocol: ProviderProtocolDeepSeekStrictToolCall,
		Endpoint: server.URL + "/beta/chat/completions",
		APIKey:   "test-secret", HTTPClient: server.Client(),
	})
	request := validDeepSeekStrictRequest()
	request.Route.ReasoningEffort = "low"
	response, err := provider.Generate(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(response.RawOutput); got != `{"summary":"ok"}` {
		t.Fatalf("raw strict-tool arguments = %q", got)
	}
	if response.Receipt.RequestID != "chatcmpl-strict-1" || response.Receipt.Provider != ProviderDeepSeek ||
		response.Receipt.Model != "deepseek-v4-pro" || response.Receipt.InputTokens != 34 || response.Receipt.OutputTokens != 13 {
		t.Fatalf("strict-tool receipt = %#v", response.Receipt)
	}
	if len(captured.Messages) != 3 || captured.Messages[0].Role != "system" ||
		captured.Messages[1].Role != "system" || captured.Messages[2].Role != "user" {
		t.Fatalf("strict-tool messages = %#v", captured.Messages)
	}
	if strings.Contains(mustJSON(t, captured), request.InvocationID) {
		t.Fatal("internal invocation identity leaked into strict-tool body")
	}
	if captured.Thinking.Type != "enabled" || captured.ReasoningEffort != "low" || captured.Stream {
		t.Fatalf("strict-tool thinking/stream = %#v/%q/%v", captured.Thinking, captured.ReasoningEffort, captured.Stream)
	}
	if capturedRaw["stream"] != false || capturedRaw["reasoning_effort"] != "low" {
		t.Fatalf("strict-tool top-level controls = %#v", capturedRaw)
	}
	if _, nested := capturedRaw["thinking"].(map[string]any)["reasoning_effort"]; nested {
		t.Fatalf("reasoning_effort must not be nested under thinking: %#v", capturedRaw["thinking"])
	}
	if len(captured.Tools) != 1 || !captured.Tools[0].Function.Strict ||
		captured.Tools[0].Function.Name != "AIExplanationOutput_v1" ||
		captured.ToolChoice.Function.Name != "AIExplanationOutput_v1" {
		t.Fatalf("strict-tool constraint = %#v / %#v", captured.Tools, captured.ToolChoice)
	}
	var parameters map[string]any
	if err := json.Unmarshal(captured.Tools[0].Function.Parameters, &parameters); err != nil {
		t.Fatal(err)
	}
	if parameters["additionalProperties"] != false {
		t.Fatalf("strict-tool parameters = %#v", parameters)
	}
}

func TestDeepSeekStrictToolSchemaSupportsBothFrozenProductionContracts(t *testing.T) {
	for _, testCase := range []struct {
		name string
		raw  []byte
	}{
		{name: "generation", raw: interpretationschema.AIExplanationOutputV1()},
		{name: "semantic evaluator", raw: interpretationschema.AIExplanationSemanticEvaluationOutputV1()},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			encoded, err := deepSeekStrictToolSchema(testCase.raw)
			if err != nil {
				t.Fatal(err)
			}
			var schema map[string]any
			if err := json.Unmarshal(encoded, &schema); err != nil {
				t.Fatal(err)
			}
			assertDeepSeekStrictSchemaSubset(t, schema, "$")
		})
	}
}

func TestDeepSeekStrictToolProviderClassifiesTokenLimitWithoutLeakingArguments(t *testing.T) {
	provider := providerForDeepSeekStrictResponse(t, http.StatusOK, `{
      "id":"chatcmpl-strict-length","model":"deepseek-v4-pro",
      "choices":[{"finish_reason":"length","message":{"tool_calls":[{
        "type":"function","function":{"name":"AIExplanationOutput_v1","arguments":"sensitive partial output"}
      }]}}]
    }`)
	_, err := provider.Generate(context.Background(), validDeepSeekStrictRequest())
	classified := requireProviderError(t, err)
	if classified.Code != "provider_output_token_limit" || classified.Retryable || classified.ResultUnknown {
		t.Fatalf("strict-tool token-limit classification = %#v", classified)
	}
	if strings.Contains(err.Error(), "sensitive partial output") {
		t.Fatal("partial strict-tool arguments leaked through error")
	}
}

func TestDeepSeekThinkingMappingIsBounded(t *testing.T) {
	for _, testCase := range []struct {
		input      string
		wantType   string
		wantEffort string
	}{
		{input: "none", wantType: "disabled"},
		{input: "minimal", wantType: "enabled", wantEffort: "low"},
		{input: "low", wantType: "enabled", wantEffort: "low"},
		{input: "medium", wantType: "enabled", wantEffort: "high"},
		{input: "xhigh", wantType: "enabled", wantEffort: "high"},
		{input: "max", wantType: "enabled", wantEffort: "max"},
	} {
		got, effort := deepSeekThinkingForRoute(testCase.input)
		if got.Type != testCase.wantType || effort != testCase.wantEffort {
			t.Fatalf("thinking %q = %#v/%q", testCase.input, got, effort)
		}
	}
}

func assertDeepSeekStrictSchemaSubset(t *testing.T, node map[string]any, path string) {
	t.Helper()
	for _, forbidden := range []string{"$defs", "$ref", "const", "allOf", "if", "then", "pattern", "minItems", "maxItems", "minLength", "maxLength", "minimum", "maximum", "uniqueItems"} {
		if _, exists := node[forbidden]; exists {
			t.Fatalf("strict-tool schema contains unsupported %q at %s", forbidden, path)
		}
	}
	if properties, ok := node["properties"].(map[string]any); ok {
		if node["additionalProperties"] != false {
			t.Fatalf("object at %s does not reject additional properties", path)
		}
		required, ok := node["required"].([]any)
		if !ok || len(required) != len(properties) {
			t.Fatalf("object at %s has required=%#v properties=%d", path, node["required"], len(properties))
		}
		for name, rawChild := range properties {
			child, ok := rawChild.(map[string]any)
			if !ok {
				t.Fatalf("property %s.%s is not an object", path, name)
			}
			assertDeepSeekStrictSchemaSubset(t, child, path+"."+name)
		}
	}
	if rawItems, exists := node["items"]; exists {
		items, ok := rawItems.(map[string]any)
		if !ok {
			t.Fatalf("array items at %s are not an object", path)
		}
		assertDeepSeekStrictSchemaSubset(t, items, path+"[]")
	}
	if variants, ok := node["anyOf"].([]any); ok {
		for index, rawVariant := range variants {
			variant, ok := rawVariant.(map[string]any)
			if !ok {
				t.Fatalf("anyOf variant at %s[%d] is not an object", path, index)
			}
			assertDeepSeekStrictSchemaSubset(t, variant, path)
		}
	}
}

func providerForDeepSeekStrictResponse(t *testing.T, status int, body string) *Provider {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return mustProvider(t, Config{
		Provider: ProviderDeepSeek, Protocol: ProviderProtocolDeepSeekStrictToolCall,
		Endpoint: server.URL, APIKey: "test-secret", HTTPClient: server.Client(), MaxResponseBytes: 4096,
	})
}

func validDeepSeekStrictRequest() appport.ProviderRequest {
	request := validRequestForProvider(ProviderDeepSeek, "deepseek-v4-pro")
	request.Route.Protocol = ProviderProtocolDeepSeekStrictToolCall
	return request
}
