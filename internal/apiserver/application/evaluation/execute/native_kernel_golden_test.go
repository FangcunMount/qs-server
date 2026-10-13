package execute

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"testing"

	outcometypology "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome/typology"
	factornorm "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/norming"
	factorscoring "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/scoring"
	taskperformance "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/task_performance"
	factorclassification "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/typology"
	evalruntime "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norm"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	behavioral "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/behavioral"
	cognitive "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/cognitive"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/normruntime"
	scale "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/scale"
	typology "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// This corpus executes the real native pipeline. It is separate from the
// immutable hash-v1 fixtures, which characterize serialization and replay only.
func TestM5NativeKernelGoldenFactorScoring(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		scores []float64
		want   float64
		code   string
	}{
		{"below", []float64{2, 2}, 4, "low"},
		{"fractional", []float64{2.25, 2.5}, 4.75, "low"},
		{"boundary", []float64{3, 2}, 5, "high"},
		{"unbounded", []float64{8, 4}, 12, "high"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := nativeScoreInput(t, modelcatalog.KindScale, modelcatalog.AlgorithmScaleDefault, modelcatalog.DecisionKindScoreRange)
			for i, score := range tc.scores {
				input.AnswerSheet.Answers[i].Score = score
			}
			got := nativeExecute(t, input)
			if got.Primary == nil || got.Primary.Value != tc.want || got.Level == nil || got.Level.Code != tc.code {
				t.Fatalf("score/OutcomeCode = %#v/%#v", got.Primary, got.Level)
			}
			if len(got.Dimensions) != 1 || got.Dimensions[0].Score.Value != tc.want || got.Dimensions[0].Level.Code != tc.code {
				t.Fatalf("dimensions = %#v", got.Dimensions)
			}
			nativeFreezeReplay(t, input)
		})
	}
}

func TestM5NativeKernelGoldenFactorNorm(t *testing.T) {
	t.Parallel()
	input := nativeScoreInput(t, modelcatalog.KindBehavioralRating, modelcatalog.AlgorithmBrief2, modelcatalog.DecisionKindNormLookup)
	input.AnswerSheet.Answers[0].Score, input.AnswerSheet.Answers[1].Score = 3, 2
	got := nativeExecute(t, input)
	if got.Primary == nil || got.Primary.Value != 5 || got.Level == nil || got.Level.Code != "high" {
		t.Fatalf("norm result = %#v/%#v", got.Primary, got.Level)
	}
	d := got.Dimensions[0]
	if d.NormReference == nil || d.NormReference.TableVersion != "m5-native-norm-v1" || d.NormReference.FormVariant != "parent" || d.NormReference.MinAgeMonths != 60 || d.NormReference.MaxAgeMonths != 143 || d.NormReference.Gender != "female" {
		t.Fatalf("norm reference = %#v", d.NormReference)
	}
	if len(d.DerivedScores) != 2 || d.DerivedScores[0].Value != 60 || d.DerivedScores[1].Value != 84 {
		t.Fatalf("derived norm scores = %#v", d.DerivedScores)
	}
	nativeFreezeReplay(t, input)
	age := 180
	input.NormSubject.AgeMonths = &age
	if out, _, err := nativeResolver(t).Execute(context.Background(), nativeAssessment(t, input), input); err == nil || out != nil {
		t.Fatalf("missing cohort returned outcome=%#v err=%v", out, err)
	}
}

