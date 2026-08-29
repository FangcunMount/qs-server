// Package port defines provider-neutral application ports for AI explanation.
// Infrastructure implementations may embed Prompt packages, resolve logical
// provider routes and atomically stage requested events without exposing SDKs
// or credentials to the application layer.
package port

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
	domainartifact "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/artifact"
	domaingeneration "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/generation"
	domainoutput "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/output"
	domainprofile "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/profile"
	domainrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/run"
)

type PromptPackage struct {
	Ref                 aiexplanation.PromptRef
	SystemMessage       string
	TaskTemplate        string
	DataPreamble        string
	AllowedPlaceholders []string
}

func (p PromptPackage) Validate() error {
	if err := p.Ref.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(p.SystemMessage) == "" || strings.TrimSpace(p.TaskTemplate) == "" || strings.TrimSpace(p.DataPreamble) == "" {
		return fmt.Errorf("AI explanation Prompt package messages are required")
	}
	seen := make(map[string]struct{}, len(p.AllowedPlaceholders))
	for _, placeholder := range p.AllowedPlaceholders {
		if !strings.HasPrefix(placeholder, "{{") || !strings.HasSuffix(placeholder, "}}") {
			return fmt.Errorf("AI explanation Prompt placeholder %q is invalid", placeholder)
		}
		if _, exists := seen[placeholder]; exists {
			return fmt.Errorf("AI explanation Prompt placeholder %q is duplicated", placeholder)
		}
		seen[placeholder] = struct{}{}
	}
	return nil
}

type PromptPackageResolver interface {
	ResolvePromptPackage(ctx context.Context, templateID, version string) (PromptPackage, error)
}

type ProviderCapabilities struct {
	StructuredOutput       bool
	IdempotentRedispatch   bool
	RetrieveByInvocationID bool
}

const (
	// StructuredOutputModeJSONSchema asks the Provider to constrain output with
	// the complete output schema. It remains the legacy default for frozen
	// routes created before the mode became explicit.
	StructuredOutputModeJSONSchema = "json_schema"
	// StructuredOutputModeJSONObject asks the Provider to return one valid JSON
	// object. The application still validates that object against the complete
	// versioned output schema before accepting it.
	StructuredOutputModeJSONObject = "json_object"
)

type ProviderRoute struct {
	ExecutionSpec        aiexplanation.ProviderExecutionSpec
	Capabilities         ProviderCapabilities
	StructuredOutputMode string
	Timeout              time.Duration
	MaxOutputTokens      int
	ReasoningEffort      string
}

func (r ProviderRoute) Validate() error {
	if err := r.ExecutionSpec.Validate(); err != nil {
		return err
	}
	if !r.Capabilities.StructuredOutput {
		return fmt.Errorf("AI explanation provider route must support structured output")
	}
	if _, err := normalizeStructuredOutputMode(r.StructuredOutputMode); err != nil {
		return err
	}
	if r.Timeout <= 0 || r.MaxOutputTokens <= 0 {
		return fmt.Errorf("AI explanation provider timeout and output token limit are required")
	}
	if !validReasoningEffort(r.ReasoningEffort) {
		return fmt.Errorf("AI explanation provider reasoning effort is invalid")
	}
	return nil
}

// EffectiveStructuredOutputMode preserves json_schema as the behavior of
// legacy route fixtures and frozen route revisions where the mode was absent.
func (r ProviderRoute) EffectiveStructuredOutputMode() string {
	mode, _ := normalizeStructuredOutputMode(r.StructuredOutputMode)
	return mode
}

func normalizeStructuredOutputMode(value string) (string, error) {
	switch strings.TrimSpace(value) {
	case "", StructuredOutputModeJSONSchema:
		return StructuredOutputModeJSONSchema, nil
	case StructuredOutputModeJSONObject:
		return StructuredOutputModeJSONObject, nil
	default:
		return "", fmt.Errorf("AI explanation provider structured output mode is invalid")
	}
}

func validReasoningEffort(value string) bool {
	switch strings.TrimSpace(value) {
	case "", "none", "minimal", "low", "medium", "high", "xhigh", "max":
		return true
	default:
		return false
	}
}

