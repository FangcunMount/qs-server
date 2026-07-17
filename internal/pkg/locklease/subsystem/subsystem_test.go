package subsystem

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/locklease"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	redisobserve "github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
)

type manualTicker struct{ ch chan time.Time }

func (t *manualTicker) Chan() <-chan time.Time { return t.ch }
func (*manualTicker) Stop()                    {}

type managerStub struct {
	lease        *locklease.Lease
	acquired     bool
	acquireErr   error
	renewOwned   bool
	renewErr     error
	renewCalls   int
	releaseCalls int
}

func (m *managerStub) AcquireSpec(context.Context, locklease.Spec, string, ...time.Duration) (*locklease.Lease, bool, error) {
	return m.lease, m.acquired, m.acquireErr
}

func (m *managerStub) RenewSpec(context.Context, locklease.Spec, string, *locklease.Lease, ...time.Duration) (bool, error) {
	m.renewCalls++
	return m.renewOwned, m.renewErr
}

func (m *managerStub) ReleaseSpec(context.Context, locklease.Spec, string, *locklease.Lease) error {
	m.releaseCalls++
	return nil
}

func TestRunContentionDoesNotStartBody(t *testing.T) {
	manager := &managerStub{}
	s := New(Options{Component: "worker", Manager: manager})
	called := false
	result, err := s.Run(context.Background(), locklease.WorkloadAnswersheetProcessing, "answer:1", time.Minute, func(context.Context) error {
		called = true
		return nil
	})
	if err != nil || result.Acquired || called {
		t.Fatalf("Run() = %+v, %v; body called=%v", result, err, called)
	}
}

func TestRunClassifiesAcquireError(t *testing.T) {
	manager := &managerStub{acquireErr: errors.New("redis down")}
	s := New(Options{Component: "worker", Manager: manager})
	_, err := s.Run(context.Background(), locklease.WorkloadAnswersheetProcessing, "answer:1", time.Minute, nil)
	if !errors.Is(err, locklease.ErrLeaseAcquireFailed) {
		t.Fatalf("Run() error = %v, want ErrLeaseAcquireFailed", err)
	}
}

func TestRunRenewsAtTTLThirdAndCancelsOnOwnershipLoss(t *testing.T) {
	manual := &manualTicker{ch: make(chan time.Time, 1)}
	manager := &managerStub{
		lease:      &locklease.Lease{Key: "lock", Token: "token"},
		acquired:   true,
		renewOwned: false,
	}
	var gotInterval time.Duration
	s := New(Options{
		Component:      "worker",
		Manager:        manager,
		RenewalEnabled: true,
		tickerFactory: func(interval time.Duration) ticker {
			gotInterval = interval
			return manual
		},
	})

	bodyStarted := make(chan struct{})
	finished := make(chan error, 1)
	go func() {
		_, err := s.Run(context.Background(), locklease.WorkloadAnswersheetProcessing, "answer:1", 9*time.Second, func(ctx context.Context) error {
			close(bodyStarted)
			<-ctx.Done()
			return ctx.Err()
		})
		finished <- err
	}()
	<-bodyStarted
	manual.ch <- time.Now()
	err := <-finished
	if gotInterval != 3*time.Second {
		t.Fatalf("ticker interval = %s, want 3s", gotInterval)
	}
	if !errors.Is(err, locklease.ErrLeaseLost) {
		t.Fatalf("Run() error = %v, want ErrLeaseLost", err)
	}
	if manager.renewCalls != 1 || manager.releaseCalls != 1 {
		t.Fatalf("renew/release calls = %d/%d, want 1/1", manager.renewCalls, manager.releaseCalls)
	}
}

func TestRunRenewErrorTakesPrecedenceOverBodyCancellation(t *testing.T) {
	manual := &manualTicker{ch: make(chan time.Time, 1)}
	manager := &managerStub{
		lease:    &locklease.Lease{Key: "lock", Token: "token"},
		acquired: true,
		renewErr: errors.New("redis unavailable"),
	}
	s := New(Options{
		Component:      "collection-server",
		Manager:        manager,
		RenewalEnabled: true,
		tickerFactory:  func(time.Duration) ticker { return manual },
	})

	started := make(chan struct{})
	finished := make(chan error, 1)
	go func() {
		_, err := s.Run(context.Background(), locklease.WorkloadCollectionSubmit, "request:1", time.Minute, func(ctx context.Context) error {
			close(started)
			<-ctx.Done()
			return context.Canceled
		})
		finished <- err
	}()
	<-started
	manual.ch <- time.Now()
	err := <-finished
	if !errors.Is(err, locklease.ErrLeaseRenewFailed) {
		t.Fatalf("Run() error = %v, want ErrLeaseRenewFailed", err)
	}
}

func TestRunRenewalDisabledKeepsFixedTTLBehavior(t *testing.T) {
	manager := &managerStub{
		lease:    &locklease.Lease{Key: "lock", Token: "token"},
		acquired: true,
	}
	s := New(Options{Component: "apiserver", Manager: manager, RenewalEnabled: false})
	result, err := s.Run(context.Background(), locklease.WorkloadStatisticsSync, "stats", time.Minute, func(context.Context) error { return nil })
	if err != nil || !result.Acquired {
		t.Fatalf("Run() = %+v, %v", result, err)
	}
	if manager.renewCalls != 0 || manager.releaseCalls != 1 {
		t.Fatalf("renew/release calls = %d/%d, want 0/1", manager.renewCalls, manager.releaseCalls)
	}
}

func TestSnapshotsDeriveCatalogBindingsAndFamilyHealth(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	status := redisobserve.NewFamilyStatusRegistry("worker")
	status.Update(redisobserve.FamilyStatus{
		Component:  "worker",
		Family:     string(redisruntime.FamilyLock),
		Configured: true,
		Available:  true,
	})
	s := New(Options{
		Component: "worker",
		Handle: &redisruntime.Handle{
			Family:     redisruntime.FamilyLock,
			Client:     client,
			Builder:    keyspace.NewBuilderWithNamespace("cache:lock"),
			Configured: true,
			Available:  true,
		},
		StatusRegistry: status,
		RenewalEnabled: true,
	})

	snapshots := s.Snapshots()
	if len(snapshots) != 1 {
		t.Fatalf("len(Snapshots()) = %d, want 1", len(snapshots))
	}
	got := snapshots[0]
	if got.Name != string(locklease.WorkloadAnswersheetProcessing) || !got.Configured || got.Degraded {
		t.Fatalf("snapshot = %+v, want healthy worker capability", got)
	}
	if got.TTLSeconds != 300 || got.RenewalMode != "auto" || got.RenewEverySeconds != 100 {
		t.Fatalf("renewal projection = %+v", got)
	}
}
