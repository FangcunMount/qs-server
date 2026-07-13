package cachepolicy

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/model"
)

func TestPolicyCatalogMergesFamilyDefaultIntoObjectPolicy(t *testing.T) {
	t.Parallel()

	catalog := NewPolicyCatalog(
		map[cachemodel.Family]CachePolicy{
			cachemodel.FamilyQuery: {
				Compress:     PolicySwitchEnabled,
				Singleflight: PolicySwitchDisabled,
			},
		},
		map[CachePolicyKey]CachePolicy{
			PolicyStatsQuery: {},
		},
	)

	policy := catalog.Policy(PolicyStatsQuery)
	if !policy.Compress.Enabled(false) {
		t.Fatal("期望继承 family 级压缩配置")
	}
	if policy.Singleflight.Enabled(true) {
		t.Fatal("期望继承 family 级 singleflight 禁用配置")
	}
}

func TestFamilyForReturnsExpectedRedisFamily(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		key  CachePolicyKey
		want cachemodel.Family
	}{
		{name: "static scale", key: PolicyScale, want: cachemodel.FamilyStatic},
		{name: "static published model", key: PolicyPublishedModel, want: cachemodel.FamilyStatic},
		{name: "object plan", key: PolicyPlan, want: cachemodel.FamilyObject},
		{name: "query stats", key: PolicyStatsQuery, want: cachemodel.FamilyQuery},
		{name: "unknown", key: CachePolicyKey("unknown"), want: cachemodel.FamilyDefault},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FamilyFor(tt.key); got != tt.want {
				t.Fatalf("FamilyFor(%q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

func TestEffectiveRegistryUsesPathDerivedCapabilityIDs(t *testing.T) {
	registry := NewEffectiveRegistry(NewPolicyCatalog(nil, nil))
	entries := registry.Snapshot()
	if len(entries) != 8 {
		t.Fatalf("registry entries = %d, want 8", len(entries))
	}
	if entries[0].Capability != "catalog.scale" || entries[0].Source != "cache.capabilities.catalog.scale" {
		t.Fatalf("first entry = %#v", entries[0])
	}
}
