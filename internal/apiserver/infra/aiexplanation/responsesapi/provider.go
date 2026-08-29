// Package responsesapi implements the one-shot AI explanation provider port
// for explicitly supported Responses API providers. It deliberately exposes no
// SDK types outside this package and never logs prompts, assessment data, model
// output or credentials.
package responsesapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	appport "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/port"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
	domainrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/run"
)

const (
	ProviderOpenAI          = "openai"
	ProviderDeepSeek        = "deepseek"
	openAIDefaultEndpoint   = "https://api.openai.com/v1/responses"
	deepSeekDefaultEndpoint = "https://api.deepseek.com/responses"
	defaultMaxResponseBytes = int64(4 << 20)
)

var schemaNamePartPattern = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

type Config struct {
	Provider         string
	Endpoint         string
	APIKey           string
	HTTPClient       *http.Client
	MaxResponseBytes int64
}

type Provider struct {
	name             string
	endpoint         string
	apiKey           string
	httpClient       *http.Client
	maxResponseBytes int64
	strictJSONSchema bool
	explicitStore    bool
}

func NewProvider(config Config) (*Provider, error) {
	name := strings.ToLower(strings.TrimSpace(config.Provider))
	profile, ok := profileForProvider(name)
	if !ok {
		return nil, fmt.Errorf("unsupported AI explanation Responses API provider %q", name)
	}
	endpoint := strings.TrimSpace(config.Endpoint)
	if endpoint == "" {
		endpoint = profile.defaultEndpoint
	}
	parsed, err := url.ParseRequestURI(endpoint)
	if err != nil || parsed.Scheme != "https" && parsed.Scheme != "http" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid %s Responses endpoint", name)
	}
	if strings.TrimSpace(config.APIKey) == "" {
		return nil, fmt.Errorf("%s API key is required", name)
	}
	client := config.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	limit := config.MaxResponseBytes
	if limit == 0 {
		limit = defaultMaxResponseBytes
	}
	if limit < 1 {
		return nil, fmt.Errorf("responses API response byte limit must be positive")
	}
	return &Provider{
		name: name, endpoint: endpoint, apiKey: config.APIKey, httpClient: client,
		maxResponseBytes: limit, strictJSONSchema: profile.strictJSONSchema, explicitStore: profile.explicitStore,
	}, nil
}

type providerProfile struct {
	defaultEndpoint  string
	strictJSONSchema bool
	explicitStore    bool
}

func profileForProvider(name string) (providerProfile, bool) {
	switch name {
	case ProviderOpenAI:
		return providerProfile{defaultEndpoint: openAIDefaultEndpoint, strictJSONSchema: true, explicitStore: true}, true
	case ProviderDeepSeek:
		return providerProfile{defaultEndpoint: deepSeekDefaultEndpoint, strictJSONSchema: false}, true
	default:
		return providerProfile{}, false
	}
}

type responseRequest struct {
	Model           string            `json:"model"`
	Instructions    string            `json:"instructions"`
	Input           []inputMessage    `json:"input"`
	Text            textConfiguration `json:"text"`
	Reasoning       *reasoningConfig  `json:"reasoning,omitempty"`
	MaxOutputTokens int               `json:"max_output_tokens"`
	Store           *bool             `json:"store,omitempty"`
}

type reasoningConfig struct {
	Effort string `json:"effort"`
}

type inputMessage struct {
	Role    string             `json:"role"`
	Content []inputTextContent `json:"content"`
}

type inputTextContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type textConfiguration struct {
	Format responseFormat `json:"format"`
}

type responseFormat struct {
	Type   string          `json:"type"`
	Name   string          `json:"name,omitempty"`
	Strict *bool           `json:"strict,omitempty"`
	Schema json.RawMessage `json:"schema,omitempty"`
}

type responsesAPIResponse struct {
	ID                string             `json:"id"`
	Status            string             `json:"status"`
	Model             string             `json:"model"`
	Error             *responseError     `json:"error"`
	IncompleteDetails *incompleteDetails `json:"incomplete_details"`
	Output            []outputItem       `json:"output"`
	Usage             *responseUsage     `json:"usage"`
}

type responseError struct {
	Code string `json:"code"`
}

type incompleteDetails struct {
	Reason string `json:"reason"`
}

type outputItem struct {
	Type    string          `json:"type"`
	Content []outputContent `json:"content"`
}

type outputContent struct {
	Type    string `json:"type"`
	Text    string `json:"text"`
	Refusal string `json:"refusal"`
}

type responseUsage struct {
	InputTokens         int64                `json:"input_tokens"`
	OutputTokens        int64                `json:"output_tokens"`
	OutputTokensDetails *outputTokensDetails `json:"output_tokens_details"`
}

type outputTokensDetails struct {
	ReasoningTokens int64 `json:"reasoning_tokens"`
}

