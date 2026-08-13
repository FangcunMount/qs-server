package systemgovernance

import (
	"testing"

	govcomponent "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance/component"
	sharedgovernance "github.com/FangcunMount/qs-server/internal/pkg/cache/governance"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
)

func registrySnapshot(instanceID, policySHA string, enabled bool) *sharedgovernance.ComponentCacheGovernanceSnapshot {
	return &sharedgovernance.ComponentCacheGovernanceSnapshot{
		Component: "collection-server", InstanceID: instanceID, Generation: "g-" + instanceID,
		EffectiveRegistry: sharedgovernance.EffectiveRegistrySnapshot{
			SnapshotVersion: 1, CatalogVersion: "v3",
			PolicySource: &sharedgovernance.PolicySourceView{Component: "collection-server", SchemaVersion: "1.0", PolicySHA256: policySHA},
			Capabilities: []sharedgovernance.CapabilityPolicyView{{
				Capability: "catalog.questionnaire", Owner: "catalog", Kind: "cache", Layer: "L1",
				Enabled: enabled, Family: "local", MetricLabel: "questionnaire",
			}},
		},
	}
}

func TestBuildCacheRegistryViewMergesIdenticalInstances(t *testing.T) {
	view := BuildCacheRegistryView(map[string]govcomponent.CacheGovernanceResult{
		"collection-server": {
			Available: true, DiscoveredInstanceCount: 2, AvailableInstanceCount: 2,
			Instances: map[string]*sharedgovernance.ComponentCacheGovernanceSnapshot{
				"collection-a": registrySnapshot("collection-a", "same", true),
				"collection-b": registrySnapshot("collection-b", "same", true),
			},
		},
	})
	if len(view.RegistryDrift) != 0 {
		t.Fatalf("drift = %#v, want none", view.RegistryDrift)
	}
	if len(view.CapabilityRows) != 1 || !view.CapabilityRows[0].Consistent || len(view.CapabilityRows[0].InstanceIDs) != 2 {
		t.Fatalf("capability rows = %#v", view.CapabilityRows)
	}
	if view.CapabilityRows[0].Enabled == nil || !*view.CapabilityRows[0].Enabled || len(view.CapabilityRows[0].Variants) != 0 {
		t.Fatalf("merged row = %#v", view.CapabilityRows[0])
	}
}

func TestBuildCacheRegistryViewReturnsVariantsInsteadOfRepresentativeOnDrift(t *testing.T) {
	view := BuildCacheRegistryView(map[string]govcomponent.CacheGovernanceResult{
		"collection-server": {
			Available: true, DiscoveredInstanceCount: 3, AvailableInstanceCount: 2,
			TargetErrors: map[string]string{"10.0.0.3": "empty instance_id"},
			Instances: map[string]*sharedgovernance.ComponentCacheGovernanceSnapshot{
				"collection-a": registrySnapshot("collection-a", "sha-a", true),
				"collection-b": registrySnapshot("collection-b", "sha-b", false),
			},
		},
	})
	if len(view.RegistryDrift) < 2 {
		t.Fatalf("drift = %#v, want missing instance and policy drift", view.RegistryDrift)
	}
	row := view.CapabilityRows[0]
	if row.Consistent || row.Enabled != nil || row.Effective != nil || len(row.Variants) != 2 {
		t.Fatalf("drift row = %#v, must not select a representative", row)
	}
}

func TestBuildCacheRegistryViewReportsUnavailableSingleInstance(t *testing.T) {
	view := BuildCacheRegistryView(map[string]govcomponent.CacheGovernanceResult{
		"collection-server": {Available: false, Reason: "connection refused"},
	})
	if len(view.ComponentRegistries) != 1 || view.ComponentRegistries[0].Available {
		t.Fatalf("component registries = %#v", view.ComponentRegistries)
	}
	if len(view.RegistryDrift) != 1 || view.RegistryDrift[0].Kind != "missing_registry" {
		t.Fatalf("registry drift = %#v, want missing registry warning", view.RegistryDrift)
	}
}

