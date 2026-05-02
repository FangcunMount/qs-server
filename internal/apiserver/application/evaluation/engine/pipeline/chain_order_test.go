package pipeline

import (
	"context"
	"reflect"
	"testing"
)

func TestChainExecutesHandlersInConfiguredOrder(t *testing.T) {
	t.Parallel()

	var calls []string
	chain := NewChain().
		AddHandler(newRecordingHandler("validation", &calls)).
		AddHandler(newRecordingHandler("factor_score", &calls)).
		AddHandler(newRecordingHandler("risk_level", &calls)).
		AddHandler(newRecordingHandler("interpretation", &calls)).
		AddHandler(newRecordingHandler("waiter_notify", &calls))

	if err := chain.Execute(context.Background(), &Context{}); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	want := []string{"validation", "factor_score", "risk_level", "interpretation", "waiter_notify"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
}

type recordingHandler struct {
	*BaseHandler
	calls *[]string
}

func newRecordingHandler(name string, calls *[]string) *recordingHandler {
	return &recordingHandler{BaseHandler: NewBaseHandler(name), calls: calls}
}

func (h *recordingHandler) Handle(ctx context.Context, evalCtx *Context) error {
	*h.calls = append(*h.calls, h.Name())
	return h.Next(ctx, evalCtx)
}
