package messagingruntime

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/FangcunMount/reliable-messaging/transport/rocketmq"
	"github.com/prometheus/client_golang/prometheus"
)

// The host owns this one sampler. Scrapes read cached metadata and actual SDK
// counters, never start DB work or tasks. Shutdown joins the real database call.
type protocolMonitor struct {
	mu                          sync.Mutex
	policy                      protocolProtection
	read                        func(context.Context) (protocolSample, error)
	stats                       func() rocketmq.TransactionStats
	admitted                    func() bool
	interval, budget, freshness time.Duration
	workers, queue              int
	registerer                  prometheus.Registerer
	collector                   *protocolCollector
	cancel                      context.CancelFunc
	done                        chan struct{}
	started, stopped            bool
	inflight                    bool
	initialDone                 chan struct{}
	initialOnce                 sync.Once
	seenInvalidChecks           int64
}

func newProtocolMonitor(owner string, read func(context.Context) (protocolSample, error), stats func() rocketmq.TransactionStats, admitted func() bool, limits transaction.Limits, registerer prometheus.Registerer) (*protocolMonitor, error) {
	if owner == "" || read == nil || stats == nil || admitted == nil || registerer == nil || limits.CheckWorkers < 1 || limits.CheckQueue < 0 || limits.CheckTimeout <= 0 || limits.ReconcileInterval <= 0 {
		return nil, errors.New("explicit protocol observation dependencies and budgets required")
	}
	m := &protocolMonitor{read: read, stats: stats, admitted: admitted, interval: limits.ReconcileInterval, budget: limits.CheckTimeout, freshness: 2*limits.ReconcileInterval + limits.CheckTimeout, workers: limits.CheckWorkers, queue: limits.CheckQueue, registerer: registerer}
	m.collector = newProtocolCollector(owner, m)
	return m, nil
}

func (m *protocolMonitor) start() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.started || m.stopped {
		return errors.New("protocol observation already started or stopped")
	}
	if err := m.registerer.Register(m.collector); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.started = true
	m.cancel = cancel
	m.done = make(chan struct{})
	m.initialDone = make(chan struct{})
	go func() {
		defer close(m.done)
		defer m.initialOnce.Do(func() { close(m.initialDone) })
		ticker := time.NewTicker(m.interval)
		defer ticker.Stop()
		for {
			if ctx.Err() != nil {
				return
			}
			m.sample(ctx)
			m.initialOnce.Do(func() { close(m.initialDone) })
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return nil
}
func (m *protocolMonitor) sample(ctx context.Context) {
	m.mu.Lock()
	if m.stopped || m.inflight || ctx.Err() != nil {
		m.mu.Unlock()
		return
	}
	m.inflight = true
	m.mu.Unlock()
	defer func() { m.mu.Lock(); m.inflight = false; m.mu.Unlock() }()
	check, cancel := context.WithTimeout(ctx, m.budget)
	sample, err := m.read(check)
	cancel()
	// Cancellation from shutdown is not a new database incident; ownership is
	// still held until read returned and the sampler goroutine exits.
	if ctx.Err() != nil {
		return
	}
	stats := m.stats()
	full := stats.RunningChecks+stats.QueuedChecks >= m.workers+m.queue
	bindingConflict := errors.Is(err, transaction.ErrBinding) || errors.Is(err, transaction.ErrSchema)
	m.mu.Lock()
	defer m.mu.Unlock()
	if stats.InvalidChecks > m.seenInvalidChecks {
		m.policy.latch("invalid_check")
	}
	m.seenInvalidChecks = stats.InvalidChecks
	m.policy.observe(time.Now(), sample, err, bindingConflict, full, m.freshness)
}
func (m *protocolMonitor) allowed() bool {
	if m == nil {
		return true
	} // Bare test drivers have no production composition.
	stats := m.stats()
	m.mu.Lock()
	defer m.mu.Unlock()
	return !m.stopped && stats.InvalidChecks <= m.seenInvalidChecks && m.policy.allowed(time.Now(), m.freshness)
}
func (m *protocolMonitor) review(generation uint64, review string) error {
	if m == nil {
		return ErrProtocolProtection
	}
	stats := m.stats()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stopped || stats.InvalidChecks > m.seenInvalidChecks {
		return ErrProtocolProtection
	}
	return m.policy.review(time.Now(), m.freshness, generation, review, stats.Ready, stats.RunningChecks+stats.QueuedChecks >= m.workers+m.queue)
}
func (m *protocolMonitor) close(ctx context.Context) error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	m.stopped = true
	cancel, done, started := m.cancel, m.done, m.started
	m.mu.Unlock()
	if !started {
		return nil
	}
	cancel()
	select {
	case <-done:
		m.registerer.Unregister(m.collector)
		return nil
	default:
	}
	select {
	case <-done:
		m.registerer.Unregister(m.collector)
		return nil
	case <-ctx.Done():
		return &transaction.ShutdownIncomplete{RuntimePending: true, Cause: ctx.Err()}
	}
}

// ProtocolProtectionStats contains only process-local protection diagnostics.
// It is not a message or business operation status API.
type ProtocolProtectionStats struct {
	Available, Fresh, Protected             bool
	Generation                              uint64
	Reason                                  string
	ObservationErrors                       int64
	ConfirmationGapBranches                 int64
	OldestConfirmationObservationAgeSeconds float64
	SamplerRunning, ObservationInFlight     bool
}

func (m *protocolMonitor) snapshot() ProtocolProtectionStats {
	if m == nil {
		return ProtocolProtectionStats{}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	running := m.started
	if running {
		select {
		case <-m.done:
			running = false
		default:
		}
	}
	return ProtocolProtectionStats{Available: m.policy.available, Fresh: m.policy.fresh(time.Now(), m.freshness), Protected: m.policy.reason != "", Generation: m.policy.generation, Reason: m.policy.reason, ObservationErrors: m.policy.errors,
		ConfirmationGapBranches: m.policy.sample.gaps, OldestConfirmationObservationAgeSeconds: m.policy.sample.confirmationAge,
		SamplerRunning: running, ObservationInFlight: m.inflight}
}

func (m *protocolMonitor) waitInitial(ctx context.Context) error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	initial := m.initialDone
	m.mu.Unlock()
	if initial == nil {
		return ErrProtocolProtection
	}
	select {
	case <-initial:
		return nil
	case <-ctx.Done():
		return &transaction.ShutdownIncomplete{RuntimePending: true, Cause: ctx.Err()}
	}
}

func (m *protocolMonitor) drained() bool {
	if m == nil {
		return true
	}
	m.mu.Lock()
	stopped, started, done := m.stopped, m.started, m.done
	m.mu.Unlock()
	if !stopped {
		return false
	}
	if !started {
		return true
	}
	select {
	case <-done:
		return true
	default:
		return false
	}
}

func (m *protocolMonitor) running() bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	started, done := m.started, m.done
	m.mu.Unlock()
	if !started {
		return false
	}
	select {
	case <-done:
		return false
	default:
		return true
	}
}