func TestBuildCacheRegistryViewPropagatesCatalogDriftToCapabilities(t *testing.T) {
	first := registrySnapshot("collection-a", "same", true)
	second := registrySnapshot("collection-b", "same", true)
	second.EffectiveRegistry.CatalogVersion = "v4"
	view := BuildCacheRegistryView(map[string]govcomponent.CacheGovernanceResult{
		"collection-server": {
			Available: true, DiscoveredInstanceCount: 2, AvailableInstanceCount: 2,
			Instances: map[string]*sharedgovernance.ComponentCacheGovernanceSnapshot{
				"collection-a": first, "collection-b": second,
			},
		},
	})
	if len(view.CapabilityRows) != 1 || view.CapabilityRows[0].Consistent || view.CapabilityRows[0].Enabled != nil || len(view.CapabilityRows[0].Variants) != 1 {
		t.Fatalf("capability rows = %#v, catalog drift must not expose a representative", view.CapabilityRows)
	}
}

func TestBuildCacheRuntimeViewAggregatesFamilyAndInstanceHealth(t *testing.T) {
	components := map[string]ComponentCache{
		"collection-server": {Available: true, DiscoveredInstanceCount: 2, AvailableInstanceCount: 2},
	}
	rows := []CacheFamilyRow{
		{Component: "collection-server", InstanceID: "a", Family: "ops_runtime", Profile: "ops", Namespace: "ops", Available: true, Severity: SeverityHealthy,
			MetricEvidence: []MetricEvidence{{Name: "available", Window: "5m", Available: true}}},
		{Component: "collection-server", InstanceID: "b", Family: "ops_runtime", Profile: "ops", Namespace: "ops", Available: true, Degraded: true, Severity: SeverityWarning,
			MetricEvidence: []MetricEvidence{{Name: "available", Window: "5m", Available: true}}},
	}
	view := BuildCacheRuntimeView(components, rows, nil)
	if len(view.FamilyGroups) != 1 || view.FamilyGroups[0].HealthyInstanceCount != 1 || view.FamilyGroups[0].DegradedInstanceCount != 1 {
		t.Fatalf("family groups = %#v", view.FamilyGroups)
	}
	if len(view.FamilyGroups[0].MetricEvidence) != 1 {
		t.Fatalf("metric evidence = %#v, want deduplicated", view.FamilyGroups[0].MetricEvidence)
	}
	if view.Summary.Ready || view.Summary.AbnormalFamilyGroupCount != 1 {
		t.Fatalf("summary = %#v", view.Summary)
	}
}

func TestBuildCacheRuntimeViewCanonicalizesHistoricalApiserverIdentity(t *testing.T) {
	view := BuildCacheRuntimeView(
		map[string]ComponentCache{"apiserver": {Available: true, DiscoveredInstanceCount: 1}},
		[]CacheFamilyRow{{
			Component: "apiserver", InstanceID: "api-a", Family: "static_meta", Profile: "static_cache",
			Namespace: "cache:static", Available: true, Severity: SeverityHealthy,
		}},
		nil,
	)
	if len(view.FamilyGroups) != 1 || view.FamilyGroups[0].Component != "qs-apiserver" {
		t.Fatalf("family groups = %#v, want canonical qs-apiserver", view.FamilyGroups)
	}
	if len(view.InstanceRows) != 1 || view.InstanceRows[0].Component != "qs-apiserver" {
		t.Fatalf("instance rows = %#v, want canonical qs-apiserver", view.InstanceRows)
	}
}

func TestBuildCacheRuntimeViewTreatsLastErrorAsAbnormal(t *testing.T) {
	ready := &observability.RuntimeSnapshot{Summary: observability.RuntimeSummary{Ready: true}}
	view := BuildCacheRuntimeView(
		map[string]ComponentCache{"worker": {Available: true, Snapshot: ready}},
		[]CacheFamilyRow{{
			Component: "worker", InstanceID: "worker-a", Family: "ops_runtime", Profile: "ops",
			Namespace: "ops:runtime", Available: true, Severity: SeverityHealthy, LastError: "last timeout",
		}},
		nil,
	)
	if len(view.FamilyGroups) != 1 || view.FamilyGroups[0].Severity != SeverityWarning || view.Summary.AbnormalFamilyGroupCount != 1 {
		t.Fatalf("runtime view = %#v, last error must stay visible in anomaly mode", view)
	}
}

