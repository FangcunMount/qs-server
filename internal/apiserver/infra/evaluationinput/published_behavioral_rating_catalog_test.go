package evaluationinput

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/norming"
	evaldescriptor "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime/descriptor"
	appdefinition "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/publication"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	calcnorm "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/norm"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norm"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
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

func TestPublishedBehavioralRatingRetainedReleaseReplaysExactNormToOutcome(t *testing.T) {
	t.Parallel()

	for _, algorithm := range []domain.Algorithm{domain.AlgorithmBrief2, domain.AlgorithmSPMSensory} {
		algorithm := algorithm
		t.Run(string(algorithm), func(t *testing.T) {
			t.Parallel()

			v1Table := retainedBehavioralNorm(algorithm, "norm-v1", 55)
			v2Table := retainedBehavioralNorm(algorithm, "norm-v2", 75)
			for _, table := range []*norm.Norm{v1Table, v2Table} {
				if err := norm.ValidateImport(table); err != nil {
					t.Fatalf("ValidateImport(%s): %v", table.TableVersion, err)
				}
			}

			handler := appdefinition.BehavioralRatingDefinitionHandler{NormRepo: stubNormRepository{tables: []*norm.Norm{v1Table, v2Table}}}
			publisher := publication.Publisher{Registry: appdefinition.NewRegistry(handler)}
			v1, err := publisher.BuildSnapshot(context.Background(), retainedBehavioralModel(algorithm, 1, v1Table.TableVersion))
			if err != nil {
				t.Fatalf("BuildSnapshot(v1): %v", err)
			}
			v2, err := publisher.BuildSnapshot(context.Background(), retainedBehavioralModel(algorithm, 2, v2Table.TableVersion))
			if err != nil {
				t.Fatalf("BuildSnapshot(v2): %v", err)
			}
			reader := retainedBehavioralReader{snapshots: map[string]*rulesetport.PublishedModel{
				retainedSnapshotKey(v1): v1,
				retainedSnapshotKey(v2): v2,
			}}
			catalog := NewPublishedBehavioralRatingCatalog(reader, stubNormRepository{tables: []*norm.Norm{v1Table, v2Table}})

			// v2 exists and is the newer release, but an already-created v1
			// Assessment must still materialize v1 and its exact Norm table.
			birthday := time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC)
			provider := NewBehavioralRatingModelInputProvider(
				algorithm, catalog, reader,
				retainedAnswerSheetReader{}, retainedQuestionnaireReader{},
				retainedNormSubjectReader{facts: &port.NormSubjectFacts{Gender: "female", Birthday: &birthday}},
			)
			input, err := provider.ResolveInput(context.Background(), port.InputRef{
				AnswerSheetID: 1, TesteeID: 7, AsOf: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				ModelRef: port.ModelRef{
					Kind: port.EvaluationModelKindBehavioralRating, Algorithm: string(algorithm), Code: v1.Code, Version: v1.Version,
				},
			})
			if err != nil {
				t.Fatalf("ResolveInput(v1): %v", err)
			}
			payload, ok := port.BehavioralRatingPayload(input)
			if !ok || payload.Snapshot == nil {
				t.Fatalf("behavioral payload = %#v", input.ModelPayload)
			}
			got := payload.Snapshot
			if got.Version != "v1" || got.Norming == nil || got.Norming.NormTableVersion != "norm-v1" {
				t.Fatalf("retained runtime = %#v, want v1/norm-v1", got)
			}
			if input.DefinitionV2 != v1.DefinitionV2 {
				t.Fatal("provider did not attach the exact retained DefinitionV2")
			}

			assessmentModelRef := assessment.NewEvaluationModelRefWithIdentity(
				domain.KindBehavioralRating, domain.SubKindEmpty, algorithm, meta.ID(0), meta.NewCode(v1.Code), v1.Version, v1.Title,
			)
			currentAssessment, err := assessment.NewAssessment(
				1, testee.NewID(7),
				assessment.NewQuestionnaireRefByCode(meta.NewCode("Q-BEH"), "1"),
				assessment.NewAnswerSheetRef(meta.FromUint64(1)), assessment.NewAdhocOrigin(),
				assessment.WithEvaluationModel(assessmentModelRef),
			)
			if err != nil {
				t.Fatalf("NewAssessment: %v", err)
			}
			if err := currentAssessment.Submit(); err != nil {
				t.Fatalf("Submit: %v", err)
			}
			components := norming.NewPipelineComponents(nil)
			calculationInput, err := components.InputAssembler.Assemble(evaldescriptor.ExecutionInput{
				Assessment: currentAssessment, Input: input,
			})
			if err != nil {
				t.Fatalf("InputAssembler: %v", err)
			}
			calculated, err := components.Calculator.Calculate(context.Background(), calculationInput)
			if err != nil {
				t.Fatalf("Calculator: %v", err)
			}
			assembled, err := components.OutcomeAssembler.Assemble(calculated)
			if err != nil {
				t.Fatalf("OutcomeAssembler: %v", err)
			}
			projected, ok := assembled.(*domainoutcome.Execution)
			if !ok {
				t.Fatalf("Outcome = %T, want *outcome.Execution", assembled)
			}
			dimension := projected.Dimensions[len(projected.Dimensions)-1]
			if dimension.NormReference == nil || dimension.NormReference.TableVersion != "norm-v1" || dimension.NormReference.FormVariant != "parent" {
				t.Fatalf("NormReference = %#v, want retained norm-v1/parent", dimension.NormReference)
			}
			if dimension.Level == nil || dimension.Level.Code != "normal" {
				t.Fatalf("Level = %#v, want normal from v1 T score", dimension.Level)
			}
		})
	}
}

