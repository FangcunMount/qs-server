//go:build integration

package evaluation_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	evaluationexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	evalruntime "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	calcnorm "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/norm"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norm"
	inputinfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/evaluationinput"
	mongocatalog "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/ruleengine"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	catalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/mongodbtest"
	"go.mongodb.org/mongo-driver/bson"
)

func TestM5RealMongoNormRuntimeBehavioralRetainedVersion(t *testing.T) {
	_, db := mongodbtest.ReplicaSetDatabase(t)
	ctx := t.Context()
	models, norms := mongocatalog.NewRepository(db), mongocatalog.NewNormRepository(db)
	catalog := inputinfra.NewPublishedBehavioralRatingCatalog(models, norms)
	hashes := make(map[string]string)
	for _, tc := range []struct {
		version, table, outcome string
		tScore, percentile      float64
	}{{"1.0.0", "mongo-brief-v1", "low", 60, 84}, {"2.0.0", "mongo-brief-v2", "high", 75, 99}} {
		table := &norm.Norm{TableVersion: tc.table, Kind: modelcatalog.KindBehavioralRating, Algorithm: modelcatalog.AlgorithmBrief2, FormVariant: "parent", Factors: []norm.FactorTable{{FactorCode: "total", Lookup: []norm.LookupEntry{{RawScoreMin: 0, RawScoreMax: 20, MinAgeMonths: 60, MaxAgeMonths: 143, Gender: "female", TScore: tc.tScore, Percentile: tc.percentile}}}}}
		if err := norm.ValidateImport(table); err != nil {
			t.Fatal(err)
		}
		if err := norms.UpsertNorm(ctx, table); err != nil {
			t.Fatal(err)
		}
		def := mongoNormDefinition(tc.table)
		def.Execution.Brief2 = &definition.Brief2Spec{FormVariant: "parent", PrimaryFactorCode: "total", IndexFactorCodes: []string{"total"}}
		def.Conclusions = []conclusion.Conclusion{conclusion.NormConclusion{FactorCode: "total", Primary: true, ScoreBasis: conclusion.ScoreBasisTScore, Rules: nativeRules(70)}}
		hashes[tc.version] = mongoNormSaveModel(t, models, modelcatalog.KindBehavioralRating, modelcatalog.AlgorithmBrief2, tc.version, def)
	}
	// Both versions exist before lookup: retained v1 must not silently use v2.
	for _, tc := range []struct {
		version, table, outcome string
		tScore, percentile      float64
	}{{"2.0.0", "mongo-brief-v2", "high", 75, 99}, {"1.0.0", "mongo-brief-v1", "low", 60, 84}} {
		t.Run(tc.version, func(t *testing.T) {
			ref := evaluationinput.ModelRef{Kind: evaluationinput.EvaluationModelKindBehavioralRating, Algorithm: string(modelcatalog.AlgorithmBrief2), Code: "M5_MONGO_NORM", Version: tc.version}
			payload, err := catalog.GetBehavioralRatingByRef(ctx, ref)
			if err != nil {
				t.Fatal(err)
			}
			published, err := models.GetPublishedModelByRef(ctx, catalogport.Ref{Kind: modelcatalog.KindBehavioralRating, Algorithm: modelcatalog.AlgorithmBrief2, Code: ref.Code, Version: ref.Version})
			if err != nil {
				t.Fatal(err)
			}
			input := mongoNormInput(published.DefinitionV2, []string{"q1", "q2"}, []string{"b", "a"})
			input.Model = evaluationinput.NewBehavioralRatingModelSnapshot(payload, modelcatalog.AlgorithmBrief2)
			input.ModelPayload = evaluationinput.BehavioralRatingModelPayload{Snapshot: payload}
			got, err := mongoNormExecute(t, input)
			if err != nil {
				t.Fatal(err)
			}
			mongoNormAssertResult(t, got, 5, tc.table, "parent", tc.outcome, map[domainoutcome.ScoreKind]float64{domainoutcome.ScoreKindTScore: tc.tScore, domainoutcome.ScoreKindPercentile: tc.percentile})
			age := 180
			input.NormSubject.AgeMonths = &age
			result, err := mongoNormExecute(t, input)
			if kind, classified := calcnorm.ErrorKindOf(err); !classified || kind != calcnorm.ErrorKindCohortNotFound || result != nil {
				t.Fatalf("missing demographic cohort produced outcome=%#v err=%v", result, err)
			}
			age = 120
			mongoNormAssertFrozen(t, input, got, hashes[tc.version])
		})
	}
	// Simulate lost reference material only inside the disposable database.
	// A fresh catalog read must hit Mongo and fail, rather than reuse a fixture.
	if _, err := db.Collection((&mongocatalog.NormPO{}).CollectionName()).DeleteOne(ctx, bson.M{"table_version": "mongo-brief-v1"}); err != nil {
		t.Fatal(err)
	}
	_, err := catalog.GetBehavioralRatingByRef(ctx, evaluationinput.ModelRef{Kind: evaluationinput.EvaluationModelKindBehavioralRating, Algorithm: string(modelcatalog.AlgorithmBrief2), Code: "M5_MONGO_NORM", Version: "1.0.0"})
	if !errors.Is(err, modelcatalog.ErrNotFound) {
		t.Fatalf("missing stored norm = %v", err)
	}
}

