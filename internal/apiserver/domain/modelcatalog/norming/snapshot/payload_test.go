package snapshot_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norming/snapshot"
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
	if got.Norming == nil || got.Norming.Variant != "teacher" || got.Norming.NormTableVersion != "2024" {
		t.Fatalf("norming profile = %#v", got.Norming)
	}
	if got.ToScaleSnapshot() == nil || len(got.ToScaleSnapshot().Factors) != 1 {
		t.Fatal("expected scale projection to remain available")
	}
}

func TestParseBrief2PayloadAnnotatesCompositeMetadata(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
		"dimensions": [
			{"code": "inhibit", "title": "Inhibit", "question_codes": ["q1"], "scoring_strategy": "sum"},
			{"code": "self_monitor", "title": "Self Monitor", "question_codes": ["q2"], "scoring_strategy": "sum"},
			{"code": "bri", "title": "BRI"},
			{"code": "gec", "title": "GEC"}
		],
		"brief2": {
			"index_codes": ["bri", "gec"],
			"composite_indexes": [
				{"code": "bri", "strategy": "sum", "children": ["inhibit", "self_monitor"]},
				{"code": "gec", "strategy": "sum", "children": ["bri"]}
			]
		}
	}`)
	got, err := snapshot.ParsePublishedPayload(
		"assessmentmodel.behavioral_rating.brief2.v1",
		"BR-004", "v1", "BRIEF-2", "published", raw,
	)
	if err != nil {
		t.Fatalf("ParsePublishedPayload: %v", err)
	}
	byCode := factor.IndexByCode(got.Factors)
	if byCode["bri"].ChildrenPolicy == nil || len(byCode["bri"].ChildrenPolicy.Children) != 2 {
		t.Fatalf("bri policy = %#v", byCode["bri"].ChildrenPolicy)
	}
	if byCode["inhibit"].ParentCode != "bri" {
		t.Fatalf("inhibit parent = %q", byCode["inhibit"].ParentCode)
	}
}

func TestParseBrief2PayloadAnnotatesFactorNormMetadata(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
		"dimensions": [
			{"code": "bri", "title": "BRI", "question_codes": ["q1"], "scoring_strategy": "sum"},
			{"code": "gec", "title": "GEC", "question_codes": ["q2"], "scoring_strategy": "sum"}
		],
		"brief2": {
			"norm_table_version": "2024",
			"index_codes": ["bri", "gec"],
			"norms": [{"factor_code": "gec", "lookup": [{"raw_min": 0, "raw_max": 8, "t_score": 45, "percentile": 30}]}]
		}
	}`)
	got, err := snapshot.ParsePublishedPayload(
		"assessmentmodel.behavioral_rating.brief2.v1",
		"BR-003", "v1", "BRIEF-2", "published", raw,
	)
	if err != nil {
		t.Fatalf("ParsePublishedPayload: %v", err)
	}
	if len(got.Factors) != 2 {
		t.Fatalf("factors = %#v", got.Factors)
	}
	if got.Factors[0].ResolvedRole() != factor.FactorRoleIndex {
		t.Fatalf("bri role = %s", got.Factors[0].ResolvedRole())
	}
	if got.Factors[1].Norm == nil || got.Factors[1].Norm.NormTableVersion != "2024" {
		t.Fatalf("gec norm = %#v", got.Factors[1].Norm)
	}
}

func TestParseBrief2PayloadLegacyPrimaryDimensionFallback(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
		"dimensions": [{"code": "gec", "title": "GEC", "question_codes": ["q1"], "scoring_strategy": "sum"}],
		"brief2": {
			"norm_table_version": "2024",
			"norms": [{"factor_code": "gec", "lookup": [{"raw_min": 0, "raw_max": 8, "t_score": 45, "percentile": 30}]}]
		}
	}`)
	got, err := snapshot.ParsePublishedPayload(
		"assessmentmodel.behavioral_rating.brief2.v1",
		"BR-LEGACY", "v1", "BRIEF-2", "published", raw,
	)
	if err != nil {
		t.Fatalf("ParsePublishedPayload: %v", err)
	}
	if got.Norming == nil || got.Norming.PrimaryDimensionCode != "gec" {
		t.Fatalf("primary dimension = %#v, want legacy gec fallback", got.Norming)
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
	tables := got.Norming.NormTablesOrNil()
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
