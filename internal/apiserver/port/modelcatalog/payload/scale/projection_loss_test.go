package scale

import (
	"reflect"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
)

// MC-R015 batch-2: ScaleSnapshot.Measure preserves canonical MeasureSpec while
// flat Factors remain the factor_scoring compat surface.

func TestScaleSnapshotFromDefinitionPreservesQuestionOnlyBaseline(t *testing.T) {
	t.Parallel()
	maxScore := 10.0
	def := &definition.Definition{
		Measure: definition.MeasureSpec{
			Factors: []factor.Factor{{Code: "total", Title: "总分", Role: factor.FactorRoleTotal}},
			FactorGraph: factor.FactorGraph{Roots: []string{"total"}},
			Scoring: []factor.Scoring{{
				FactorCode: "total",
				Strategy:   factor.ScoringStrategySum,
				MaxScore:   &maxScore,
				Sources: []factor.ScoringSource{
					{Kind: factor.ScoringSourceQuestion, Code: "q1"},
					{Kind: factor.ScoringSourceQuestion, Code: "q2"},
				},
			}},
		},
	}
	got := ScaleSnapshotFromDefinition(ExecutionEnvelope{Code: "SCL", ScaleVersion: "1", Status: "published"}, def)
	if got == nil || len(got.Factors) != 1 {
		t.Fatalf("snapshot = %#v", got)
	}
	f := got.Factors[0]
	if !f.IsTotalScore || f.ScoringStrategy != "sum" || f.MaxScore == nil || *f.MaxScore != 10 {
		t.Fatalf("factor = %#v", f)
	}
	if len(f.QuestionCodes) != 2 || f.QuestionCodes[0] != "q1" || f.QuestionCodes[1] != "q2" {
		t.Fatalf("QuestionCodes = %#v", f.QuestionCodes)
	}
	if !got.HasCanonicalMeasure() {
		t.Fatal("expected Measure attached on FromDefinition")
	}
}

func TestScaleSnapshotFromDefinitionPreservesFactorGraphAndFactorSources(t *testing.T) {
	t.Parallel()
	def := &definition.Definition{
		Measure: definition.MeasureSpec{
			Factors: []factor.Factor{
				{Code: "total", Title: "总分", Role: factor.FactorRoleTotal},
				{Code: "dim_a", Title: "维度A", Role: factor.FactorRoleDimension},
			},
			FactorGraph: factor.FactorGraph{
				Roots: []string{"total"},
				Edges: []factor.FactorEdge{{ParentCode: "total", ChildCode: "dim_a"}},
			},
			Scoring: []factor.Scoring{{
				FactorCode: "total",
				Strategy:   factor.ScoringStrategySum,
				Sources:    []factor.ScoringSource{{Kind: factor.ScoringSourceFactor, Code: "dim_a"}},
			}, {
				FactorCode: "dim_a",
				Strategy:   factor.ScoringStrategySum,
				Sources:    []factor.ScoringSource{{Kind: factor.ScoringSourceQuestion, Code: "q1"}},
			}},
		},
	}
	got := ScaleSnapshotFromDefinition(ExecutionEnvelope{Code: "SCL-GRAPH"}, def)
	if got == nil || !got.HasCanonicalMeasure() {
		t.Fatalf("snapshot = %#v", got)
	}
	// Flat compat: factor sources still do not fill QuestionCodes.
	total, ok := got.FindFactor("total")
	if !ok || len(total.QuestionCodes) != 0 {
		t.Fatalf("total flat QuestionCodes = %#v, want empty", total)
	}
	dim, ok := got.FindFactor("dim_a")
	if !ok || len(dim.QuestionCodes) != 1 || dim.QuestionCodes[0] != "q1" {
		t.Fatalf("dim_a = %#v", dim)
	}
	roundTrip := DefinitionFromScaleSnapshot(got)
	if !reflect.DeepEqual(roundTrip.Measure.FactorGraph, def.Measure.FactorGraph) {
		t.Fatalf("FactorGraph =\n%#v\nwant %#v", roundTrip.Measure.FactorGraph, def.Measure.FactorGraph)
	}
	totalScoring := scoringByFactor(roundTrip.Measure.Scoring, "total")
	if totalScoring == nil || len(totalScoring.Sources) != 1 || totalScoring.Sources[0].Kind != factor.ScoringSourceFactor {
		t.Fatalf("total sources = %#v", totalScoring)
	}
}

