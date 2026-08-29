package provider

import (
	"context"
	"errors"
	"testing"
	"time"

	appport "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/port"
)

func TestCatalogResolvesFrozenNonSecretRoute(t *testing.T) {
	config := validConfig()
	catalog, err := NewCatalog([]Config{config})
	if err != nil {
		t.Fatal(err)
	}
	route, err := catalog.ResolveProviderRoute(context.Background(), config.Route)
	if err != nil {
		t.Fatal(err)
	}
	if err := route.Validate(); err != nil {
		t.Fatal(err)
	}
	if route.ExecutionSpec.Route != config.Route || route.ExecutionSpec.ResolvedProvider != config.Provider || route.ExecutionSpec.ResolvedModel != config.Model {
		t.Fatalf("resolved route = %#v", route)
	}
	if route.ReasoningEffort != config.ReasoningEffort {
		t.Fatalf("resolved reasoning effort = %q, want %q", route.ReasoningEffort, config.ReasoningEffort)
	}
	if route.StructuredOutputMode != appport.StructuredOutputModeJSONSchema {
		t.Fatalf("resolved structured output mode = %q", route.StructuredOutputMode)
	}
	if route.ExecutionSpec.Fingerprint.String() == "" || len(catalog.RouteNames()) != 1 {
		t.Fatalf("route fingerprint/names = %s/%v", route.ExecutionSpec.Fingerprint, catalog.RouteNames())
	}
	frozen, err := catalog.ResolveFrozenProviderRoute(context.Background(), route.ExecutionSpec)
	if err != nil || frozen.ExecutionSpec != route.ExecutionSpec {
		t.Fatalf("frozen route/error = %#v/%v", frozen, err)
	}
}

func TestCatalogFingerprintChangesWithExecutionSemantics(t *testing.T) {
	first, err := NewCatalog([]Config{validConfig()})
	if err != nil {
		t.Fatal(err)
	}
	changed := validConfig()
	changed.Model = "model-b"
	second, err := NewCatalog([]Config{changed})
	if err != nil {
		t.Fatal(err)
	}
	a, _ := first.ResolveProviderRoute(context.Background(), changed.Route)
	b, _ := second.ResolveProviderRoute(context.Background(), changed.Route)
	if a.ExecutionSpec.Fingerprint == b.ExecutionSpec.Fingerprint {
		t.Fatal("route fingerprint did not change with resolved model")
	}

	changedReasoning := validConfig()
	changedReasoning.ReasoningEffort = "high"
	third, err := NewCatalog([]Config{changedReasoning})
	if err != nil {
		t.Fatal(err)
	}
	c, _ := third.ResolveProviderRoute(context.Background(), changedReasoning.Route)
	if a.ExecutionSpec.Fingerprint == c.ExecutionSpec.Fingerprint {
		t.Fatal("route fingerprint did not change with reasoning effort")
	}

	changedOutputMode := validConfig()
	changedOutputMode.StructuredOutputMode = appport.StructuredOutputModeJSONObject
	fourth, err := NewCatalog([]Config{changedOutputMode})
	if err != nil {
		t.Fatal(err)
	}
	d, _ := fourth.ResolveProviderRoute(context.Background(), changedOutputMode.Route)
	if a.ExecutionSpec.Fingerprint == d.ExecutionSpec.Fingerprint {
		t.Fatal("route fingerprint did not change with structured output mode")
	}
}

func TestCatalogCanonicalizesReasoningEffortBeforeFreezingRoute(t *testing.T) {
	config := validConfig()
	config.ReasoningEffort = " low "
	catalog, err := NewCatalog([]Config{config})
	if err != nil {
		t.Fatal(err)
	}
	route, err := catalog.ResolveProviderRoute(context.Background(), config.Route)
	if err != nil {
		t.Fatal(err)
	}
	if route.ReasoningEffort != "low" {
		t.Fatalf("canonical reasoning effort = %q, want low", route.ReasoningEffort)
	}
}

func TestCatalogPreservesLegacyJSONSchemaFingerprint(t *testing.T) {
	legacy := validConfig()
	legacy.StructuredOutputMode = ""
	legacyCatalog, err := NewCatalog([]Config{legacy})
	if err != nil {
		t.Fatal(err)
	}
	explicitCatalog, err := NewCatalog([]Config{validConfig()})
	if err != nil {
		t.Fatal(err)
	}
	legacyRoute, _ := legacyCatalog.ResolveProviderRoute(context.Background(), legacy.Route)
	explicitRoute, _ := explicitCatalog.ResolveProviderRoute(context.Background(), legacy.Route)
	if legacyRoute.ExecutionSpec.Fingerprint != explicitRoute.ExecutionSpec.Fingerprint {
		t.Fatalf("legacy/explicit json_schema fingerprints differ: %s/%s", legacyRoute.ExecutionSpec.Fingerprint, explicitRoute.ExecutionSpec.Fingerprint)
	}
	if legacyRoute.StructuredOutputMode != appport.StructuredOutputModeJSONSchema {
		t.Fatalf("legacy mode = %q", legacyRoute.StructuredOutputMode)
	}
}

func TestCatalogRejectsUnsupportedOrDuplicateRoutes(t *testing.T) {
	invalid := validConfig()
	invalid.Capabilities.StructuredOutput = false
	_, err := NewCatalog([]Config{invalid})
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("invalid config error = %v", err)
	}

	invalidMode := validConfig()
	invalidMode.StructuredOutputMode = "markdown"
	_, err = NewCatalog([]Config{invalidMode})
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("invalid structured output mode error = %v", err)
	}

	config := validConfig()
	duplicateCurrent := config
	duplicateCurrent.Revision = "v2"
	_, err = NewCatalog([]Config{config, duplicateCurrent})
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("duplicate config error = %v", err)
	}

	catalog, err := NewCatalog(nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = catalog.ResolveProviderRoute(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing route error = %v", err)
	}
}

func validConfig() Config {
	return Config{
		Route: "balanced_text_v1", Revision: "v1", Provider: "provider-a", Model: "model-a", Current: true,
		Capabilities: appport.ProviderCapabilities{StructuredOutput: true}, Timeout: 45 * time.Second,
		StructuredOutputMode: appport.StructuredOutputModeJSONSchema,
		MaxOutputTokens:      4096, ReasoningEffort: "low",
	}
}