func TestM5NativeKernelGoldenTaskPerformance(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name            string
		values          []string
		raw, percentile float64
		code            string
	}{
		{"correct", []string{"b", "a", "b"}, 3, 99, "high"},
		{"mixed", []string{"b", "b", "a"}, 1, 25, "low"},
		{"incorrect", []string{"a", "b", "a"}, 0, 10, "low"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := nativeSPMInput(t)
			for i, value := range tc.values {
				input.AnswerSheet.Answers[i].Value = value
				input.AnswerSheet.Answers[i].Score = 999
			}
			got := nativeExecute(t, input)
			if got.Primary == nil || got.Primary.Value != tc.raw || got.Level == nil || got.Level.Code != tc.code {
				t.Fatalf("SPM result = %#v/%#v", got.Primary, got.Level)
			}
			total := got.Dimensions[len(got.Dimensions)-1]
			if total.NormReference == nil || total.NormReference.TableVersion != "m5-spm-norm-v1" {
				t.Fatalf("SPM norm reference = %#v", total.NormReference)
			}
			found := false
			for _, s := range total.DerivedScores {
				if s.Kind == domainoutcome.ScoreKindPercentile && s.Value == tc.percentile {
					found = true
				}
			}
			if !found {
				t.Fatalf("SPM percentile = %#v", total.DerivedScores)
			}
			for i := range input.AnswerSheet.Answers {
				input.AnswerSheet.Answers[i].Score = -999
			}
			if again := nativeExecute(t, input); !reflect.DeepEqual(got, again) {
				t.Fatal("SPM trusted client scores instead of frozen answer keys")
			}
			nativeFreezeReplay(t, input)
		})
	}
	input := nativeSPMInput(t)
	payload, _ := evaluationinput.CognitivePayload(input)
	payload.Snapshot.SPM.NormTables = nil
	if out, _, err := nativeResolver(t).Execute(context.Background(), nativeAssessment(t, input), input); err == nil || out != nil {
		t.Fatalf("missing required norm returned outcome=%#v err=%v", out, err)
	}
}

func TestM5NativeKernelGoldenFactorClassification(t *testing.T) {
	t.Parallel()
	input := nativeTypologyInput(t, false)
	got := nativeExecute(t, input)
	detail, err := outcometypology.PersonalityTypeDetailFromPayload(got.Detail.Payload)
	if err != nil || got.Profile == nil || got.Profile.Code != "INTJ" || got.Level.Code != "INTJ" || len(detail.Dimensions) != 4 {
		t.Fatalf("four-pole result = %#v/%#v err=%v", got.Profile, detail, err)
	}
	for i, want := range []struct {
		code, left, right string
		score             float64
	}{{"EI", "I", "E", 23}, {"SN", "S", "N", 29}, {"TF", "T", "F", 23}, {"JP", "J", "P", 23}} {
		d := detail.Dimensions[i]
		if d.Code != want.code || d.LeftPole != want.left || d.RightPole != want.right || d.RawScore != want.score {
			t.Fatalf("pole %d = %#v", i, d)
		}
	}
	other := nativeTypologyInput(t, false)
	for i, score := range []float64{-1, 5, -1, -1} {
		other.AnswerSheet.Answers[i].Score = score
	}
	if opposite := nativeExecute(t, other); opposite.Profile == nil || opposite.Profile.Code != "ENFP" || opposite.Level.Code != "ENFP" {
		t.Fatalf("opposite pole result = %#v", opposite.Profile)
	}
	nativeFreezeReplay(t, input)
	pattern := nativeTypologyInput(t, true)
	got = nativeExecute(t, pattern)
	if got.Profile == nil || got.Profile.Code != "balanced" {
		t.Fatalf("nearest_pattern result = %#v", got.Profile)
	}
	nativeFreezeReplay(t, pattern)
	c := pattern.DefinitionV2.Conclusions[0].(conclusion.TypeConclusion)
	c.Decision.LevelRule = nil
	pattern.DefinitionV2.Conclusions[0] = c
	payload, err := typology.PayloadFromDefinition(typology.DefinitionEnvelope{Code: pattern.Model.Code, Version: "v1", Algorithm: modelcatalog.AlgorithmPersonalityTypology}, pattern.DefinitionV2)
	if err != nil {
		t.Fatal(err)
	}
	pattern.ModelPayload = evaluationinput.TypologyModelPayload{Payload: payload}
	if out, _, err := nativeResolver(t).Execute(context.Background(), nativeAssessment(t, pattern), pattern); err == nil || out != nil {
		t.Fatalf("missing explicit level rule returned outcome=%#v err=%v", out, err)
	}
}

