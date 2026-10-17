package evaluation_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	evaluationexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	evalruntime "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime"
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime/descriptor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/ruleengine"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	scale "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/scale"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestM5NativeKernelGoldenScaleReverseContribution(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, forward, reverse, outcome string
		want                            float64
	}{
		{"below", "a", "a", "low", 3},     // 5 - 2
		{"boundary", "a", "b", "high", 5}, // 5 - 0
		{"above", "b", "a", "high", 6},    // 8 - 2
	} {
		t.Run(tc.name, func(t *testing.T) {
			def := nativeDefinition()
			def.Measure.Scoring[0].Sources = []factor.ScoringSource{
				{Kind: factor.ScoringSourceQuestion, Code: "q1", ScoringMode: factor.QuestionScoringModeOptionOverride, Sign: 1, Weight: 1, OptionScores: map[string]float64{"a": 5, "b": 8}},
				{Kind: factor.ScoringSourceQuestion, Code: "q2", ScoringMode: factor.QuestionScoringModeOptionOverride, Sign: -1, Weight: 1, OptionScores: map[string]float64{"a": 2, "b": 0}},
			}
			def.Conclusions = []conclusion.Conclusion{conclusion.RiskConclusion{FactorCode: "total", Rules: nativeRules(5)}}
			input := nativeScaleChoiceInput(t, def, []string{tc.forward, tc.reverse})
			nativeAssertScaleChoiceResult(t, input, tc.want, tc.outcome, map[string]nativeScaleFactorExpectation{"total": {tc.want, tc.outcome}})
		})
	}
}

func TestM5NativeKernelGoldenScaleWeightedContributions(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, first, second, outcome string
		want                         float64
	}{
		{"below", "a", "a", "low", 4},     // 2*0.5 + 1.5*2
		{"boundary", "b", "a", "high", 5}, // 4*0.5 + 1.5*2
		{"above", "b", "b", "high", 8},    // 4*0.5 + 3*2
	} {
		t.Run(tc.name, func(t *testing.T) {
			def := nativeDefinition()
			def.Measure.Scoring[0].Sources = []factor.ScoringSource{
				{Kind: factor.ScoringSourceQuestion, Code: "q1", ScoringMode: factor.QuestionScoringModeOptionOverride, Sign: 1, Weight: 0.5, OptionScores: map[string]float64{"a": 2, "b": 4}},
				{Kind: factor.ScoringSourceQuestion, Code: "q2", ScoringMode: factor.QuestionScoringModeOptionOverride, Sign: 1, Weight: 2, OptionScores: map[string]float64{"a": 1.5, "b": 3}},
			}
			def.Conclusions = []conclusion.Conclusion{conclusion.RiskConclusion{FactorCode: "total", Rules: nativeRules(5)}}
			input := nativeScaleChoiceInput(t, def, []string{tc.first, tc.second})
			nativeAssertScaleChoiceResult(t, input, tc.want, tc.outcome, map[string]nativeScaleFactorExpectation{"total": {tc.want, tc.outcome}})
		})
	}
}

func TestM5NativeKernelGoldenScaleComposite(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, first, second, outcome string
		left, right, total           float64
		leftOutcome, rightOutcome    string
	}{
		{"below", "a", "a", "low", 4, 1, 5, "low", "low"},      // left + right
		{"boundary", "a", "b", "high", 4, 2, 6, "low", "high"}, // 4 + 2
		{"above", "b", "b", "high", 8, 2, 10, "high", "high"},  // 8 + 2
	} {
		t.Run(tc.name, func(t *testing.T) {
			def := nativeCompositeScaleDefinition(factor.ScoringStrategySum, nil)
			input := nativeScaleChoiceInput(t, def, []string{tc.first, tc.second})
			replayed := nativeAssertScaleChoiceResult(t, input, tc.total, tc.outcome, map[string]nativeScaleFactorExpectation{
				"total": {tc.total, tc.outcome}, "left": {tc.left, tc.leftOutcome}, "right": {tc.right, tc.rightOutcome},
			})
			def.Measure.Factors[0].Title = "later total title"
			wantCatalog := []evaluationinput.FactorCatalogEntry{
				{Code: "total", Title: "Total", IsTotalScore: true},
				{Code: "left", Title: "Left"},
				{Code: "right", Title: "Right"},
			}
			if !reflect.DeepEqual(replayed.FactorCatalog, wantCatalog) {
				t.Fatalf("frozen composite report catalog = %#v, want %#v", replayed.FactorCatalog, wantCatalog)
			}
		})
	}
}

