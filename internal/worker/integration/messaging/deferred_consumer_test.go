package messaging

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	"github.com/FangcunMount/reliable-messaging/transaction"
	rmtransport "github.com/FangcunMount/reliable-messaging/transport"
	"github.com/stretchr/testify/require"
)

type deferredDispatchFunc func(context.Context, string, []byte) (eventruntime.DispatchResult, error)

func (f deferredDispatchFunc) DispatchEvent(ctx context.Context, kind string, body []byte) (eventruntime.DispatchResult, error) {
	return f(ctx, kind, body)
}

type deferredStoreStub struct {
	claims, renews, completed, failedCount atomic.Int32
	renewFn                                func(context.Context) error
	failedFn                               func(deferredFailure)
	items                                  int32
}

func (s *deferredStoreStub) claim(context.Context, time.Time, time.Duration) (*deferredItem, error) {
	if s.claims.Add(1) > s.items {
		return nil, nil
	}
	return &deferredItem{ID: 1, Token: "owned", EventType: "evaluation.retry.requested", Message: rmtransport.Received{ID: "original-id", Payload: []byte("original frozen payload")}}, nil
}
func (s *deferredStoreStub) renew(ctx context.Context, _ *deferredItem, _ time.Time, _ time.Duration) error {
	s.renews.Add(1)
	if s.renewFn != nil {
		return s.renewFn(ctx)
	}
	return nil
}
func (s *deferredStoreStub) complete(ctx context.Context, _ *deferredItem, _ time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.completed.Add(1)
	return nil
}
func (s *deferredStoreStub) failed(_ context.Context, _ *deferredItem, _ time.Time, kind deferredFailure) error {
	s.failedCount.Add(1)
	if s.failedFn != nil {
		s.failedFn(kind)
	}
	return nil
}

func testDeferredConsumer(t *testing.T, s *deferredStoreStub, dispatch deferredDispatchFunc) *DeferredConsumer {
	t.Helper()
	o := DefaultDeferredConsumerOptions(10 * time.Millisecond)
	o.Interval = time.Hour
	o.RenewEvery = 5 * time.Millisecond
	r, err := newDeferredConsumer(s, dispatch, o)
	require.NoError(t, err)
	return r
}
func awaitDeferred(t *testing.T, ready func() bool) {
	t.Helper()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for !ready() {
		select {
		case <-deadline.C:
			t.Fatal("owned deferred work did not reach expected state")
		case <-tick.C:
		}
	}
}
func waitDeferredClosed(t *testing.T, r *DeferredConsumer) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, r.Close(ctx))
}

