package norming_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation"
	calcnorm "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/norm"
)

func TestPrimaryDimensionUsesConfiguredCode(t *testing.T) {
	t.Parallel()

	result := calcnorm.Projection{PrimaryDimensionCode: "bri"}.Apply(&calculation.Result{
		Dimensions: []calculation.DimensionResult{
			{Code: "gec", Level: &calculation.ResultLevel{Code: "legacy"}},
			{Code: "bri", Level: &calculation.ResultLevel{Code: "configured", Label: "configured label"}},
		},
	})
	if result.Level == nil || result.Level.Code != "configured" {
		t.Fatalf("result level = %#v, want configured", result.Level)
	}
}

func TestPrimaryDimensionFallsBackToLegacyGEC(t *testing.T) {
	t.Parallel()

	result := calcnorm.Projection{}.Apply(&calculation.Result{
		Dimensions: []calculation.DimensionResult{
			{Code: "inhibit"},
			{Code: "gec", Level: &calculation.ResultLevel{Code: "legacy", Label: "legacy label"}},
		},
	})
	if result.Level == nil || result.Level.Code != "legacy" {
		t.Fatalf("result level = %#v, want legacy gec fallback", result.Level)
	}
}
