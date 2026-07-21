package norming_test

import (
	"testing"

	factornorm "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/norming"
	calcnorm "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/norm"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	catalognorm "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norm"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	behavioralsnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/behavioral"
)

func TestApplyFactorProjectionsRollsUpAndAppliesNorm(t *testing.T) {
	t.Parallel()

	outcome := &domainoutcome.Execution{
		Dimensions: []domainoutcome.DimensionResult{
			{Code: "inhibit", Score: score(4)},
			{Code: "self_monitor", Score: score(6)},
		},
	}
	snapshot := &behavioralsnapshot.Snapshot{
		Factors: []behavioralsnapshot.FactorSnapshot{
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
				Norm: &catalognorm.Ref{FactorCode: "gec", NormTableVersion: "2024"},
			},
		},
		Norming: &behavioralsnapshot.NormingProfile{
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
	input := &evaluationinput.InputSnapshot{DefinitionV2: &modeldefinition.Definition{Measure: modeldefinition.MeasureSpec{
		Factors: []factor.Factor{
			{Code: "inhibit", Title: "Inhibit", Role: factor.FactorRoleDimension},
			{Code: "self_monitor", Title: "Self Monitor", Role: factor.FactorRoleDimension},
			{Code: "bri", Title: "BRI", Role: factor.FactorRoleIndex},
			{Code: "gec", Title: "GEC", Role: factor.FactorRoleIndex},
		},
		FactorGraph: factor.FactorGraph{
			Roots: []string{"gec"},
			Edges: []factor.FactorEdge{
				{ParentCode: "bri", ChildCode: "inhibit"},
				{ParentCode: "bri", ChildCode: "self_monitor"},
				{ParentCode: "gec", ChildCode: "bri"},
			},
		},
		Scoring: []factor.Scoring{
			{FactorCode: "bri", Strategy: factor.ScoringStrategySum, Sources: []factor.ScoringSource{{Kind: factor.ScoringSourceFactor, Code: "inhibit"}, {Kind: factor.ScoringSourceFactor, Code: "self_monitor"}}},
			{FactorCode: "gec", Strategy: factor.ScoringStrategySum, Sources: []factor.ScoringSource{{Kind: factor.ScoringSourceFactor, Code: "bri"}}},
		},
	}}}

	enriched, err := factornorm.ApplyFactorProjectionsForInput(outcome, input, snapshot, calcnorm.Subject{})
	if err != nil {
		t.Fatalf("ApplyFactorProjections: %v", err)
	}
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
		if derived.Kind == domainoutcome.ScoreKindTScore && derived.Value == 65 {
			return
		}
	}
	t.Fatalf("gec derived = %#v, want t_score 65", gec.DerivedScores)
}

func score(value float64) *domainoutcome.ScoreValue {
	return &domainoutcome.ScoreValue{Kind: domainoutcome.ScoreKindRawTotal, Value: value}
}

func dimensionScore(dimensions []domainoutcome.DimensionResult, code string) float64 {
	dim := findDimension(dimensions, code)
	if dim == nil || dim.Score == nil {
		return 0
	}
	return dim.Score.Value
}

func findDimension(dimensions []domainoutcome.DimensionResult, code string) *domainoutcome.DimensionResult {
	for i := range dimensions {
		if dimensions[i].Code == code {
			return &dimensions[i]
		}
	}
	return nil
}
