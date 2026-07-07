package evaluationinput

import (
	"context"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

func TestPublishedBehavioralRatingCatalogDecodesPublishedModelSnapshot(t *testing.T) {
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
	reader := stubPublishedBehavioralRatingReader{snapshot: &domain.PublishedModelSnapshot{
		SchemaVersion: domain.SchemaVersionV2,
		PayloadFormat: domain.PayloadFormatBehavioralRatingDefaultV1,
		Model: domain.ModelDefinition{
			Kind:      domain.KindBehavioralRating,
			Algorithm: domain.AlgorithmBehavioralRatingDefault,
			Code:      "BR-001",
			Version:   "1.0.0",
			Title:     "行为评分",
			Status:    "published",
		},
		Binding: domain.QuestionnaireBinding{
			QuestionnaireCode:    "Q-001",
			QuestionnaireVersion: "1.0.0",
		},
		Payload: raw,
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
		"brief2": {"form_variant": "parent", "norm_table_version": "2024", "index_codes": ["gec"]}
	}`)
	reader := stubPublishedBehavioralRatingReader{snapshot: &domain.PublishedModelSnapshot{
		SchemaVersion: domain.SchemaVersionV2,
		PayloadFormat: domain.PayloadFormatBehavioralRatingBrief2V1,
		Model: domain.ModelDefinition{
			Kind:      domain.KindBehavioralRating,
			Algorithm: domain.AlgorithmBrief2,
			Code:      "BR-BRIEF2",
			Version:   "1.0.0",
			Title:     "BRIEF-2",
			Status:    "published",
		},
		Payload: raw,
	}}
	catalog := NewPublishedBehavioralRatingCatalog(reader)
	got, err := catalog.GetBehavioralRatingByRef(context.Background(), port.ModelRef{
		Kind:      port.EvaluationModelKindBehavioralRating,
		Algorithm: string(domain.AlgorithmBrief2),
		Code:      "BR-BRIEF2",
		Version:   "1.0.0",
	})
	if err != nil {
		t.Fatalf("GetBehavioralRatingByRef: %v", err)
	}
	if got.Brief2 == nil || got.Brief2.FormVariant != "parent" {
		t.Fatalf("brief2 profile = %#v", got.Brief2)
	}
}

type stubPublishedBehavioralRatingReader struct {
	snapshot *domain.PublishedModelSnapshot
	err      error
}

func (s stubPublishedBehavioralRatingReader) GetPublishedModelByRef(context.Context, rulesetport.Ref) (*domain.PublishedModelSnapshot, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.snapshot, nil
}

func (s stubPublishedBehavioralRatingReader) FindPublishedModelByQuestionnaire(context.Context, string, string) (*domain.PublishedModelSnapshot, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.snapshot, nil
}
