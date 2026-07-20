package factor_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/capability"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
)

func TestValidateExecutableScoringCapabilityRequiresScaleTotalSources(t *testing.T) {
	t.Parallel()
	issues := factor.ValidateExecutableScoringCapability(capability.PathScaleDescriptor, []factor.Factor{{
		Code: "TOTAL", Role: factor.FactorRoleTotal,
	}}, nil)
	if len(issues) != 1 || issues[0].Code != "factor.scoring.executable_required" {
		t.Fatalf("issues = %#v, want factor.scoring.executable_required", issues)
	}
}

func TestValidateExecutableScoringCapabilityAllowsCognitiveTotalWithoutMeasureScoring(t *testing.T) {
	t.Parallel()
	issues := factor.ValidateExecutableScoringCapability(capability.PathCognitiveDescriptor, []factor.Factor{{
		Code: "TOTAL", Role: factor.FactorRoleTotal,
	}}, nil)
	if len(issues) != 0 {
		t.Fatalf("issues = %#v, want none for cognitive total", issues)
	}
}

func TestValidateExecutableScoringCapabilityAcceptsScaleWithSources(t *testing.T) {
	t.Parallel()
	issues := factor.ValidateExecutableScoringCapability(capability.PathScaleDescriptor, []factor.Factor{{
		Code: "TOTAL", Role: factor.FactorRoleTotal,
	}}, []factor.Scoring{{
		FactorCode: "TOTAL",
		Strategy:   factor.ScoringStrategySum,
		Sources:    []factor.ScoringSource{{Kind: factor.ScoringSourceQuestion, Code: "Q1"}},
	}})
	if len(issues) != 0 {
		t.Fatalf("issues = %#v, want none", issues)
	}
}
