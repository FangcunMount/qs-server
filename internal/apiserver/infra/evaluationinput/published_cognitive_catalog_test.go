package evaluationinput

import (
	"context"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

func TestPublishedCognitiveCatalogDecodesPublishedModelSnapshot(t *testing.T) {
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
	reader := stubPublishedCognitiveReader{snapshot: &domain.PublishedModelSnapshot{
		SchemaVersion: domain.SchemaVersionV2,
		PayloadFormat: domain.PayloadFormatCognitiveDefaultV1,
		Model: domain.ModelDefinition{
			Kind:      domain.KindCognitive,
			Algorithm: domain.AlgorithmSPM,
			Code:      "COG-001",
			Version:   "1.0.0",
			Title:     "认知测评",
			Status:    "published",
		},
		Binding: domain.QuestionnaireBinding{
			QuestionnaireCode:    "Q-001",
			QuestionnaireVersion: "1.0.0",
		},
		Payload: raw,
	}}
	catalog := NewPublishedCognitiveCatalog(reader)
	got, err := catalog.GetCognitiveByRef(context.Background(), port.ModelRef{
		Kind:    port.EvaluationModelKindCognitive,
		Code:    "COG-001",
		Version: "1.0.0",
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

	raw := []byte(`{
		"dimensions": [{"code": "total", "title": "总分", "question_codes": ["q1"], "scoring_strategy": "sum"}],
		"interpret_rules": [{"dimension_code": "total", "ranges": [{"min_score": 0, "max_score": 10, "conclusion": "ok"}]}],
		"spm": {"time_limit_seconds": 900, "item_set_codes": ["A", "B"], "norm_table_version": "2024"}
	}`)
	reader := stubPublishedCognitiveReader{snapshot: &domain.PublishedModelSnapshot{
		SchemaVersion: domain.SchemaVersionV2,
		PayloadFormat: domain.PayloadFormatCognitiveSPMV1,
		Model: domain.ModelDefinition{
			Kind:      domain.KindCognitive,
			Algorithm: domain.AlgorithmSPM,
			Code:      "COG-SPM",
			Version:   "1.0.0",
			Title:     "SPM",
			Status:    "published",
		},
		Payload: raw,
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
	if got.SPM == nil || got.SPM.TimeLimitSeconds != 900 {
		t.Fatalf("spm profile = %#v", got.SPM)
	}
}

type stubPublishedCognitiveReader struct {
	snapshot *domain.PublishedModelSnapshot
	err      error
}

func (s stubPublishedCognitiveReader) GetPublishedModelByRef(context.Context, rulesetport.Ref) (*domain.PublishedModelSnapshot, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.snapshot, nil
}

func (s stubPublishedCognitiveReader) FindPublishedModelByQuestionnaire(context.Context, string, string) (*domain.PublishedModelSnapshot, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.snapshot, nil
}
