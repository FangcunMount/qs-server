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

func TestCatalogRejectsUnsupportedOrDuplicateRoutes(t *testing.T) {
	invalid := validConfig()
	invalid.Capabilities.StructuredOutput = false
	_, err := NewCatalog([]Config{invalid})
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("invalid config error = %v", err)
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
		MaxOutputTokens: 4096, ReasoningEffort: "low",
	}
}