func TestM5RealMongoNormRuntimeCognitiveFrozenAnswers(t *testing.T) {
	_, db := mongodbtest.ReplicaSetDatabase(t)
	ctx := t.Context()
	models, norms := mongocatalog.NewRepository(db), mongocatalog.NewNormRepository(db)
	table := &norm.Norm{TableVersion: "mongo-spm-v1", FormVariant: "standard", Kind: modelcatalog.KindCognitive, Algorithm: modelcatalog.AlgorithmSPM, Factors: []norm.FactorTable{{FactorCode: "total"}}}
	for raw, percentile := range []float64{10, 25, 75, 99} {
		table.Factors[0].Lookup = append(table.Factors[0].Lookup, norm.LookupEntry{RawScoreMin: float64(raw), RawScoreMax: float64(raw), MinAgeMonths: 60, MaxAgeMonths: 143, Gender: "female", Percentile: percentile})
	}
	if err := norm.ValidateImport(table); err != nil {
		t.Fatal(err)
	}
	if err := norms.UpsertNorm(ctx, table); err != nil {
		t.Fatal(err)
	}
	def := mongoNormDefinition(table.TableVersion)
	def.Measure.Scoring[0].Sources = []factor.ScoringSource{{Kind: factor.ScoringSourceQuestion, Code: "A1"}, {Kind: factor.ScoringSourceQuestion, Code: "A2"}, {Kind: factor.ScoringSourceQuestion, Code: "B1"}}
	def.Execution.SPM = &definition.SPMSpec{TimeLimitSeconds: 600, TotalFactorCode: "total", ItemSets: []definition.SPMItemSet{{Code: "A", Items: []definition.SPMItem{{QuestionCode: "A1", CorrectOptionCode: "b"}, {QuestionCode: "A2", CorrectOptionCode: "a"}}}, {Code: "B", Items: []definition.SPMItem{{QuestionCode: "B1", CorrectOptionCode: "b"}}}}}
	def.Conclusions = []conclusion.Conclusion{conclusion.AbilityConclusion{FactorCode: "total", ScoreBasis: conclusion.ScoreBasisPercentile, Primary: true, Rules: nativeRules(50)}}
	hash := mongoNormSaveModel(t, models, modelcatalog.KindCognitive, modelcatalog.AlgorithmSPM, "1.0.0", def)
	catalog := inputinfra.NewPublishedCognitiveCatalog(models, norms)
	ref := evaluationinput.ModelRef{Kind: evaluationinput.EvaluationModelKindCognitive, Algorithm: string(modelcatalog.AlgorithmSPM), Code: "M5_MONGO_NORM", Version: "1.0.0"}
	for _, tc := range []struct {
		name, outcome   string
		answers         []string
		raw, percentile float64
	}{{"correct", "high", []string{"b", "a", "b"}, 3, 99}, {"mixed", "low", []string{"b", "b", "a"}, 1, 25}, {"incorrect", "low", []string{"a", "b", "a"}, 0, 10}} {
		t.Run(tc.name, func(t *testing.T) {
			payload, err := catalog.GetCognitiveByRef(ctx, ref)
			if err != nil {
				t.Fatal(err)
			}
			published, err := models.GetPublishedModelByRef(ctx, catalogport.Ref{Kind: modelcatalog.KindCognitive, Algorithm: modelcatalog.AlgorithmSPM, Code: ref.Code, Version: ref.Version})
			if err != nil {
				t.Fatal(err)
			}
			input := mongoNormInput(published.DefinitionV2, []string{"A1", "A2", "B1"}, tc.answers)
			input.Model = evaluationinput.NewCognitiveModelSnapshot(payload)
			input.ModelPayload = evaluationinput.CognitiveModelPayload{Snapshot: payload}
			got, err := mongoNormExecute(t, input)
			if err != nil {
				t.Fatal(err)
			}
			mongoNormAssertResult(t, got, tc.raw, table.TableVersion, "standard", tc.outcome, map[domainoutcome.ScoreKind]float64{domainoutcome.ScoreKindPercentile: tc.percentile})
			for i := range input.AnswerSheet.Answers {
				input.AnswerSheet.Answers[i].Score = -999
			}
			if again, err := mongoNormExecute(t, input); err != nil || !reflect.DeepEqual(again, got) {
				t.Fatalf("SPM used client score instead of frozen answers: %v", err)
			}
			mongoNormAssertFrozen(t, input, got, hash)
		})
	}
	if _, err := db.Collection((&mongocatalog.NormPO{}).CollectionName()).DeleteOne(ctx, bson.M{"table_version": table.TableVersion}); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.GetCognitiveByRef(ctx, ref); !errors.Is(err, modelcatalog.ErrNotFound) {
		t.Fatalf("missing SPM stored norm = %v", err)
	}
}

