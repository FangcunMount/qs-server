package container

import (
	"context"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	"github.com/FangcunMount/qs-server/internal/worker/options"
	"github.com/stretchr/testify/require"
)

func TestWorkerPhysicalAndDeferredExecutionsShareOriginalBudget(t *testing.T) {
	b := newDispatchBudget(2)
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	done := make(chan error, 2)
	var active atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for i := 0; i < 2; i++ {
		go func() {
			_, err := b.run(ctx, func() (eventruntime.DispatchResult, error) {
				active.Add(1)
				entered <- struct{}{}
				<-release
				active.Add(-1)
				return eventruntime.DispatchResult{Outcome: eventruntime.DispatchHandled}, nil
			})
			done <- err
		}()
	}
	t.Cleanup(func() {
		close(release)
		for i := 0; i < 2; i++ {
			select {
			case err := <-done:
				require.NoError(t, err)
			case <-time.After(time.Second):
				t.Error("execution failed to drain")
			}
		}
	})
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("original work not running")
		}
	}
	cancel()
	require.EqualValues(t, 2, active.Load())
	require.Len(t, b.slots, 2, "cancellation released still-running work")
	wait, cancelWait := context.WithCancel(context.Background())
	queued := make(chan error, 1)
	go func() {
		_, err := b.run(wait, func() (eventruntime.DispatchResult, error) {
			t.Error("deferred input exceeded shared worker concurrency")
			return eventruntime.DispatchResult{}, nil
		})
		queued <- err
	}()
	cancelWait()
	select {
	case err := <-queued:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("queued cancellation did not return")
	}
	require.EqualValues(t, 2, active.Load())
}

func TestWorkerConstructsBudgetWithoutChangingConfiguredConcurrency(t *testing.T) {
	o := options.NewOptions()
	o.Worker.Concurrency = 48
	c, err := NewContainer(o, slog.New(slog.NewTextHandler(io.Discard, nil)), nil, nil, nil)
	require.NoError(t, err)
	require.Equal(t, 48, cap(c.dispatchBudget.slots))
	require.Equal(t, 48, o.Worker.Concurrency)
	var absent *dispatchBudget
	_, err = absent.run(context.Background(), func() (eventruntime.DispatchResult, error) {
		t.Error("missing budget executed work")
		return eventruntime.DispatchResult{}, nil
	})
	require.Error(t, err)
	c.Cleanup()
	require.Error(t, c.Initialize(), "closed worker reinitialized its background resources")
}
