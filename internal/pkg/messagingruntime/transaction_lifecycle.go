package messagingruntime

import (
	"context"
	"errors"
	"sync"

	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/FangcunMount/reliable-messaging/transport/rocketmq"
)

// The host gate can pause writes while the SDK's Checker remains running.
// StopAdmission is final for this runtime; reopening requires a new instance.
var ErrTransactionAdmissionClosed = errors.New("host transaction admission closed")

type transactionDriver interface {
	transaction.Runtime
	transaction.Sender
	Reserve(context.Context, transaction.Request, transaction.FrozenPlan) (transaction.Reservation, error)
	Stats() rocketmq.TransactionStats
}

type TransactionRuntime struct {
	sender              transactionDriver
	mu                  sync.RWMutex
	admission, stopping bool
}

var _ transaction.Runtime = (*TransactionRuntime)(nil)
var _ transaction.Sender = (*TransactionRuntime)(nil)

func (r *TransactionRuntime) Start(ctx context.Context) error { return r.sender.Start(ctx) }
func (r *TransactionRuntime) SetAdmission(open bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopping || (open && !r.sender.Stats().Ready) {
		return ErrTransactionAdmissionClosed
	}
	r.admission = open
	return nil
}
func (r *TransactionRuntime) admitted() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.admission && !r.stopping
}
func (r *TransactionRuntime) Reserve(ctx context.Context, q transaction.Request, p transaction.FrozenPlan) (transaction.Reservation, error) {
	if !r.admitted() {
		return transaction.Reservation{}, ErrTransactionAdmissionClosed
	}
	return r.sender.Reserve(ctx, q, p)
}
func (r *TransactionRuntime) Execute(ctx context.Context, e transaction.Execution, local transaction.LocalExecutor) (transaction.Result, error) {
	if !r.admitted() {
		return transaction.InitialResult(e), ErrTransactionAdmissionClosed
	}
	return r.sender.Execute(ctx, e, local)
}
func (r *TransactionRuntime) StopAdmission() {
	r.mu.Lock()
	r.stopping = true
	r.admission = false
	r.mu.Unlock()
	r.sender.StopAdmission()
}
func (r *TransactionRuntime) Close(ctx context.Context) error {
	r.StopAdmission()
	return r.sender.Close(ctx)
}
func (r *TransactionRuntime) Wait(ctx context.Context) error { return r.sender.Wait(ctx) }
func (r *TransactionRuntime) Stats() rocketmq.TransactionStats {
	stats := r.sender.Stats()
	stats.Ready = stats.Ready && r.admitted()
	return stats
}