func TestM5NativeKernelGoldenScaleCompositeWeightedSum(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, first, second, outcome string
		left, right, total           float64
		leftOutcome, rightOutcome    string
	}{
		{"below", "a", "a", "low", 4, 1, 4, "low", "low"},      // left*0.5 + right*2
		{"boundary", "a", "b", "high", 4, 2, 6, "low", "high"}, // 4*0.5 + 2*2
		{"above", "b", "b", "high", 8, 2, 8, "high", "high"},   // 8*0.5 + 2*2
	} {
		t.Run(tc.name, func(t *testing.T) {
			def := nativeCompositeScaleDefinition(factor.ScoringStrategyWeightedSum, map[string]float64{"left": 0.5, "right": 2})
			input := nativeScaleChoiceInput(t, def, []string{tc.first, tc.second})
			replayed := nativeAssertScaleChoiceResult(t, input, tc.total, tc.outcome, map[string]nativeScaleFactorExpectation{
				"total": {tc.total, tc.outcome}, "left": {tc.left, tc.leftOutcome}, "right": {tc.right, tc.rightOutcome},
			})
			def.Measure.Factors[0].Title = "later total title"
			wantCatalog := []evaluationinput.FactorCatalogEntry{
				{Code: "total", Title: "Total", IsTotalScore: true},
				{Code: "left", Title: "Left"},
				{Code: "right", Title: "Right"},
			}
			if !reflect.DeepEqual(replayed.FactorCatalog, wantCatalog) {
				t.Fatalf("frozen composite report catalog = %#v, want %#v", replayed.FactorCatalog, wantCatalog)
			}
		})
	}
}

func nativeCompositeScaleDefinition(strategy factor.ScoringStrategy, weights map[string]float64) *definition.Definition {
	def := nativeDefinition()
	def.Measure.Factors = []factor.Factor{
		{Code: "total", Title: "Total", Role: factor.FactorRoleTotal},
		{Code: "left", Title: "Left", Role: factor.FactorRoleDimension},
		{Code: "right", Title: "Right", Role: factor.FactorRoleDimension},
	}
	def.Measure.FactorGraph = factor.FactorGraph{
		Roots:      []string{"total"},
		Edges:      []factor.FactorEdge{{ParentCode: "total", ChildCode: "left"}, {ParentCode: "total", ChildCode: "right"}},
		SortOrders: map[string]int{"total": 1, "left": 2, "right": 3},
	}
	def.Measure.Scoring = []factor.Scoring{
		{FactorCode: "total", Strategy: strategy, Weights: weights, Sources: []factor.ScoringSource{{Kind: factor.ScoringSourceFactor, Code: "left"}, {Kind: factor.ScoringSourceFactor, Code: "right"}}},
		{FactorCode: "left", Strategy: factor.ScoringStrategySum, Sources: []factor.ScoringSource{{Kind: factor.ScoringSourceQuestion, Code: "q1", ScoringMode: factor.QuestionScoringModeOptionOverride, Sign: 1, Weight: 1, OptionScores: map[string]float64{"a": 4, "b": 8}}}},
		{FactorCode: "right", Strategy: factor.ScoringStrategySum, Sources: []factor.ScoringSource{{Kind: factor.ScoringSourceQuestion, Code: "q2", ScoringMode: factor.QuestionScoringModeOptionOverride, Sign: 1, Weight: 1, OptionScores: map[string]float64{"a": 1, "b": 2}}}},
	}
	def.Conclusions = []conclusion.Conclusion{
		conclusion.RiskConclusion{FactorCode: "total", Rules: nativeRules(6)},
		conclusion.RiskConclusion{FactorCode: "left", Rules: nativeRules(6)},
		conclusion.RiskConclusion{FactorCode: "right", Rules: nativeRules(2)},
	}
	return def
}

