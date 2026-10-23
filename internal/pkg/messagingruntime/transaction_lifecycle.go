package messagingruntime

import (
	"context"
	"errors"
	"sync"
	"time"

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
	monitor             *protocolMonitor
	cleanupBudget       time.Duration
	mu                  sync.RWMutex
	admission, stopping bool
	startupHeld         bool
}

var _ transaction.Runtime = (*TransactionRuntime)(nil)
var _ transaction.Sender = (*TransactionRuntime)(nil)

func (r *TransactionRuntime) Start(ctx context.Context) error {
	// Establish the initial authoritative observation before producer registration
	// can resolve old halves. Protection never prevents the Checker from starting.
	if r.monitor != nil {
		if err := r.startObservation(ctx); err != nil {
			return r.startFailure(err)
		}
	}
	if err := r.sender.Start(ctx); err != nil {
		if r.monitor == nil {
			return err
		}
		return r.startFailure(err)
	}
	return nil
}
func (r *TransactionRuntime) startFailure(cause error) error {
	budget := r.cleanupBudget
	if budget <= 0 {
		budget = 5 * time.Second
	}
	cleanup, cancel := context.WithTimeout(context.Background(), budget)
	defer cancel()
	return errors.Join(cause, r.Close(cleanup))
}

func (r *TransactionRuntime) SetAdmission(open bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopping || (open && (r.startupHeld || !r.sender.Stats().Ready || !r.monitor.allowed())) {
		return ErrTransactionAdmissionClosed
	}
	r.admission = open
	return nil
}

// HoldStartupAdmission stages the host without changing configured write intent.
// All owners must register before ReleaseStartupAdmission. Neither operation
// clears protection, overrides SDK health, or reopens a stopping runtime.
func (r *TransactionRuntime) HoldStartupAdmission() error    { return r.setStartupHeld(true) }
func (r *TransactionRuntime) ReleaseStartupAdmission() error { return r.setStartupHeld(false) }
func (r *TransactionRuntime) setStartupHeld(held bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopping {
		return ErrTransactionAdmissionClosed
	}
	r.startupHeld = held
	return nil
}

func (r *TransactionRuntime) admitted() bool {
	r.mu.RLock()
	open := r.admission && !r.stopping && !r.startupHeld
	r.mu.RUnlock()
	return open && r.sender.Stats().Ready && r.monitor.allowed()
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
	if err := r.sender.Close(ctx); err != nil {
		return err
	}
	return r.monitor.close(ctx)
}
func (r *TransactionRuntime) Wait(ctx context.Context) error {
	if err := r.sender.Wait(ctx); err != nil {
		return err
	}
	return r.monitor.close(ctx)
}
func (r *TransactionRuntime) Stats() rocketmq.TransactionStats {
	stats := r.sender.Stats()
	stats.Ready = stats.Ready && r.admitted()
	stats.Stopped = stats.Stopped && r.monitor.drained()
	return stats
}

// ReviewProtection cannot open configured admission or restart a stopping owner.
// The maintenance caller records its named review; no review ID is a metric label.
func (r *TransactionRuntime) ReviewProtection(generation uint64, review string) error {
	r.mu.RLock()
	stopped := r.stopping
	r.mu.RUnlock()
	if stopped {
		return ErrProtocolProtection
	}
	return r.monitor.review(generation, review)
}

func (r *TransactionRuntime) ProtectionStats() ProtocolProtectionStats { return r.monitor.snapshot() }

func (r *TransactionRuntime) startObservation(ctx context.Context) error {
	if err := r.monitor.start(); err != nil {
		return err
	}
	return r.monitor.waitInitial(ctx)
}
