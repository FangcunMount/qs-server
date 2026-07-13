package options

import (
	"strings"
	"testing"
	"time"

	"github.com/spf13/pflag"
)

func TestCacheRawSettingsAcceptCanonicalModuleCapabilities(t *testing.T) {
	settings := map[string]any{"cache": map[string]any{
		"defaults": map[string]any{"compress_payload": false, "static": map[string]any{"negative_ttl": "5m"}},
		"capabilities": map[string]any{
			"survey":        map[string]any{"questionnaire": map[string]any{"enabled": true, "ttl": "2h"}},
			"modelcatalog":  map[string]any{"published_model": map[string]any{"enabled": true, "ttl": "2h"}},
			"evaluation":    map[string]any{"assessment_detail": map[string]any{"enabled": true}, "assessment_list": map[string]any{"enabled": true}},
			"actor":         map[string]any{"testee": map[string]any{"enabled": true}},
			"plan":          map[string]any{"detail": map[string]any{"enabled": true}},
			"statistics":    map[string]any{"query": map[string]any{"enabled": true}},
			"report_status": map[string]any{"ttl_seconds": 172800},
		},
	}}
	if err := NewOptions().ValidateRawSettings(settings); err != nil {
		t.Fatalf("ValidateRawSettings() error = %v", err)
	}
}

func TestCapabilityFlagsCoverPolicyOverrides(t *testing.T) {
	cache := NewCacheOptions()
	flags := pflag.NewFlagSet("cache", pflag.ContinueOnError)
	cache.AddFlags(flags)
	if err := flags.Parse([]string{
		"--cache.capabilities.evaluation.assessment_detail.enabled=false",
		"--cache.capabilities.evaluation.assessment_detail.negative_ttl=45s",
		"--cache.capabilities.evaluation.assessment_detail.ttl_jitter_ratio=0.3",
		"--cache.capabilities.evaluation.assessment_detail.compress=true",
		"--cache.capabilities.evaluation.assessment_detail.singleflight=false",
		"--cache.capabilities.evaluation.assessment_detail.negative=true",
	}); err != nil {
		t.Fatal(err)
	}
	got := cache.Capabilities.Evaluation.AssessmentDetail
	if got.Enabled || got.NegativeTTL != 45*time.Second || got.TTLJitterRatio != 0.3 {
		t.Fatalf("capability flags = %#v", got)
	}
	if got.Compress == nil || !*got.Compress || got.Singleflight == nil || *got.Singleflight || got.Negative == nil || !*got.Negative {
		t.Fatalf("capability switch flags = %#v", got)
	}
}

func TestCacheRawSettingsRejectLegacySchema(t *testing.T) {
	for name, settings := range map[string]map[string]any{
		"disable switch": {"cache": map[string]any{"capabilities": map[string]any{"disable_evaluation_cache": true}}},
		"ttl matrix":     {"cache": map[string]any{"defaults": map[string]any{"ttl": map[string]any{"questionnaire": "2h"}}}},
	} {
		t.Run(name, func(t *testing.T) {
			err := NewOptions().ValidateRawSettings(settings)
			if err == nil || !strings.Contains(err.Error(), "unknown configuration field") {
				t.Fatalf("ValidateRawSettings() error = %v, want unknown field", err)
			}
		})
	}
}
