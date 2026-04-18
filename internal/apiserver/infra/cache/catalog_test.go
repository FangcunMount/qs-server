package cache

import "testing"

func TestCacheCatalogMergesFamilyPolicyIntoObjectPolicy(t *testing.T) {
	catalog := NewCacheCatalogWithPolicies("prod", nil, map[CacheFamily]CachePolicy{
		CacheFamilyQuery: {
			TTL:          123,
			NegativeTTL:  45,
			Compress:     PolicySwitchEnabled,
			Singleflight: PolicySwitchDisabled,
			JitterRatio:  0.2,
		},
	}, map[CachePolicyKey]CachePolicy{
		PolicyStatsQuery: {},
	})

	policy := catalog.Policy(PolicyStatsQuery)
	if policy.TTL != 123 {
		t.Fatalf("expected family ttl to apply, got %v", policy.TTL)
	}
	if policy.NegativeTTL != 45 {
		t.Fatalf("expected family negative ttl to apply, got %v", policy.NegativeTTL)
	}
	if !policy.Compress.Enabled(false) {
		t.Fatalf("expected family compression to apply")
	}
	if policy.Singleflight.Enabled(true) {
		t.Fatalf("expected family singleflight disable to apply")
	}
	if policy.JitterRatio != 0.2 {
		t.Fatalf("expected family jitter to apply, got %v", policy.JitterRatio)
	}
}