func TestBuildCacheRuntimeViewCountsAbnormalL1Capability(t *testing.T) {
	snapshot := registrySnapshot("collection-a", "same", true)
	snapshot.L1Runtime = []sharedgovernance.L1CapabilityRuntime{{
		Capability: "catalog.questionnaire", Enabled: true,
		Buckets:       []sharedgovernance.L1BucketRuntime{{Bucket: "detail", MaxEntries: 64}},
		SignalWatcher: sharedgovernance.SignalWatcherStatus{Configured: true, Status: "reconnecting", LastError: "pubsub timeout"},
	}}
	ready := &observability.RuntimeSnapshot{Summary: observability.RuntimeSummary{Ready: true}}
	view := BuildCacheRuntimeView(
		map[string]ComponentCache{"collection-server": {Available: true, Snapshot: ready}},
		nil,
		map[string]govcomponent.CacheGovernanceResult{
			"collection-server": {Available: true, Snapshot: snapshot, Instances: map[string]*sharedgovernance.ComponentCacheGovernanceSnapshot{"collection-a": snapshot}},
		},
	)
	if view.Summary.AbnormalL1CapabilityCount != 1 || view.Summary.Ready {
		t.Fatalf("runtime summary = %#v, reconnecting watcher must degrade runtime", view.Summary)
	}
}

func TestBuildCacheTopologyViewOrdersL1L2AndSource(t *testing.T) {
	enabled := true
	registry := CacheRegistryView{CapabilityRows: []RegistryCapabilityRow{
		{Component: "qs-apiserver", Capability: "survey.questionnaire", Layer: "L2", Consistent: true, Enabled: &enabled, TopologyGroup: "questionnaire", TopologyOrder: 20},
		{Component: "collection-server", Capability: "catalog.questionnaire", Layer: "L1", Consistent: true, Enabled: &enabled, TopologyGroup: "questionnaire", TopologyOrder: 10},
	}}
	runtime := CacheRuntimeView{L1CapabilityRuntime: []CacheL1CapabilityRuntimeRow{{
		Component: "collection-server", InstanceID: "collection-a", Capability: "catalog.questionnaire", Enabled: true,
	}}, FamilyGroups: []CacheFamilyGroup{{Component: "qs-apiserver", Family: "static_meta", Severity: SeverityHealthy}}}
	registry.CapabilityRows[0].Family = "static_meta"
	view := BuildCacheTopologyView(registry, runtime, nil)
	if len(view.Topologies) != 4 {
		t.Fatalf("topologies = %#v", view.Topologies)
	}
	topology := view.Topologies[0]
	if topology.TopologyGroup != "questionnaire" || len(topology.Nodes) != 2 || topology.Nodes[0].Layer != "L1" || topology.Nodes[1].Layer != "L2" {
		t.Fatalf("questionnaire topology = %#v", topology)
	}
	if len(topology.Edges) != 2 || topology.Edges[1].To != topology.Source.ID {
		t.Fatalf("edges = %#v", topology.Edges)
	}
}

func TestBuildCacheTopologyViewDoesNotChooseStructuralDriftRepresentative(t *testing.T) {
	enabled := true
	registry := CacheRegistryView{CapabilityRows: []RegistryCapabilityRow{{
		Component: "collection-server", Capability: "catalog.questionnaire", Layer: "L1", Consistent: false,
		Enabled: &enabled,
		Variants: []RegistryCapabilityVariant{
			{TopologyGroup: "questionnaire", TopologyOrder: 10, ReadModel: "questionnaire published Mongo read model"},
			{TopologyGroup: "published-model", TopologyOrder: 10, ReadModel: "published-model Mongo snapshot"},
		},
	}}}
	view := BuildCacheTopologyView(registry, CacheRuntimeView{}, nil)
	for _, topology := range view.Topologies {
		if len(topology.Nodes) != 0 {
			t.Fatalf("topology %q selected a drift representative: %#v", topology.TopologyGroup, topology.Nodes)
		}
	}
}

func TestHasWindowSamplesRejectsZeroAndUnavailableEvidence(t *testing.T) {
	zero, one := 0.0, 1.0
	if hasWindowSamples(nil) || hasWindowSamples(&MetricEvidence{Available: false, Value: &one}) || hasWindowSamples(&MetricEvidence{Available: true, Value: &zero}) {
		t.Fatal("zero or unavailable samples must not produce a hit rate")
	}
	if !hasWindowSamples(&MetricEvidence{Available: true, Value: &one}) {
		t.Fatal("positive available samples should allow a hit rate")
	}
}