func TestPublishedBehavioralRatingMissingNormSubjectProducesNoPartialOutcome(t *testing.T) {
	t.Parallel()

	table := retainedBehavioralNorm(domain.AlgorithmBrief2, "norm-v1", 55)
	if err := norm.ValidateImport(table); err != nil {
		t.Fatalf("ValidateImport: %v", err)
	}
	handler := appdefinition.BehavioralRatingDefinitionHandler{NormRepo: stubNormRepository{tables: []*norm.Norm{table}}}
	publisher := publication.Publisher{Registry: appdefinition.NewRegistry(handler)}
	published, err := publisher.BuildSnapshot(context.Background(), retainedBehavioralModel(domain.AlgorithmBrief2, 1, table.TableVersion))
	if err != nil {
		t.Fatalf("BuildSnapshot: %v", err)
	}
	reader := retainedBehavioralReader{snapshots: map[string]*rulesetport.PublishedModel{retainedSnapshotKey(published): published}}
	provider := NewBehavioralRatingModelInputProvider(
		domain.AlgorithmBrief2,
		NewPublishedBehavioralRatingCatalog(reader, stubNormRepository{tables: []*norm.Norm{table}}),
		reader, retainedAnswerSheetReader{}, retainedQuestionnaireReader{},
		retainedNormSubjectReader{facts: &port.NormSubjectFacts{Gender: "female"}},
	)
	input, err := provider.ResolveInput(context.Background(), port.InputRef{
		AnswerSheetID: 1, TesteeID: 7, AsOf: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		ModelRef: port.ModelRef{
			Kind: port.EvaluationModelKindBehavioralRating, Algorithm: string(domain.AlgorithmBrief2),
			Code: published.Code, Version: published.Version,
		},
	})
	if err != nil {
		t.Fatalf("ResolveInput: %v", err)
	}
	modelRef := assessment.NewEvaluationModelRefWithIdentity(
		domain.KindBehavioralRating, domain.SubKindEmpty, domain.AlgorithmBrief2,
		meta.ID(0), meta.NewCode(published.Code), published.Version, published.Title,
	)
	currentAssessment, err := assessment.NewAssessment(
		1, testee.NewID(7), assessment.NewQuestionnaireRefByCode(meta.NewCode("Q-BEH"), "1"),
		assessment.NewAnswerSheetRef(meta.FromUint64(1)), assessment.NewAdhocOrigin(), assessment.WithEvaluationModel(modelRef),
	)
	if err != nil {
		t.Fatalf("NewAssessment: %v", err)
	}
	if err := currentAssessment.Submit(); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	components := norming.NewPipelineComponents(nil)
	calculationInput, err := components.InputAssembler.Assemble(evaldescriptor.ExecutionInput{Assessment: currentAssessment, Input: input})
	if err != nil {
		t.Fatalf("InputAssembler: %v", err)
	}
	calculated, err := components.Calculator.Calculate(context.Background(), calculationInput)
	if err != nil {
		t.Fatalf("Calculator: %v", err)
	}
	assembled, err := components.OutcomeAssembler.Assemble(calculated)
	if err == nil {
		t.Fatalf("OutcomeAssembler = %#v, nil error; want missing subject failure", assembled)
	}
	if partial, ok := assembled.(*domainoutcome.Execution); !ok || partial != nil {
		t.Fatalf("partial Outcome = %#v, want typed nil", assembled)
	}
	kind, ok := calcnorm.ErrorKindOf(err)
	if !ok || kind != calcnorm.ErrorKindSubjectMissing {
		t.Fatalf("error = %v, kind = %q; want norm_subject_missing", err, kind)
	}
}