func TestDeferredCancellationKeepsLeaseAndCapacityUntilRealCallbackReturns(t *testing.T) {
	entered, release := make(chan context.Context, 1), make(chan struct{})
	var once sync.Once
	s := &deferredStoreStub{items: 1}
	r := testDeferredConsumer(t, s, func(ctx context.Context, kind string, body []byte) (eventruntime.DispatchResult, error) {
		if kind != "evaluation.retry.requested" || string(body) != "original frozen payload" {
			t.Error("message identity/body changed")
		}
		entered <- ctx
		<-release
		return eventruntime.DispatchResult{Outcome: eventruntime.DispatchHandled}, nil
	})
	t.Cleanup(func() { once.Do(func() { close(release) }); waitDeferredClosed(t, r) })
	require.Zero(t, s.claims.Load(), "constructor performed IO")
	require.NoError(t, r.Start(context.Background()))
	require.NoError(t, r.Start(context.Background()))
	var work context.Context
	select {
	case work = <-entered:
	case <-time.After(time.Second):
		t.Fatal("callback absent")
	}
	select {
	case <-work.Done():
	case <-time.After(time.Second):
		t.Fatal("handler deadline absent")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var incomplete *transaction.ShutdownIncomplete
	require.ErrorAs(t, r.Close(ctx), &incomplete)
	before := s.renews.Load()
	awaitDeferred(t, func() bool { return s.renews.Load() >= before+2 })
	require.EqualValues(t, 1, r.Stats().Active)
	require.Zero(t, s.completed.Load())
	once.Do(func() { close(release) })
	waitDeferredClosed(t, r)
	require.EqualValues(t, 1, s.completed.Load())
	require.Zero(t, r.Stats().Active)
	require.EqualValues(t, 1, s.claims.Load())
	require.NoError(t, r.Wait(ctx), "real completion lost to caller cancellation")
}

func TestDeferredJoinsRealRenewalEvenAfterCallbackReturned(t *testing.T) {
	renewEntered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	s := &deferredStoreStub{items: 1, renewFn: func(context.Context) error { close(renewEntered); <-release; return nil }}
	r := testDeferredConsumer(t, s, func(context.Context, string, []byte) (eventruntime.DispatchResult, error) {
		<-renewEntered
		return eventruntime.DispatchResult{Outcome: eventruntime.DispatchHandled}, nil
	})
	t.Cleanup(func() { once.Do(func() { close(release) }); waitDeferredClosed(t, r) })
	require.NoError(t, r.Start(context.Background()))
	select {
	case <-renewEntered:
	case <-time.After(time.Second):
		t.Fatal("renewal absent")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.Error(t, r.Close(ctx))
	require.EqualValues(t, 1, r.Stats().Active)
	require.Zero(t, s.completed.Load())
	once.Do(func() { close(release) })
	waitDeferredClosed(t, r)
	require.EqualValues(t, 1, s.completed.Load())
}

func TestDeferredUnknownLeaseCancelsButDoesNotReleaseActualWork(t *testing.T) {
	cancelled, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	s := &deferredStoreStub{items: 1, renewFn: func(context.Context) error { return ErrDeferredClaimLost }}
	r := testDeferredConsumer(t, s, func(ctx context.Context, _ string, _ []byte) (eventruntime.DispatchResult, error) {
		<-ctx.Done()
		close(cancelled)
		<-release
		return eventruntime.DispatchResult{Outcome: eventruntime.DispatchHandled}, nil
	})
	t.Cleanup(func() { once.Do(func() { close(release) }); waitDeferredClosed(t, r) })
	require.NoError(t, r.Start(context.Background()))
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("uncertain ownership did not cancel work")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.Error(t, r.Close(ctx))
	require.EqualValues(t, 1, r.Stats().Active)
	once.Do(func() { close(release) })
	waitDeferredClosed(t, r)
	require.Zero(t, s.completed.Load())
	require.Zero(t, s.failedCount.Load(), "unknown lease cannot confirm either terminal state")
}

func TestDeferredUnknownHandlerIsHeldForManualDisposition(t *testing.T) {
	seen := make(chan deferredFailure, 1)
	s := &deferredStoreStub{items: 1, failedFn: func(kind deferredFailure) { seen <- kind }}
	r := testDeferredConsumer(t, s, func(context.Context, string, []byte) (eventruntime.DispatchResult, error) {
		return eventruntime.DispatchResult{Outcome: eventruntime.DispatchUnknown}, nil
	})
	t.Cleanup(func() { waitDeferredClosed(t, r) })
	require.NoError(t, r.Start(context.Background()))
	select {
	case kind := <-seen:
		require.Equal(t, deferredUnknownHandler, kind)
	case <-time.After(time.Second):
		t.Fatal("manual outcome absent")
	}
	require.Zero(t, s.completed.Load())
}

func TestDeferredBatchAndLifecycleRemainBounded(t *testing.T) {
	s := &deferredStoreStub{items: 60}
	r := testDeferredConsumer(t, s, func(context.Context, string, []byte) (eventruntime.DispatchResult, error) {
		return eventruntime.DispatchResult{Outcome: eventruntime.DispatchHandled}, nil
	})
	t.Cleanup(func() { waitDeferredClosed(t, r) })
	require.NoError(t, r.Start(context.Background()))
	awaitDeferred(t, func() bool { return r.Stats().Completed == 50 })
	waitDeferredClosed(t, r)
	require.EqualValues(t, 50, s.claims.Load())
	require.Error(t, r.Start(context.Background()))
	idle := &deferredStoreStub{}
	closed := testDeferredConsumer(t, idle, func(context.Context, string, []byte) (eventruntime.DispatchResult, error) {
		t.Error("closed constructor executed business")
		return eventruntime.DispatchResult{}, nil
	})
	waitDeferredClosed(t, closed)
	require.Error(t, closed.Start(context.Background()))
	require.Zero(t, idle.claims.Load())
	bad := DefaultDeferredConsumerOptions(time.Second)
	bad.Batch = 51
	_, err := newDeferredConsumer(s, deferredDispatchFunc(func(context.Context, string, []byte) (eventruntime.DispatchResult, error) {
		return eventruntime.DispatchResult{}, nil
	}), bad)
	require.Error(t, err)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	require.Error(t, r.Start(cancelled))
}