func (p *Provider) Generate(ctx context.Context, request appport.ProviderRequest) (response *appport.ProviderResponse, resultErr error) {
	metricStartedAt := time.Now()
	defer func() {
		observeProviderInvocation(request.OutputSchema.Version, time.Since(metricStartedAt), response, resultErr)
	}()

	if err := p.validateProviderRequest(request); err != nil {
		return nil, providerError(domainrun.FailureKindProviderTransport, "provider_request_invalid", false, false, err)
	}
	if err := ctx.Err(); err != nil {
		return nil, providerError(classifyContextKind(err), classifyContextCode(err), false, false, err)
	}
	body, err := json.Marshal(responseRequest{
		Model: request.Route.ExecutionSpec.ResolvedModel, Instructions: request.SystemMessage,
		Input: []inputMessage{
			{Role: "developer", Content: []inputTextContent{{Type: "input_text", Text: request.TaskMessage}}},
			{Role: "user", Content: []inputTextContent{{Type: "input_text", Text: request.DataPreamble + "\n\n" + string(request.DataJSON)}}},
		},
		Text:            textConfiguration{Format: p.responseFormat(request)},
		Reasoning:       reasoningForRoute(request.Route.ReasoningEffort),
		MaxOutputTokens: request.Route.MaxOutputTokens, Store: p.storeFlag(),
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

	var decoded responsesAPIResponse
	if err := json.Unmarshal(responseBody, &decoded); err != nil {
		return nil, providerError(domainrun.FailureKindProviderTransport, "provider_response_invalid", false, false, nil)
	}
	observeDecodedProviderResponse(request.OutputSchema.Version, decoded)
	if err := validateCompletedResponse(decoded, request.Route.ExecutionSpec.ResolvedModel); err != nil {
		return nil, err
	}
	rawOutput, err := extractSingleOutput(decoded.Output)
	if err != nil {
		return nil, err
	}
	observeProviderOutputEnvelope(request.OutputSchema.Version, rawOutput)
	validationOutput := validationOutputForProvider(p.name, rawOutput)
	observeProviderOutputNormalization(request.OutputSchema.Version, len(validationOutput) > 0)
	usage := responseUsage{}
	if decoded.Usage != nil {
		usage = *decoded.Usage
	}
	if usage.InputTokens < 0 || usage.OutputTokens < 0 ||
		(usage.OutputTokensDetails != nil && (usage.OutputTokensDetails.ReasoningTokens < 0 || usage.OutputTokensDetails.ReasoningTokens > usage.OutputTokens)) {
		return nil, providerError(domainrun.FailureKindProviderTransport, "provider_usage_invalid", false, false, nil)
	}
	return &appport.ProviderResponse{
		RawOutput: append([]byte(nil), rawOutput...), ValidationOutput: validationOutput,
		Receipt: aiexplanation.ProviderReceipt{
			InvocationID: request.InvocationID, RequestID: decoded.ID, Provider: p.name,
			Model: decoded.Model, InputTokens: usage.InputTokens, OutputTokens: usage.OutputTokens, Latency: latency,
		},
	}, nil
}

// validationOutputForProvider compensates only for a reviewed Provider wire
// quirk. DeepSeek can return an otherwise valid structured response inside one
// Markdown JSON fence even when json_schema was requested. Preserve the exact
// Provider text in RawOutput, but expose the enclosed object to the existing
// strict Schema/reference/Profile validators. Ambiguous or malformed fences
// deliberately remain unchanged and therefore fail closed.
func validationOutputForProvider(provider, output string) []byte {
	if provider != ProviderDeepSeek {
		return nil
	}
	trimmed := strings.TrimSpace(output)
	openingEnd := strings.IndexByte(trimmed, '\n')
	if openingEnd < 0 || !strings.EqualFold(strings.TrimSpace(trimmed[:openingEnd]), "```json") {
		return nil
	}
	closingStart := strings.LastIndex(trimmed, "\n```")
	if closingStart <= openingEnd || strings.TrimSpace(trimmed[closingStart+1:]) != "```" {
		return nil
	}
	candidate := strings.TrimSpace(trimmed[openingEnd+1 : closingStart])
	if candidate == "" || candidate[0] != '{' || !json.Valid([]byte(candidate)) {
		return nil
	}
	return []byte(candidate)
}

func (p *Provider) responseFormat(request appport.ProviderRequest) responseFormat {
	if request.Route.EffectiveStructuredOutputMode() == appport.StructuredOutputModeJSONObject {
		return responseFormat{Type: appport.StructuredOutputModeJSONObject}
	}
	return responseFormat{
		Type: appport.StructuredOutputModeJSONSchema, Name: normalizedSchemaName(request.OutputSchema.Name),
		Strict: p.strictSchemaFlag(), Schema: json.RawMessage(request.OutputSchema.JSON),
	}
}

func reasoningForRoute(effort string) *reasoningConfig {
	effort = strings.TrimSpace(effort)
	if effort == "" {
		return nil
	}
	return &reasoningConfig{Effort: effort}
}

func (p *Provider) validateProviderRequest(request appport.ProviderRequest) error {
	if strings.TrimSpace(request.InvocationID) == "" {
		return fmt.Errorf("invocation ID is required")
	}
	if err := request.Route.Validate(); err != nil {
		return err
	}
	if request.Route.ExecutionSpec.ResolvedProvider != p.name {
		return fmt.Errorf("route provider does not match configured Responses API provider")
	}
	if strings.TrimSpace(request.SystemMessage) == "" || strings.TrimSpace(request.TaskMessage) == "" || strings.TrimSpace(request.DataPreamble) == "" {
		return fmt.Errorf("prompt messages are required")
	}
	if request.Route.EffectiveStructuredOutputMode() == appport.StructuredOutputModeJSONObject &&
		!strings.Contains(strings.ToLower(request.SystemMessage+"\n"+request.TaskMessage), "json") {
		return fmt.Errorf("json_object mode requires an explicit JSON instruction")
	}
	if !json.Valid(request.DataJSON) {
		return fmt.Errorf("provider data must be valid JSON")
	}
	var dataObject map[string]json.RawMessage
	if err := json.Unmarshal(request.DataJSON, &dataObject); err != nil || dataObject == nil {
		return fmt.Errorf("provider data must be a JSON object")
	}
	return request.OutputSchema.Validate()
}

func (p *Provider) strictSchemaFlag() *bool {
	if !p.strictJSONSchema {
		return nil
	}
	value := true
	return &value
}

func (p *Provider) storeFlag() *bool {
	if !p.explicitStore {
		return nil
	}
	value := false
	return &value
}

func normalizedSchemaName(name string) string {
	value := strings.Trim(schemaNamePartPattern.ReplaceAllString(strings.TrimSpace(name), "_"), "_")
	if value == "" {
		return "ai_explanation_output"
	}
	if len(value) > 64 {
		return value[:64]
	}
	return value
}

func validateCompletedResponse(response responsesAPIResponse, expectedModel string) error {
	if response.Status != "completed" {
		return classifyResponseStatus(response)
	}
	if strings.TrimSpace(response.ID) == "" {
		return providerError(domainrun.FailureKindProviderTransport, "provider_response_id_missing", false, false, nil)
	}
	if response.Model != expectedModel {
		return providerError(domainrun.FailureKindProviderTransport, "provider_model_mismatch", false, false, nil)
	}
	return nil
}

func classifyResponseStatus(response responsesAPIResponse) error {
	switch response.Status {
	case "failed":
		code := "provider_response_failed"
		if response.Error != nil && isRefusalCode(response.Error.Code) {
			return providerError(domainrun.FailureKindProviderRefusal, "provider_refusal", false, false, nil)
		}
		if response.Error != nil && isRetryableProviderCode(response.Error.Code) {
			return providerError(domainrun.FailureKindProviderTransport, code, true, false, nil)
		}
		return providerError(domainrun.FailureKindProviderTransport, code, false, false, nil)
	case "incomplete":
		if response.IncompleteDetails != nil {
			switch strings.ToLower(strings.TrimSpace(response.IncompleteDetails.Reason)) {
			case "max_output_tokens":
				return providerErrorWithSafeMessage(
					domainrun.FailureKindProviderTransport,
					"provider_output_token_limit",
					"Provider 输出达到 token 上限，未形成完整结构化结果",
					false,
					false,
					nil,
				)
			case "content_filter":
				return providerError(domainrun.FailureKindProviderRefusal, "provider_refusal", false, false, nil)
			}
		}
		return providerError(domainrun.FailureKindProviderTransport, "provider_response_incomplete", false, false, nil)
	case "cancelled":
		return providerError(domainrun.FailureKindProviderTransport, "provider_response_cancelled", true, false, nil)
	case "queued", "in_progress":
		return providerError(domainrun.FailureKindProviderTransport, "provider_response_not_terminal", true, false, nil)
	default:
		return providerError(domainrun.FailureKindProviderTransport, "provider_response_status_invalid", false, false, nil)
	}
}

func extractSingleOutput(items []outputItem) (string, error) {
	messageCount := 0
	outputTextCount := 0
	var output strings.Builder
	for _, item := range items {
		if item.Type != "message" {
			continue
		}
		messageCount++
		for _, content := range item.Content {
			switch content.Type {
			case "refusal":
				return "", providerError(domainrun.FailureKindProviderRefusal, "provider_refusal", false, false, nil)
			case "output_text":
				outputTextCount++
				if content.Text != "" {
					output.WriteString(content.Text)
				}
			}
		}
	}
	switch {
	case messageCount == 0:
		return "", providerErrorWithSafeMessage(
			domainrun.FailureKindProviderTransport,
			"provider_output_cardinality_invalid",
			"Provider 未返回结构化消息",
			false,
			false,
			nil,
		)
	case messageCount > 1:
		return "", providerErrorWithSafeMessage(
			domainrun.FailureKindProviderTransport,
			"provider_output_cardinality_invalid",
			"Provider 返回了多个结构化消息",
			false,
			false,
			nil,
		)
	case outputTextCount == 0:
		return "", providerErrorWithSafeMessage(
			domainrun.FailureKindProviderTransport,
			"provider_output_cardinality_invalid",
			"Provider 结构化消息缺少输出文本",
			false,
			false,
			nil,
		)
	case strings.TrimSpace(output.String()) == "":
		return "", providerErrorWithSafeMessage(
			domainrun.FailureKindProviderTransport,
			"provider_output_cardinality_invalid",
			"Provider 结构化消息的输出文本为空",
			false,
			false,
			nil,
		)
	}
	return output.String(), nil
}

func classifyHTTPStatus(statusCode int) error {
	switch {
	case statusCode == http.StatusTooManyRequests:
		return providerError(domainrun.FailureKindProviderRateLimit, "provider_rate_limited", true, false, nil)
	case statusCode == http.StatusRequestTimeout || statusCode == http.StatusGatewayTimeout:
		return providerError(domainrun.FailureKindProviderTimeout, "provider_timeout", true, false, nil)
	case statusCode >= http.StatusInternalServerError:
		return providerError(domainrun.FailureKindProviderTransport, "provider_server_error", true, false, nil)
	case statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden:
		return providerError(domainrun.FailureKindProviderTransport, "provider_authentication_failed", false, false, nil)
	default:
		return providerError(domainrun.FailureKindProviderTransport, "provider_request_rejected", false, false, nil)
	}
}

func classifyTransportError(err error) (domainrun.FailureKind, string) {
	if errors.Is(err, context.DeadlineExceeded) {
		return domainrun.FailureKindProviderTimeout, "provider_timeout"
	}
	if errors.Is(err, context.Canceled) {
		return domainrun.FailureKindProviderTransport, "provider_request_cancelled"
	}
	return domainrun.FailureKindProviderTransport, "provider_transport_error"
}

// classifyResponseReadError is deliberately different from a pre-dispatch or
// connect failure. Once response headers have been received, the provider may
// already have completed and charged the request even when the body cannot be
// read locally. The result must therefore remain unknown and must not be
// blindly replayed.
func classifyResponseReadError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return providerError(domainrun.FailureKindProviderTimeout, "provider_timeout", true, true, context.DeadlineExceeded)
	}
	if errors.Is(err, context.Canceled) {
		return providerError(domainrun.FailureKindProviderTransport, "provider_response_cancelled", false, true, context.Canceled)
	}
	return providerError(domainrun.FailureKindProviderTransport, "provider_response_read_failed", true, true, nil)
}

