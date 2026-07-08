package typology

import (
	"context"
	"testing"

	evaluationexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	outcometypology "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome/typology"
	personalityconfigured "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/typology/runtime/configured"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/typology"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
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
	reportRegistry := DefaultReportAdapterRegistry().Register(
		contractReportAdapter,
		func(_ evaloutcome.Outcome) (*domainReport.InterpretReport, error) {
			return domainReport.NewInterpretReport(
				domainReport.ID(1),
				"Injected",
				"INJECTED",
				0,
				domainReport.RiskLevelNone,
				"injected-conclusion",
				nil,
				nil,
				nil,
			), nil
		},
	)

	registry := NewPersonalityRuntimeRegistryWith(PersonalityRuntimeOptions{
		DetailRegistry:  detailRegistry,
		OutcomeRegistry: outcomeRegistry,
		ReportRegistry:  reportRegistry,
	}).AsModuleRegistry()

	executor, err := NewConfiguredTypologyExecutorWithRegistry(registry)
	if err != nil {
		t.Fatalf("NewConfiguredTypologyExecutorWithRegistry: %v", err)
	}
	reportBuilder, err := NewConfiguredReportBuilderWithRegistry(registry)
	if err != nil {
		t.Fatalf("NewConfiguredReportBuilderWithRegistry: %v", err)
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

	report, err := reportBuilder.Build(context.Background(), evaloutcome.Outcome{
		Assessment: assessmentEntity,
		Input:      snapshot,
		Execution:  outcome,
	})
	if err != nil {
		t.Fatalf("Build report: %v", err)
	}
	if report.Conclusion() != "injected-conclusion" {
		t.Fatalf("conclusion = %q, want injected-conclusion", report.Conclusion())
	}
}

func contractInjectedInputSnapshot() *evaluationinput.InputSnapshot {
	payload := contractInjectedRuntimePayload()
	return &evaluationinput.InputSnapshot{
		Model:        evaluationinput.NewTypologyModelSnapshot(payload),
		ModelPayload: evaluationinput.TypologyModelPayload{Payload: payload},
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
		DimensionOrder:       []string{"EI", "SN", "TF", "JP"},
		Dimensions: map[string]modeltypology.Dimension{
			"EI": {Code: "EI", Name: "外向-内向", LeftPole: "I", RightPole: "E", Constant: 24, Threshold: 24},
			"SN": {Code: "SN", Name: "感觉-直觉", LeftPole: "S", RightPole: "N", Constant: 24, Threshold: 24},
			"TF": {Code: "TF", Name: "思考-情感", LeftPole: "T", RightPole: "F", Constant: 24, Threshold: 24},
			"JP": {Code: "JP", Name: "判断-知觉", LeftPole: "J", RightPole: "P", Constant: 24, Threshold: 24},
		},
		QuestionMappings: []modeltypology.QuestionMapping{
			{QuestionCode: "Q_EI", Dimension: "EI", Sign: -1},
			{QuestionCode: "Q_SN", Dimension: "SN", Sign: 1},
			{QuestionCode: "Q_TF", Dimension: "TF", Sign: -1},
			{QuestionCode: "Q_JP", Dimension: "JP", Sign: -1},
		},
		Outcomes: []modeltypology.Outcome{
			{Code: "INTJ", Name: "建筑师", OneLiner: "独立战略家"},
		},
		Runtime: &modeltypology.RuntimeSpec{
			FactorGraph: modeltypology.FactorGraphSpec{
				DimensionOrder: []string{"EI", "SN", "TF", "JP"},
				Dimensions: map[string]modeltypology.Dimension{
					"EI": {Code: "EI", Name: "外向-内向", LeftPole: "I", RightPole: "E", Constant: 24, Threshold: 24},
					"SN": {Code: "SN", Name: "感觉-直觉", LeftPole: "S", RightPole: "N", Constant: 24, Threshold: 24},
					"TF": {Code: "TF", Name: "思考-情感", LeftPole: "T", RightPole: "F", Constant: 24, Threshold: 24},
					"JP": {Code: "JP", Name: "判断-知觉", LeftPole: "J", RightPole: "P", Constant: 24, Threshold: 24},
				},
				QuestionMappings: []modeltypology.QuestionMapping{
					{QuestionCode: "Q_EI", Dimension: "EI", Sign: -1},
					{QuestionCode: "Q_SN", Dimension: "SN", Sign: 1},
					{QuestionCode: "Q_TF", Dimension: "TF", Sign: -1},
					{QuestionCode: "Q_JP", Dimension: "JP", Sign: -1},
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

func contractInjectedAssessment(t *testing.T) *assessment.Assessment {
	t.Helper()
	modelRef := assessment.NewEvaluationModelRefWithIdentity(
		assessment.EvaluationModelKindPersonality,
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