// Choose options, not precomputed factor scores: the production contribution
// evaluator must resolve each option override before applying sign and weight.
func nativeScaleChoiceInput(t *testing.T, def *definition.Definition, choices []string) *evaluationinput.InputSnapshot {
	t.Helper()
	if issues := definition.Validate(*def); len(issues) != 0 {
		t.Fatalf("illegal canonical scale fixture: %#v", issues)
	}
	input := &evaluationinput.InputSnapshot{
		Model:         &evaluationinput.ModelSnapshot{Kind: evaluationinput.EvaluationModelKindScale, Algorithm: string(modelcatalog.AlgorithmScaleDefault), DecisionKind: string(modelcatalog.DecisionKindScoreRange), Code: "M5_NATIVE_SCALE", Version: "v1", Title: "Native scale corpus"},
		DefinitionV2:  def,
		Questionnaire: &evaluationinput.QuestionnaireSnapshot{Code: "M5_NATIVE_Q", Version: "v1"},
		AnswerSheet:   &evaluationinput.AnswerSheetSnapshot{ID: 7101, QuestionnaireCode: "M5_NATIVE_Q", QuestionnaireVersion: "v1"},
	}
	for _, code := range []string{"q1", "q2"} {
		input.Questionnaire.Questions = append(input.Questionnaire.Questions, evaluationinput.QuestionSnapshot{Code: code, Type: "radio", Options: []evaluationinput.OptionSnapshot{{Code: "a", Score: 0}, {Code: "b", Score: 1}}})
		input.AnswerSheet.Answers = append(input.AnswerSheet.Answers, evaluationinput.AnswerSnapshot{QuestionCode: code})
	}
	definition.MaterializeLayers(def)
	evaluationinput.AttachCanonicalDefinition(input, def)
	if len(choices) != len(input.AnswerSheet.Answers) {
		t.Fatal("choice count does not match the frozen questionnaire")
	}
	for i, choice := range choices {
		input.AnswerSheet.Answers[i].Value = choice
		input.AnswerSheet.Answers[i].Score = 999 // Deliberately disagrees with every canonical option override.
	}
	input.ModelPayload = evaluationinput.ScaleModelPayload{Scale: scale.ScaleSnapshotFromDefinition(scale.ExecutionEnvelope{Code: input.Model.Code, ScaleVersion: "v1", QuestionnaireCode: "M5_NATIVE_Q", QuestionnaireVersion: "v1", Status: "published"}, def)}
	return input
}

type nativeScaleFactorExpectation struct {
	score   float64
	outcome string
}

