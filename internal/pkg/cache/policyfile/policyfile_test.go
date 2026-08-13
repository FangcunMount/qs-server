package policyfile

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	genericoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
	"github.com/FangcunMount/qs-server/pkg/app"
)

type testPolicy struct {
	Capabilities struct {
		Demo struct {
			Enabled    bool `mapstructure:"enabled" json:"enabled"`
			TTLSeconds int  `mapstructure:"ttl_seconds" json:"ttl_seconds"`
		} `mapstructure:"demo" json:"demo"`
	} `mapstructure:"capabilities" json:"capabilities"`
}

func TestLoadResolvesRelativeToMainConfigAndAppliesPrecedence(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "apiserver.yaml")
	policyPath := filepath.Join(dir, "cache", "policy.yaml")
	if err := os.MkdirAll(filepath.Dir(policyPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mainPath, []byte("cache: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(policyPath, []byte("version: \"1.0\"\ncomponent: test-component\ncapabilities:\n  demo:\n    enabled: true\n    ttl_seconds: 60\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TEST_POLICY_CACHE_CAPABILITIES_DEMO_TTL_SECONDS", "120")
	originalWorkingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalWorkingDirectory) })

	document, err := Load(context.Background(), LoadOptions{
		ConfiguredPath: "cache/policy.yaml", ExpectedComponent: "test-component",
		RequiredRoots: []string{"capabilities"}, Schema: testPolicySchema(), OverridePrefix: "cache",
		Runtime: app.RuntimeConfigContext{MainConfigFile: mainPath, EnvPrefix: "TEST_POLICY", ExplicitFlags: map[string]string{
			"cache.capabilities.demo.ttl_seconds": "180",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if document.Path() != policyPath {
		t.Fatalf("Path() = %q, want %q", document.Path(), policyPath)
	}
	var decoded testPolicy
	if err := document.Unmarshal(&decoded); err != nil {
		t.Fatal(err)
	}
	if !decoded.Capabilities.Demo.Enabled || decoded.Capabilities.Demo.TTLSeconds != 180 {
		t.Fatalf("decoded policy = %#v", decoded)
	}
}

func TestLoadRejectsEnvelopeAndUnknownFields(t *testing.T) {
	for name, test := range map[string]struct {
		body string
		want string
	}{
		"version":   {"version: \"2.0\"\ncomponent: test-component\ncapabilities: {demo: {enabled: true, ttl_seconds: 1}}\n", "version"},
		"component": {"version: \"1.0\"\ncomponent: other\ncapabilities: {demo: {enabled: true, ttl_seconds: 1}}\n", "component"},
		"unknown":   {"version: \"1.0\"\ncomponent: test-component\ncapabilities: {demo: {enabled: true, ttl_seconds: 1, secret: no}}\n", "unknown configuration field"},
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "policy.yaml")
			if err := os.WriteFile(path, []byte(test.body), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := Load(context.Background(), LoadOptions{ConfiguredPath: path, ExpectedComponent: "test-component", RequiredRoots: []string{"capabilities"}, Schema: testPolicySchema()})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Load() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestSourceHashesNormalizedEffectivePolicy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "policy.yaml")
	if err := os.WriteFile(path, []byte("version: \"1.0\"\ncomponent: test-component\ncapabilities: {demo: {enabled: true, ttl_seconds: 60}}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	document, err := Load(context.Background(), LoadOptions{ConfiguredPath: path, ExpectedComponent: "test-component", RequiredRoots: []string{"capabilities"}, Schema: testPolicySchema()})
	if err != nil {
		t.Fatal(err)
	}
	one, err := document.Source(map[string]any{"enabled": true, "ttl": 60})
	if err != nil {
		t.Fatal(err)
	}
	two, _ := document.Source(struct {
		Enabled bool `json:"enabled"`
		TTL     int  `json:"ttl"`
	}{Enabled: true, TTL: 60})
	changed, _ := document.Source(map[string]any{"enabled": true, "ttl": 61})
	if one.PolicySHA256 != two.PolicySHA256 {
		t.Fatalf("equivalent normalized policies hashed differently: %s != %s", one.PolicySHA256, two.PolicySHA256)
	}
	if one.PolicySHA256 == changed.PolicySHA256 {
		t.Fatal("effective policy change did not change hash")
	}
}

func testPolicySchema() genericoptions.FieldSchema {
	leaf := genericoptions.FieldSchema(nil)
	return genericoptions.FieldSchema{"capabilities": {"demo": {"enabled": leaf, "ttl_seconds": leaf}}}
}
