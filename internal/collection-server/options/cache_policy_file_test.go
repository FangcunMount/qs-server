package options

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FangcunMount/qs-server/pkg/app"
)

func TestCollectionPolicyFilesPreserveDevProdEnablement(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", "..", ".."))
	for _, test := range []struct {
		name, capability string
		wantEnabled      bool
	}{
		{name: "dev", capability: "published_model", wantEnabled: true},
		{name: "prod", capability: "published_model", wantEnabled: true},
		{name: "prod", capability: "questionnaire", wantEnabled: true},
		{name: "dev", capability: "assessment_detail", wantEnabled: true},
		{name: "dev", capability: "assessment_access", wantEnabled: true},
		{name: "prod", capability: "assessment_detail", wantEnabled: true},
		{name: "prod", capability: "assessment_access", wantEnabled: true},
	} {
		t.Run(test.name+"_"+test.capability, func(t *testing.T) {
			mainPath := filepath.Join(root, "configs", "collection-server."+test.name+".yaml")
			policy, metadata, err := loadCollectionCachePolicy(context.Background(), "cache/collection-server."+test.name+".yaml", app.RuntimeConfigContext{MainConfigFile: mainPath})
			if err != nil {
				t.Fatal(err)
			}
			var enabled bool
			switch test.capability {
			case "questionnaire":
				enabled = policy.Capabilities.Catalog.Questionnaire.Enabled
			case "published_model":
				enabled = policy.Capabilities.Catalog.PublishedModel.Enabled
			case "assessment_detail":
				enabled = policy.Capabilities.Evaluation.AssessmentDetail.Enabled
			case "assessment_access":
				enabled = policy.Capabilities.Evaluation.AssessmentAccess.Enabled
			}
			if enabled != test.wantEnabled || len(metadata.PolicySHA256) != 64 || !filepath.IsAbs(metadata.Path) {
				t.Fatalf("enabled=%v metadata=%#v", enabled, metadata)
			}
		})
	}
}

func TestCollectionPolicyRequiresEveryLeaf(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", "..", ".."))
	original, err := os.ReadFile(filepath.Join(root, "configs", "cache", "collection-server.dev.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	body := strings.Replace(string(original), ", signal_evict_enabled: true", "", 1)
	path := filepath.Join(t.TempDir(), "policy.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err = loadCollectionCachePolicy(context.Background(), path, app.RuntimeConfigContext{})
	if err == nil || !strings.Contains(err.Error(), "signal_evict_enabled is required") {
		t.Fatalf("loadCollectionCachePolicy() error = %v", err)
	}
}

func TestCollectionPolicyRejectsInvalidValuesWhileCapabilityDisabled(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", "..", ".."))
	original, err := os.ReadFile(filepath.Join(root, "configs", "cache", "collection-server.prod.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name        string
		old         string
		replacement string
		wantError   string
	}{
		{
			name:        "ttl",
			old:         "published_model: {enabled: true, ttl_seconds: 180",
			replacement: "published_model: {enabled: false, ttl_seconds: 0",
			wantError:   "published_model_cache.ttl_seconds must be greater than 0",
		},
		{
			name:        "jitter",
			old:         "published_model: {enabled: true, ttl_seconds: 180, ttl_jitter_ratio: 0.2",
			replacement: "published_model: {enabled: false, ttl_seconds: 180, ttl_jitter_ratio: 1.1",
			wantError:   "published_model_cache.ttl_jitter_ratio must be between 0 and 1",
		},
		{
			name:        "capacity",
			old:         "published_model: {enabled: true, ttl_seconds: 180, ttl_jitter_ratio: 0.2, max_entries: 64",
			replacement: "published_model: {enabled: false, ttl_seconds: 180, ttl_jitter_ratio: 0.2, max_entries: 0",
			wantError:   "published_model_cache.max_entries must be greater than 0",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := strings.Replace(string(original), test.old, test.replacement, 1)
			if body == string(original) {
				t.Fatalf("test fixture did not replace %q", test.old)
			}
			path := filepath.Join(t.TempDir(), "policy.yaml")
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}

			_, _, err := loadCollectionCachePolicy(context.Background(), path, app.RuntimeConfigContext{})
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("loadCollectionCachePolicy() error = %v, want substring %q", err, test.wantError)
			}
		})
	}
}
