package eventtransaction

import (
	"context"
	"testing"

	port "github.com/FangcunMount/qs-server/internal/apiserver/port/eventtransaction"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/stretchr/testify/require"
)

type runnerFunc func(context.Context, func(context.Context) error) error

func (f runnerFunc) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	return f(ctx, fn)
}

type capturedSender struct{ local transaction.LocalExecutor }

func (s *capturedSender) Execute(_ context.Context, e transaction.Execution, fn transaction.LocalExecutor) (transaction.Result, error) {
	s.local = fn
	return transaction.InitialResult(e), context.Canceled
}

func TestNoLateExecutorCanOpenAnOriginalRootAfterReturn(t *testing.T) {
	o, _, _, e, _ := ownerFixture(t)
	sender := &capturedSender{}
	o.sender = sender
	called := false
	runner := runnerFunc(func(context.Context, func(context.Context) error) error { called = true; return nil })
	result, err := o.ExecuteOriginal(t.Context(), e, runner, func(context.Context) error { t.Fatal("business unexpectedly executed"); return nil })
	require.ErrorIs(t, err, port.ErrUnresolved)
	require.Equal(t, transaction.LocalNotStarted, result.Local)
	local, err := sender.local(t.Context(), e)
	require.ErrorIs(t, err, transaction.ErrStaleExecution)
	require.Equal(t, transaction.LocalUnknown, local)
	require.False(t, called)
}

type proofUnknownSender struct{ reported transaction.LocalOutcome }

func (s *proofUnknownSender) Execute(ctx context.Context, e transaction.Execution, fn transaction.LocalExecutor) (transaction.Result, error) {
	s.reported, _ = fn(ctx, e)
	r := transaction.InitialResult(e)
	r.Local = transaction.LocalUnknown // Explicit simulated missing authority proof, not a Broker test.
	return r, nil
}

func TestRunnerNilWithoutOriginalGuardCannotProveCommit(t *testing.T) {
	o, _, _, e, _ := ownerFixture(t)
	sender := &proofUnknownSender{}
	o.sender = sender
	runner := runnerFunc(func(context.Context, func(context.Context) error) error { return nil })
	result, err := o.ExecuteOriginal(t.Context(), e, runner, func(context.Context) error { t.Fatal("business unexpectedly executed"); return nil })
	require.ErrorIs(t, err, port.ErrUnresolved)
	require.Equal(t, transaction.LocalUnknown, sender.reported)
	require.Equal(t, transaction.LocalUnknown, result.Local)
}
