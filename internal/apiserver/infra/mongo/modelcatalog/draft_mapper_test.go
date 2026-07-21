package modelcatalog

import (
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
)

func TestDraftMapperRoundTrip(t *testing.T) {
	original, err := domain.NewAssessmentModel(domain.NewAssessmentModelInput{
		Code: "personality_demo", Kind: domain.KindTypology,
		SubKind: domain.SubKindTypology, Algorithm: domain.AlgorithmPersonalityTypology, Title: "Demo",
		Category: "personality", Stages: []string{"intake"}, ApplicableAges: []string{"adult"}, Reporters: []string{"self"}, Tags: []string{"demo"},
	})
	if err != nil {
		t.Fatalf("NewAssessmentModel: %v", err)
	}
	_ = original.UpdateDefinition(sampleDefinitionV2(), original.CreatedAt)

	mapper := NewDraftMapper()
	po := mapper.ToPO(original)
	if po.DefinitionSchemaVersion != domain.SchemaVersionV2 {
		t.Fatalf("definition schema version = %q", po.DefinitionSchemaVersion)
	}
	if po.DefinitionV2 == nil || len(po.DefinitionV2.Measure.Factors) != 2 {
		t.Fatalf("definition_v2 po = %#v", po.DefinitionV2)
	}
	got := mapper.ToDomain(po)
	if got.Code != original.Code || got.Algorithm != original.Algorithm {
		t.Fatalf("round trip = %#v", got)
	}
	if got.Category != "personality" || got.Stages[0] != "intake" || got.ApplicableAges[0] != "adult" ||
		got.Reporters[0] != "self" || got.Tags[0] != "demo" {
		t.Fatalf("metadata round trip = %#v", got)
	}
	assertDefinitionV2RoundTrip(t, got.DefinitionV2)
}

func TestDraftMapperRejectsNoDefinitionByLeavingItNil(t *testing.T) {
	po := &AssessmentModelPO{
		Code: "invalid", Kind: string(domain.KindBehavioralRating), Title: "Invalid", Status: string(domain.ModelStatusDraft),
	}
	got := NewDraftMapper().ToDomain(po)
	if got.DefinitionV2 != nil {
		t.Fatalf("definition v2 = %#v, want nil for old document", got.DefinitionV2)
	}
}

func sampleDefinitionV2() *domain.Definition {
	maxScore := 10.0
	return &domain.Definition{
		Measure: domain.MeasureSpec{
			Factors: []domain.Factor{
				{Code: "total", Title: "总分", Role: factor.FactorRoleTotal},
				{Code: "raw", Title: "原始分", Role: factor.FactorRoleDimension},
			},
			FactorGraph: factor.FactorGraph{
				Roots:      []string{"total"},
				Edges:      []factor.FactorEdge{{ParentCode: "total", ChildCode: "raw"}},
				SortOrders: map[string]int{"total": 1, "raw": 2},
			},
			Scoring: []factor.Scoring{{
				FactorCode: "raw",
				Sources: []factor.ScoringSource{
					{Kind: factor.ScoringSourceQuestion, Code: "q1"},
					{Kind: factor.ScoringSourceQuestion, Code: "q2"},
				},
				Strategy: factor.ScoringStrategySum,
				Params:   &factor.ScoringParams{CntOptionContents: []string{"yes"}},
				MaxScore: &maxScore,
			}},
		},
		Calibration: domain.Calibration{
			NormRefs: []domain.NormRef{{FactorCode: "total", NormTableVersion: "2024"}},
		},
		Conclusions: []domain.Conclusion{
			domain.RiskConclusion{
				FactorCode: "total",
				Rules: []domain.ScoreRangeOutcome{{
					MinScore: 0, MaxScore: 10, OutcomeCode: "low", Title: "低风险", Summary: "保持", Description: "继续观察",
				}},
				Outcomes: []domain.Outcome{{Code: "low", Title: "低风险"}},
			},
		},
		Outcomes: []domain.Outcome{{Code: "low", Title: "低风险", Summary: "保持"}},
		ReportMap: domain.ReportMap{
			Sections: []domain.ReportSection{{Code: "summary", Title: "总览", SourceRefs: []string{"total"}}},
		},
	}
}

func assertDefinitionV2RoundTrip(t *testing.T, got *domain.Definition) {
	t.Helper()
	if got == nil {
		t.Fatal("definition v2 is nil")
	}
	if len(got.Measure.Factors) != 2 || got.Measure.Factors[0].Code != "total" {
		t.Fatalf("factors = %#v", got.Measure.Factors)
	}
	if len(got.Measure.FactorGraph.Edges) != 1 || got.Measure.FactorGraph.Edges[0].ChildCode != "raw" {
		t.Fatalf("graph = %#v", got.Measure.FactorGraph)
	}
	if len(got.Measure.Scoring) != 1 ||
		got.Measure.Scoring[0].Sources[1].Code != "q2" ||
		got.Measure.Scoring[0].Params.CntOptionContents[0] != "yes" {
		t.Fatalf("scoring = %#v", got.Measure.Scoring)
	}
	if len(got.Calibration.NormRefs) != 1 || got.Calibration.NormRefs[0].NormTableVersion != "2024" {
		t.Fatalf("calibration = %#v", got.Calibration)
	}
	if len(got.Conclusions) != 1 {
		t.Fatalf("conclusions = %#v", got.Conclusions)
	}
	if risk, ok := got.Conclusions[0].(domain.RiskConclusion); !ok || risk.FactorCode != "total" || len(risk.Rules) != 1 || risk.Rules[0].MaxScore != 10 {
		t.Fatalf("risk conclusion = %#v", got.Conclusions[0])
	}
	if len(got.ReportMap.Sections) != 1 || got.ReportMap.Sections[0].SourceRefs[0] != "total" {
		t.Fatalf("report map = %#v", got.ReportMap)
	}
}
