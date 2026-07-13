package process

import (
	"errors"
	"testing"
)

func TestRunRuntimeStageInvokesCacheEventsAndSchedulers(t *testing.T) {
	t.Parallel()

	var cacheCalled bool
	var eventsCalled bool
	var schedulersCalled bool
	output := &runtimeOutput{}

	if err := runRuntimeStage(runtimeStageDeps{
		startCache: func() {
			cacheCalled = true
		},
		startEvents: func() error {
			eventsCalled = true
			return nil
		},
		startSchedulers: func(output *runtimeOutput) {
			schedulersCalled = true
			output.lifecycle.AddShutdownHook("stop schedulers", func() error { return nil })
		},
	}, output); err != nil {
		t.Fatal(err)
	}

	if !cacheCalled {
		t.Fatal("cache subsystem was not started")
	}
	if !eventsCalled {
		t.Fatal("event subsystem was not started")
	}
	if !schedulersCalled {
		t.Fatal("startSchedulers was not called")
	}
	if got := output.lifecycle.Len(); got != 1 {
		t.Fatalf("shutdown hook count = %d, want 1", got)
	}

	runPrepareRunShutdownHooks(output.lifecycle)
}

func TestRunRuntimeStageStopsWhenEventSubsystemFails(t *testing.T) {
	wantErr := errors.New("consumer binding missing")
	var schedulersCalled bool
	err := runRuntimeStage(runtimeStageDeps{
		startEvents:     func() error { return wantErr },
		startSchedulers: func(*runtimeOutput) { schedulersCalled = true },
	}, &runtimeOutput{})
	if !errors.Is(err, wantErr) {
		t.Fatalf("runRuntimeStage error = %v, want %v", err, wantErr)
	}
	if schedulersCalled {
		t.Fatal("schedulers started after event subsystem failure")
	}
}
