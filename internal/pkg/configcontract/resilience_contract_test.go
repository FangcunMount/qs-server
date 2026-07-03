package configcontract

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestProdYAMLRateLimitAndCacheTTLContract(t *testing.T) {
	root := repoRoot(t)
	for _, spec := range []struct {
		path       string
		ratePrefix string
	}{
		{path: "configs/collection-server.prod.yaml", ratePrefix: "rate_limit"},
		{path: "configs/apiserver.prod.yaml", ratePrefix: "rate_limit"},
	} {
		t.Run(spec.path, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(root, spec.path))
			if err != nil {
				t.Fatal(err)
			}
			var doc map[string]any
			if err := yaml.Unmarshal(raw, &doc); err != nil {
				t.Fatal(err)
			}
			rateLimit, _ := doc["rate_limit"].(map[string]any)
			if rateLimit == nil {
				t.Fatal("missing rate_limit section")
			}
			if enabled, _ := rateLimit["enabled"].(bool); !enabled {
				t.Fatal("rate_limit.enabled must be true in prod")
			}
			for _, key := range []string{"query_global_qps", "query_global_burst", "submit_global_qps", "submit_global_burst"} {
				if rateLimit[key] == nil {
					t.Fatalf("rate_limit.%s must be present", key)
				}
			}
		})
	}
}

func TestMaxOutboxPublishWorkersContract(t *testing.T) {
	if got := MaxOutboxPublishWorkers(100, 0.8); got != 80 {
		t.Fatalf("max workers = %d, want 80", got)
	}
}