func mongoNormDefinition(table string) *definition.Definition {
	def := nativeDefinition()
	// Existing BSON represents a graph with no edges as []; v1 hashes keep
	// null and [] distinct. Use that stored shape, never normalize old hashes.
	def.Measure.FactorGraph.Edges = []factor.FactorEdge{}
	def.Measure.Scoring[0].Sources = []factor.ScoringSource{
		{Kind: factor.ScoringSourceQuestion, Code: "q1", ScoringMode: factor.QuestionScoringModeOptionOverride, Sign: 1, Weight: 1, OptionScores: map[string]float64{"a": 2, "b": 3}},
		{Kind: factor.ScoringSourceQuestion, Code: "q2", ScoringMode: factor.QuestionScoringModeOptionOverride, Sign: 1, Weight: 1, OptionScores: map[string]float64{"a": 2, "b": 3}},
	}
	def.Calibration.NormRefs = []norm.Ref{{FactorCode: "total", NormTableVersion: table}}
	return def
}

func mongoNormSaveModel(t *testing.T, repo *mongocatalog.Repository, kind modelcatalog.Kind, algorithm modelcatalog.Algorithm, version string, def *definition.Definition) string {
	t.Helper()
	if issues := definition.Validate(*def); len(issues) != 0 {
		t.Fatalf("illegal canonical norm fixture: %#v", issues)
	}
	definition.MaterializeLayers(def)
	hash, err := definition.CanonicalContentHash(def)
	if err != nil {
		t.Fatal(err)
	}
	decision := modelcatalog.DecisionKindNormLookup
	if kind == modelcatalog.KindCognitive {
		decision = modelcatalog.DecisionKindAbilityLevel
	}
	model := &catalogport.PublishedModel{SchemaVersion: modelcatalog.SchemaVersionV2, Kind: kind, SubKind: modelcatalog.CanonicalSubKindFor(kind), Algorithm: algorithm, DecisionKind: decision, Code: "M5_MONGO_NORM", Version: version, Title: "Mongo-backed norm corpus", Status: "published", QuestionnaireCode: "M5_MONGO_Q", QuestionnaireVersion: "1.0.0", DefinitionV2: def}
	catalogport.AttachDefinitionHash(model, hash)
	if err := repo.Save(t.Context(), model); err != nil {
		t.Fatal(err)
	}
	return hash
}

