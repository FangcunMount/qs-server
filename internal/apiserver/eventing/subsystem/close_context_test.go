package subsystem

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	rmtransport "github.com/FangcunMount/reliable-messaging/transport"
)

type blockedLifecycleSubscriber struct {
	enteredSubscribe, releaseSubscribe, enteredClose, releaseClose chan struct{}
	stopCalls, closeCalls                                          atomic.Int32
}

func (s *blockedLifecycleSubscriber) Subscribe(string, string, rmtransport.Handler) error {
	close(s.enteredSubscribe)
	<-s.releaseSubscribe
	return nil
}
func (s *blockedLifecycleSubscriber) Stop() { s.stopCalls.Add(1) }
func (s *blockedLifecycleSubscriber) Close() error {
	s.closeCalls.Add(1)
	close(s.enteredClose)
	<-s.releaseClose
	return nil
}

func TestCloseDeadlineRetainsActualStartupAndSubscriberWorkUntilWait(t *testing.T) {
	sub := &blockedLifecycleSubscriber{enteredSubscribe: make(chan struct{}), releaseSubscribe: make(chan struct{}), enteredClose: make(chan struct{}), releaseClose: make(chan struct{})}
	var releaseStart, releaseClose sync.Once
	t.Cleanup(func() {
		releaseStart.Do(func() { close(sub.releaseSubscribe) })
		releaseClose.Do(func() { close(sub.releaseClose) })
	})
	s, err := New(Options{Catalog: loadCatalog(t), PublisherMode: eventruntime.PublishModeMQ, WirePublisher: fakePublisher{}, SDKSubscriberFactory: func() (SDKSubscriber, error) { return sub, nil }})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.RegisterConsumer(hotRankConsumerID, func(context.Context, string, []byte) error { return nil }); err != nil {
		t.Fatal(err)
	}
	started := make(chan error, 1)
	go func() { started <- s.Start(context.Background()) }()
	select {
	case <-sub.enteredSubscribe:
	case <-time.After(time.Second):
		t.Fatal("startup did not enter original subscriber")
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	for i := 0; i < 8; i++ {
		err = s.CloseContext(cancelled)
		var incomplete *ShutdownIncomplete
		if !errors.As(err, &incomplete) || incomplete.PendingClosures != 1 || !errors.Is(err, context.Canceled) {
			t.Fatal("lost actual pending close", err)
		}
	}
	select {
	case <-sub.enteredClose:
		t.Fatal("subscriber closed before startup returned")
	default:
	}
	releaseStart.Do(func() { close(sub.releaseSubscribe) })
	select {
	case <-sub.enteredClose:
	case <-time.After(time.Second):
		t.Fatal("late startup subscriber not owned by closer")
	}
	if err = s.Wait(cancelled); err == nil {
		t.Fatal("blocked subscriber reported drained")
	}
	releaseClose.Do(func() { close(sub.releaseClose) })
	ctx, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	if err = s.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	if err = <-started; err == nil {
		t.Fatal("startup reported ready after shutdown request")
	}
	if err = s.CloseContext(cancelled); err != nil {
		t.Fatal("completed close lost precedence over cancelled caller", err)
	}
	if sub.stopCalls.Load() != 1 || sub.closeCalls.Load() != 1 {
		t.Fatal("multiple close owners", sub.stopCalls.Load(), sub.closeCalls.Load())
	}
}

func TestWaitDoesNotStartImplicitShutdown(t *testing.T) {
	s := &Subsystem{}
	if err := s.Wait(context.Background()); err == nil {
		t.Fatal("wait implicitly started shutdown")
	}
	if s.closed || s.closeStarted {
		t.Fatal("wait mutated admission")
	}
}