func nativeResolver(t *testing.T) *RuntimeResolver {
	t.Helper()
	registry, err := evalruntime.DefaultRuntimeDescriptorRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if err := evalruntime.AttachNativePipelines(registry, evalruntime.NativePipelineDeps{ScaleScorer: factorscoring.NewPipelineComponents(nil), FactorNorm: factornorm.NewPipelineComponents(nil), TaskPerformance: taskperformance.NewPipelineComponents(nil), FactorClassification: factorclassification.NewPipelineComponents()}); err != nil {
		t.Fatal(err)
	}
	return NewRuntimeResolver(registry, descriptorDrivenExecutor{})
}
func nativeExecute(t *testing.T, input *evaluationinput.InputSnapshot) *domainoutcome.Execution {
	t.Helper()
	got, resolved, err := nativeResolver(t).Execute(context.Background(), nativeAssessment(t, input), input)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || string(resolved.DescriptorKey.DecisionKind) != input.Model.DecisionKind {
		t.Fatalf("resolved native route = %#v", resolved.DescriptorKey)
	}
	return got
}
func nativeAssessment(t *testing.T, input *evaluationinput.InputSnapshot) *assessment.Assessment {
	t.Helper()
	m := input.Model
	ref := assessment.NewEvaluationModelRefWithIdentity(modelcatalog.Kind(m.Kind), modelcatalog.CanonicalSubKindFor(modelcatalog.Kind(m.Kind)), modelcatalog.Algorithm(m.Algorithm), 0, meta.NewCode(m.Code), m.Version, m.Title)
	a, err := assessment.NewAssessment(1, testee.NewID(1), assessment.NewQuestionnaireRefByCode(meta.NewCode(input.Questionnaire.Code), input.Questionnaire.Version), assessment.NewAnswerSheetRef(meta.FromUint64(input.AnswerSheet.ID)), assessment.NewAdhocOrigin(), assessment.WithEvaluationModel(ref))
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Submit(); err != nil {
		t.Fatal(err)
	}
	return a
}
func nativeInput(kind modelcatalog.Kind, algorithm modelcatalog.Algorithm, decision modelcatalog.DecisionKind, def *definition.Definition, questions []string) *evaluationinput.InputSnapshot {
	input := &evaluationinput.InputSnapshot{Model: &evaluationinput.ModelSnapshot{Kind: evaluationinput.EvaluationModelKind(kind), Algorithm: string(algorithm), DecisionKind: string(decision), Code: "M5_NATIVE_" + string(decision), Version: "v1", Title: "Native kernel corpus"}, DefinitionV2: def, Questionnaire: &evaluationinput.QuestionnaireSnapshot{Code: "M5_NATIVE_Q", Version: "v1"}, AnswerSheet: &evaluationinput.AnswerSheetSnapshot{ID: 7101, QuestionnaireCode: "M5_NATIVE_Q", QuestionnaireVersion: "v1"}}
	for _, code := range questions {
		input.Questionnaire.Questions = append(input.Questionnaire.Questions, evaluationinput.QuestionSnapshot{Code: code, Type: "radio", Options: []evaluationinput.OptionSnapshot{{Code: "a", Score: 0}, {Code: "b", Score: 1}}})
		input.AnswerSheet.Answers = append(input.AnswerSheet.Answers, evaluationinput.AnswerSnapshot{QuestionCode: code, Value: "b"})
	}
	definition.MaterializeLayers(def)
	evaluationinput.AttachCanonicalDefinition(input, def)
	return input
}
func nativeRules(boundary float64) []conclusion.ScoreRangeOutcome {
	return []conclusion.ScoreRangeOutcome{{MinScore: 0, MaxScore: boundary, Level: "legacy_low", OutcomeCode: "low"}, {MinScore: boundary, UnboundedMax: true, Level: "legacy_high", OutcomeCode: "high"}}
}
func nativeDefinition() *definition.Definition {
	return &definition.Definition{Measure: definition.MeasureSpec{Factors: []factor.Factor{{Code: "total", Title: "Total", Role: factor.FactorRoleTotal}}, FactorGraph: factor.FactorGraph{Roots: []string{"total"}, SortOrders: map[string]int{"total": 1}}, Scoring: []factor.Scoring{{FactorCode: "total", Strategy: factor.ScoringStrategySum, Sources: []factor.ScoringSource{{Kind: factor.ScoringSourceQuestion, Code: "q1"}, {Kind: factor.ScoringSourceQuestion, Code: "q2"}}}}}, Outcomes: []conclusion.Outcome{{Code: "low", Title: "Low", Summary: "frozen low"}, {Code: "high", Title: "High", Summary: "frozen high"}}, ReportMap: definition.ReportMap{Sections: []definition.ReportSection{{Code: "scores", Kind: "factor_scores", SourceRefs: []string{"total"}}}}}
}
func nativeScoreInput(t *testing.T, kind modelcatalog.Kind, algorithm modelcatalog.Algorithm, decision modelcatalog.DecisionKind) *evaluationinput.InputSnapshot {
	t.Helper()
	def := nativeDefinition()
	if kind == modelcatalog.KindScale {
		def.Conclusions = []conclusion.Conclusion{conclusion.RiskConclusion{FactorCode: "total", Rules: nativeRules(5)}}
	} else {
		def.Conclusions = []conclusion.Conclusion{conclusion.NormConclusion{FactorCode: "total", Primary: true, ScoreBasis: conclusion.ScoreBasisTScore, Rules: nativeRules(60)}}
		def.Calibration.NormRefs = []norm.Ref{{FactorCode: "total", NormTableVersion: "m5-native-norm-v1"}}
		def.Execution.Brief2 = &definition.Brief2Spec{FormVariant: "parent", PrimaryFactorCode: "total", IndexFactorCodes: []string{"total"}}
	}
	input := nativeInput(kind, algorithm, decision, def, []string{"q1", "q2"})
	nativeValidate(t, def)
	if kind == modelcatalog.KindScale {
		input.ModelPayload = evaluationinput.ScaleModelPayload{Scale: scale.ScaleSnapshotFromDefinition(scale.ExecutionEnvelope{Code: input.Model.Code, ScaleVersion: "v1", QuestionnaireCode: "M5_NATIVE_Q", QuestionnaireVersion: "v1", Status: "published"}, def)}
	} else {
		table := &norm.Norm{TableVersion: "m5-native-norm-v1", FormVariant: "parent", Kind: kind, Algorithm: algorithm, Factors: []norm.FactorTable{{FactorCode: "total", Lookup: []norm.LookupEntry{{RawScoreMin: 0, RawScoreMax: 20, MinAgeMonths: 60, MaxAgeMonths: 143, Gender: "female", TScore: 60, Percentile: 84}}}}}
		payload, err := behavioral.SnapshotFromDefinition(behavioral.DefinitionEnvelope{Code: input.Model.Code, Version: "v1", QuestionnaireCode: "M5_NATIVE_Q", QuestionnaireVersion: "v1", Status: "published"}, def, map[string]*norm.Norm{table.TableVersion: table})
		if err != nil {
			t.Fatal(err)
		}
		input.ModelPayload = evaluationinput.BehavioralRatingModelPayload{Snapshot: payload}
		age := 120
		input.NormSubject = &evaluationinput.NormSubjectSnapshot{AgeMonths: &age, Gender: "female"}
	}
	return input
}
func nativeSPMInput(t *testing.T) *evaluationinput.InputSnapshot {
	t.Helper()
	def := nativeDefinition()
	def.Measure.Scoring[0].Sources = []factor.ScoringSource{{Kind: factor.ScoringSourceQuestion, Code: "A1"}, {Kind: factor.ScoringSourceQuestion, Code: "A2"}, {Kind: factor.ScoringSourceQuestion, Code: "B1"}}
	def.Execution.SPM = &definition.SPMSpec{TimeLimitSeconds: 600, TotalFactorCode: "total", ItemSets: []definition.SPMItemSet{{Code: "A", Items: []definition.SPMItem{{QuestionCode: "A1", CorrectOptionCode: "b"}, {QuestionCode: "A2", CorrectOptionCode: "a"}}}, {Code: "B", Items: []definition.SPMItem{{QuestionCode: "B1", CorrectOptionCode: "b"}}}}}
	def.Calibration.NormRefs = []norm.Ref{{FactorCode: "total", NormTableVersion: "m5-spm-norm-v1"}}
	def.Conclusions = []conclusion.Conclusion{conclusion.AbilityConclusion{FactorCode: "total", ScoreBasis: conclusion.ScoreBasisPercentile, Primary: true, Rules: nativeRules(50)}}
	input := nativeInput(modelcatalog.KindCognitive, modelcatalog.AlgorithmSPM, modelcatalog.DecisionKindAbilityLevel, def, []string{"A1", "A2", "B1"})
	nativeValidate(t, def)
	payload, err := cognitive.SnapshotFromDefinition(cognitive.DefinitionEnvelope{Code: input.Model.Code, Version: "v1", QuestionnaireCode: "M5_NATIVE_Q", QuestionnaireVersion: "v1", Status: "published"}, def)
	if err != nil {
		t.Fatal(err)
	}
	table := &norm.Norm{TableVersion: "m5-spm-norm-v1", FormVariant: "standard", Kind: modelcatalog.KindCognitive, Algorithm: modelcatalog.AlgorithmSPM, Factors: []norm.FactorTable{{FactorCode: "total"}}}
	for raw, percentile := range []float64{10, 25, 75, 99} {
		table.Factors[0].Lookup = append(table.Factors[0].Lookup, norm.LookupEntry{RawScoreMin: float64(raw), RawScoreMax: float64(raw), MinAgeMonths: 60, MaxAgeMonths: 143, Gender: "female", Percentile: percentile})
	}
	payload.SPM.NormTables, err = normruntime.FromCatalog(table)
	if err != nil {
		t.Fatal(err)
	}
	input.ModelPayload = evaluationinput.CognitiveModelPayload{Snapshot: payload}
	age := 120
	input.NormSubject = &evaluationinput.NormSubjectSnapshot{AgeMonths: &age, Gender: "female"}
	return input
}
func nativeTypologyInput(t *testing.T, pattern bool) *evaluationinput.InputSnapshot {
	t.Helper()
	def := &definition.Definition{}
	kind := modelcatalog.DecisionKindPoleComposition
	c := conclusion.TypeConclusion{Decision: conclusion.TypeDecision{Kind: kind}, OutcomeMapping: conclusion.TypeOutcomeMapping{DetailKind: "personality_type", DetailAdapterKey: "personality_type"}}
	for i, axis := range []struct {
		code, left, right string
		sign              float64
	}{{"EI", "I", "E", -1}, {"SN", "S", "N", 1}, {"TF", "T", "F", -1}, {"JP", "J", "P", -1}} {
		def.Measure.Factors = append(def.Measure.Factors, factor.Factor{Code: axis.code, Title: axis.code, Role: factor.FactorRoleDimension})
		def.Measure.FactorGraph.Roots = append(def.Measure.FactorGraph.Roots, axis.code)
		def.Measure.Scoring = append(def.Measure.Scoring, factor.Scoring{FactorCode: axis.code, Constant: 24, Strategy: factor.ScoringStrategySum, Sources: []factor.ScoringSource{{Kind: factor.ScoringSourceQuestion, Code: axis.code, ScoringMode: factor.QuestionScoringModeQuestionScore, Sign: axis.sign, Weight: 1}}})
		c.FactorCodes = append(c.FactorCodes, axis.code)
		c.Decision.Poles = append(c.Decision.Poles, conclusion.TypePole{FactorCode: axis.code, LeftPole: axis.left, RightPole: axis.right, Threshold: 24})
		if def.Measure.FactorGraph.SortOrders == nil {
			def.Measure.FactorGraph.SortOrders = map[string]int{}
		}
		def.Measure.FactorGraph.SortOrders[axis.code] = i + 1
	}
	for _, code := range []string{"INTJ", "ENFP"} {
		def.Outcomes = append(def.Outcomes, conclusion.Outcome{Code: code, Title: code, Summary: "Frozen " + code})
		c.Profiles = append(c.Profiles, conclusion.TypeOutcomeProfile{OutcomeCode: code, Pattern: code})
	}
	def.ReportMap.Sections = []definition.ReportSection{{Code: "type", Kind: "personality_type", AdapterKey: "personality_type", TemplateID: "mbti", TemplateVersion: "v1"}}
	if pattern {
		kind = modelcatalog.DecisionKindNearestPattern
		c.Decision = conclusion.TypeDecision{Kind: kind, LevelRule: &conclusion.TypeLevelRule{LowMax: 23, HighMin: 29}}
		c.OutcomeMapping = conclusion.TypeOutcomeMapping{DetailKind: "personality_type", DetailAdapterKey: "personality_type"}
		def.Outcomes = []conclusion.Outcome{{Code: "balanced", Title: "Balanced", Summary: "frozen pattern"}, {Code: "high", Title: "High", Summary: "frozen high pattern"}}
		c.Profiles = []conclusion.TypeOutcomeProfile{{OutcomeCode: "balanced", Pattern: "LHLL"}, {OutcomeCode: "high", Pattern: "HHHH"}}
		def.ReportMap.Sections = []definition.ReportSection{{Code: "type", Kind: "personality_type", AdapterKey: "personality_type", TemplateID: "sbti", TemplateVersion: "v1"}}
	}
	def.Conclusions = []conclusion.Conclusion{c}
	input := nativeInput(modelcatalog.KindTypology, modelcatalog.AlgorithmPersonalityTypology, kind, def, []string{"EI", "SN", "TF", "JP"})
	nativeValidate(t, def)
	for i, score := range []float64{1, 5, 1, 1} {
		input.AnswerSheet.Answers[i].Score = score
	}
	payload, err := typology.PayloadFromDefinition(typology.DefinitionEnvelope{Code: input.Model.Code, Version: "v1", QuestionnaireCode: "M5_NATIVE_Q", QuestionnaireVersion: "v1", Status: "published", Algorithm: modelcatalog.AlgorithmPersonalityTypology}, def)
	if err != nil {
		t.Fatal(err)
	}
	input.ModelPayload = evaluationinput.TypologyModelPayload{Payload: payload}
	return input
}
func nativeValidate(t *testing.T, def *definition.Definition) {
	t.Helper()
	if issues := definition.Validate(*def); len(issues) > 0 {
		t.Fatalf("illegal canonical kernel fixture: %#v", issues)
	}
}
func nativeFreezeReplay(t *testing.T, input *evaluationinput.InputSnapshot) {
	t.Helper()
	before, ok := evaluationinput.NewInputSnapshotIdentity(input)
	if !ok {
		t.Fatal("native input identity incomplete")
	}
	hash, err := definition.CanonicalContentHash(input.DefinitionV2)
	if err != nil {
		t.Fatal(err)
	}
	options := evaluationinput.BuildFreezeOptionsFromSnapshot(input, input.Model.ModelRef(), modelcatalog.DecisionKind(input.Model.DecisionKind))
	frozen, err := evaluationinput.MarshalReportInput(options)
	if err != nil {
		t.Fatal(err)
	}
	original := append([]byte(nil), frozen...)
	assets, err := json.Marshal(input.InterpretationAssets)
	if err != nil {
		t.Fatal(err)
	}
	input.DefinitionV2.InterpretationAssets.Outcomes[0].Summary = "later stored copy"
	input.DefinitionV2.DecisionSpec.OutcomeRefs = nil
	after, ok := evaluationinput.NewInputSnapshotIdentity(input)
	if !ok || before != after {
		t.Fatal("derived storage changed native identity")
	}
	again, err := definition.CanonicalContentHash(input.DefinitionV2)
	if err != nil || again != hash {
		t.Fatal("derived storage changed fixed v1 hash")
	}
	input.DefinitionV2.Outcomes[0].Summary = "later canonical copy"
	replayed, err := evaluationinput.SnapshotFromReportInput(frozen, input.Model.ModelRef())
	if err != nil {
		t.Fatal(err)
	}
	got, err := json.Marshal(replayed.InterpretationAssets)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, assets) || !bytes.Equal(frozen, original) {
		t.Fatal("native report replay replaced frozen assets")
	}
	if input.Model.Kind == evaluationinput.EvaluationModelKindTypology {
		wantTemplate := "mbti"
		if input.Model.DecisionKind == string(modelcatalog.DecisionKindNearestPattern) {
			wantTemplate = "sbti"
		}
		if replayed.TypologyRouting == nil || replayed.TypologyRouting.AdapterKey != "personality_type" || replayed.TypologyRouting.TemplateID != wantTemplate || replayed.TypologyRouting.TemplateVersion != "v1" {
			t.Fatalf("frozen native route = %#v", replayed.TypologyRouting)
		}
	}
}
