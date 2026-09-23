//go:build reliable_messaging_m4

package standardoutbox

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/FangcunMount/reliable-messaging/message"
	"github.com/FangcunMount/reliable-messaging/outbox"
	"github.com/FangcunMount/reliable-messaging/relay"
	"github.com/FangcunMount/reliable-messaging/transport"
)

type recoveringScanStore struct {
	mu            sync.Mutex
	scans         int
	secondStarted chan struct{}
	releaseSecond chan struct{}
}

func (s *recoveringScanStore) ClaimDue(ctx context.Context, _ int, _ time.Duration) ([]outbox.Claim, error) {
	s.mu.Lock()
	s.scans++
	scan := s.scans
	s.mu.Unlock()
	if scan == 1 {
		return nil, errors.New("injected scan failure")
	}
	if scan == 2 {
		close(s.secondStarted)
		select {
		case <-s.releaseSecond:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return nil, nil
}
func (*recoveringScanStore) Confirm(context.Context, outbox.Claim) error { return nil }
func (*recoveringScanStore) Retry(context.Context, outbox.Claim, time.Duration, string) error {
	return nil
}
func (*recoveringScanStore) Quarantine(context.Context, outbox.Claim, string) error { return nil }

type confirmedPublisher struct{}

func (confirmedPublisher) Publish(context.Context, message.Message) transport.Result {
	return transport.Result{Outcome: transport.Confirmed}
}

func TestRelaySupervisorKeepsScanUnhealthyUntilActualRecovery(t *testing.T) {
	store := &recoveringScanStore{secondStarted: make(chan struct{}), releaseSecond: make(chan struct{})}
	s, err := NewRelaySupervisor(SupervisorOptions{
		Name: "mongo-domain-events", InitialBackoff: 10 * time.Millisecond, MaxBackoff: 40 * time.Millisecond,
		NewRelay: func(observe relay.Observer) (RelayRunner, error) {
			return relay.New(store, confirmedPublisher{}, relay.Config{
				Concurrency: 1, PollInterval: 50 * time.Millisecond, Lease: time.Second,
				PublishTimeout: 100 * time.Millisecond, WriteTimeout: 100 * time.Millisecond,
				Retry: SDKRetryPolicy(), Observe: observe,
			})
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	select {
	case <-store.secondStarted:
	case <-time.After(time.Second):
		t.Fatal("failed scan was not restarted")
	}
	before := s.Snapshot()
	if !before.Running || before.ScanHealthy || before.Restarts != 1 || before.ConsecutiveFailures != 1 || before.LastFailureKind != "relay_run_failed" {
		t.Fatalf("unhealthy scan was hidden: %+v", before)
	}
	close(store.releaseSecond)
	deadline := time.After(time.Second)
	for !s.Snapshot().ScanHealthy {
		select {
		case <-deadline:
			t.Fatal("successful recovery scan did not restore health")
		case <-time.After(time.Millisecond):
		}
	}
	after := s.Snapshot()
	if after.ConsecutiveFailures != 0 || after.LastSuccessfulScanAt.IsZero() {
		t.Fatalf("recovery status = %+v", after)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("supervisor did not stop")
	}
	if stopped := s.Snapshot(); stopped.Running || stopped.ScanHealthy {
		t.Fatalf("stopped relay reported healthy: %+v", stopped)
	}
}

func TestSupervisorBackoffIsCapped(t *testing.T) {
	for _, tc := range []struct {
		failures uint64
		want     time.Duration
	}{
		{1, 10 * time.Millisecond}, {2, 20 * time.Millisecond}, {3, 40 * time.Millisecond}, {4, 50 * time.Millisecond}, {100, 50 * time.Millisecond},
	} {
		if got := supervisorBackoff(10*time.Millisecond, 50*time.Millisecond, tc.failures); got != tc.want {
			t.Fatalf("backoff(%d) = %s, want %s", tc.failures, got, tc.want)
		}
	}
}

type oneClaimStore struct {
	claimed   atomic.Bool
	confirmed chan struct{}
}

func (s *oneClaimStore) ClaimDue(context.Context, int, time.Duration) ([]outbox.Claim, error) {
	if s.claimed.CompareAndSwap(false, true) {
		return []outbox.Claim{{}}, nil
	}
	return nil, nil
}
func (s *oneClaimStore) Confirm(context.Context, outbox.Claim) error {
	close(s.confirmed)
	return nil
}
func (*oneClaimStore) Retry(context.Context, outbox.Claim, time.Duration, string) error { return nil }
func (*oneClaimStore) Quarantine(context.Context, outbox.Claim, string) error           { return nil }

type heldPublisher struct {
	entered chan struct{}
	release chan struct{}
}

func (p heldPublisher) Publish(context.Context, message.Message) transport.Result {
	close(p.entered)
	<-p.release
	return transport.Result{Outcome: transport.Confirmed}
}

func TestRelaySupervisorWaitsForAdmittedDeliveryOnShutdown(t *testing.T) {
	store := &oneClaimStore{confirmed: make(chan struct{})}
	publisher := heldPublisher{entered: make(chan struct{}), release: make(chan struct{})}
	s, err := NewRelaySupervisor(SupervisorOptions{
		Name: "assessment-mysql-outbox", InitialBackoff: time.Millisecond, MaxBackoff: time.Second,
		NewRelay: func(observe relay.Observer) (RelayRunner, error) {
			return relay.New(store, publisher, relay.Config{
				Concurrency: 1, PollInterval: time.Second, Lease: 4 * time.Second,
				PublishTimeout: 2 * time.Second, WriteTimeout: time.Second,
				Retry: SDKRetryPolicy(), Observe: observe,
			})
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	select {
	case <-publisher.entered:
	case <-time.After(time.Second):
		cancel()
		t.Fatal("delivery was not admitted")
	}
	cancel()
	select {
	case <-done:
		t.Fatal("supervisor returned before admitted delivery settled")
	default:
	}
	close(publisher.release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("supervisor did not finish draining")
	}
	select {
	case <-store.confirmed:
	default:
		t.Fatal("confirmed delivery was not persisted before shutdown")
	}
}
