package evaluationinput

import (
	"context"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norm"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

func TestPublishedBehavioralRatingCatalogDecodesPublishedModel(t *testing.T) {
	t.Parallel()

	reader := stubPublishedBehavioralRatingReader{snapshot: &rulesetport.PublishedModel{
		SchemaVersion:        domain.SchemaVersionV2,
		Kind:                 domain.KindBehavioralRating,
		Algorithm:            domain.AlgorithmBrief2,
		Code:                 "BR-001",
		Version:              "1.0.0",
		Title:                "行为评分",
		Status:               "published",
		QuestionnaireCode:    "Q-001",
		QuestionnaireVersion: "1.0.0",
		DefinitionV2:         behavioralDefinition(false),
	}}
	catalog := NewPublishedBehavioralRatingCatalog(reader)
	got, err := catalog.GetBehavioralRatingByRef(context.Background(), port.ModelRef{
		Kind:      port.EvaluationModelKindBehavioralRating,
		Algorithm: string(domain.AlgorithmBrief2),
		Code:      "BR-001",
		Version:   "1.0.0",
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

	definition := behavioralDefinition(true)
	table := &norm.Norm{
		Kind: domain.KindBehavioralRating, Algorithm: domain.AlgorithmBrief2,
		TableVersion: "2024", FormVariant: "parent",
		Factors: []norm.FactorTable{{FactorCode: "gec", Lookup: []norm.LookupEntry{{RawScoreMin: 0, RawScoreMax: 10, TScore: 50, Percentile: 50}}}},
	}
	reader := stubPublishedBehavioralRatingReader{snapshot: &rulesetport.PublishedModel{
		SchemaVersion: domain.SchemaVersionV2,
		Kind:          domain.KindBehavioralRating,
		Algorithm:     domain.AlgorithmBrief2,
		Code:          "BR-BRIEF2",
		Version:       "1.0.0",
		Title:         "BRIEF-2",
		Status:        "published",
		DefinitionV2:  definition,
	}}
	catalog := NewPublishedBehavioralRatingCatalog(reader, stubNormRepository{tables: []*norm.Norm{table}})
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

func TestBehavioralRatingLookupRefsExactAlgorithmOnly(t *testing.T) {
	t.Parallel()
	refs, err := behavioralRatingLookupRefs(port.ModelRef{
		Kind: port.EvaluationModelKindBehavioralRating, Code: "BR", Version: "1",
		Algorithm: string(domain.AlgorithmBrief2),
	})
	if err != nil {
		t.Fatalf("behavioralRatingLookupRefs: %v", err)
	}
	if len(refs) != 1 || refs[0].Algorithm != domain.AlgorithmBrief2 {
		t.Fatalf("refs = %#v", refs)
	}
}

func TestBehavioralRatingLookupRefsRejectsEmptyAlgorithm(t *testing.T) {
	t.Parallel()
	if _, err := behavioralRatingLookupRefs(port.ModelRef{
		Kind: port.EvaluationModelKindBehavioralRating, Code: "BR", Version: "1",
	}); err == nil {
		t.Fatal("expected empty algorithm rejection")
	}
}

func TestPublishedBehavioralRatingCatalogRejectsMismatchedPublishedAlgorithm(t *testing.T) {
	t.Parallel()
	mismatched := &rulesetport.PublishedModel{
		SchemaVersion: domain.SchemaVersionV2,
		Kind:          domain.KindBehavioralRating,
		Algorithm:     domain.AlgorithmSPM,
		Code:          "BR-ALIAS",
		Version:       "1.0.0",
		Status:        "published",
		DefinitionV2:  behavioralDefinition(false),
	}
	reader := stubPublishedBehavioralRatingReader{byAlgorithm: map[domain.Algorithm]*rulesetport.PublishedModel{
		domain.AlgorithmBrief2: mismatched,
	}}
	catalog := NewPublishedBehavioralRatingCatalog(reader)
	if _, err := catalog.GetBehavioralRatingByRef(context.Background(), port.ModelRef{
		Kind:      port.EvaluationModelKindBehavioralRating,
		Algorithm: string(domain.AlgorithmBrief2),
		Code:      "BR-ALIAS",
		Version:   "1.0.0",
	}); err == nil {
		t.Fatal("mismatched published algorithm must fail closed")
	}
}

func behavioralDefinition(withNorm bool) *modeldefinition.Definition {
	factorCode := "total"
	if withNorm {
		factorCode = "gec"
	}
	def := &modeldefinition.Definition{
		Measure: modeldefinition.MeasureSpec{
			Factors:     []factor.Factor{{Code: factorCode, Title: "总分", Role: factor.FactorRoleTotal}},
			FactorGraph: factor.FactorGraph{Roots: []string{factorCode}},
			Scoring:     []factor.Scoring{{FactorCode: factorCode, Sources: []factor.ScoringSource{{Kind: factor.ScoringSourceQuestion, Code: "q1"}}, Strategy: factor.ScoringStrategySum}},
		},
		Execution: modeldefinition.ExecutionSpec{Brief2: &modeldefinition.Brief2Spec{PrimaryFactorCode: factorCode}},
	}
	if withNorm {
		def.Calibration.NormRefs = []norm.Ref{{FactorCode: factorCode, NormTableVersion: "2024"}}
		def.Conclusions = []conclusion.Conclusion{conclusion.NormConclusion{FactorCode: factorCode, ScoreBasis: conclusion.ScoreBasisTScore, Primary: true}}
	}
	return def
}

type stubNormRepository struct {
	tables []*norm.Norm
}

func (s stubNormRepository) UpsertNorm(context.Context, *norm.Norm) error { return nil }

func (s stubNormRepository) ListNorms(context.Context, rulesetport.NormListFilter) ([]*norm.Norm, int64, error) {
	return s.tables, int64(len(s.tables)), nil
}

func (s stubNormRepository) FindNorm(_ context.Context, version string) (*norm.Norm, error) {
	for _, table := range s.tables {
		if table != nil && table.TableVersion == version {
			return table, nil
		}
	}
	return nil, domain.ErrNotFound
}

type stubPublishedBehavioralRatingReader struct {
	snapshot    *rulesetport.PublishedModel
	byAlgorithm map[domain.Algorithm]*rulesetport.PublishedModel
	err         error
}

func (s stubPublishedBehavioralRatingReader) GetPublishedModelByRef(_ context.Context, ref rulesetport.Ref) (*rulesetport.PublishedModel, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.byAlgorithm != nil {
		if snap, ok := s.byAlgorithm[ref.Algorithm]; ok {
			return snap, nil
		}
		return nil, domain.ErrNotFound
	}
	return s.snapshot, nil
}

func (s stubPublishedBehavioralRatingReader) FindPublishedModelByQuestionnaire(context.Context, string, string) (*rulesetport.PublishedModel, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.snapshot, nil
}