type ProviderRouteResolver interface {
	ResolveProviderRoute(ctx context.Context, route string) (ProviderRoute, error)
}

type StructuredOutputSchema struct {
	Version     string
	Name        string
	JSON        []byte
	Fingerprint aiexplanation.Fingerprint
}

func (s StructuredOutputSchema) Validate() error {
	if strings.TrimSpace(s.Version) == "" || strings.TrimSpace(s.Name) == "" || len(s.JSON) == 0 {
		return fmt.Errorf("AI explanation structured output schema is required")
	}
	if err := s.Fingerprint.Validate(); err != nil {
		return err
	}
	if s.Fingerprint != aiexplanation.NewFingerprint(s.JSON) {
		return fmt.Errorf("AI explanation structured output schema fingerprint mismatch")
	}
	return nil
}

type OutputSchemaResolver interface {
	ResolveOutputSchema(ctx context.Context, version string) (StructuredOutputSchema, error)
}

// FrozenProviderRouteResolver resolves the exact execution semantics already
// frozen on a Generation. It must never silently substitute a newer route.
type FrozenProviderRouteResolver interface {
	ResolveFrozenProviderRoute(ctx context.Context, spec aiexplanation.ProviderExecutionSpec) (ProviderRoute, error)
}

type ProviderRequest struct {
	InvocationID  string
	Route         ProviderRoute
	SystemMessage string
	TaskMessage   string
	DataPreamble  string
	DataJSON      []byte
	OutputSchema  StructuredOutputSchema
}

type ProviderResponse struct {
	RawOutput []byte
	Receipt   aiexplanation.ProviderReceipt
}

type Provider interface {
	Generate(ctx context.Context, request ProviderRequest) (*ProviderResponse, error)
}

type ProviderError struct {
	Kind          domainrun.FailureKind
	Code          string
	SafeMessage   string
	Retryable     bool
	ResultUnknown bool
	Cause         error
}

func (e *ProviderError) Error() string {
	if e == nil {
		return ""
	}
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Code, e.Cause)
	}
	return e.Code
}

func (e *ProviderError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

type SafetyRequest struct {
	Content domainoutput.Content
	Input   []byte
	Policy  domainprofile.SafetyPolicy
}

type SafetyResult struct {
	Allowed          bool
	ValidatorVersion string
	FailureCode      string
	SafeMessage      string
}

type SafetyEvaluator interface {
	Evaluate(ctx context.Context, request SafetyRequest) (SafetyResult, error)
}

// RequestCommitter atomically creates a pending Generation and stages its
// requested event. Implementations must surface generation.ErrAlreadyExists
// when another request already owns the semantic key.
type RequestCommitter interface {
	CommitRequested(ctx context.Context, generation *domaingeneration.AIExplanationGeneration) error
}

// ExecutionCommitter owns the transactional state boundaries around a single
// provider invocation. SaveDispatching must durably complete before Generate
// is called. CommitStart acquires the exact distributed participant execution
// slot; success and failure atomically commit Run + Generation and release that
// slot. Success additionally inserts the immutable Artifact and terminal event.
type ExecutionCommitter interface {
	CommitStart(ctx context.Context, generation *domaingeneration.AIExplanationGeneration, run *domainrun.AIExplanationRun, expectedGenerationVersion uint64) error
	SaveDispatching(ctx context.Context, run *domainrun.AIExplanationRun) error
	CommitSuccess(ctx context.Context, generation *domaingeneration.AIExplanationGeneration, run *domainrun.AIExplanationRun, artifact *domainartifact.AIExplanationArtifact, expectedGenerationVersion uint64) error
	CommitFailure(ctx context.Context, generation *domaingeneration.AIExplanationGeneration, run *domainrun.AIExplanationRun, expectedGenerationVersion uint64) error
}

type RetryAuthorizationCommitter interface {
	CommitRetryAuthorization(context.Context, *domaingeneration.AIExplanationGeneration, *domainrun.AIExplanationRun, domainrun.RetryAuthorization) (*domainrun.AIExplanationRun, bool, error)
}
