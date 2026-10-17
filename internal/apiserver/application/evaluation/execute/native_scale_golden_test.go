package execute

import (
	"reflect"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	scale "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/scale"
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
			nativeAssertScaleChoiceResult(t, input, tc.want, tc.outcome, map[string]float64{"total": tc.want})
		})
	}
}

// Choose options, not precomputed factor scores: the production contribution
// evaluator must resolve each option override before applying sign and weight.
func nativeScaleChoiceInput(t *testing.T, def *definition.Definition, choices []string) *evaluationinput.InputSnapshot {
	t.Helper()
	nativeValidate(t, def)
	input := nativeInput(modelcatalog.KindScale, modelcatalog.AlgorithmScaleDefault, modelcatalog.DecisionKindScoreRange, def, []string{"q1", "q2"})
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

func nativeAssertScaleChoiceResult(t *testing.T, input *evaluationinput.InputSnapshot, want float64, outcome string, factors map[string]float64) {
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
		wantScore, exists := factors[d.Code]
		if !exists || seen[d.Code] || d.Score == nil || d.Score.Value != wantScore {
			t.Fatalf("factor %s = %#v, want %v (known=%v, duplicate=%v)", d.Code, d, wantScore, exists, seen[d.Code])
		}
		seen[d.Code] = true
		if d.Code == "total" && (d.Level == nil || d.Level.Code != outcome) {
			t.Fatalf("total OutcomeCode = %#v, want %s", d.Level, outcome)
		}
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
	// Reuse the original corpus's canonical hash, input identity and immutable
	// ReportInput checks after changing both stored and canonical report assets.
	nativeFreezeReplay(t, input)
}
