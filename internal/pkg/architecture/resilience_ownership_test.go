package architecture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResilienceSwaggerSchemaNamesRemainCompatible(t *testing.T) {
	root := repoRoot(t)
	artifacts := []string{
		"internal/apiserver/docs/swagger.json",
		"api/rest/apiserver.yaml",
	}
	schemas := []string{
		"resilienceplane.RuntimeSnapshot",
		"resilienceplane.RuntimeSummary",
		"resilienceplane.CapabilitySnapshot",
		"resilienceplane.QueueSnapshot",
		"resilienceplane.BackpressureSnapshot",
	}
	for _, rel := range artifacts {
		content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		for _, schema := range schemas {
			if !strings.Contains(string(content), schema) {
				t.Fatalf("%s no longer exposes compatible schema %s", rel, schema)
			}
		}
	}
}

func TestSharedResiliencePackagesAreNested(t *testing.T) {
	root := repoRoot(t)
	for _, rel := range []string{
		"internal/pkg/resilienceplane",
		"internal/pkg/resiliencecontrol",
		"internal/pkg/ratelimit",
		"internal/pkg/backpressure",
		"internal/pkg/admission",
		"internal/pkg/locklease",
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); !os.IsNotExist(err) {
			t.Fatalf("legacy resilience package path still exists: %s", rel)
		}
	}

	legacyImports := []string{
		"internal/pkg/resilienceplane",
		"internal/pkg/resiliencecontrol",
		"internal/pkg/ratelimit",
		"internal/pkg/backpressure",
		"internal/pkg/admission",
		"internal/pkg/locklease",
	}
	walkGoFiles(t, filepath.Join(root, "internal"), func(path, text string) {
		for _, legacy := range legacyImports {
			if strings.Contains(text, `"github.com/FangcunMount/qs-server/`+legacy) {
				t.Fatalf("%s imports legacy resilience path %s", mustRel(t, root, path), legacy)
			}
		}
	})
}

func TestResilienceCoreDoesNotImportChildPackages(t *testing.T) {
	root := repoRoot(t)
	paths, err := filepath.Glob(filepath.Join(root, "internal", "pkg", "resilience", "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(content), `"github.com/FangcunMount/qs-server/internal/pkg/resilience/`) {
			t.Fatalf("%s imports a resilience child package; the core must remain dependency-free", mustRel(t, root, path))
		}
	}
}

func TestTransportsDoNotConstructRateLimiters(t *testing.T) {
	root := repoRoot(t)
	for _, rel := range []string{
		"internal/apiserver/transport",
		"internal/collection-server/transport",
	} {
		walkGoFiles(t, filepath.Join(root, filepath.FromSlash(rel)), func(path, text string) {
			if strings.HasSuffix(path, "_test.go") {
				return
			}
			for _, constructor := range []string{"ratelimit.NewLocalLimiter(", "ratelimit.NewKeyedLocalLimiter(", "ratelimit.NewDistributedLimiter("} {
				if strings.Contains(text, constructor) {
					t.Fatalf("%s constructs a rate limiter; obtain a shared budget from the process resilience subsystem", mustRel(t, root, path))
				}
			}
		})
	}
}

func TestCollectionContainerDoesNotConstructConcurrencyGates(t *testing.T) {
	root := repoRoot(t)
	walkGoFiles(t, filepath.Join(root, "internal", "collection-server", "container"), func(path, text string) {
		if strings.HasSuffix(path, "_test.go") {
			return
		}
		if strings.Contains(text, "concurrency.NewGate(") {
			t.Fatalf("%s constructs a concurrency gate; gates belong to collection-server/resilience/subsystem", mustRel(t, root, path))
		}
	})
}

func TestBusinessLayersDoNotImportResilienceRedisAdapter(t *testing.T) {
	root := repoRoot(t)
	for _, rel := range []string{
		"internal/apiserver/application",
		"internal/collection-server/application",
		"internal/worker/handlers",
	} {
		walkGoFiles(t, filepath.Join(root, filepath.FromSlash(rel)), func(path, text string) {
			if strings.Contains(text, "internal/pkg/resilience/control/redisadapter") {
				t.Fatalf("%s imports the Redis control adapter; business code must depend on a narrow resilience port", mustRel(t, root, path))
			}
		})
	}
}
