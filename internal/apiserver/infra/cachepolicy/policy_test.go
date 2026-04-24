package cachepolicy

import (
	"bytes"
	"testing"
)

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

func TestCachePolicyCompressValueUsesExplicitPolicyOnly(t *testing.T) {
	raw := []byte("payload large enough to demonstrate explicit compression policy")

	EnableCompression = true
	t.Cleanup(func() {
		EnableCompression = false
	})

	implicit := CachePolicy{}.CompressValue(raw)
	if !bytes.Equal(implicit, raw) {
		t.Fatal("expected inherited compression policy to ignore deprecated global default")
	}

	enabled := CachePolicy{Compress: PolicySwitchEnabled}.CompressValue(raw)
	if bytes.Equal(enabled, raw) {
		t.Fatal("expected explicit enabled compression policy to compress payload")
	}
	if got := (CachePolicy{}).DecompressValue(enabled); !bytes.Equal(got, raw) {
		t.Fatal("expected compressed payload to round trip")
	}

	disabled := CachePolicy{Compress: PolicySwitchDisabled}.CompressValue(raw)
	if !bytes.Equal(disabled, raw) {
		t.Fatal("expected explicit disabled compression policy to keep raw payload")
	}
}