func retainedBehavioralNorm(algorithm domain.Algorithm, version string, tScore float64) *norm.Norm {
	return &norm.Norm{
		TableVersion: version, FormVariant: "parent", Kind: domain.KindBehavioralRating, Algorithm: algorithm,
		Factors: []norm.FactorTable{{FactorCode: "total", Lookup: []norm.LookupEntry{{
			RawScoreMin: 10, RawScoreMax: 10, TScore: tScore, Percentile: 50,
			MinAgeMonths: 60, MaxAgeMonths: 95, Gender: "female",
		}}}},
	}
}

func retainedBehavioralModel(algorithm domain.Algorithm, revision int64, normVersion string) *domain.AssessmentModel {
	execution := modeldefinition.ExecutionSpec{}
	if algorithm == domain.AlgorithmBrief2 {
		execution.Brief2 = &modeldefinition.Brief2Spec{FormVariant: "parent", PrimaryFactorCode: "total"}
	}
	return &domain.AssessmentModel{
		Kind: domain.KindBehavioralRating, Algorithm: algorithm, Code: "behavioral-retained", Version: revision,
		Title: "Behavioral retained", Status: domain.ModelStatusPublished,
		Binding: domain.QuestionnaireBinding{QuestionnaireCode: "Q-BEH", QuestionnaireVersion: "1"},
		DefinitionV2: &modeldefinition.Definition{
			Measure: modeldefinition.MeasureSpec{
				Factors:     []factor.Factor{{Code: "total", Title: "Total", Role: factor.FactorRoleTotal}},
				FactorGraph: factor.FactorGraph{Roots: []string{"total"}},
				Scoring: []factor.Scoring{{FactorCode: "total", Sources: []factor.ScoringSource{{
					Kind: factor.ScoringSourceQuestion, Code: "q1", Sign: 1, Weight: 1, ScoringMode: factor.QuestionScoringModeQuestionScore,
				}}, Strategy: factor.ScoringStrategySum}},
			},
			Calibration: modeldefinition.Calibration{NormRefs: []norm.Ref{{FactorCode: "total", NormTableVersion: normVersion}}},
			Execution:   execution,
			Outcomes: []conclusion.Outcome{
				{Code: "normal", Title: "Normal"}, {Code: "elevated", Title: "Elevated"},
			},
			Conclusions: []conclusion.Conclusion{conclusion.NormConclusion{
				FactorCode: "total", ScoreBasis: conclusion.ScoreBasisTScore, Primary: true,
				Rules: []conclusion.ScoreRangeOutcome{
					{MinScore: 0, MaxScore: 60, OutcomeCode: "normal", Level: "normal"},
					{MinScore: 60, MaxScore: 100, MaxInclusive: true, OutcomeCode: "elevated", Level: "elevated"},
				},
			}},
		},
	}
}

func retainedSnapshotKey(snapshot *rulesetport.PublishedModel) string {
	return fmt.Sprintf("%s/%s/%s/%s", snapshot.Kind, snapshot.Algorithm, snapshot.Code, snapshot.Version)
}

type retainedBehavioralReader struct {
	snapshots map[string]*rulesetport.PublishedModel
}

type retainedAnswerSheetReader struct{}

func (retainedAnswerSheetReader) GetAnswerSheet(context.Context, uint64) (*port.AnswerSheetSnapshot, error) {
	return &port.AnswerSheetSnapshot{
		ID: 1, QuestionnaireCode: "Q-BEH", QuestionnaireVersion: "1",
		Answers: []port.AnswerSnapshot{{QuestionCode: "q1", Score: 10}},
	}, nil
}

type retainedQuestionnaireReader struct{}

func (retainedQuestionnaireReader) GetQuestionnaire(context.Context, string, string) (*port.QuestionnaireSnapshot, error) {
	return &port.QuestionnaireSnapshot{Code: "Q-BEH", Version: "1"}, nil
}

type retainedNormSubjectReader struct {
	facts *port.NormSubjectFacts
}

func (r retainedNormSubjectReader) ReadNormSubjectFacts(context.Context, uint64) (*port.NormSubjectFacts, error) {
	return r.facts, nil
}

func (r retainedBehavioralReader) GetPublishedModelByRef(_ context.Context, ref rulesetport.Ref) (*rulesetport.PublishedModel, error) {
	key := fmt.Sprintf("%s/%s/%s/%s", ref.Kind, ref.Algorithm, ref.Code, ref.Version)
	if snapshot := r.snapshots[key]; snapshot != nil {
		return snapshot, nil
	}
	return nil, domain.ErrNotFound
}

func (r retainedBehavioralReader) FindPublishedModelByQuestionnaire(context.Context, string, string) (*rulesetport.PublishedModel, error) {
	return nil, domain.ErrNotFound
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
