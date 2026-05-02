package pipeline

import (
	"context"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	domainAssessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type completionNotifierStub struct {
	called bool
	ctx    *Context
}

func (n *completionNotifierStub) NotifyCompletion(_ context.Context, evalCtx *Context) {
	n.called = true
	n.ctx = evalCtx
}

func TestWaiterNotifyHandlerDelegatesCompletionNotification(t *testing.T) {
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
	evalCtx := NewContext(a, nil)
	evalCtx.EvaluationResult = domainAssessment.NewEvaluationResult(
		88,
		domainAssessment.RiskLevelHigh,
		"high risk",
		"follow up",
		nil,
	)

	notifier := &completionNotifierStub{}
	handler := NewWaiterNotifyHandlerWithNotifier(notifier)
	if err := handler.Handle(context.Background(), evalCtx); err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	if !notifier.called || notifier.ctx != evalCtx {
		t.Fatalf("expected waiter notify handler to delegate completion notification")
	}
}
