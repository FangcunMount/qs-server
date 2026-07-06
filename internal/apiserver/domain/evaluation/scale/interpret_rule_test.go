package scale

import (
	"testing"

	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scale/snapshot"
)

func TestFindInterpretRuleUsesLeftClosedRightOpenIntervals(t *testing.T) {
	factor := scalesnapshot.FactorSnapshot{
		InterpretRules: []scalesnapshot.InterpretRuleSnapshot{
			{Min: 0, Max: 10, RiskLevel: string(RiskLevelLow)},
			{Min: 10, Max: 100, RiskLevel: string(RiskLevelSevere)},
		},
	}

	got := findInterpretRule(factor, 9.9)
	if got == nil || got.RiskLevel != string(RiskLevelLow) {
		t.Fatalf("score 9.9 = %#v, want low on [0,10)", got)
	}

	got = findInterpretRule(factor, 10)
	if got == nil || got.RiskLevel != string(RiskLevelSevere) {
		t.Fatalf("score 10 = %#v, want severe on [10,100)", got)
	}

	got = findInterpretRule(factor, 100)
	if got != nil {
		t.Fatalf("score 100 = %#v, want no match on [10,100)", got)
	}
}
