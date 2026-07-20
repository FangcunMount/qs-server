package factor_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/capability"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
)

func TestValidateScoringStrategyCapabilityRejectsUnsupportedScaleStrategy(t *testing.T) {
	t.Parallel()
	issues := factor.ValidateScoringStrategyCapability(capability.PathScaleDescriptor, []factor.Scoring{{
		FactorCode: "TOTAL",
		Strategy:   factor.ScoringStrategyWeightedSum,
		Sources:    []factor.ScoringSource{{Kind: factor.ScoringSourceQuestion, Code: "Q1"}},
	}})
	if len(issues) != 1 || issues[0].Code != "strategy.unsupported_for_path" {
		t.Fatalf("issues = %#v, want strategy.unsupported_for_path", issues)
	}
}

func TestValidateScoringStrategyCapabilityAcceptsScaleSum(t *testing.T) {
	t.Parallel()
	issues := factor.ValidateScoringStrategyCapability(capability.PathScaleDescriptor, []factor.Scoring{{
		FactorCode: "TOTAL",
		Strategy:   factor.ScoringStrategySum,
		Sources:    []factor.ScoringSource{{Kind: factor.ScoringSourceQuestion, Code: "Q1"}},
	}})
	if len(issues) != 0 {
		t.Fatalf("issues = %#v, want none", issues)
	}
}

func TestValidateScoringStrategyCapabilityAcceptsAverageAlias(t *testing.T) {
	t.Parallel()
	issues := factor.ValidateScoringStrategyCapability(capability.PathScaleDescriptor, []factor.Scoring{{
		FactorCode: "TOTAL",
		Strategy:   "average",
		Sources:    []factor.ScoringSource{{Kind: factor.ScoringSourceQuestion, Code: "Q1"}},
	}})
	if len(issues) != 0 {
		t.Fatalf("issues = %#v, want none for average alias", issues)
	}
}

func TestValidateScoringStrategyCapabilityTypologyLeafRejectsAvg(t *testing.T) {
	t.Parallel()
	issues := factor.ValidateScoringStrategyCapability(capability.PathTypologyDescriptor, []factor.Scoring{{
		FactorCode: "E",
		Strategy:   factor.ScoringStrategyAvg,
		Sources:    []factor.ScoringSource{{Kind: factor.ScoringSourceQuestion, Code: "Q1"}},
	}})
	if len(issues) != 1 || issues[0].Code != "strategy.unsupported_for_path" {
		t.Fatalf("issues = %#v, want typology leaf rejection of avg", issues)
	}
}

func TestValidateScoringStrategyCapabilityTypologyCompositeAcceptsWeightedAvg(t *testing.T) {
	t.Parallel()
	issues := factor.ValidateScoringStrategyCapability(capability.PathTypologyDescriptor, []factor.Scoring{{
		FactorCode: "TYPE",
		Strategy:   factor.ScoringStrategyWeightedAvg,
		Sources:    []factor.ScoringSource{{Kind: factor.ScoringSourceFactor, Code: "E"}},
	}})
	if len(issues) != 0 {
		t.Fatalf("issues = %#v, want none for typology weighted_avg", issues)
	}
}

func TestValidateScoringStrategyCapabilityBehavioralRejectsMax(t *testing.T) {
	t.Parallel()
	issues := factor.ValidateScoringStrategyCapability(capability.PathBehavioralRatingDescriptor, []factor.Scoring{{
		FactorCode: "bri",
		Strategy:   factor.ScoringStrategy("max"),
		Sources:    []factor.ScoringSource{{Kind: factor.ScoringSourceQuestion, Code: "q1"}},
	}})
	if len(issues) != 1 || issues[0].Code != "strategy.unsupported_for_path" {
		t.Fatalf("issues = %#v, want behavioral rejection of max", issues)
	}
}
