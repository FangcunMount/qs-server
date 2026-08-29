// Package provider resolves logical AI explanation routes into immutable,
// non-secret execution specifications. Credentials and endpoints stay inside
// concrete Provider adapters and are never stored in a Generation.
package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	appport "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/port"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
)

var (
	ErrInvalidConfig = errors.New("AI explanation provider route configuration is invalid")
	ErrNotFound      = errors.New("AI explanation provider route not found")
)

type Config struct {
	Route                string
	Revision             string
	Provider             string
	Model                string
	Current              bool
	Capabilities         appport.ProviderCapabilities
	StructuredOutputMode string
	Timeout              time.Duration
	MaxOutputTokens      int
	ReasoningEffort      string
}

type Catalog struct {
	current       map[string]appport.ProviderRoute
	byFingerprint map[aiexplanation.Fingerprint]appport.ProviderRoute
}

func NewCatalog(configs []Config) (*Catalog, error) {
	current := make(map[string]appport.ProviderRoute, len(configs))
	byFingerprint := make(map[aiexplanation.Fingerprint]appport.ProviderRoute, len(configs))
	configuredRoutes := make(map[string]struct{}, len(configs))
	for _, config := range configs {
		route, err := buildRoute(config)
		if err != nil {
			return nil, err
		}
		configuredRoutes[config.Route] = struct{}{}
		if _, exists := byFingerprint[route.ExecutionSpec.Fingerprint]; exists {
			return nil, fmt.Errorf("%w: duplicate frozen route %q/%q", ErrInvalidConfig, config.Route, config.Revision)
		}
		byFingerprint[route.ExecutionSpec.Fingerprint] = route
		if config.Current {
			if _, exists := current[config.Route]; exists {
				return nil, fmt.Errorf("%w: multiple current revisions for route %q", ErrInvalidConfig, config.Route)
			}
			current[config.Route] = route
		}
	}
	for route := range configuredRoutes {
		if _, ok := current[route]; !ok {
			return nil, fmt.Errorf("%w: route %q has no current revision", ErrInvalidConfig, route)
		}
	}
	return &Catalog{current: current, byFingerprint: byFingerprint}, nil
}

func (c *Catalog) ResolveProviderRoute(_ context.Context, route string) (appport.ProviderRoute, error) {
	if c == nil {
		return appport.ProviderRoute{}, ErrNotFound
	}
	resolved, ok := c.current[route]
	if !ok {
		return appport.ProviderRoute{}, ErrNotFound
	}
	return resolved, nil
}

func (c *Catalog) ResolveFrozenProviderRoute(_ context.Context, spec aiexplanation.ProviderExecutionSpec) (appport.ProviderRoute, error) {
	if c == nil || spec.Validate() != nil {
		return appport.ProviderRoute{}, ErrNotFound
	}
	resolved, ok := c.byFingerprint[spec.Fingerprint]
	if !ok || resolved.ExecutionSpec != spec {
		return appport.ProviderRoute{}, ErrNotFound
	}
	return resolved, nil
}

func buildRoute(config Config) (appport.ProviderRoute, error) {
	reasoningEffort := strings.TrimSpace(config.ReasoningEffort)
	structuredOutputMode := strings.TrimSpace(config.StructuredOutputMode)
	if structuredOutputMode == "" {
		structuredOutputMode = appport.StructuredOutputModeJSONSchema
	}
	// Keep the historical json_schema route fingerprint stable. Only a route
	// that opts into a different wire contract adds this field and therefore
	// requires a new frozen revision.
	fingerprintStructuredOutputMode := structuredOutputMode
	if fingerprintStructuredOutputMode == appport.StructuredOutputModeJSONSchema {
		fingerprintStructuredOutputMode = ""
	}
	fingerprintDocument := struct {
		Route                  string `json:"route"`
		Revision               string `json:"revision"`
		Provider               string `json:"provider"`
		Model                  string `json:"model"`
		StructuredOutput       bool   `json:"structured_output"`
		IdempotentRedispatch   bool   `json:"idempotent_redispatch"`
		RetrieveByInvocationID bool   `json:"retrieve_by_invocation_id"`
		TimeoutMilliseconds    int64  `json:"timeout_milliseconds"`
		MaxOutputTokens        int    `json:"max_output_tokens"`
		ReasoningEffort        string `json:"reasoning_effort,omitempty"`
		StructuredOutputMode   string `json:"structured_output_mode,omitempty"`
	}{
		Route: config.Route, Revision: config.Revision, Provider: config.Provider, Model: config.Model,
		StructuredOutput: config.Capabilities.StructuredOutput, IdempotentRedispatch: config.Capabilities.IdempotentRedispatch,
		RetrieveByInvocationID: config.Capabilities.RetrieveByInvocationID, TimeoutMilliseconds: config.Timeout.Milliseconds(),
		MaxOutputTokens: config.MaxOutputTokens, ReasoningEffort: reasoningEffort,
		StructuredOutputMode: fingerprintStructuredOutputMode,
	}
	raw, err := json.Marshal(fingerprintDocument)
	if err != nil {
		return appport.ProviderRoute{}, fmt.Errorf("%w: fingerprint route: %v", ErrInvalidConfig, err)
	}
	resolved := appport.ProviderRoute{
		ExecutionSpec: aiexplanation.ProviderExecutionSpec{
			Route: config.Route, RouteRevision: config.Revision, ResolvedProvider: config.Provider,
			ResolvedModel: config.Model, Fingerprint: aiexplanation.NewFingerprint(raw),
		},
		Capabilities: config.Capabilities, StructuredOutputMode: structuredOutputMode,
		Timeout: config.Timeout, MaxOutputTokens: config.MaxOutputTokens, ReasoningEffort: reasoningEffort,
	}
	if err := resolved.Validate(); err != nil {
		return appport.ProviderRoute{}, fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}
	return resolved, nil
}

// RouteNames is intended for readiness and operator diagnostics only. It does
// not expose provider credentials or endpoints.
func (c *Catalog) RouteNames() []string {
	if c == nil {
		return nil
	}
	names := make([]string, 0, len(c.current))
	for name := range c.current {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
