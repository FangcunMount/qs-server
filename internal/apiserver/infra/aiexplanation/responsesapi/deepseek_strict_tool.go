package responsesapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	appport "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/port"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
	domainrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/run"
)

const deepSeekStrictToolDescription = "Return exactly one AI explanation object matching the supplied schema."

type deepSeekChatRequest struct {
	Model           string                `json:"model"`
	Messages        []deepSeekChatMessage `json:"messages"`
	Thinking        deepSeekThinking      `json:"thinking"`
	ReasoningEffort string                `json:"reasoning_effort,omitempty"`
	MaxTokens       int                   `json:"max_tokens"`
	Stream          bool                  `json:"stream"`
	Tools           []deepSeekTool        `json:"tools"`
	ToolChoice      deepSeekToolChoice    `json:"tool_choice"`
}

type deepSeekChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type deepSeekThinking struct {
	Type string `json:"type"`
}

type deepSeekTool struct {
	Type     string               `json:"type"`
	Function deepSeekToolFunction `json:"function"`
}

type deepSeekToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Strict      bool            `json:"strict"`
	Parameters  json.RawMessage `json:"parameters"`
}

type deepSeekToolChoice struct {
	Type     string                     `json:"type"`
	Function deepSeekToolChoiceFunction `json:"function"`
}

type deepSeekToolChoiceFunction struct {
	Name string `json:"name"`
}

type deepSeekChatResponse struct {
	ID      string               `json:"id"`
	Model   string               `json:"model"`
	Choices []deepSeekChatChoice `json:"choices"`
	Usage   *deepSeekChatUsage   `json:"usage"`
}

type deepSeekChatChoice struct {
	FinishReason string                   `json:"finish_reason"`
	Message      deepSeekAssistantMessage `json:"message"`
}

type deepSeekAssistantMessage struct {
	Refusal   string             `json:"refusal"`
	ToolCalls []deepSeekToolCall `json:"tool_calls"`
}

type deepSeekToolCall struct {
	Type     string                   `json:"type"`
	Function deepSeekToolCallFunction `json:"function"`
}

type deepSeekToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type deepSeekChatUsage struct {
	PromptTokens            int64                           `json:"prompt_tokens"`
	CompletionTokens        int64                           `json:"completion_tokens"`
	CompletionTokensDetails *deepSeekCompletionTokenDetails `json:"completion_tokens_details"`
}

type deepSeekCompletionTokenDetails struct {
	ReasoningTokens int64 `json:"reasoning_tokens"`
}

