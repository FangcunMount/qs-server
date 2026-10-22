package process

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/FangcunMount/reliable-messaging/transaction"
)

func TestWorkerShutdownTimeoutKeepsClientsUntilActualDrain(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	var drains, dependencies atomic.Int32
	s := newWorkerShutdown(lifecycleDeps{
		stopSubscriber:   func() error { drains.Add(1); close(entered); <-release; return nil },
		closeGRPCManager: func() error { dependencies.Add(1); return nil },
		closeDatabase:    func() error { dependencies.Add(1); return nil },
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var pending *transaction.ShutdownIncomplete
	for i := 0; i < 8; i++ {
		if err := s.Close(ctx); !errors.As(err, &pending) || !pending.RuntimePending {
			t.Fatal("in-flight drain lost", err)
		}
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("actual drain never started")
	}
	if drains.Load() != 1 || dependencies.Load() != 0 {
		t.Fatal("duplicate drain or prematurely closed clients")
	}
	once.Do(func() { close(release) })
	wait, cancelWait := context.WithTimeout(context.Background(), time.Second)
	defer cancelWait()
	if err := s.Wait(wait); err != nil {
		t.Fatal(err)
	}
	if dependencies.Load() != 2 {
		t.Fatal("drained dependencies not closed")
	}
	if err := s.Wait(ctx); err != nil {
		t.Fatal("actual completion lost to cancellation", err)
	}
}

func TestWorkerShutdownFailureRetainsDownstreamDependencies(t *testing.T) {
	want := errors.New("subscriber still running")
	s := newWorkerShutdown(lifecycleDeps{
		stopSubscriber:   func() error { return want },
		cleanupContainer: func() error { t.Error("container closed after incomplete subscriber"); return nil },
		closeGRPCManager: func() error { t.Error("gRPC closed after incomplete subscriber"); return nil },
		closeDatabase:    func() error { t.Error("database closed after incomplete subscriber"); return nil },
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.Close(ctx); !errors.Is(err, want) {
		t.Fatal("drain error lost", err)
	}
	if err := s.Close(ctx); !errors.Is(err, want) {
		t.Fatal("repeat close fabricated recovery", err)
	}
}

func TestWorkerMainJoinsTheCallbackOwnedDrain(t *testing.T) {
	entered, release, finished := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	var once sync.Once
	defer once.Do(func() { close(release) })
	s := newWorkerShutdown(lifecycleDeps{stopSubscriber: func() error { close(entered); <-release; return nil }})
	prepared := preparedServer{shutdown: s, startShutdown: func() error {
		go func() { _ = s.Close(context.Background()) }()
		return nil
	}}
	go func() { finished <- prepared.Run() }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("shutdown callback not entered")
	}
	select {
	case err := <-finished:
		t.Fatal("main returned while handler is still running", err)
	default:
	}
	once.Do(func() { close(release) })
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("main failed to join actual completion")
	}
}

func TestWorkerMainStartupFailureDrainsAndPreservesBothErrors(t *testing.T) {
	startup, drain := errors.New("signal manager start failed"), errors.New("drain failed")
	prepared := preparedServer{shutdown: newWorkerShutdown(lifecycleDeps{stopSubscriber: func() error { return drain }}), startShutdown: func() error { return startup }}
	if err := prepared.Run(); !errors.Is(err, startup) || !errors.Is(err, drain) {
		t.Fatal("startup/drain error lost", err)
	}
	if err := (preparedServer{}).Run(); err == nil {
		t.Fatal("missing owned lifecycle accepted")
	}
}
