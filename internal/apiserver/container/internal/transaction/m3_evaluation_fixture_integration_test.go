//go:build integration && m3_evaluation_broker

package transaction_test

import (
	"testing"

	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/interpretationassets"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func m3EvaluationInput() *evaluationinput.InputSnapshot {
	def := &modeldefinition.Definition{
		Measure: modeldefinition.MeasureSpec{
			Factors: []factor.Factor{{Code: "total", Title: "总分", Role: factor.FactorRoleTotal}},
		},
	}
	return &evaluationinput.InputSnapshot{
		Model: &evaluationinput.ModelSnapshot{
			Kind: evaluationinput.EvaluationModelKindScale, Algorithm: string(modelcatalog.AlgorithmScaleDefault),
			DecisionKind: string(modelcatalog.DecisionKindScoreRange),
			Code:         "SCALE-1", Version: "1.0.0",
		},
		DefinitionV2:         def,
		InterpretationAssets: &interpretationassets.Assets{Outcomes: []interpretationassets.OutcomePresentation{{OutcomeCode: "high", Title: "高风险"}}},
	}
}

func m3EvaluationFixture(t *testing.T) (*assessment.Assessment, *domainoutcome.Execution) {
	t.Helper()
	modelRef := assessment.NewScaleEvaluationModelRef(meta.ID(0), meta.NewCode("SCALE-1"), "1.0.0", "scale")
	a, err := assessment.NewAssessment(
		1,
		testee.NewID(1001),
		assessment.NewQuestionnaireRefByCode(meta.NewCode("Q-1"), "1.0.0"),
		assessment.NewAnswerSheetRef(meta.FromUint64(2001)),
		assessment.NewAdhocOrigin(),
		assessment.WithID(assessment.NewID(5001)),
		assessment.WithEvaluationModel(modelRef),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Submit(); err != nil {
		t.Fatal(err)
	}
	a.ClearEvents()
	execution := domainoutcome.NewExecution(evaloutcome.ModelRefFromAssessment(modelRef), domainoutcome.Summary{PrimaryLabel: "high"}, domainoutcome.Detail{Kind: modelcatalog.KindScale})
	execution.Primary = &domainoutcome.ScoreValue{Kind: domainoutcome.ScoreKindRawTotal, Value: 12}
	execution.Level = &domainoutcome.ResultLevel{Code: "high", Label: "高风险", Severity: "high"}
	return a, execution
}