func (p *Provider) generateDeepSeekStrictToolCall(ctx context.Context, request appport.ProviderRequest) (*appport.ProviderResponse, error) {
	if err := p.validateProviderRequest(request); err != nil {
		return nil, providerError(domainrun.FailureKindProviderTransport, "provider_request_invalid", false, false, err)
	}
	if err := ctx.Err(); err != nil {
		return nil, providerError(classifyContextKind(err), classifyContextCode(err), false, false, err)
	}
	strictSchema, err := deepSeekCompatibleOutputSchema(request.OutputSchema.JSON)
	if err != nil {
		return nil, providerError(domainrun.FailureKindProviderTransport, "provider_request_invalid", false, false, err)
	}
	toolName := normalizedSchemaName(request.OutputSchema.Name)
	thinking, reasoningEffort := deepSeekThinkingForRoute(request.Route.ReasoningEffort)
	body, err := json.Marshal(deepSeekChatRequest{
		Model: request.Route.ExecutionSpec.ResolvedModel,
		Messages: []deepSeekChatMessage{
			{Role: "system", Content: request.SystemMessage},
			{Role: "system", Content: request.TaskMessage},
			{Role: "user", Content: request.DataPreamble + "\n\n" + string(request.DataJSON)},
		},
		Thinking: thinking, ReasoningEffort: reasoningEffort,
		MaxTokens: request.Route.MaxOutputTokens, Stream: false,
		Tools: []deepSeekTool{{Type: "function", Function: deepSeekToolFunction{
			Name: toolName, Description: deepSeekStrictToolDescription, Strict: true, Parameters: strictSchema,
		}}},
		ToolChoice: deepSeekToolChoice{Type: "function", Function: deepSeekToolChoiceFunction{Name: toolName}},
	})
	if err != nil {
		return nil, providerError(domainrun.FailureKindProviderTransport, "provider_request_encode_failed", false, false, err)
	}

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, providerError(domainrun.FailureKindProviderTransport, "provider_request_build_failed", false, false, err)
	}
	httpRequest.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "application/json")

	startedAt := time.Now()
	httpResponse, err := p.httpClient.Do(httpRequest)
	latency := time.Since(startedAt)
	if err != nil {
		kind, code := classifyTransportError(err)
		return nil, providerError(kind, code, true, true, sanitizedTransportCause(err))
	}
	defer func() { _ = httpResponse.Body.Close() }()

	responseBody, tooLarge, err := readLimited(httpResponse.Body, p.maxResponseBytes)
	if err != nil {
		return nil, classifyResponseReadError(err)
	}
	if tooLarge {
		return nil, providerError(domainrun.FailureKindProviderTransport, "provider_response_too_large", false, false, nil)
	}
	if httpResponse.StatusCode < http.StatusOK || httpResponse.StatusCode >= http.StatusMultipleChoices {
		return nil, classifyHTTPStatus(httpResponse.StatusCode)
	}

	var decoded deepSeekChatResponse
	if err := json.Unmarshal(responseBody, &decoded); err != nil {
		return nil, providerError(domainrun.FailureKindProviderTransport, "provider_response_invalid", false, false, nil)
	}
	observeDeepSeekStrictToolResponse(request.OutputSchema.Version, decoded)
	if strings.TrimSpace(decoded.ID) == "" {
		return nil, providerError(domainrun.FailureKindProviderTransport, "provider_response_id_missing", false, false, nil)
	}
	if decoded.Model != request.Route.ExecutionSpec.ResolvedModel {
		return nil, providerError(domainrun.FailureKindProviderTransport, "provider_model_mismatch", false, false, nil)
	}
	rawOutput, err := extractDeepSeekStrictToolOutput(decoded, toolName)
	if err != nil {
		return nil, err
	}
	usage := deepSeekChatUsage{}
	if decoded.Usage != nil {
		usage = *decoded.Usage
	}
	if usage.PromptTokens < 0 || usage.CompletionTokens < 0 ||
		(usage.CompletionTokensDetails != nil &&
			(usage.CompletionTokensDetails.ReasoningTokens < 0 || usage.CompletionTokensDetails.ReasoningTokens > usage.CompletionTokens)) {
		return nil, providerError(domainrun.FailureKindProviderTransport, "provider_usage_invalid", false, false, nil)
	}
	observeProviderOutputEnvelope(request.OutputSchema.Version, rawOutput)
	validationOutput, normalization := validationOutputForProviderWithKind(p.name, rawOutput)
	observeProviderOutputNormalization(request.OutputSchema.Version, normalization)
	return &appport.ProviderResponse{
		RawOutput: []byte(rawOutput), ValidationOutput: validationOutput,
		Receipt: aiexplanation.ProviderReceipt{
			InvocationID: request.InvocationID, RequestID: decoded.ID, Provider: p.name, Model: decoded.Model,
			InputTokens: usage.PromptTokens, OutputTokens: usage.CompletionTokens, Latency: latency,
		},
	}, nil
}

func extractDeepSeekStrictToolOutput(response deepSeekChatResponse, expectedTool string) (string, error) {
	if len(response.Choices) != 1 {
		return "", providerError(domainrun.FailureKindProviderTransport, "provider_output_cardinality_invalid", false, false, nil)
	}
	choice := response.Choices[0]
	if strings.TrimSpace(choice.Message.Refusal) != "" || choice.FinishReason == "content_filter" {
		return "", providerError(domainrun.FailureKindProviderRefusal, "provider_refusal", false, false, nil)
	}
	switch choice.FinishReason {
	case "tool_calls":
	case "length":
		return "", providerErrorWithSafeMessage(
			domainrun.FailureKindProviderTransport, "provider_output_token_limit",
			"Provider 输出达到 token 上限，未形成完整结构化结果", false, false, nil,
		)
	case "insufficient_system_resource":
		return "", providerError(domainrun.FailureKindProviderTransport, "provider_server_error", true, false, nil)
	default:
		return "", providerError(domainrun.FailureKindProviderTransport, "provider_output_cardinality_invalid", false, false, nil)
	}
	if len(choice.Message.ToolCalls) != 1 {
		return "", providerError(domainrun.FailureKindProviderTransport, "provider_output_cardinality_invalid", false, false, nil)
	}
	call := choice.Message.ToolCalls[0]
	if call.Type != "function" || call.Function.Name != expectedTool || strings.TrimSpace(call.Function.Arguments) == "" {
		return "", providerError(domainrun.FailureKindProviderTransport, "provider_output_cardinality_invalid", false, false, nil)
	}
	return call.Function.Arguments, nil
}

