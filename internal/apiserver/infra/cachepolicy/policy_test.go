package cachepolicy

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
		t.Fatal("期望子策略显式关闭压缩")
	}
	if !merged.Singleflight.Enabled(false) {
		t.Fatal("期望继承父级 singleflight 配置")
	}
	if !merged.Negative.Enabled(false) {
		t.Fatal("期望子策略显式开启 negative cache")
	}
}
