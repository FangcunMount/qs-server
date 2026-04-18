package cache

import "testing"

func TestCachePolicyMergeWithSupportsExplicitDisable(t *testing.T) {
	t.Parallel()

	parent := CachePolicy{
		Compress:     PolicySwitchEnabled,
		Singleflight: PolicySwitchEnabled,
		Negative:     PolicySwitchDisabled,
	}
	child := CachePolicy{
		Compress: PolicySwitchDisabled,
		Negative: PolicySwitchEnabled,
	}

	merged := child.MergeWith(parent)
	if merged.Compress.Enabled(true) {
		t.Fatal("expected child policy to explicitly disable compression")
	}
	if !merged.Singleflight.Enabled(false) {
		t.Fatal("expected parent singleflight to be inherited")
	}
	if !merged.Negative.Enabled(false) {
		t.Fatal("expected child policy to explicitly enable negative cache")
	}
}
