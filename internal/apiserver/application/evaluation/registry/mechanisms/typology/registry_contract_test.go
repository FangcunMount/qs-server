package typology

import (
	"context"
	"testing"

	outcometypology "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome/typology"
	personalityconfigured "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/typology/runtime/configured"
	evaluationexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime/descriptor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

const (
	contractDetailAdapter = modeltypology.DetailAdapterKey("contract_injected_detail")
	contractReportAdapter = modeltypology.ReportAdapterKey("contract_injected_report")
)

func TestInjectedAdapterRegistriesRunThroughConfiguredRuntime(t *testing.T) {
	t.Parallel()

	detailRegistry := personalityconfigured.DefaultDetailAssemblerRegistry().Register(
		contractDetailAdapter,
		func(_ personalityconfigured.DetailInput) (any, error) {
			return outcometypology.PersonalityTypeDetail{
				TypeCode:     "INJECTED",
				MatchPercent: 42,
			}, nil
		},
	)
	outcomeRegistry := DefaultOutcomeAdapterRegistry().Register(
		contractDetailAdapter,
		assembleGenericPersonalityTypeOutcome,
	)
	runtime := NewPersonalityRuntime(PersonalityRuntimeOptions{
		DetailRegistry:  detailRegistry,
		OutcomeRegistry: outcomeRegistry,
	})

	executor, err := NewConfiguredTypologyExecutorWithRuntime(runtime)
	if err != nil {
		t.Fatalf("NewConfiguredTypologyExecutorWithRuntime: %v", err)
	}
	assessmentEntity := contractInjectedAssessment(t)
	snapshot := contractInjectedInputSnapshot()
	payload, ok := evaluationinput.TypologyPayload(snapshot)
	if !ok || payload == nil {
		t.Fatal("expected typology payload")
	}

	outcome, err := executor.Execute(context.Background(), evaluationexecute.ExecutionInput{
		Assessment: assessmentEntity,
		Input:      snapshot,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	detail, ok := outcome.Detail.Payload.(outcometypology.PersonalityTypeDetail)
	if !ok {
		t.Fatalf("detail type = %T, want PersonalityTypeDetail", outcome.Detail.Payload)
	}
	if detail.TypeCode != "INJECTED" || detail.MatchPercent != 42 {
		t.Fatalf("detail = %#v, want injected marker", detail)
	}

}

func contractInjectedInputSnapshot() *evaluationinput.InputSnapshot {
	payload := contractInjectedRuntimePayload()
	return &evaluationinput.InputSnapshot{
		Model:        evaluationinput.NewTypologyModelSnapshot(payload),
		ModelPayload: evaluationinput.TypologyModelPayload{Payload: payload},
		DefinitionV2: contractInjectedDefinition(),
		AnswerSheet: &evaluationinput.AnswerSheetSnapshot{
			QuestionnaireCode:    payload.QuestionnaireCode,
			QuestionnaireVersion: payload.QuestionnaireVersion,
			Answers: []evaluationinput.AnswerSnapshot{
				{QuestionCode: "Q_EI", Score: 1},
				{QuestionCode: "Q_SN", Score: 5},
				{QuestionCode: "Q_TF", Score: 1},
				{QuestionCode: "Q_JP", Score: 1},
			},
		},
		Questionnaire: &evaluationinput.QuestionnaireSnapshot{
			Code:    payload.QuestionnaireCode,
			Version: payload.QuestionnaireVersion,
		},
	}
}

func contractInjectedRuntimePayload() *modeltypology.Payload {
	return &modeltypology.Payload{
		Code:                 "CONTRACT_INJECTED_V1",
		Version:              "1.0.0",
		QuestionnaireCode:    "CONTRACT_INJECTED_V1",
		QuestionnaireVersion: "1.0.0",
		Status:               "published",
		Outcomes: []modeltypology.Outcome{
			{Code: "INTJ", Name: "建筑师", OneLiner: "独立战略家"},
		},
		Runtime: &modeltypology.RuntimeSpec{
			FactorGraph: modeltypology.FactorGraphSpec{
				Factors: map[string]modeltypology.FactorSpec{
					"EI": contractLeafSpec("EI", "外向-内向", "Q_EI", -1),
					"SN": contractLeafSpec("SN", "感觉-直觉", "Q_SN", 1),
					"TF": contractLeafSpec("TF", "思考-情感", "Q_TF", -1),
					"JP": contractLeafSpec("JP", "判断-知觉", "Q_JP", -1),
				},
				Roots: []string{"EI", "SN", "TF", "JP"},
				Dimensions: map[string]modeltypology.Dimension{
					"EI": {Code: "EI", Name: "外向-内向", LeftPole: "I", RightPole: "E", Constant: 24, Threshold: 24},
					"SN": {Code: "SN", Name: "感觉-直觉", LeftPole: "S", RightPole: "N", Constant: 24, Threshold: 24},
					"TF": {Code: "TF", Name: "思考-情感", LeftPole: "T", RightPole: "F", Constant: 24, Threshold: 24},
					"JP": {Code: "JP", Name: "判断-知觉", LeftPole: "J", RightPole: "P", Constant: 24, Threshold: 24},
				},
			},
			Decision: modeltypology.PersonalityDecisionSpec{
				Kind: modelcatalog.DecisionKindPoleComposition,
			},
			OutcomeMapping: modeltypology.OutcomeMappingSpec{
				DetailKind:       modeltypology.OutcomeDetailPersonalityType,
				DetailAdapterKey: contractDetailAdapter,
			},
			Report: modeltypology.ReportSpec{
				Kind:          modeltypology.ReportKindPersonalityType,
				AdapterKey:    contractReportAdapter,
				CategoryLabel: "Contract Injected Model",
			},
		},
	}
}

func contractLeafSpec(code, name, question string, sign float64) modeltypology.FactorSpec {
	return modeltypology.FactorSpec{
		ID: code, Code: code, Name: name, Kind: modeltypology.FactorSpecKindLeaf, Constant: 24,
		Contributions: []modeltypology.FactorContributionSpec{{QuestionCode: question, ScoringMode: modeltypology.QuestionScoringModeQuestionScore, Sign: sign, Weight: 1}},
	}
}

func contractInjectedDefinition() *modeldefinition.Definition {
	codes := []string{"EI", "SN", "TF", "JP"}
	names := map[string]string{"EI": "外向-内向", "SN": "感觉-直觉", "TF": "思考-情感", "JP": "判断-知觉"}
	questions := map[string]string{"EI": "Q_EI", "SN": "Q_SN", "TF": "Q_TF", "JP": "Q_JP"}
	signs := map[string]float64{"EI": -1, "SN": 1, "TF": -1, "JP": -1}
	poles := map[string][2]string{"EI": {"I", "E"}, "SN": {"S", "N"}, "TF": {"T", "F"}, "JP": {"J", "P"}}
	def := &modeldefinition.Definition{Measure: modeldefinition.MeasureSpec{FactorGraph: factor.FactorGraph{Roots: append([]string(nil), codes...), SortOrders: map[string]int{}}}}
	for i, code := range codes {
		def.Measure.Factors = append(def.Measure.Factors, factor.Factor{Code: code, Title: names[code], Role: factor.FactorRoleDimension})
		def.Measure.FactorGraph.SortOrders[code] = i + 1
		def.Measure.Scoring = append(def.Measure.Scoring, factor.Scoring{
			FactorCode: code, Strategy: factor.ScoringStrategySum, Constant: 24,
			Sources: []factor.ScoringSource{{Kind: factor.ScoringSourceQuestion, Code: questions[code], ScoringMode: factor.QuestionScoringModeQuestionScore, Sign: signs[code], Weight: 1}},
		})
	}
	typeConclusion := conclusion.TypeConclusion{
		FactorCodes: codes,
		Decision:    conclusion.TypeDecision{Kind: modelcatalog.DecisionKindPoleComposition},
		OutcomeMapping: conclusion.TypeOutcomeMapping{
			DetailKind: string(modeltypology.OutcomeDetailPersonalityType), DetailAdapterKey: string(contractDetailAdapter),
		},
	}
	for _, code := range codes {
		typeConclusion.Decision.Poles = append(typeConclusion.Decision.Poles, conclusion.TypePole{FactorCode: code, LeftPole: poles[code][0], RightPole: poles[code][1], Threshold: 24})
	}
	def.Conclusions = []conclusion.Conclusion{typeConclusion}
	def.Outcomes = []conclusion.Outcome{{Code: "INTJ", Title: "建筑师", Description: "独立战略家"}}
	def.ReportMap.Sections = []modeldefinition.ReportSection{{Code: string(modeltypology.ReportKindPersonalityType), Kind: string(modeltypology.ReportKindPersonalityType), AdapterKey: string(contractReportAdapter), CategoryLabel: "Contract Injected Model"}}
	return def
}

func contractInjectedAssessment(t *testing.T) *assessment.Assessment {
	t.Helper()
	modelRef := assessment.NewEvaluationModelRefWithIdentity(
		assessment.EvaluationModelKindTypology,
		modelcatalog.SubKindTypology,
		modelcatalog.AlgorithmPersonalityTypology,
		meta.ID(0),
		meta.NewCode("CONTRACT_INJECTED_V1"),
		"1.0.0",
		"Contract Injected Model",
	)
	a, err := assessment.NewAssessment(
		1,
		testee.NewID(9010),
		assessment.NewQuestionnaireRefByCode(meta.NewCode("CONTRACT_INJECTED_V1"), "1.0.0"),
		assessment.NewAnswerSheetRef(meta.FromUint64(8010)),
		assessment.NewAdhocOrigin(),
		assessment.WithID(assessment.NewID(7010)),
		assessment.WithEvaluationModel(modelRef),
	)
	if err != nil {
		t.Fatalf("NewAssessment: %v", err)
	}
	if err := a.Submit(); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	a.ClearEvents()
	return a
}
