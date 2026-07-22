package evaluationinput

import (
	"context"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/task_performance"
	evaldescriptor "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime/descriptor"
	appdefinition "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/publication"
	questionnaireapp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
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

func TestPublishedCognitiveSPMImportToOutcomeUsesFrozenNormAndIdentity(t *testing.T) {
	t.Parallel()

	standardScore := 110.0
	table := &norm.Norm{
		TableVersion: "spm-cn-2024", FormVariant: "standard",
		Kind: domain.KindCognitive, Algorithm: domain.AlgorithmSPM,
		Factors: []norm.FactorTable{{FactorCode: "total", Lookup: []norm.LookupEntry{
			{
				RawScoreMin: 1, RawScoreMax: 1,
				MinAgeMonths: 60, MaxAgeMonths: 95, Gender: "female",
				TScore: 60, Percentile: 75, StandardScore: &standardScore,
			},
		}}},
	}
	if err := norm.ValidateImport(table); err != nil {
		t.Fatalf("ValidateImport: %v", err)
	}

	model := cognitiveSPMModel(table.TableVersion)
	questionnaire := &questionnaireapp.QuestionnaireResult{
		Code: "Q-COG", Version: "1", Status: "published",
		Questions: []questionnaireapp.QuestionResult{{
			Code: "q1", Type: "radio", Options: []questionnaireapp.OptionResult{{Value: "A"}, {Value: "B"}},
		}},
	}
	handler := appdefinition.CognitiveDefinitionHandler{
		NormRepo:           stubNormRepository{tables: []*norm.Norm{table}},
		QuestionnaireQuery: cognitiveQuestionnaireQuery{result: questionnaire},
	}
	if issues := handler.ValidateForPublish(context.Background(), model); domain.HasValidationErrors(issues) {
		t.Fatalf("ValidateForPublish issues = %#v", issues)
	}
	publisher := publication.Publisher{Registry: appdefinition.NewRegistry(handler)}
	published, err := publisher.BuildSnapshot(context.Background(), model)
	if err != nil {
		t.Fatalf("BuildSnapshot: %v", err)
	}
	if published.Algorithm != domain.AlgorithmSPM || published.DecisionKind != domain.DecisionKindAbilityLevel {
		t.Fatalf("published identity = kind=%q algorithm=%q family=%q decision=%q", published.Kind, published.Algorithm, published.AlgorithmFamily, published.DecisionKind)
	}

	reader := stubPublishedCognitiveReader{snapshot: published}
	birthday := time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC)
	provider := NewCognitiveModelInputProvider(
		domain.AlgorithmSPM,
		NewPublishedCognitiveCatalog(reader, stubNormRepository{tables: []*norm.Norm{table}}),
		reader, cognitiveAnswerSheetReader{}, cognitiveQuestionnaireReader{},
		retainedNormSubjectReader{facts: &port.NormSubjectFacts{Gender: "female", Birthday: &birthday}},
	)
	input, err := provider.ResolveInput(context.Background(), port.InputRef{
		AnswerSheetID: 1, TesteeID: 7, AsOf: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		ModelRef: port.ModelRef{
			Kind: port.EvaluationModelKindCognitive, Algorithm: string(domain.AlgorithmSPM),
			Code: published.Code, Version: published.Version,
		},
	})
	if err != nil {
		t.Fatalf("ResolveInput: %v", err)
	}
	if input.DefinitionV2 != published.DefinitionV2 {
		t.Fatal("provider did not attach the exact published DefinitionV2")
	}
	identity, ok := port.NewInputSnapshotIdentity(input)
	if !ok || !port.IsIdentityRef(identity.Ref()) {
		t.Fatalf("input identity = %#v, ref = %q", identity, identity.Ref())
	}

	modelRef := assessment.NewEvaluationModelRefWithIdentity(
		domain.KindCognitive, domain.SubKindEmpty, domain.AlgorithmSPM,
		meta.ID(0), meta.NewCode(published.Code), published.Version, published.Title,
	)
	currentAssessment, err := assessment.NewAssessment(
		1, testee.NewID(7), assessment.NewQuestionnaireRefByCode(meta.NewCode("Q-COG"), "1"),
		assessment.NewAnswerSheetRef(meta.FromUint64(1)), assessment.NewAdhocOrigin(),
		assessment.WithEvaluationModel(modelRef),
	)
	if err != nil {
		t.Fatalf("NewAssessment: %v", err)
	}
	if err := currentAssessment.Submit(); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	components := task_performance.NewPipelineComponents(nil)
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
	outcome, ok := assembled.(*domainoutcome.Execution)
	if !ok || outcome == nil || len(outcome.Dimensions) == 0 {
		t.Fatalf("Outcome = %#v", assembled)
	}
	dimension := outcome.Dimensions[len(outcome.Dimensions)-1]
	if dimension.NormReference == nil || dimension.NormReference.TableVersion != "spm-cn-2024" ||
		dimension.NormReference.FormVariant != "standard" || dimension.NormReference.MinAgeMonths != 60 ||
		dimension.NormReference.MaxAgeMonths != 95 || dimension.NormReference.Gender != "female" {
		t.Fatalf("NormReference = %#v", dimension.NormReference)
	}
	if dimension.Level == nil || dimension.Level.Code != "above_average" {
		t.Fatalf("Level = %#v, want above_average", dimension.Level)
	}
	if !hasDerivedScore(dimension.DerivedScores, domainoutcome.ScoreKindStandardScore, standardScore) {
		t.Fatalf("DerivedScores = %#v, want standard score %.0f", dimension.DerivedScores, standardScore)
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

func cognitiveSPMModel(normVersion string) *domain.AssessmentModel {
	return &domain.AssessmentModel{
		Kind: domain.KindCognitive, Algorithm: domain.AlgorithmSPM, Code: "COG-SPM-E2E", Version: 1,
		Title: "SPM end-to-end", Status: domain.ModelStatusPublished,
		Binding: domain.QuestionnaireBinding{QuestionnaireCode: "Q-COG", QuestionnaireVersion: "1"},
		DefinitionV2: &modeldefinition.Definition{
			Measure: modeldefinition.MeasureSpec{
				Factors:     []factor.Factor{{Code: "total", Title: "Total", Role: factor.FactorRoleTotal}},
				FactorGraph: factor.FactorGraph{Roots: []string{"total"}},
				Scoring: []factor.Scoring{{
					FactorCode: "total", Strategy: factor.ScoringStrategySum,
					Sources: []factor.ScoringSource{{Kind: factor.ScoringSourceQuestion, Code: "q1", Sign: 1, Weight: 1}},
				}},
			},
			Calibration: modeldefinition.Calibration{NormRefs: []norm.Ref{{FactorCode: "total", NormTableVersion: normVersion}}},
			Execution: modeldefinition.ExecutionSpec{SPM: &modeldefinition.SPMSpec{
				TimeLimitSeconds: 900, TotalFactorCode: "total",
				ItemSets: []modeldefinition.SPMItemSet{{
					Code: "A", Items: []modeldefinition.SPMItem{{QuestionCode: "q1", CorrectOptionCode: "A"}},
				}},
			}},
			Outcomes: []conclusion.Outcome{{Code: "above_average", Title: "Above average"}},
			Conclusions: []conclusion.Conclusion{conclusion.AbilityConclusion{
				FactorCode: "total", ScoreBasis: conclusion.ScoreBasisStandardScore, Primary: true,
				Rules: []conclusion.ScoreRangeOutcome{{
					MinScore: 100, MaxScore: 120, MaxInclusive: true,
					OutcomeCode: "above_average", Level: "above_average",
				}},
			}},
		},
	}
}

func hasDerivedScore(scores []domainoutcome.ScoreValue, kind domainoutcome.ScoreKind, value float64) bool {
	for _, score := range scores {
		if score.Kind == kind && score.Value == value {
			return true
		}
	}
	return false
}

type cognitiveQuestionnaireQuery struct {
	questionnaireapp.QuestionnaireQueryService
	result *questionnaireapp.QuestionnaireResult
}

func (s cognitiveQuestionnaireQuery) GetPublishedByCodeVersion(context.Context, string, string) (*questionnaireapp.QuestionnaireResult, error) {
	return s.result, nil
}

type cognitiveAnswerSheetReader struct{}

func (cognitiveAnswerSheetReader) GetAnswerSheet(context.Context, uint64) (*port.AnswerSheetSnapshot, error) {
	return &port.AnswerSheetSnapshot{
		ID: 1, QuestionnaireCode: "Q-COG", QuestionnaireVersion: "1",
		Answers: []port.AnswerSnapshot{{QuestionCode: "q1", Value: "A"}},
	}, nil
}

type cognitiveQuestionnaireReader struct{}

func (cognitiveQuestionnaireReader) GetQuestionnaire(context.Context, string, string) (*port.QuestionnaireSnapshot, error) {
	return &port.QuestionnaireSnapshot{
		Code: "Q-COG", Version: "1",
		Questions: []port.QuestionSnapshot{{
			Code: "q1", Type: "radio", Options: []port.OptionSnapshot{{Code: "A"}, {Code: "B"}},
		}},
	}, nil
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
