package cachepolicy

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/model"
	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
)

func TestPolicyCatalogMergesFamilyAndCapabilityDefaults(t *testing.T) {
	catalog := NewPolicyCatalog(map[cachemodel.Family]sharedcache.Policy{
		cachemodel.FamilyQuery: {Compress: PolicySwitchEnabled, Singleflight: PolicySwitchEnabled},
	}, map[sharedcache.Capability]Binding{
		CapabilityStatisticsQuery: {Enabled: true},
	})
	binding, ok := catalog.Resolve(CapabilityStatisticsQuery)
	if !ok || !binding.Enabled {
		t.Fatal("statistics binding missing or disabled")
	}
	if !binding.Policy.Compress.Enabled(false) {
		t.Fatal("expected family compression")
	}
	if binding.Policy.Singleflight.Enabled(true) {
		t.Fatal("expected capability default to disable singleflight")
	}
}

func TestSpecsHaveUniqueModuleOwnedIdentity(t *testing.T) {
	seen := map[sharedcache.Capability]bool{}
	for _, spec := range Specs() {
		if seen[spec.ID] {
			t.Fatalf("duplicate capability %q", spec.ID)
		}
		seen[spec.ID] = true
		if spec.Owner == "" || spec.ConfigPath == "" || spec.MetricLabel == "" {
			t.Fatalf("incomplete spec: %#v", spec)
		}
	}
	if seen[sharedcache.Capability("catalog.scale")] {
		t.Fatal("catalog.scale must not be registered")
	}
}

func TestEffectiveRegistryUsesCanonicalIDsAndLegacyMetricLabels(t *testing.T) {
	registry := NewEffectiveRegistry(NewPolicyCatalog(nil, nil))
	entries := registry.Snapshot()
	if len(entries) != 8 {
		t.Fatalf("registry entries = %d, want 8", len(entries))
	}
	first := entries[0]
	if first.Capability != CapabilitySurveyQuestionnaire || first.Owner != "survey" || first.Source != "cache.capabilities.survey.questionnaire" || first.MetricLabel != "questionnaire" || first.Version != "v2" {
		t.Fatalf("first entry = %#v", first)
	}
	last := entries[len(entries)-1]
	if last.Capability != CapabilityReportStatus || last.Kind != sharedcache.KindOperationalState || last.Layer != sharedcache.LayerRuntime {
		t.Fatalf("report status entry = %#v", last)
	}
	if last.Policy.TTL != 48*time.Hour {
		t.Fatalf("report status TTL = %v, want 48h", last.Policy.TTL)
	}
}
