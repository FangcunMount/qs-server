package process

import (
	"context"
	"testing"
	"time"
)

func TestRunRuntimeStageInvokesWarmupSchedulersAndRelays(t *testing.T) {
	t.Parallel()

	var warmupCalled bool
	var schedulersCalled bool
	dispatchCalled := make(chan struct{}, 1)
	output := &runtimeOutput{}

	runRuntimeStage(runtimeStageDeps{
		warmup: func() {
			warmupCalled = true
		},
		startSchedulers: func(output *runtimeOutput) {
			schedulersCalled = true
			output.lifecycle.AddShutdownHook("stop schedulers", func() error { return nil })
		},
		relays: []relayRuntimeDeps{
			{
				stopHookName: "stop relay",
				startLogName: "relay",
				failureLog:   "relay",
				interval:     time.Millisecond,
				dispatch: func(ctx context.Context) error {
					select {
					case dispatchCalled <- struct{}{}:
					default:
					}
					<-ctx.Done()
					return nil
				},
			},
		},
	}, output)

	select {
	case <-dispatchCalled:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("relay dispatch was not called")
	}

	if !warmupCalled {
		t.Fatal("warmup was not called")
	}
	if !schedulersCalled {
		t.Fatal("startSchedulers was not called")
	}
	if got := output.lifecycle.Len(); got != 2 {
		t.Fatalf("shutdown hook count = %d, want 2", got)
	}

	runPrepareRunShutdownHooks(output.lifecycle)
}

func TestStartRelayLoopNoopWithoutDispatch(t *testing.T) {
	t.Parallel()

	output := &runtimeOutput{}
	startRelayLoop(relayRuntimeDeps{}, output)

	if got := output.lifecycle.Len(); got != 0 {
		t.Fatalf("shutdown hook count = %d, want 0", got)
	}
}
