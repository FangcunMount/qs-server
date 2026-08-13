package options

import (
	"strings"
	"testing"
	"time"

	"github.com/spf13/pflag"
)

func TestCacheRawSettingsAcceptPolicyReferenceAndRuntimeState(t *testing.T) {
	settings := map[string]any{"cache": map[string]any{
		"policy_file": "cache/apiserver.dev.yaml",
	}, "runtime_state": map[string]any{"report_status": map[string]any{"ttl_seconds": 172800}}}
	if err := NewOptions().ValidateRawSettings(settings); err != nil {
		t.Fatalf("ValidateRawSettings() error = %v", err)
	}
}

func TestCacheRawSettingsRejectRetiredAssessmentListCapability(t *testing.T) {
	err := NewOptions().ValidateRawSettings(map[string]any{"cache": map[string]any{
		"capabilities": map[string]any{"evaluation": map[string]any{
			"assessment_list": map[string]any{"enabled": true},
		}},
	}})
	if err == nil || !strings.Contains(err.Error(), "unknown configuration field") {
		t.Fatalf("ValidateRawSettings() error = %v, want unknown field", err)
	}
}

func TestCacheRawSettingsRejectInlinePolicy(t *testing.T) {
	err := NewOptions().ValidateRawSettings(map[string]any{"cache": map[string]any{
		"defaults": map[string]any{"compress_payload": false},
	}})
	if err == nil || !strings.Contains(err.Error(), "unknown configuration field cache.defaults") {
		t.Fatalf("ValidateRawSettings() error = %v, want inline policy rejection", err)
	}
}

func TestCapabilityFlagsDoNotExposeRetiredAssessmentList(t *testing.T) {
	flags := pflag.NewFlagSet("cache", pflag.ContinueOnError)
	NewCacheOptions().AddFlags(flags)
	if flags.Lookup("cache.capabilities.evaluation.assessment_list.enabled") != nil {
		t.Fatal("retired assessment-list CLI flag is still registered")
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

func TestIAMRawSettingsRejectRemovedFetchStrategies(t *testing.T) {
	for _, key := range []string{"fetch-strategies", "fetch_strategies"} {
		t.Run(key, func(t *testing.T) {
			err := NewOptions().ValidateRawSettings(map[string]any{
				"iam": map[string]any{"jwks": map[string]any{key: []string{"http", "grpc", "cache"}}},
			})
			if err == nil || !strings.Contains(err.Error(), "iam.jwks.fetch-strategies has been removed") {
				t.Fatalf("expected removed fetch-strategies error, got %v", err)
			}
		})
	}
}
