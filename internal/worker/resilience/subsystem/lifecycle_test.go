package subsystem

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/resilience/control"
)

type blockingHeartbeater struct {
	control.StateStore
	entered, release chan struct{}
	calls            atomic.Int32
}

func (s *blockingHeartbeater) Heartbeat(context.Context, control.InstanceIdentity, time.Duration) error {
	s.calls.Add(1)
	close(s.entered)
	<-s.release
	return nil
}

func TestWorkerHeartbeatOwnsOneLoopAndWaitsForActualRedisWork(t *testing.T) {
	store := &blockingHeartbeater{entered: make(chan struct{}), release: make(chan struct{})}
	var once sync.Once
	defer once.Do(func() { close(store.release) })
	s, err := New(Options{StateStore: store})
	if err != nil {
		t.Fatal(err)
	}
	stop := s.Start(context.Background())
	defer stop()
	select {
	case <-store.entered:
	case <-time.After(time.Second):
		t.Fatal("heartbeat absent")
	}
	s.Start(context.Background())()
	stop()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.Wait(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal("cancel falsely completed borrowed Redis work", err)
	}
	if store.calls.Load() != 1 {
		t.Fatal("duplicate background heartbeat")
	}
	once.Do(func() { close(store.release) })
	wait, cancelWait := context.WithTimeout(context.Background(), time.Second)
	defer cancelWait()
	if err := s.Wait(wait); err != nil {
		t.Fatal(err)
	}
	if err := s.Wait(ctx); err != nil {
		t.Fatal("completed heartbeat lost to cancellation", err)
	}
}
