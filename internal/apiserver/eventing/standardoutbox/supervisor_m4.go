//go:build reliable_messaging_m4

package standardoutbox

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/FangcunMount/reliable-messaging/relay"
)

type RelayRunner interface {
	Run(context.Context) error
}

type SupervisorOptions struct {
	Name                       string
	InitialBackoff, MaxBackoff time.Duration
	// NewRelay must only construct a runner from host-owned resources. Its
	// observer is wired before Run, without starting a goroutine or doing I/O.
	NewRelay func(relay.Observer) (RelayRunner, error)
	Observe  relay.Observer
}

// SupervisorSnapshot reports scan progress, not end-to-end message delivery.
// Outbox backlog and publisher outcomes must be monitored separately.
type SupervisorSnapshot struct {
	Running              bool
	ScanHealthy          bool
	Restarts             uint64
	ConsecutiveFailures  uint64
	LastFailureKind      string
	LastFailureAt        time.Time
	LastSuccessfulScanAt time.Time
}

// RelaySupervisor is owned and run by the QS host. It neither starts itself
// nor closes a shared database, NSQ producer or SDK publisher.
type RelaySupervisor struct {
	name                       string
	runner                     RelayRunner
	initialBackoff, maxBackoff time.Duration
	observe                    relay.Observer
	running                    atomic.Bool
	mu                         sync.Mutex
	status                     SupervisorSnapshot
}

func NewRelaySupervisor(opts SupervisorOptions) (*RelaySupervisor, error) {
	if opts.Name == "" || opts.NewRelay == nil || opts.InitialBackoff <= 0 || opts.MaxBackoff < opts.InitialBackoff {
		return nil, fmt.Errorf("relay supervisor requires name, relay factory and valid backoff bounds")
	}
	s := &RelaySupervisor{name: opts.Name, initialBackoff: opts.InitialBackoff, maxBackoff: opts.MaxBackoff, observe: opts.Observe}
	runner, err := opts.NewRelay(s.observeEvent)
	if err != nil {
		return nil, err
	}
	if runner == nil {
		return nil, errors.New("relay supervisor factory returned nil runner")
	}
	s.runner = runner
	return s, nil
}

func (s *RelaySupervisor) observeEvent(evt relay.Event) {
	if evt.Kind == "scan_succeeded" {
		s.mu.Lock()
		s.status.ScanHealthy = true
		s.status.ConsecutiveFailures = 0
		s.status.LastSuccessfulScanAt = time.Now()
		s.mu.Unlock()
	}
	if s.observe != nil {
		s.observe(evt)
	}
}

func (s *RelaySupervisor) Snapshot() SupervisorSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

// Run restarts a failed scan with capped backoff. Cancellation stops new
// admission; SDK Relay.Run drains work before this method returns. The host
// must wait for Run before draining the publisher and closing the producer.
func (s *RelaySupervisor) Run(ctx context.Context) error {
	if ctx == nil {
		return errors.New("relay supervisor requires context")
	}
	if !s.running.CompareAndSwap(false, true) {
		return errors.New("relay supervisor already running")
	}
	defer s.running.Store(false)
	s.mu.Lock()
	s.status.Running = true
	s.status.ScanHealthy = false
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.status.Running = false
		s.status.ScanHealthy = false
		s.mu.Unlock()
	}()
	for {
		if ctx.Err() != nil {
			return nil
		}
		err := s.runner.Run(ctx)
		if ctx.Err() != nil {
			return nil
		}
		if err == nil {
			err = errors.New("relay exited without cancellation")
		}
		s.mu.Lock()
		s.status.ScanHealthy = false
		s.status.ConsecutiveFailures++
		s.status.LastFailureKind = "relay_run_failed"
		s.status.LastFailureAt = time.Now()
		delay := supervisorBackoff(s.initialBackoff, s.maxBackoff, s.status.ConsecutiveFailures)
		s.mu.Unlock()
		slog.Warn("standard outbox relay stopped; retrying", "profile", s.name, "error", err, "retry_after", delay)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
		s.mu.Lock()
		s.status.Restarts++
		s.mu.Unlock()
	}
}

func supervisorBackoff(initial, max time.Duration, failures uint64) time.Duration {
	delay := initial
	for i := uint64(1); i < failures && delay < max; i++ {
		if delay >= max/2 {
			return max
		}
		delay *= 2
	}
	return delay
}
