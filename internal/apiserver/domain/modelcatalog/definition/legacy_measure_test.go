package definition_test

import (
	"reflect"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
)

func TestMeasureAndCalibrationFromLegacyFactorsSplitsTargetLayers(t *testing.T) {
	t.Parallel()

	maxScore := 20.0
	legacy := []factor.LegacyFactor{
		{
			Code: "gec", Title: "GEC", Role: factor.FactorRoleIndex, SortOrder: 1,
			ChildrenPolicy: &factor.ChildrenPolicy{
				Strategy: factor.ChildrenAggregationWeightedSum,
				Children: []string{"bri", "mi"},
				Weights:  map[string]float64{"bri": 0.4, "mi": 0.6},
			},
			Norm: &factor.NormRef{FactorCode: "gec", NormTableVersion: "2024"},
		},
		{
			Code: "bri", Title: "BRI", Role: factor.FactorRoleIndex, ParentCode: "gec", SortOrder: 2,
			ChildrenPolicy: &factor.ChildrenPolicy{
				Strategy: factor.ChildrenAggregationSum,
				Children: []string{"inhibit"},
			},
		},
		{
			Code: "inhibit", Title: "Inhibit", ParentCode: "bri", SortOrder: 3,
			QuestionCodes: []string{"q1", "q2"}, ScoringStrategy: "sum", MaxScore: &maxScore,
		},
	}

	measure, calibration := definition.MeasureAndCalibrationFromLegacyFactors(legacy)

	wantFactors := []factor.Factor{
		{Code: "gec", Title: "GEC", Role: factor.FactorRoleIndex},
		{Code: "bri", Title: "BRI", Role: factor.FactorRoleIndex},
		{Code: "inhibit", Title: "Inhibit", Role: factor.FactorRoleDimension},
	}
	if !reflect.DeepEqual(measure.Factors, wantFactors) {
		t.Fatalf("factors\n got: %#v\nwant: %#v", measure.Factors, wantFactors)
	}
	wantEdges := []factor.FactorEdge{
		{ParentCode: "gec", ChildCode: "bri"},
		{ParentCode: "gec", ChildCode: "mi"},
		{ParentCode: "bri", ChildCode: "inhibit"},
	}
	if !reflect.DeepEqual(measure.FactorGraph.Edges, wantEdges) {
		t.Fatalf("edges\n got: %#v\nwant: %#v", measure.FactorGraph.Edges, wantEdges)
	}
	if got := measure.FactorGraph.SortOrders["inhibit"]; got != 3 {
		t.Fatalf("sort order = %d, want 3", got)
	}
	if len(measure.Scoring) != 3 {
		t.Fatalf("scoring = %#v", measure.Scoring)
	}
	if measure.Scoring[0].Sources[0].Kind != factor.ScoringSourceFactor ||
		measure.Scoring[0].Weights["mi"] != 0.6 {
		t.Fatalf("composite scoring = %#v", measure.Scoring[0])
	}
	if measure.Scoring[2].Sources[0].Kind != factor.ScoringSourceQuestion ||
		measure.Scoring[2].Sources[0].Code != "q1" ||
		*measure.Scoring[2].MaxScore != maxScore {
		t.Fatalf("question scoring = %#v", measure.Scoring[2])
	}
	if len(calibration.NormRefs) != 1 ||
		calibration.NormRefs[0].FactorCode != "gec" ||
		calibration.NormRefs[0].NormTableVersion != "2024" {
		t.Fatalf("calibration = %#v", calibration)
	}
}

func TestLegacyFactorsFromMeasureSpecProjectsCompatibilityShape(t *testing.T) {
	t.Parallel()

	measure := definition.MeasureSpec{
		Factors: []factor.Factor{
			{Code: "total", Title: "总分", Role: factor.FactorRoleTotal},
			{Code: "raw", Title: "原始分", Role: factor.FactorRoleDimension},
		},
		FactorGraph: factor.FactorGraph{
			Roots:      []string{"total"},
			Edges:      []factor.FactorEdge{{ParentCode: "total", ChildCode: "raw"}},
			SortOrders: map[string]int{"raw": 2},
		},
		Scoring: []factor.Scoring{{
			FactorCode: "raw",
			Sources: []factor.ScoringSource{
				{Kind: factor.ScoringSourceQuestion, Code: "q1"},
			},
			Strategy: factor.ScoringStrategySum,
		}},
	}
	calibration := definition.Calibration{}

	legacy := definition.LegacyFactorsFromMeasureSpec(measure, calibration)
	if len(legacy) != 2 {
		t.Fatalf("legacy = %#v", legacy)
	}
	if legacy[1].ParentCode != "total" || legacy[1].Level != 2 || legacy[1].SortOrder != 2 {
		t.Fatalf("projected hierarchy = %#v", legacy[1])
	}
	if legacy[1].QuestionCodes[0] != "q1" || legacy[1].ScoringStrategy != "sum" {
		t.Fatalf("projected scoring = %#v", legacy[1])
	}
}

func TestValidateMeasureSpecRejectsMixedSourceKinds(t *testing.T) {
	t.Parallel()

	issues := definition.ValidateMeasureSpec(definition.MeasureSpec{
		Factors: []factor.Factor{{Code: "total", Role: factor.FactorRoleTotal}},
		Scoring: []factor.Scoring{{
			FactorCode: "total",
			Sources: []factor.ScoringSource{
				{Kind: factor.ScoringSourceQuestion, Code: "q1"},
				{Kind: factor.ScoringSourceFactor, Code: "f1"},
			},
		}},
	})
	if len(issues) == 0 {
		t.Fatal("expected mixed source issue")
	}
}

func TestParseMeasureSpecFromDefinitionBody(t *testing.T) {
	t.Parallel()

	measure, calibration := definition.ParseMeasureSpecFromDefinitionBody([]factor.DimensionRule{{
		Code: "total", Title: "总分", Role: string(factor.FactorRoleTotal),
		QuestionCodes: []string{"q1"}, ScoringStrategy: "sum",
	}}, []factor.InterpretRule{{
		DimensionCode: "total",
		Ranges:        []factor.ScoreRangeRule{{MinScore: 0, MaxScore: 10, Level: "low"}},
	}})
	if len(measure.Factors) != 1 || measure.Factors[0].Role != factor.FactorRoleTotal {
		t.Fatalf("factors = %#v", measure.Factors)
	}
	if len(measure.Scoring) != 1 ||
		measure.Scoring[0].Sources[0].Kind != factor.ScoringSourceQuestion ||
		measure.Scoring[0].Sources[0].Code != "q1" {
		t.Fatalf("scoring = %#v", measure.Scoring)
	}
	if len(calibration.NormRefs) != 0 {
		t.Fatalf("calibration = %#v", calibration)
	}
}
