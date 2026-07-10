package evaluationinput

import (
	"context"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norm"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	behavioral "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/behavioral"
)

func TestPublishedBehavioralRatingCatalogDecodesPublishedModel(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
		"dimensions": [{
			"code": "total",
			"title": "总分",
			"question_codes": ["q1", "q2"],
			"scoring_strategy": "sum",
			"is_total_score": true
		}],
		"interpret_rules": [{
			"dimension_code": "total",
			"ranges": [{"min_score": 0, "max_score": 10, "conclusion": "low", "level": "low"}]
		}]
	}`)
	materialized, err := behavioral.MaterializeDefinition(raw)
	if err != nil {
		t.Fatalf("MaterializeDefinition: %v", err)
	}
	reader := stubPublishedBehavioralRatingReader{snapshot: &rulesetport.PublishedModel{
		SchemaVersion:        domain.SchemaVersionV2,
		PayloadFormat:        domain.PayloadFormatBehavioralRatingDefaultV1,
		Kind:                 domain.KindBehavioralRating,
		Algorithm:            domain.AlgorithmBehavioralRatingDefault,
		Code:                 "BR-001",
		Version:              "1.0.0",
		Title:                "行为评分",
		Status:               "published",
		QuestionnaireCode:    "Q-001",
		QuestionnaireVersion: "1.0.0",
		Payload:              []byte("not-json"),
		DefinitionV2:         materialized.Definition,
	}}
	catalog := NewPublishedBehavioralRatingCatalog(reader)
	got, err := catalog.GetBehavioralRatingByRef(context.Background(), port.ModelRef{
		Kind:    port.EvaluationModelKindBehavioralRating,
		Code:    "BR-001",
		Version: "1.0.0",
	})
	if err != nil {
		t.Fatalf("GetBehavioralRatingByRef: %v", err)
	}
	if got.Code != "BR-001" || got.QuestionnaireCode != "Q-001" {
		t.Fatalf("snapshot = %#v", got)
	}
	scale := got.ToScaleSnapshot()
	if scale == nil || len(scale.Factors) != 1 || scale.Factors[0].Code != "total" {
		t.Fatalf("scale projection = %#v", scale)
	}
}

func TestPublishedBehavioralRatingCatalogDecodesBrief2Snapshot(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
		"dimensions": [{"code": "gec", "title": "GEC", "question_codes": ["q1"], "scoring_strategy": "sum"}],
		"interpret_rules": [{"dimension_code": "gec", "ranges": [{"min_score": 0, "max_score": 10, "conclusion": "ok"}]}],
		"brief2": {
			"form_variant": "parent", "norm_table_version": "2024", "index_codes": ["gec"],
			"norms": [{"factor_code": "gec", "lookup": [{"raw_min": 0, "raw_max": 10, "t_score": 50, "percentile": 50}]}],
			"t_score_rules": [{"factor_code": "gec", "ranges": [{"min_t": 0, "max_t": 100, "level": "average", "conclusion": "ok"}]}]
		}
	}`)
	materialized, err := behavioral.MaterializeDefinition(raw)
	if err != nil {
		t.Fatalf("MaterializeDefinition: %v", err)
	}
	reader := stubPublishedBehavioralRatingReader{snapshot: &rulesetport.PublishedModel{
		SchemaVersion: domain.SchemaVersionV2,
		PayloadFormat: domain.PayloadFormatBehavioralRatingBrief2V1,
		Kind:          domain.KindBehavioralRating,
		Algorithm:     domain.AlgorithmBrief2,
		Code:          "BR-BRIEF2",
		Version:       "1.0.0",
		Title:         "BRIEF-2",
		Status:        "published",
		Payload:       []byte("not-json"),
		DefinitionV2:  materialized.Definition,
	}}
	catalog := NewPublishedBehavioralRatingCatalog(reader, stubNormRepository{tables: materialized.Norms})
	got, err := catalog.GetBehavioralRatingByRef(context.Background(), port.ModelRef{
		Kind:      port.EvaluationModelKindBehavioralRating,
		Algorithm: string(domain.AlgorithmBrief2),
		Code:      "BR-BRIEF2",
		Version:   "1.0.0",
	})
	if err != nil {
		t.Fatalf("GetBehavioralRatingByRef: %v", err)
	}
	if got.Norming == nil || got.Norming.Variant != "parent" {
		t.Fatalf("norming profile = %#v", got.Norming)
	}
}

type stubNormRepository struct {
	tables []*norm.Norm
}

func (s stubNormRepository) UpsertNorm(context.Context, *norm.Norm) error { return nil }

func (s stubNormRepository) FindNorm(_ context.Context, version string) (*norm.Norm, error) {
	for _, table := range s.tables {
		if table != nil && table.TableVersion == version {
			return table, nil
		}
	}
	return nil, domain.ErrNotFound
}

type stubPublishedBehavioralRatingReader struct {
	snapshot *rulesetport.PublishedModel
	err      error
}

func (s stubPublishedBehavioralRatingReader) GetPublishedModelByRef(context.Context, rulesetport.Ref) (*rulesetport.PublishedModel, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.snapshot, nil
}

func (s stubPublishedBehavioralRatingReader) FindPublishedModelByQuestionnaire(context.Context, string, string) (*rulesetport.PublishedModel, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.snapshot, nil
}
