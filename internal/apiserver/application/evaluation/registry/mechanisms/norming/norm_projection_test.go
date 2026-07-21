package norming_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation"
	calcnorm "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/norm"
)

func TestPrimaryDimensionUsesConfiguredCode(t *testing.T) {
	t.Parallel()

	result, err := calcnorm.Projection{PrimaryDimensionCode: "bri"}.Apply(&calculation.Result{
		Dimensions: []calculation.DimensionResult{
			{Code: "gec", Level: &calculation.ResultLevel{Code: "legacy"}},
			{Code: "bri", Level: &calculation.ResultLevel{Code: "configured", Label: "configured label"}},
		},
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if result.Level == nil || result.Level.Code != "configured" {
		t.Fatalf("result level = %#v, want configured", result.Level)
	}
}

func TestPrimaryDimensionRequiresConfiguredCode(t *testing.T) {
	t.Parallel()

	result, err := calcnorm.Projection{}.Apply(&calculation.Result{
		Dimensions: []calculation.DimensionResult{
			{Code: "inhibit"},
			{Code: "gec", Level: &calculation.ResultLevel{Code: "legacy", Label: "legacy label"}},
		},
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if result.Level != nil {
		t.Fatalf("result level = %#v, want nil without configured primary_dimension_code", result.Level)
	}
}
