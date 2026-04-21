package cachepolicy

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/redisplane"
)

func TestPolicyCatalogMergesFamilyDefaultIntoObjectPolicy(t *testing.T) {
	t.Parallel()

	catalog := NewPolicyCatalog(
		map[redisplane.Family]CachePolicy{
			redisplane.FamilyQuery: {
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
		want redisplane.Family
	}{
		{name: "static scale", key: PolicyScale, want: redisplane.FamilyStatic},
		{name: "object plan", key: PolicyPlan, want: redisplane.FamilyObject},
		{name: "query stats", key: PolicyStatsQuery, want: redisplane.FamilyQuery},
		{name: "unknown", key: CachePolicyKey("unknown"), want: redisplane.FamilyDefault},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FamilyFor(tt.key); got != tt.want {
				t.Fatalf("FamilyFor(%q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}