func TestScaleSnapshotFromDefinitionPreservesQuestionSourceMetadata(t *testing.T) {
	t.Parallel()
	def := &definition.Definition{
		Measure: definition.MeasureSpec{
			Factors: []factor.Factor{{Code: "dim", Title: "维度", Role: factor.FactorRoleDimension}},
			FactorGraph: factor.FactorGraph{Roots: []string{"dim"}},
			Scoring: []factor.Scoring{{
				FactorCode:    "dim",
				Strategy:      factor.ScoringStrategySum,
				Constant:      1.5,
				OptionScoring: factor.OptionScoringCompat,
				Weights:       map[string]float64{"q1": 0.7},
				Sources: []factor.ScoringSource{{
					Kind:         factor.ScoringSourceQuestion,
					Code:         "q1",
					ScoringMode:  factor.QuestionScoringModeOptionOverride,
					Sign:         -1,
					Weight:       0.5,
					OptionScores: map[string]float64{"A": 2, "B": 1},
				}},
			}},
		},
	}
	got := ScaleSnapshotFromDefinition(ExecutionEnvelope{Code: "SCL-META"}, def)
	f, ok := got.FindFactor("dim")
	if !ok || len(f.QuestionCodes) != 1 || f.QuestionCodes[0] != "q1" {
		t.Fatalf("flat factor = %#v", f)
	}
	roundTrip := DefinitionFromScaleSnapshot(got)
	rule := scoringByFactor(roundTrip.Measure.Scoring, "dim")
	if rule == nil || len(rule.Sources) != 1 {
		t.Fatalf("round-trip scoring = %#v", roundTrip.Measure.Scoring)
	}
	src := rule.Sources[0]
	if src.Sign != -1 || src.Weight != 0.5 || src.ScoringMode != factor.QuestionScoringModeOptionOverride {
		t.Fatalf("source metadata = %#v", src)
	}
	if src.OptionScores["A"] != 2 || src.OptionScores["B"] != 1 {
		t.Fatalf("OptionScores = %#v", src.OptionScores)
	}
	if rule.Constant != 1.5 || rule.OptionScoring != factor.OptionScoringCompat || rule.Weights["q1"] != 0.7 {
		t.Fatalf("scoring metadata = %#v", rule)
	}
}

func TestHistoricalFlatPayloadStillReadsWithoutMeasure(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"Code":"LEGACY","ScaleVersion":"1","Status":"published","Factors":[{"Code":"total","Title":"总分","IsTotalScore":true,"QuestionCodes":["q1"],"ScoringStrategy":"sum"}]}`)
	got, err := ParsePublishedPayload(raw)
	if err != nil {
		t.Fatalf("ParsePublishedPayload: %v", err)
	}
	if got.HasCanonicalMeasure() {
		t.Fatalf("legacy payload should omit Measure, got %#v", got.Measure)
	}
	def := DefinitionFromScaleSnapshot(got)
	if len(def.Measure.Factors) != 1 || def.Measure.Factors[0].Code != "total" {
		t.Fatalf("flat reconstruct = %#v", def.Measure)
	}
	if len(def.Measure.Scoring) != 1 || len(def.Measure.Scoring[0].Sources) != 1 {
		t.Fatalf("flat scoring = %#v", def.Measure.Scoring)
	}
}

func scoringByFactor(items []factor.Scoring, code string) *factor.Scoring {
	for i := range items {
		if items[i].FactorCode == code {
			return &items[i]
		}
	}
	return nil
}
