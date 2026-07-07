package snapshot_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/behavioral_rating/snapshot"
)

func TestParseDefinitionPayloadProjectsToScaleSnapshot(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
		"dimensions": [
			{
				"code": "total",
				"title": "总分",
				"question_codes": ["q1", "q2"],
				"scoring_strategy": "sum",
				"is_total_score": true
			}
		],
		"interpret_rules": [
			{
				"dimension_code": "total",
				"ranges": [
					{"min_score": 0, "max_score": 10, "conclusion": "low", "level": "low"}
				]
			}
		]
	}`)
	got, err := snapshot.ParseDefinitionPayload("BA-001", "1.0.0", "行为能力", "published", raw)
	if err != nil {
		t.Fatalf("ParseDefinitionPayload: %v", err)
	}
	scale := got.ToScaleSnapshot()
	if scale == nil || len(scale.Factors) != 1 {
		t.Fatalf("scale factors = %#v", scale)
	}
	if scale.Factors[0].Code != "total" || scale.Factors[0].ScoringStrategy != "sum" {
		t.Fatalf("factor = %#v", scale.Factors[0])
	}
	if len(scale.Factors[0].InterpretRules) != 1 || scale.Factors[0].InterpretRules[0].RiskLevel != "low" {
		t.Fatalf("interpret rules = %#v", scale.Factors[0].InterpretRules)
	}
}

func TestParseBrief2PayloadPreservesProfile(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
		"dimensions": [{"code": "gec", "title": "GEC", "question_codes": ["q1"], "scoring_strategy": "sum"}],
		"interpret_rules": [{"dimension_code": "gec", "ranges": [{"min_score": 0, "max_score": 10, "conclusion": "ok"}]}],
		"brief2": {
			"form_variant": "teacher",
			"norm_table_version": "2024",
			"index_codes": ["bri", "gec"],
			"validity_codes": ["inconsistency"]
		}
	}`)
	got, err := snapshot.ParsePublishedPayload(
		"assessmentmodel.behavioral_rating.brief2.v1",
		"BR-001", "v1", "BRIEF-2", "published", raw,
	)
	if err != nil {
		t.Fatalf("ParsePublishedPayload: %v", err)
	}
	if got.Brief2 == nil || got.Brief2.FormVariant != "teacher" || got.Brief2.NormTableVersion != "2024" {
		t.Fatalf("brief2 profile = %#v", got.Brief2)
	}
	if got.ToScaleSnapshot() == nil || len(got.ToScaleSnapshot().Factors) != 1 {
		t.Fatal("expected scale projection to remain available")
	}
}

func TestParseBrief2PayloadNormTables(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
		"dimensions": [{"code": "gec", "title": "GEC", "question_codes": ["q1"], "scoring_strategy": "sum"}],
		"brief2": {
			"form_variant": "parent",
			"norm_table_version": "2024",
			"norms": [{
				"factor_code": "gec",
				"lookup": [{"raw_min": 0, "raw_max": 8, "t_score": 45, "percentile": 30}]
			}],
			"t_score_rules": [{
				"factor_code": "gec",
				"ranges": [{"min_t": 60, "max_t": 100, "level": "elevated", "conclusion": "升高"}]
			}]
		}
	}`)
	got, err := snapshot.ParsePublishedPayload(
		"assessmentmodel.behavioral_rating.brief2.v1",
		"BR-002", "v1", "BRIEF-2", "published", raw,
	)
	if err != nil {
		t.Fatalf("ParsePublishedPayload: %v", err)
	}
	tables := got.Brief2.NormTablesOrNil()
	if tables == nil || len(tables.Factors) != 1 || tables.Factors[0].FactorCode != "gec" {
		t.Fatalf("norm factors = %#v", tables)
	}
	if len(tables.Factors[0].Lookup) != 1 || tables.Factors[0].Lookup[0].TScore != 45 {
		t.Fatalf("lookup = %#v", tables.Factors[0].Lookup)
	}
	if len(tables.TScoreRules) != 1 || tables.TScoreRules[0].Ranges[0].Level != "elevated" {
		t.Fatalf("t_score rules = %#v", tables.TScoreRules)
	}
}
