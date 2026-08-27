// Package openai implements the one-shot AI explanation provider port with
// OpenAI's Responses API. It deliberately exposes no SDK types outside this
// package and never logs prompts, assessment data, model output or credentials.
package openai

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
	ProviderName            = "openai"
	defaultEndpoint         = "https://api.openai.com/v1/responses"
	defaultMaxResponseBytes = int64(4 << 20)
)

var schemaNamePartPattern = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

type Config struct {
	Endpoint         string
	APIKey           string
	HTTPClient       *http.Client
	MaxResponseBytes int64
}

type Provider struct {
	endpoint         string
	apiKey           string
	httpClient       *http.Client
	maxResponseBytes int64
}

func NewProvider(config Config) (*Provider, error) {
	endpoint := strings.TrimSpace(config.Endpoint)
	if endpoint == "" {
		endpoint = defaultEndpoint
	}
	parsed, err := url.ParseRequestURI(endpoint)
	if err != nil || parsed.Scheme != "https" && parsed.Scheme != "http" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid OpenAI Responses endpoint")
	}
	if strings.TrimSpace(config.APIKey) == "" {
		return nil, fmt.Errorf("OpenAI API key is required")
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
		return nil, fmt.Errorf("OpenAI response byte limit must be positive")
	}
	return &Provider{endpoint: endpoint, apiKey: config.APIKey, httpClient: client, maxResponseBytes: limit}, nil
}

type responseRequest struct {
	Model           string            `json:"model"`
	Instructions    string            `json:"instructions"`
	Input           []inputMessage    `json:"input"`
	Text            textConfiguration `json:"text"`
	MaxOutputTokens int               `json:"max_output_tokens"`
	Store           bool              `json:"store"`
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
	Name   string          `json:"name"`
	Strict bool            `json:"strict"`
	Schema json.RawMessage `json:"schema"`
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
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
}

func (p *Provider) Generate(ctx context.Context, request appport.ProviderRequest) (response *appport.ProviderResponse, resultErr error) {
	metricStartedAt := time.Now()
	defer func() {
		observeProviderInvocation(request.OutputSchema.Version, time.Since(metricStartedAt), response, resultErr)
	}()

	if err := validateProviderRequest(request); err != nil {
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
		Text: textConfiguration{Format: responseFormat{
			Type: "json_schema", Name: normalizedSchemaName(request.OutputSchema.Name), Strict: true,
			Schema: json.RawMessage(request.OutputSchema.JSON),
		}},
		MaxOutputTokens: request.Route.MaxOutputTokens, Store: false,
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
	defer httpResponse.Body.Close()

	responseBody, tooLarge, err := readLimited(httpResponse.Body, p.maxResponseBytes)
	if err != nil {
		return nil, providerError(domainrun.FailureKindProviderTransport, "provider_response_read_failed", true, false, err)
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
	if err := validateCompletedResponse(decoded, request.Route.ExecutionSpec.ResolvedModel); err != nil {
		return nil, err
	}
	rawOutput, err := extractSingleOutput(decoded.Output)
	if err != nil {
		return nil, err
	}
	usage := responseUsage{}
	if decoded.Usage != nil {
		usage = *decoded.Usage
	}
	if usage.InputTokens < 0 || usage.OutputTokens < 0 {
		return nil, providerError(domainrun.FailureKindProviderTransport, "provider_usage_invalid", false, false, nil)
	}
	return &appport.ProviderResponse{
		RawOutput: []byte(rawOutput),
		Receipt: aiexplanation.ProviderReceipt{
			InvocationID: request.InvocationID, RequestID: decoded.ID, Provider: ProviderName,
			Model: decoded.Model, InputTokens: usage.InputTokens, OutputTokens: usage.OutputTokens, Latency: latency,
		},
	}, nil
}

func validateProviderRequest(request appport.ProviderRequest) error {
	if strings.TrimSpace(request.InvocationID) == "" {
		return fmt.Errorf("invocation ID is required")
	}
	if err := request.Route.Validate(); err != nil {
		return err
	}
	if request.Route.ExecutionSpec.ResolvedProvider != ProviderName {
		return fmt.Errorf("route provider is not OpenAI")
	}
	if strings.TrimSpace(request.SystemMessage) == "" || strings.TrimSpace(request.TaskMessage) == "" || strings.TrimSpace(request.DataPreamble) == "" {
		return fmt.Errorf("prompt messages are required")
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
	texts := make([]string, 0, 1)
	for _, item := range items {
		if item.Type != "message" {
			continue
		}
		for _, content := range item.Content {
			switch content.Type {
			case "refusal":
				return "", providerError(domainrun.FailureKindProviderRefusal, "provider_refusal", false, false, nil)
			case "output_text":
				if strings.TrimSpace(content.Text) != "" {
					texts = append(texts, content.Text)
				}
			}
		}
	}
	if len(texts) != 1 {
		return "", providerError(domainrun.FailureKindProviderTransport, "provider_output_cardinality_invalid", false, false, nil)
	}
	return texts[0], nil
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
	return &appport.ProviderError{
		Kind: kind, Code: code, SafeMessage: "AI 解读暂时不可用，请稍后再试",
		Retryable: retryable, ResultUnknown: resultUnknown, Cause: cause,
	}
}
