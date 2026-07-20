package factor_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/capability"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
)

func TestScoringStrategyConstantsMatchDeclaredAuthoringCatalog(t *testing.T) {
	t.Parallel()
	declared := map[string]struct{}{}
	for _, code := range capability.DeclaredAuthoringStrategyCodes() {
		declared[code] = struct{}{}
	}
	consts := []factor.ScoringStrategy{
		factor.ScoringStrategySum,
		factor.ScoringStrategyAvg,
		factor.ScoringStrategyWeightedSum,
		factor.ScoringStrategyWeightedAvg,
		factor.ScoringStrategyCnt,
		factor.ScoringStrategyNone,
		factor.ScoringStrategyLookup,
		factor.ScoringStrategyCustom,
	}
	if len(consts) != len(declared) {
		t.Fatalf("ScoringStrategy const count %d != declared %d (%v)", len(consts), len(declared), capability.DeclaredAuthoringStrategyCodes())
	}
	for _, item := range consts {
		if _, ok := declared[string(item)]; !ok {
			t.Fatalf("ScoringStrategy %q not in DeclaredAuthoringStrategyCodes", item)
		}
	}
}
