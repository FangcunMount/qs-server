package options

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/pkg/app"
)

func TestAPIServerPolicyFilesPreserveDevProdValues(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", "..", ".."))
	for _, test := range []struct {
		name             string
		questionnaireTTL time.Duration
		planTTL          time.Duration
		jitter           float64
		statisticsWarmup bool
	}{
		{name: "dev", questionnaireTTL: 8 * time.Hour, planTTL: time.Hour, jitter: 0.1, statisticsWarmup: false},
		{name: "prod", questionnaireTTL: 2 * time.Hour, planTTL: 12 * time.Hour, jitter: 0.2, statisticsWarmup: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			mainPath := filepath.Join(root, "configs", "apiserver."+test.name+".yaml")
			source, err := newCachePolicySource("cache/apiserver."+test.name+".yaml", app.RuntimeConfigContext{MainConfigFile: mainPath})
			if err != nil {
				t.Fatal(err)
			}
			policy, metadata, err := source.Read(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if policy.Capabilities.Survey.Questionnaire.TTL != test.questionnaireTTL || policy.Capabilities.Plan.Detail.TTL != test.planTTL {
				t.Fatalf("capability policy = %#v", policy.Capabilities)
			}
			if policy.Defaults.TTLJitterRatio != test.jitter || policy.Governance.StatisticsWarmup.Enable != test.statisticsWarmup {
				t.Fatalf("defaults/governance = %#v / %#v", policy.Defaults, policy.Governance)
			}
			if metadata.Component != apiserverCachePolicyComponent || metadata.SchemaVersion != "1.0" || len(metadata.PolicySHA256) != 64 || !filepath.IsAbs(metadata.Path) {
				t.Fatalf("metadata = %#v", metadata)
			}
		})
	}
}

func TestAPIServerPolicyRequiresEveryCapabilityEnabled(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", "..", ".."))
	original, err := os.ReadFile(filepath.Join(root, "configs", "cache", "apiserver.dev.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	body := strings.Replace(string(original), "questionnaire: {enabled: true,", "questionnaire: {", 1)
	path := filepath.Join(t.TempDir(), "policy.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	source, err := newCachePolicySource(path, app.RuntimeConfigContext{})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = source.Read(context.Background())
	if err == nil || !strings.Contains(err.Error(), "capabilities.survey.questionnaire.enabled is required") {
		t.Fatalf("Read() error = %v", err)
	}
}

func TestAPIServerPolicyHashIncludesEnvironmentOverrides(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", "..", ".."))
	mainPath := filepath.Join(root, "configs", "apiserver.dev.yaml")
	runtime := app.RuntimeConfigContext{MainConfigFile: mainPath, EnvPrefix: "QS_APISERVER"}
	t.Setenv("QS_APISERVER_CACHE_CAPABILITIES_SURVEY_QUESTIONNAIRE_TTL", "8h")
	source, err := newCachePolicySource("cache/apiserver.dev.yaml", runtime)
	if err != nil {
		t.Fatal(err)
	}
	_, baseline, err := source.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("QS_APISERVER_CACHE_CAPABILITIES_SURVEY_QUESTIONNAIRE_TTL", "9h")
	overridden, changed, err := source.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if overridden.Capabilities.Survey.Questionnaire.TTL != 9*time.Hour {
		t.Fatalf("questionnaire TTL = %s", overridden.Capabilities.Survey.Questionnaire.TTL)
	}
	if baseline.PolicySHA256 == changed.PolicySHA256 {
		t.Fatal("environment override did not change normalized policy hash")
	}
}