func deepSeekThinkingForRoute(effort string) (deepSeekThinking, string) {
	switch strings.TrimSpace(effort) {
	case "", "none":
		return deepSeekThinking{Type: "disabled"}, ""
	case "minimal", "low":
		return deepSeekThinking{Type: "enabled"}, "low"
	case "medium", "high", "xhigh":
		return deepSeekThinking{Type: "enabled"}, "high"
	case "max":
		return deepSeekThinking{Type: "enabled"}, "max"
	default:
		return deepSeekThinking{Type: "disabled"}, ""
	}
}

// deepSeekCompatibleOutputSchema projects the canonical output Schema onto the
// JSON-Schema subset DeepSeek documents for strict structured output. Both the
// Responses json_schema path and the beta strict-tool path use this wire shape.
// The canonical server-side validator still enforces every omitted lexical and
// cross-field constraint after the Provider returns.
func deepSeekCompatibleOutputSchema(raw []byte) (json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var root map[string]any
	if err := decoder.Decode(&root); err != nil || root == nil {
		return nil, fmt.Errorf("decode canonical output schema")
	}
	definitions, _ := root["$defs"].(map[string]any)
	transformed, err := transformDeepSeekCompatibleSchema(root, definitions)
	if err != nil {
		return nil, err
	}
	value, err := json.Marshal(transformed)
	if err != nil {
		return nil, fmt.Errorf("encode DeepSeek compatible output schema: %w", err)
	}
	return value, nil
}

func transformDeepSeekCompatibleSchema(value any, definitions map[string]any) (map[string]any, error) {
	node, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("DeepSeek compatible output schema node is not an object")
	}
	if ref, ok := node["$ref"].(string); ok {
		const prefix = "#/$defs/"
		if !strings.HasPrefix(ref, prefix) {
			return nil, fmt.Errorf("DeepSeek compatible output schema ref is unsupported")
		}
		resolved, exists := definitions[strings.TrimPrefix(ref, prefix)]
		if !exists {
			return nil, fmt.Errorf("DeepSeek compatible output schema ref is unresolved")
		}
		return transformDeepSeekCompatibleSchema(resolved, definitions)
	}

	result := make(map[string]any)
	if description, ok := node["description"].(string); ok && strings.TrimSpace(description) != "" {
		result["description"] = description
	}
	if schemaType, ok := node["type"].(string); ok {
		result["type"] = schemaType
	}
	if enum, ok := node["enum"].([]any); ok {
		result["enum"] = enum
		if _, typed := result["type"]; !typed {
			if inferred := inferredEnumType(enum); inferred != "" {
				result["type"] = inferred
			}
		}
	}
	if constant, ok := node["const"]; ok {
		result["enum"] = []any{constant}
		if inferred := inferredValueType(constant); inferred != "" {
			result["type"] = inferred
		}
	}
	if properties, ok := node["properties"].(map[string]any); ok {
		keys := make([]string, 0, len(properties))
		transformed := make(map[string]any, len(properties))
		for name, property := range properties {
			child, err := transformDeepSeekCompatibleSchema(property, definitions)
			if err != nil {
				return nil, fmt.Errorf("DeepSeek compatible output property %q: %w", name, err)
			}
			keys = append(keys, name)
			transformed[name] = child
		}
		sort.Strings(keys)
		result["type"] = "object"
		result["properties"] = transformed
		result["required"] = keys
		result["additionalProperties"] = false
	}
	if items, exists := node["items"]; exists {
		transformed, err := transformDeepSeekCompatibleSchema(items, definitions)
		if err != nil {
			return nil, err
		}
		result["type"] = "array"
		result["items"] = transformed
	}
	if variants, ok := node["anyOf"].([]any); ok {
		transformed := make([]any, 0, len(variants))
		for _, variant := range variants {
			child, err := transformDeepSeekCompatibleSchema(variant, definitions)
			if err != nil {
				return nil, err
			}
			transformed = append(transformed, child)
		}
		result["anyOf"] = transformed
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("DeepSeek compatible output schema node has no supported constraints")
	}
	return result, nil
}

func inferredEnumType(values []any) string {
	if len(values) == 0 {
		return ""
	}
	typeName := inferredValueType(values[0])
	for _, value := range values[1:] {
		if inferredValueType(value) != typeName {
			return ""
		}
	}
	return typeName
}

func inferredValueType(value any) string {
	switch value.(type) {
	case string:
		return "string"
	case bool:
		return "boolean"
	case json.Number:
		return "number"
	default:
		return ""
	}
}
