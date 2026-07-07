package behavioralrating

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation"
)

func TestPrimaryDimensionUsesConfiguredCode(t *testing.T) {
	t.Parallel()

	dimensions := []calculation.DimensionResult{
		{Code: "gec", Level: &calculation.ResultLevel{Code: "legacy"}},
		{Code: "bri", Level: &calculation.ResultLevel{Code: "configured"}},
	}
	got := primaryDimension(dimensions, "bri")
	if got == nil || got.Code != "bri" {
		t.Fatalf("primaryDimension = %#v, want bri", got)
	}
}

func TestPrimaryDimensionFallsBackToLegacyGEC(t *testing.T) {
	t.Parallel()

	dimensions := []calculation.DimensionResult{
		{Code: "inhibit"},
		{Code: "gec", Level: &calculation.ResultLevel{Code: "legacy"}},
	}
	got := primaryDimension(dimensions, "")
	if got == nil || got.Code != "gec" {
		t.Fatalf("primaryDimension = %#v, want legacy gec fallback", got)
	}
}
