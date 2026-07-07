package norming_test

import (
	"testing"

	factornorm "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/norming"
	calcnorm "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/norm"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	behavioralsnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/behavioral_rating/snapshot"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
)

func TestApplyFactorProjectionsRollsUpAndAppliesNorm(t *testing.T) {
	t.Parallel()

	outcome := &assessment.AssessmentOutcome{
		Dimensions: []assessment.DimensionResult{
			{Code: "inhibit", Score: score(4)},
			{Code: "self_monitor", Score: score(6)},
		},
	}
	snapshot := &behavioralsnapshot.Snapshot{
		Factors: []factor.FactorSnapshot{
			{Code: "inhibit", Title: "Inhibit"},
			{Code: "self_monitor", Title: "Self Monitor"},
			{
				Code: "bri", Title: "BRI", Role: factor.FactorRoleIndex,
				ChildrenPolicy: &factor.ChildrenPolicy{
					Strategy: factor.ChildrenAggregationSum,
					Children: []string{"inhibit", "self_monitor"},
				},
			},
			{
				Code: "gec", Title: "GEC", Role: factor.FactorRoleIndex,
				ChildrenPolicy: &factor.ChildrenPolicy{
					Strategy: factor.ChildrenAggregationSum,
					Children: []string{"bri"},
				},
				Norm: &factor.NormRef{FactorCode: "gec", NormTableVersion: "2024"},
			},
		},
		Brief2: &behavioralsnapshot.Brief2Profile{
			NormTables: &calcnorm.NormTables{
				Factors: []calcnorm.FactorNormTable{{
					FactorCode: "gec",
					Lookup: []calcnorm.NormLookupEntry{
						{RawMin: 0, RawMax: 20, TScore: 65, Percentile: 92},
					},
				}},
			},
		},
	}

	enriched := factornorm.ApplyFactorProjections(outcome, snapshot, calcnorm.Subject{})
	if got := dimensionScore(enriched.Dimensions, "bri"); got != 10 {
		t.Fatalf("bri raw = %v, want 10", got)
	}
	if got := dimensionScore(enriched.Dimensions, "gec"); got != 10 {
		t.Fatalf("gec raw = %v, want 10", got)
	}
	gec := findDimension(enriched.Dimensions, "gec")
	if gec == nil {
		t.Fatalf("gec derived = %#v", gec)
	}
	if gec.Role != string(factor.FactorRoleIndex) || gec.ParentCode != "" || gec.HierarchyLevel != 1 {
		t.Fatalf("gec hierarchy = role=%q parent=%q level=%d", gec.Role, gec.ParentCode, gec.HierarchyLevel)
	}
	bri := findDimension(enriched.Dimensions, "bri")
	if bri == nil || bri.ParentCode != "gec" {
		t.Fatalf("bri hierarchy = %#v, want parent gec", bri)
	}
	for _, derived := range gec.DerivedScores {
		if derived.Kind == assessment.OutcomeScoreKindTScore && derived.Value == 65 {
			return
		}
	}
	t.Fatalf("gec derived = %#v, want t_score 65", gec.DerivedScores)
}

func score(value float64) *assessment.OutcomeScoreValue {
	return &assessment.OutcomeScoreValue{Kind: assessment.OutcomeScoreKindRawTotal, Value: value}
}

func dimensionScore(dimensions []assessment.DimensionResult, code string) float64 {
	dim := findDimension(dimensions, code)
	if dim == nil || dim.Score == nil {
		return 0
	}
	return dim.Score.Value
}

func findDimension(dimensions []assessment.DimensionResult, code string) *assessment.DimensionResult {
	for i := range dimensions {
		if dimensions[i].Code == code {
			return &dimensions[i]
		}
	}
	return nil
}
