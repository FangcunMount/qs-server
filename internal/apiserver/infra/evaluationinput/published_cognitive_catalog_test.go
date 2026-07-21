package evaluationinput

import (
	"context"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norm"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

func TestPublishedCognitiveCatalogDecodesPublishedModel(t *testing.T) {
	t.Parallel()

	reader := stubPublishedCognitiveReader{snapshot: &rulesetport.PublishedModel{
		SchemaVersion:        domain.SchemaVersionV2,
		Kind:                 domain.KindCognitive,
		Algorithm:            domain.AlgorithmSPM,
		Code:                 "COG-001",
		Version:              "1.0.0",
		Title:                "认知测评",
		Status:               "published",
		QuestionnaireCode:    "Q-001",
		QuestionnaireVersion: "1.0.0",
		DefinitionV2:         cognitiveDefinition(false),
	}}
	catalog := NewPublishedCognitiveCatalog(reader)
	got, err := catalog.GetCognitiveByRef(context.Background(), port.ModelRef{
		Kind:      port.EvaluationModelKindCognitive,
		Algorithm: string(domain.AlgorithmSPM),
		Code:      "COG-001",
		Version:   "1.0.0",
	})
	if err != nil {
		t.Fatalf("GetCognitiveByRef: %v", err)
	}
	if got.Code != "COG-001" || got.QuestionnaireCode != "Q-001" {
		t.Fatalf("snapshot = %#v", got)
	}
	scale := got.ToScaleSnapshot()
	if scale == nil || len(scale.Factors) != 1 || scale.Factors[0].Code != "total" {
		t.Fatalf("scale projection = %#v", scale)
	}
}

func TestPublishedCognitiveCatalogDecodesSPMSnapshot(t *testing.T) {
	t.Parallel()

	reader := stubPublishedCognitiveReader{snapshot: &rulesetport.PublishedModel{
		SchemaVersion: domain.SchemaVersionV2,
		Kind:          domain.KindCognitive,
		Algorithm:     domain.AlgorithmSPM,
		Code:          "COG-SPM",
		Version:       "1.0.0",
		Title:         "SPM",
		Status:        "published",
		DefinitionV2:  cognitiveDefinition(true),
	}}
	catalog := NewPublishedCognitiveCatalog(reader)
	got, err := catalog.GetCognitiveByRef(context.Background(), port.ModelRef{
		Kind:      port.EvaluationModelKindCognitive,
		Algorithm: string(domain.AlgorithmSPM),
		Code:      "COG-SPM",
		Version:   "1.0.0",
	})
	if err != nil {
		t.Fatalf("GetCognitiveByRef: %v", err)
	}
	if got.Factors[0].Norm == nil || got.Factors[0].Norm.NormTableVersion != "2024" {
		t.Fatalf("total factor norm = %#v", got.Factors[0].Norm)
	}
}

func cognitiveDefinition(withNorm bool) *modeldefinition.Definition {
	def := &modeldefinition.Definition{
		Measure: modeldefinition.MeasureSpec{
			Factors:     []factor.Factor{{Code: "total", Title: "总分", Role: factor.FactorRoleTotal}},
			FactorGraph: factor.FactorGraph{Roots: []string{"total"}},
			Scoring:     []factor.Scoring{{FactorCode: "total", Sources: []factor.ScoringSource{{Kind: factor.ScoringSourceQuestion, Code: "q1"}}, Strategy: factor.ScoringStrategySum}},
		},
		Execution: modeldefinition.ExecutionSpec{SPM: &modeldefinition.SPMSpec{TimeLimitSeconds: 900, TotalFactorCode: "total"}},
	}
	if withNorm {
		def.Calibration.NormRefs = []norm.Ref{{FactorCode: "total", NormTableVersion: "2024"}}
	}
	return def
}

type stubPublishedCognitiveReader struct {
	snapshot *rulesetport.PublishedModel
	err      error
}

func (s stubPublishedCognitiveReader) GetPublishedModelByRef(context.Context, rulesetport.Ref) (*rulesetport.PublishedModel, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.snapshot, nil
}

func (s stubPublishedCognitiveReader) FindPublishedModelByQuestionnaire(context.Context, string, string) (*rulesetport.PublishedModel, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.snapshot, nil
}