func mongoNormInput(def *definition.Definition, questions, answers []string) *evaluationinput.InputSnapshot {
	age := 120
	input := &evaluationinput.InputSnapshot{Questionnaire: &evaluationinput.QuestionnaireSnapshot{Code: "M5_MONGO_Q", Version: "1.0.0"}, AnswerSheet: &evaluationinput.AnswerSheetSnapshot{ID: 7102, QuestionnaireCode: "M5_MONGO_Q", QuestionnaireVersion: "1.0.0"}, NormSubject: &evaluationinput.NormSubjectSnapshot{AgeMonths: &age, Gender: "female"}}
	for i, code := range questions {
		input.Questionnaire.Questions = append(input.Questionnaire.Questions, evaluationinput.QuestionSnapshot{Code: code, Type: "radio", Options: []evaluationinput.OptionSnapshot{{Code: "a", Score: 0}, {Code: "b", Score: 1}}})
		input.AnswerSheet.Answers = append(input.AnswerSheet.Answers, evaluationinput.AnswerSnapshot{QuestionCode: code, Value: answers[i], Score: 999})
	}
	evaluationinput.AttachCanonicalDefinition(input, def)
	return input
}

func mongoNormExecute(t *testing.T, input *evaluationinput.InputSnapshot) (*domainoutcome.Execution, error) {
	t.Helper()
	registry, err := evalruntime.DefaultRuntimeDescriptorRegistry()
	if err != nil {
		t.Fatal(err)
	}
	deps := evalruntime.WiringDeps{ScaleScorer: ruleengine.NewScaleFactorScorer()}
	if err := evalruntime.AttachNativePipelines(registry, evalruntime.NativePipelineDeps{ScaleScorer: evalruntime.MaterializeFactorScoringPipelineComponents(deps), FactorNorm: evalruntime.MaterializeFactorNormPipelineComponents(deps), TaskPerformance: evalruntime.MaterializeTaskPerformancePipelineComponents(deps), FactorClassification: evalruntime.MaterializeFactorClassificationPipelineComponents(deps)}); err != nil {
		t.Fatal(err)
	}
	kind := modelcatalog.Kind(input.Model.Kind)
	ref := assessment.NewEvaluationModelRefWithIdentity(kind, modelcatalog.CanonicalSubKindFor(kind), modelcatalog.Algorithm(input.Model.Algorithm), 0, meta.NewCode(input.Model.Code), input.Model.Version, input.Model.Title)
	a, err := assessment.NewAssessment(1, testee.NewID(1), assessment.NewQuestionnaireRefByCode(meta.NewCode(input.Questionnaire.Code), input.Questionnaire.Version), assessment.NewAnswerSheetRef(meta.FromUint64(input.AnswerSheet.ID)), assessment.NewAdhocOrigin(), assessment.WithEvaluationModel(ref))
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Submit(); err != nil {
		t.Fatal(err)
	}
	out, resolved, err := evaluationexecute.NewRuntimeResolver(registry, nativeScaleStages{}).Execute(t.Context(), a, input)
	if err == nil && string(resolved.DescriptorKey.DecisionKind) != input.Model.DecisionKind {
		t.Fatalf("native norm descriptor = %#v", resolved.DescriptorKey)
	}
	return out, err
}

