package subsystem

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/locklease"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	redisobserve "github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
	renewFunc    func(context.Context) (bool, error)
	releaseErr   error
	renewCalls   int
	releaseCalls int
}

func (m *managerStub) AcquireSpec(context.Context, locklease.Spec, string, ...time.Duration) (*locklease.Lease, bool, error) {
	return m.lease, m.acquired, m.acquireErr
}

func (m *managerStub) RenewSpec(ctx context.Context, _ locklease.Spec, _ string, _ *locklease.Lease, _ ...time.Duration) (bool, error) {
	m.renewCalls++
	if m.renewFunc != nil {
		return m.renewFunc(ctx)
	}
	return m.renewOwned, m.renewErr
}

func (m *managerStub) ReleaseSpec(context.Context, locklease.Spec, string, *locklease.Lease) error {
	m.releaseCalls++
	return m.releaseErr
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

func TestRunParentCancellationDuringAcquireIsNotAcquireFailure(t *testing.T) {
	manager := &managerStub{acquireErr: errors.New("manager should not be called")}
	s := New(Options{Component: "worker", Manager: manager})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := s.Run(ctx, locklease.WorkloadAnswersheetProcessing, "answer:cancel", time.Minute, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
	if errors.Is(err, locklease.ErrLeaseAcquireFailed) {
		t.Fatalf("Run() error = %v, must not be ErrLeaseAcquireFailed", err)
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

func TestRunRenewErrorTakesPrecedenceOverGRPCCancellation(t *testing.T) {
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
		_, err := s.Run(context.Background(), locklease.WorkloadCollectionSubmit, "request:grpc", time.Minute, func(ctx context.Context) error {
			close(started)
			<-ctx.Done()
			return status.Error(codes.Canceled, "context canceled")
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

func TestRunParentCancellationDuringRenewIsNotLeaseFailure(t *testing.T) {
	manual := &manualTicker{ch: make(chan time.Time, 1)}
	renewStarted := make(chan struct{})
	manager := &managerStub{
		lease:    &locklease.Lease{Key: "lock", Token: "token"},
		acquired: true,
		renewFunc: func(ctx context.Context) (bool, error) {
			close(renewStarted)
			<-ctx.Done()
			return false, ctx.Err()
		},
	}
	s := New(Options{
		Component:      "worker",
		Manager:        manager,
		RenewalEnabled: true,
		tickerFactory:  func(time.Duration) ticker { return manual },
	})

	parent, cancel := context.WithCancel(context.Background())
	bodyStarted := make(chan struct{})
	finished := make(chan error, 1)
	go func() {
		_, err := s.Run(parent, locklease.WorkloadAnswersheetProcessing, "answer:cancel", time.Minute, func(ctx context.Context) error {
			close(bodyStarted)
			<-ctx.Done()
			return ctx.Err()
		})
		finished <- err
	}()
	<-bodyStarted
	manual.ch <- time.Now()
	<-renewStarted
	cancel()
	err := <-finished
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
	if errors.Is(err, locklease.ErrLeaseRenewFailed) {
		t.Fatalf("Run() error = %v, must not be ErrLeaseRenewFailed", err)
	}
}

func TestRunBodyCompletionCancelsInFlightRenew(t *testing.T) {
	manual := &manualTicker{ch: make(chan time.Time, 1)}
	bodyStarted := make(chan struct{})
	allowBodyReturn := make(chan struct{})
	renewStarted := make(chan struct{})
	unblockRenew := make(chan struct{})
	manager := &managerStub{
		lease:    &locklease.Lease{Key: "lock", Token: "token"},
		acquired: true,
		renewFunc: func(ctx context.Context) (bool, error) {
			close(renewStarted)
			select {
			case <-ctx.Done():
				return false, ctx.Err()
			case <-unblockRenew:
				return true, nil
			}
		},
	}
	s := New(Options{
		Component:      "worker",
		Manager:        manager,
		RenewalEnabled: true,
		tickerFactory:  func(time.Duration) ticker { return manual },
	})

	finished := make(chan error, 1)
	go func() {
		_, err := s.Run(context.Background(), locklease.WorkloadAnswersheetProcessing, "answer:done", time.Minute, func(context.Context) error {
			close(bodyStarted)
			<-allowBodyReturn
			return nil
		})
		finished <- err
	}()
	<-bodyStarted
	manual.ch <- time.Now()
	<-renewStarted
	close(allowBodyReturn)
	select {
	case err := <-finished:
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		close(unblockRenew)
		<-finished
		t.Fatal("Run() did not cancel the in-flight renewal after body completion")
	}
}

func TestRunPreservesBusinessErrorAndReportsReleaseError(t *testing.T) {
	bodyErr := errors.New("body failed")
	releaseErr := errors.New("release failed")
	manager := &managerStub{
		lease:      &locklease.Lease{Key: "lock", Token: "token"},
		acquired:   true,
		releaseErr: releaseErr,
	}
	s := New(Options{Component: "worker", Manager: manager})

	result, err := s.Run(context.Background(), locklease.WorkloadAnswersheetProcessing, "answer:error", time.Minute, func(context.Context) error {
		return bodyErr
	})
	if !errors.Is(err, bodyErr) {
		t.Fatalf("Run() error = %v, want body error", err)
	}
	if !errors.Is(result.ReleaseErr, releaseErr) {
		t.Fatalf("Run() release error = %v, want %v", result.ReleaseErr, releaseErr)
	}
}

func TestRunWarnsUntilBodyStopsAfterParentCancellation(t *testing.T) {
	manual := &manualTicker{ch: make(chan time.Time, 2)}
	manager := &managerStub{
		lease:      &locklease.Lease{Key: "lock", Token: "token"},
		acquired:   true,
		renewOwned: true,
	}
	warnings := make(chan string, 1)
	s := New(Options{
		Component:      "worker",
		Manager:        manager,
		RenewalEnabled: true,
		Warn:           func(message string) { warnings <- message },
		tickerFactory:  func(time.Duration) ticker { return manual },
	})

	parent, cancel := context.WithCancel(context.Background())
	bodyStarted := make(chan struct{})
	allowBodyReturn := make(chan struct{})
	finished := make(chan error, 1)
	go func() {
		_, err := s.Run(parent, locklease.WorkloadAnswersheetProcessing, "answer:warn", time.Minute, func(context.Context) error {
			close(bodyStarted)
			<-allowBodyReturn
			return context.Canceled
		})
		finished <- err
	}()
	<-bodyStarted
	cancel()
	manual.ch <- time.Now()
	manual.ch <- time.Now()

	select {
	case warning := <-warnings:
		if !strings.Contains(warning, "component worker") || !strings.Contains(warning, "answersheet_processing") {
			t.Fatalf("warning = %q", warning)
		}
	case <-time.After(time.Second):
		t.Fatal("expected non-responsive body warning")
	}
	close(allowBodyReturn)
	if err := <-finished; !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
}

func TestRunWarnsUntilBodyStopsAfterLeaseLoss(t *testing.T) {
	manual := &manualTicker{ch: make(chan time.Time, 2)}
	manager := &managerStub{
		lease:      &locklease.Lease{Key: "lock", Token: "token"},
		acquired:   true,
		renewOwned: false,
	}
	warnings := make(chan string, 1)
	s := New(Options{
		Component:      "collection-server",
		Manager:        manager,
		RenewalEnabled: true,
		Warn:           func(message string) { warnings <- message },
		tickerFactory:  func(time.Duration) ticker { return manual },
	})

	bodyStarted := make(chan struct{})
	allowBodyReturn := make(chan struct{})
	finished := make(chan error, 1)
	go func() {
		_, err := s.Run(context.Background(), locklease.WorkloadCollectionSubmit, "request:warn", time.Minute, func(context.Context) error {
			close(bodyStarted)
			<-allowBodyReturn
			return status.Error(codes.Canceled, "context canceled")
		})
		finished <- err
	}()
	<-bodyStarted
	manual.ch <- time.Now()
	manual.ch <- time.Now()

	select {
	case warning := <-warnings:
		if !strings.Contains(warning, "component collection-server") || !strings.Contains(warning, "collection_submit") {
			t.Fatalf("warning = %q", warning)
		}
	case <-time.After(time.Second):
		t.Fatal("expected lease-loss body warning")
	}
	close(allowBodyReturn)
	if err := <-finished; !errors.Is(err, locklease.ErrLeaseLost) {
		t.Fatalf("Run() error = %v, want ErrLeaseLost", err)
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

func TestRelinquishLeaderCancelsBodyBeforeTokenSafeRelease(t *testing.T) {
	manager := &managerStub{
		lease:    &locklease.Lease{Key: "lock", Token: "token"},
		acquired: true,
	}
	s := New(Options{Component: "apiserver", Manager: manager, RenewalEnabled: false})
	bodyStarted := make(chan struct{})
	runFinished := make(chan error, 1)
	go func() {
		_, err := s.Run(context.Background(), locklease.WorkloadPlanSchedulerLeader, "leader", time.Minute, func(ctx context.Context) error {
			close(bodyStarted)
			<-ctx.Done()
			return context.Cause(ctx)
		})
		runFinished <- err
	}()
	<-bodyStarted

	result, err := s.RelinquishLeader(context.Background(), locklease.WorkloadPlanSchedulerLeader, locklease.RelinquishOptions{
		Cooldown: time.Minute,
		Timeout:  time.Second,
	})
	if err != nil || result.ActiveCount != 1 || result.Relinquished != 1 {
		t.Fatalf("RelinquishLeader() = %+v, %v", result, err)
	}
	if err := <-runFinished; !errors.Is(err, locklease.ErrLeaseRelinquished) {
		t.Fatalf("Run() error = %v", err)
	}
	if manager.releaseCalls != 1 {
		t.Fatalf("release calls = %d, want 1", manager.releaseCalls)
	}
	second, err := s.Run(context.Background(), locklease.WorkloadPlanSchedulerLeader, "leader", time.Minute, func(context.Context) error {
		t.Fatal("body must not run during cooldown")
		return nil
	})
	if err != nil || second.Acquired {
		t.Fatalf("Run() during cooldown = %+v, %v", second, err)
	}
}

func TestRelinquishLeaderTimeoutDoesNotReleaseUnstoppedBody(t *testing.T) {
	manager := &managerStub{
		lease:    &locklease.Lease{Key: "lock", Token: "token"},
		acquired: true,
	}
	s := New(Options{Component: "apiserver", Manager: manager})
	bodyStarted := make(chan struct{})
	allowReturn := make(chan struct{})
	runFinished := make(chan error, 1)
	go func() {
		_, err := s.Run(context.Background(), locklease.WorkloadPlanSchedulerLeader, "leader", time.Minute, func(context.Context) error {
			close(bodyStarted)
			<-allowReturn
			return nil
		})
		runFinished <- err
	}()
	<-bodyStarted
	result, err := s.RelinquishLeader(context.Background(), locklease.WorkloadPlanSchedulerLeader, locklease.RelinquishOptions{Timeout: 20 * time.Millisecond})
	if !errors.Is(err, context.DeadlineExceeded) || result.Relinquished != 0 {
		t.Fatalf("RelinquishLeader() = %+v, %v", result, err)
	}
	if manager.releaseCalls != 0 {
		t.Fatalf("release calls before body exit = %d", manager.releaseCalls)
	}
	close(allowReturn)
	if err := <-runFinished; err != nil {
		t.Fatal(err)
	}
	if manager.releaseCalls != 1 {
		t.Fatalf("release calls after body exit = %d", manager.releaseCalls)
	}
}

func TestRelinquishLeaderRejectsNonLeaderKinds(t *testing.T) {
	s := New(Options{Component: "apiserver", Manager: &managerStub{}})
	if _, err := s.RelinquishLeader(context.Background(), locklease.WorkloadStatisticsSync, locklease.RelinquishOptions{}); err == nil {
		t.Fatal("expected task lock relinquish rejection")
	}
	worker := New(Options{Component: "worker", Manager: &managerStub{}})
	if _, err := worker.RelinquishLeader(context.Background(), locklease.WorkloadAnswersheetProcessing, locklease.RelinquishOptions{}); err == nil {
		t.Fatal("expected duplicate suppression relinquish rejection")
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