func nativeAssertScaleChoiceResult(t *testing.T, input *evaluationinput.InputSnapshot, want float64, outcome string, factors map[string]nativeScaleFactorExpectation) *evaluationinput.InputSnapshot {
	t.Helper()
	got := nativeExecute(t, input)
	if got.Primary == nil || got.Primary.Value != want || got.Level == nil || got.Level.Code != outcome {
		t.Fatalf("score/OutcomeCode = %#v/%#v, want %v/%s", got.Primary, got.Level, want, outcome)
	}
	if len(got.Dimensions) != len(factors) {
		t.Fatalf("dimensions = %#v, want exactly %d factors", got.Dimensions, len(factors))
	}
	seen := make(map[string]bool, len(factors))
	for _, d := range got.Dimensions {
		wantFactor, exists := factors[d.Code]
		if !exists || seen[d.Code] || d.Score == nil || d.Score.Value != wantFactor.score || d.Level == nil || d.Level.Code != wantFactor.outcome {
			t.Fatalf("factor %s = %#v, want %#v (known=%v, duplicate=%v)", d.Code, d, wantFactor, exists, seen[d.Code])
		}
		seen[d.Code] = true
	}
	for i := range input.AnswerSheet.Answers {
		input.AnswerSheet.Answers[i].Score = -999
	}
	if again := nativeExecute(t, input); !reflect.DeepEqual(got, again) {
		t.Fatal("scale option-override calculation trusted the supplied answer scores")
	}
	options := evaluationinput.BuildFreezeOptionsFromSnapshot(input, input.Model.ModelRef(), modelcatalog.DecisionKindScoreRange)
	frozen, err := evaluationinput.MarshalReportInput(options)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := evaluationinput.SnapshotFromReportInput(frozen, input.Model.ModelRef())
	if err != nil {
		t.Fatal(err)
	}
	assets := replayed.InterpretationAssets
	if assets == nil || len(assets.Outcomes) != 2 || assets.Outcomes[0].OutcomeCode != "low" || assets.Outcomes[0].Summary != "frozen low" || assets.Outcomes[1].OutcomeCode != "high" || assets.Outcomes[1].Summary != "frozen high" {
		t.Fatalf("frozen outcome assets = %#v", assets)
	}
	if len(assets.ReportSpec.Sections) != 1 || assets.ReportSpec.Sections[0].Kind != "factor_scores" || !reflect.DeepEqual(assets.ReportSpec.Sections[0].SourceRefs, []string{"total"}) {
		t.Fatalf("frozen scale report section = %#v", assets.ReportSpec.Sections)
	}
	// Apply the same canonical hash, input identity and immutable ReportInput
	// checks as the existing four-family corpus, using production scale wiring.
	nativeFreezeReplay(t, input)
	return replayed
}

func nativeDefinition() *definition.Definition {
	return &definition.Definition{
		Measure: definition.MeasureSpec{
			Factors:     []factor.Factor{{Code: "total", Title: "Total", Role: factor.FactorRoleTotal}},
			FactorGraph: factor.FactorGraph{Roots: []string{"total"}, SortOrders: map[string]int{"total": 1}},
			Scoring:     []factor.Scoring{{FactorCode: "total", Strategy: factor.ScoringStrategySum}},
		},
		Outcomes:  []conclusion.Outcome{{Code: "low", Title: "Low", Summary: "frozen low"}, {Code: "high", Title: "High", Summary: "frozen high"}},
		ReportMap: definition.ReportMap{Sections: []definition.ReportSection{{Code: "scores", Kind: "factor_scores", SourceRefs: []string{"total"}}}},
	}
}

func nativeRules(boundary float64) []conclusion.ScoreRangeOutcome {
	return []conclusion.ScoreRangeOutcome{{MinScore: 0, MaxScore: boundary, Level: "legacy_low", OutcomeCode: "low"}, {MinScore: boundary, UnboundedMax: true, Level: "legacy_high", OutcomeCode: "high"}}
}

