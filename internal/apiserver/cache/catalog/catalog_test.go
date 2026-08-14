package cachepolicy

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/model"
	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
)

func TestPolicyCatalogMergesFamilyAndCapabilityDefaults(t *testing.T) {
	catalog := NewPolicyCatalog(sharedcache.Policy{}, map[cachemodel.Family]sharedcache.Policy{
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
	if !binding.Policy.Singleflight.Enabled(false) {
		t.Fatal("expected family default to override spec default")
	}
	catalog = NewPolicyCatalog(
		sharedcache.Policy{Compress: PolicySwitchDisabled},
		map[cachemodel.Family]sharedcache.Policy{cachemodel.FamilyQuery: {Compress: PolicySwitchEnabled}},
		map[sharedcache.Capability]Binding{CapabilityStatisticsQuery: {Enabled: true, Policy: sharedcache.Policy{Compress: PolicySwitchDisabled}}},
	)
	binding, _ = catalog.Resolve(CapabilityStatisticsQuery)
	if binding.Policy.Compress.Enabled(true) {
		t.Fatal("expected capability override to win")
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
	registry := NewEffectiveRegistry(NewPolicyCatalog(sharedcache.Policy{}, nil, nil))
	entries := registry.All()
	if len(entries) != 8 {
		t.Fatalf("registry entries = %d, want 8", len(entries))
	}
	access, ok := registry.Resolve(CapabilityEvaluationAssessmentAccess)
	if !ok || access.Layer != sharedcache.LayerL2 || access.MetricLabel != "assessment_access" {
		t.Fatalf("assessment access entry = %#v", access)
	}
	if _, ok := registry.Resolve(sharedcache.Capability("evaluation.assessment_list")); ok {
		t.Fatal("retired evaluation.assessment_list capability is still registered")
	}
	questionnaire, ok := registry.Resolve(CapabilitySurveyQuestionnaire)
	if !ok || questionnaire.Owner != "survey" || questionnaire.Source != "cache.capabilities.survey.questionnaire" || questionnaire.MetricLabel != "questionnaire" || questionnaire.CatalogVersion != "v3" {
		t.Fatalf("questionnaire entry = %#v", questionnaire)
	}
	if questionnaire.TopologyGroup != "questionnaire" || questionnaire.TopologyOrder != 20 || questionnaire.ReadModel == "" {
		t.Fatalf("questionnaire topology = %#v", questionnaire)
	}
	reportStatus, ok := registry.Resolve(CapabilityReportStatus)
	if !ok || reportStatus.Kind != sharedcache.KindOperationalState || reportStatus.Layer != sharedcache.LayerRuntime {
		t.Fatalf("report status entry = %#v", reportStatus)
	}
	if reportStatus.Policy.TTL != 48*time.Hour {
		t.Fatalf("report status TTL = %v, want 48h", reportStatus.Policy.TTL)
	}
}

func TestFixedTopologySpecsMatchCacheLayers(t *testing.T) {
	want := map[sharedcache.Capability]struct {
		group string
		layer sharedcache.Layer
	}{
		CapabilitySurveyQuestionnaire:        {group: "questionnaire", layer: sharedcache.LayerL2},
		CapabilityModelCatalogPublished:      {group: "published-model", layer: sharedcache.LayerL1L2},
		CapabilityEvaluationAssessmentDetail: {group: "assessment-detail", layer: sharedcache.LayerL2},
		CapabilityEvaluationAssessmentAccess: {group: "assessment-access", layer: sharedcache.LayerL2},
	}
	for capability, expected := range want {
		spec, ok := Lookup(capability)
		if !ok || spec.TopologyGroup != expected.group || spec.TopologyOrder != 20 || spec.ReadModel == "" || spec.Layer != expected.layer {
			t.Fatalf("cache topology spec %q = %#v, found=%v", capability, spec, ok)
		}
	}
}