func classifyContextKind(err error) domainrun.FailureKind {
	if errors.Is(err, context.DeadlineExceeded) {
		return domainrun.FailureKindProviderTimeout
	}
	return domainrun.FailureKindProviderTransport
}

func classifyContextCode(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "provider_timeout"
	}
	return "provider_request_cancelled"
}

func sanitizedTransportCause(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	return nil
}

func isRefusalCode(code string) bool {
	switch strings.ToLower(strings.TrimSpace(code)) {
	case "content_filter", "safety", "policy_violation":
		return true
	default:
		return false
	}
}

func isRetryableProviderCode(code string) bool {
	switch strings.ToLower(strings.TrimSpace(code)) {
	case "server_error", "rate_limit_exceeded", "temporarily_unavailable":
		return true
	default:
		return false
	}
}

func readLimited(reader io.Reader, limit int64) ([]byte, bool, error) {
	limited := &io.LimitedReader{R: reader, N: limit + 1}
	value, err := io.ReadAll(limited)
	if err != nil {
		return nil, false, err
	}
	if int64(len(value)) > limit {
		return nil, true, nil
	}
	return value, false, nil
}

func providerError(kind domainrun.FailureKind, code string, retryable, resultUnknown bool, cause error) *appport.ProviderError {
	return providerErrorWithSafeMessage(kind, code, "AI 解读暂时不可用，请稍后再试", retryable, resultUnknown, cause)
}

func providerErrorWithSafeMessage(
	kind domainrun.FailureKind,
	code string,
	safeMessage string,
	retryable bool,
	resultUnknown bool,
	cause error,
) *appport.ProviderError {
	return &appport.ProviderError{
		Kind: kind, Code: code, SafeMessage: safeMessage,
		Retryable: retryable, ResultUnknown: resultUnknown, Cause: cause,
	}
}
