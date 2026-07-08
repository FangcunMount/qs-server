package policy_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestReportProfileForDecisionKind(t *testing.T) {
	cases := []struct {
		decision modelcatalog.DecisionKind
		want     policy.ReportProfile
	}{
		{modelcatalog.DecisionKindScoreRange, policy.ReportProfileScale},
		{modelcatalog.DecisionKindNormLookup, policy.ReportProfileNorm},
		{modelcatalog.DecisionKindAbilityLevel, policy.ReportProfileTask},
		{modelcatalog.DecisionKindPoleComposition, policy.ReportProfilePersonalityType},
		{modelcatalog.DecisionKindTraitProfile, policy.ReportProfileTrait},
		{modelcatalog.DecisionKindNearestPattern, policy.ReportProfilePattern},
	}
	for _, tc := range cases {
		if got := policy.ReportProfileForDecisionKind(tc.decision); got != tc.want {
			t.Fatalf("decision %s: got %q want %q", tc.decision, got, tc.want)
		}
	}
}
