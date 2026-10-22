package process

import (
	"context"
	"errors"
	eventsubsystem "github.com/FangcunMount/qs-server/internal/apiserver/eventing/subsystem"
	"sync/atomic"
	"testing"
	"time"
)

func TestProcessCloseTimeoutRetainsOriginalTransportAndDependencies(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var gate, transport, container, publisher, database atomic.Int32
	closer := newProcessShutdown(processLifecycleDeps{
		transport: transportLifecycleDeps{closeGRPC: func() { transport.Add(1); close(entered); <-release }},
		container: containerLifecycleDeps{stopAdmission: func() { gate.Add(1) }, containerCleanup: func() error { container.Add(1); return nil }},
		resource:  resourceLifecycleDeps{closePublisher: func() error { publisher.Add(1); return nil }, closeDatabase: func() error { database.Add(1); return nil }},
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for i := 0; i < 8; i++ {
		var pending *eventsubsystem.ShutdownIncomplete
		err := closer.Close(ctx)
		if !errors.As(err, &pending) || pending.PendingClosures != 1 || !errors.Is(err, context.Canceled) {
			t.Fatal("false process shutdown completion", err)
		}
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("real transport drain absent")
	}
	if gate.Load() < 1 || transport.Load() != 1 || container.Load() != 0 || publisher.Load() != 0 || database.Load() != 0 {
		t.Fatal("work restarted or dependencies released early")
	}
	close(release)
	wait, cancelWait := context.WithTimeout(context.Background(), time.Second)
	defer cancelWait()
	if err := closer.Wait(wait); err != nil {
		t.Fatal(err)
	}
	if container.Load() != 1 || publisher.Load() != 1 || database.Load() != 1 {
		t.Fatal("continued shutdown did not release exactly once")
	}
	if err := closer.Close(ctx); err != nil {
		t.Fatal("completion lost to cancelled waiter", err)
	}
}

func TestProcessWaitDoesNotStartShutdown(t *testing.T) {
	var calls atomic.Int32
	closer := newProcessShutdown(processLifecycleDeps{container: containerLifecycleDeps{stopAdmission: func() { calls.Add(1) }}})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var pending *eventsubsystem.ShutdownIncomplete
	err := closer.Wait(ctx)
	if err == nil || errors.As(err, &pending) || calls.Load() != 0 {
		t.Fatal("Wait manufactured shutdown work", err)
	}
}