func nativeExecute(t *testing.T, input *evaluationinput.InputSnapshot) *domainoutcome.Execution {
	t.Helper()
	registry, err := evalruntime.DefaultRuntimeDescriptorRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// Match wireEvaluationEngine's actual production adapter, not the default
	// nil-scorer evaluator used by the separate calculation-only corpus.
	deps := evalruntime.WiringDeps{ScaleScorer: ruleengine.NewScaleFactorScorer()}
	if err := evalruntime.AttachNativePipelines(registry, evalruntime.NativePipelineDeps{
		ScaleScorer:          evalruntime.MaterializeFactorScoringPipelineComponents(deps),
		FactorNorm:           evalruntime.MaterializeFactorNormPipelineComponents(deps),
		TaskPerformance:      evalruntime.MaterializeTaskPerformancePipelineComponents(deps),
		FactorClassification: evalruntime.MaterializeFactorClassificationPipelineComponents(deps),
	}); err != nil {
		t.Fatal(err)
	}
	ref := assessment.NewEvaluationModelRefWithIdentity(modelcatalog.KindScale, modelcatalog.CanonicalSubKindFor(modelcatalog.KindScale), modelcatalog.AlgorithmScaleDefault, 0, meta.NewCode(input.Model.Code), input.Model.Version, input.Model.Title)
	a, err := assessment.NewAssessment(1, testee.NewID(1), assessment.NewQuestionnaireRefByCode(meta.NewCode(input.Questionnaire.Code), input.Questionnaire.Version), assessment.NewAnswerSheetRef(meta.FromUint64(input.AnswerSheet.ID)), assessment.NewAdhocOrigin(), assessment.WithEvaluationModel(ref))
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Submit(); err != nil {
		t.Fatal(err)
	}
	got, resolved, err := evaluationexecute.NewRuntimeResolver(registry, nativeScaleStages{}).Execute(context.Background(), a, input)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || string(resolved.DescriptorKey.DecisionKind) != input.Model.DecisionKind {
		t.Fatalf("scale runtime route = %#v", resolved.DescriptorKey)
	}
	return got
}

// The production descriptor executor is private to application/execute. This
// test bridge only sequences the real descriptor stages; it never computes a
// score or substitutes a result. Keeping this at composition root preserves
// the application-to-infrastructure dependency boundary.
type nativeScaleStages struct{}

func (nativeScaleStages) Execute(ctx context.Context, desc evalpipeline.RuntimeDescriptor, input evalpipeline.ExecutionInput) (*domainoutcome.Execution, error) {
	assembledInput, err := desc.InputAssembler.Assemble(input)
	if err != nil {
		return nil, err
	}
	calculated, err := desc.Calculator.Calculate(ctx, assembledInput)
	if err != nil {
		return nil, err
	}
	assembled, err := desc.OutcomeAssembler.Assemble(calculated)
	if err != nil {
		return nil, err
	}
	outcome, ok := assembled.(*domainoutcome.Execution)
	if !ok || outcome == nil {
		return nil, fmt.Errorf("invalid native scale outcome %T", assembled)
	}
	return outcome, nil
}

func nativeFreezeReplay(t *testing.T, input *evaluationinput.InputSnapshot) {
	t.Helper()
	before, ok := evaluationinput.NewInputSnapshotIdentity(input)
	if !ok {
		t.Fatal("scale input identity incomplete")
	}
	hash, err := definition.CanonicalContentHash(input.DefinitionV2)
	if err != nil {
		t.Fatal(err)
	}
	frozen, err := evaluationinput.MarshalReportInput(evaluationinput.BuildFreezeOptionsFromSnapshot(input, input.Model.ModelRef(), modelcatalog.DecisionKindScoreRange))
	if err != nil {
		t.Fatal(err)
	}
	wantAssets, err := json.Marshal(input.InterpretationAssets)
	if err != nil {
		t.Fatal(err)
	}
	input.DefinitionV2.InterpretationAssets.Outcomes[0].Summary = "later stored copy"
	input.DefinitionV2.DecisionSpec.OutcomeRefs = nil
	after, ok := evaluationinput.NewInputSnapshotIdentity(input)
	if !ok || before != after {
		t.Fatal("derived storage changed native scale identity")
	}
	if again, err := definition.CanonicalContentHash(input.DefinitionV2); err != nil || again != hash {
		t.Fatal("derived storage changed canonical v1 hash")
	}
	input.DefinitionV2.Outcomes[0].Summary = "later canonical copy"
	replayed, err := evaluationinput.SnapshotFromReportInput(frozen, input.Model.ModelRef())
	if err != nil {
		t.Fatal(err)
	}
	gotAssets, err := json.Marshal(replayed.InterpretationAssets)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotAssets, wantAssets) {
		t.Fatal("scale report replay replaced frozen assets")
	}
}
