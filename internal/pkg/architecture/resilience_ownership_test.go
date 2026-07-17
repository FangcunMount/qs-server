package architecture

import (
	"path/filepath"
	"strings"
	"testing"
)

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
			if strings.Contains(text, "internal/pkg/resiliencecontrol/redisadapter") {
				t.Fatalf("%s imports the Redis control adapter; business code must depend on a narrow resilience port", mustRel(t, root, path))
			}
		})
	}
}
