package pipeline

import (
	"context"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	domainAssessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type riskScoreWriterStub struct {
	called bool
	ctx    *Context
}

func (w *riskScoreWriterStub) SaveAssessmentScore(_ context.Context, evalCtx *Context) error {
	w.called = true
	w.ctx = evalCtx
	return nil
}

func TestRiskLevelHandlerCalculatesRiskBeforeDelegatingScoreWriter(t *testing.T) {
	a, err := domainAssessment.NewAssessment(
		1,
		testee.NewID(8001),
		domainAssessment.NewQuestionnaireRefByCode(meta.NewCode("q-code"), "v1"),
		domainAssessment.NewAnswerSheetRef(meta.FromUint64(6001)),
		domainAssessment.NewAdhocOrigin(),
		domainAssessment.WithID(domainAssessment.NewID(7001)),
	)
	if err != nil {
		t.Fatalf("NewAssessment returned error: %v", err)
	}

	writer := &riskScoreWriterStub{}
	handler := NewRiskLevelHandler(NewRiskClassifier(), writer)
	evalCtx := NewContext(a, nil)
	evalCtx.TotalScore = 88
	evalCtx.FactorScores = []domainAssessment.FactorScoreResult{
		domainAssessment.NewFactorScoreResult(
			domainAssessment.NewFactorCode("total"),
			"总分",
			88,
			domainAssessment.RiskLevelNone,
			"",
			"",
			true,
		),
	}

	if err := handler.Handle(context.Background(), evalCtx); err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	if evalCtx.RiskLevel != domainAssessment.RiskLevelSevere {
		t.Fatalf("expected severe risk before persistence, got %s", evalCtx.RiskLevel)
	}
	if !writer.called || writer.ctx != evalCtx {
		t.Fatalf("expected score writer to receive evaluation context")
	}
}
