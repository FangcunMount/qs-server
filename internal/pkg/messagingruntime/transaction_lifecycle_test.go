package messagingruntime

import (
	"context"
	"errors"
	"testing"

	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/FangcunMount/reliable-messaging/transport/rocketmq"
)

type transactionDriverStub struct {
	ready                                                        bool
	reserveCalls, executeCalls, stopCalls, closeCalls, waitCalls int
	closeErr                                                     error
}

func (s *transactionDriverStub) Start(context.Context) error { s.ready = true; return nil }
func (s *transactionDriverStub) StopAdmission()              { s.stopCalls++; s.ready = false }
func (s *transactionDriverStub) Close(context.Context) error { s.closeCalls++; return s.closeErr }
func (s *transactionDriverStub) Wait(context.Context) error  { s.waitCalls++; return nil }
func (s *transactionDriverStub) Stats() rocketmq.TransactionStats {
	return rocketmq.TransactionStats{Ready: s.ready}
}
func (s *transactionDriverStub) Reserve(context.Context, transaction.Request, transaction.FrozenPlan) (transaction.Reservation, error) {
	s.reserveCalls++
	return transaction.Reservation{}, nil
}
func (s *transactionDriverStub) Execute(context.Context, transaction.Execution, transaction.LocalExecutor) (transaction.Result, error) {
	s.executeCalls++
	return transaction.Result{}, nil
}

func TestTransactionPauseKeepsCheckerAndGatesBothEntryPoints(t *testing.T) {
	ctx := context.Background()
	driver := &transactionDriverStub{}
	runtime := &TransactionRuntime{sender: driver}
	if err := runtime.SetAdmission(true); err == nil {
		t.Fatal("opened before SDK ready")
	}
	if err := runtime.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if runtime.Stats().Ready {
		t.Fatal("startup ignored closed host admission")
	}
	if _, err := runtime.Reserve(ctx, transaction.Request{}, transaction.FrozenPlan{}); !errors.Is(err, ErrTransactionAdmissionClosed) {
		t.Fatal(err)
	}
	if _, err := runtime.Execute(ctx, transaction.Execution{}, nil); !errors.Is(err, ErrTransactionAdmissionClosed) {
		t.Fatal(err)
	}
	if driver.reserveCalls+driver.executeCalls != 0 {
		t.Fatal("paused entry reached SDK")
	}
	if err := runtime.SetAdmission(true); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Reserve(ctx, transaction.Request{}, transaction.FrozenPlan{}); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Execute(ctx, transaction.Execution{}, nil); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SetAdmission(false); err != nil {
		t.Fatal(err)
	}
	if driver.stopCalls != 0 || !driver.ready || driver.reserveCalls != 1 || driver.executeCalls != 1 {
		t.Fatal("pause stopped Checker or lost calls")
	}
}

func TestTransactionClosePreservesIncompleteAndContinuedWait(t *testing.T) {
	pending := &transaction.ShutdownIncomplete{ActiveSends: 1}
	driver := &transactionDriverStub{ready: true, closeErr: pending}
	runtime := &TransactionRuntime{sender: driver, admission: true}
	if err := runtime.Close(context.Background()); err != pending {
		t.Fatalf("lost incomplete result: %v", err)
	}
	if runtime.Stats().Ready || driver.stopCalls != 1 || driver.closeCalls != 1 {
		t.Fatal("close did not seal admission")
	}
	if err := runtime.SetAdmission(true); err == nil {
		t.Fatal("reopened stopped runtime")
	}
	if err := runtime.Wait(context.Background()); err != nil || driver.waitCalls != 1 {
		t.Fatal("wait unavailable after incomplete")
	}
	if err := runtime.Close(context.Background()); err != pending {
		t.Fatal("repeated close lost result")
	}
}
