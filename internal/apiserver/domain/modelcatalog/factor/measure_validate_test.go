package factor_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
)

func TestValidateMeasureSpecPartsRequiresFactors(t *testing.T) {
	t.Parallel()

	issues := factor.ValidateMeasureSpecParts(nil, factor.FactorGraph{}, nil)
	if len(issues) != 1 || issues[0].Code != "measure.factors.required" {
		t.Fatalf("issues = %#v, want measure.factors.required", issues)
	}
}

func TestValidateMeasureSpecPartsAcceptsSingleFactor(t *testing.T) {
	t.Parallel()

	issues := factor.ValidateMeasureSpecParts([]factor.Factor{{
		Code: "total", Role: factor.FactorRoleTotal,
	}}, factor.FactorGraph{}, nil)
	if len(issues) != 0 {
		t.Fatalf("issues = %#v, want none", issues)
	}
}