func mongoNormAssertResult(t *testing.T, got *domainoutcome.Execution, raw float64, version, form, outcome string, scores map[domainoutcome.ScoreKind]float64) {
	t.Helper()
	if got == nil || got.Primary == nil || got.Primary.Value != raw || got.Level == nil || got.Level.Code != outcome {
		t.Fatalf("norm result = %#v, want raw=%v outcome=%s", got, raw, outcome)
	}
	var total *domainoutcome.DimensionResult
	for i := range got.Dimensions {
		if got.Dimensions[i].Code == "total" {
			total = &got.Dimensions[i]
		}
	}
	if total == nil || total.NormReference == nil || total.NormReference.TableVersion != version || total.NormReference.FormVariant != form || total.NormReference.MinAgeMonths != 60 || total.NormReference.MaxAgeMonths != 143 || total.NormReference.Gender != "female" {
		t.Fatalf("actual norm reference = %#v, expected exact stored version %s", total, version)
	}
	for kind, want := range scores {
		found := false
		for _, score := range total.DerivedScores {
			if score.Kind == kind && score.Value == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing derived %s=%v in %#v", kind, want, total.DerivedScores)
		}
	}
}

func mongoNormAssertFrozen(t *testing.T, input *evaluationinput.InputSnapshot, result *domainoutcome.Execution, hash string) {
	t.Helper()
	if after, err := definition.CanonicalContentHash(input.DefinitionV2); err != nil || after != hash {
		t.Fatalf("stored canonical norm definition hash = %s, want %s, err=%v", after, hash, err)
	}
	before, ok := evaluationinput.NewInputSnapshotIdentity(input)
	if !ok {
		t.Fatal("Mongo-backed norm input identity is incomplete")
	}
	frozen, err := evaluationinput.MarshalReportInput(evaluationinput.BuildFreezeOptionsFromSnapshot(input, input.Model.ModelRef(), modelcatalog.DecisionKind(input.Model.DecisionKind)))
	if err != nil {
		t.Fatal(err)
	}
	name := strings.ReplaceAll(t.Name(), "/", "_") + ".json"
	if dir := os.Getenv("QS_M5_NATIVE_NORM_ARTIFACT_DIR"); dir != "" {
		body, err := json.MarshalIndent(map[string]any{"case": t.Name(), "definition_hash": hash, "input_identity": before, "input": input, "calculation": result, "report_input": json.RawMessage(frozen)}, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, name)
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		_, writeErr := file.Write(body)
		err = errors.Join(writeErr, file.Sync(), file.Close())
		if err != nil {
			t.Fatal(err)
		}
	}
	// Keep the semantic identity/result/frozen-report corpus independent of the
	// public Definition representation, whose derived fields retire separately.
	actual, err := json.Marshal(map[string]any{"case": t.Name(), "definition_hash": hash, "input_identity": before, "calculation": result, "report_input": json.RawMessage(frozen)})
	if err != nil {
		t.Fatal(err)
	}
	wantGolden, err := os.ReadFile(filepath.Join("testdata", "m5_mongo_norm", name))
	if err != nil {
		t.Fatal(err)
	}
	var actualValue, expectedValue any
	if err := json.Unmarshal(actual, &actualValue); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(wantGolden, &expectedValue); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(actualValue, expectedValue) {
		t.Fatal("fixed native norm identity, outcome or frozen report input changed")
	}
	want, err := json.Marshal(input.InterpretationAssets)
	if err != nil {
		t.Fatal(err)
	}
	input.DefinitionV2.Outcomes[0].Summary = "later authoring text"
	replayed, err := evaluationinput.SnapshotFromReportInput(frozen, input.Model.ModelRef())
	if err != nil {
		t.Fatal(err)
	}
	got, err := json.Marshal(replayed.InterpretationAssets)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatal("report readback replaced frozen norm report assets")
	}
	if before.ModelDigest == "" || before.CompositeDigest == "" {
		t.Fatal("Mongo-backed norm digest missing")
	}
}
