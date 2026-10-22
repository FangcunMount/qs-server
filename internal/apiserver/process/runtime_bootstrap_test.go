package process

import (
	"errors"
	"reflect"
	"testing"
)

func TestRuntimeStartsDueDispatchAfterOwnersAndStopsOnPreflightFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		var calls []string
		wantErr := errors.New("retry schedule binding unavailable")
		err := runRuntimeStage(runtimeStageDeps{
			startEvents: func() error { calls = append(calls, "owners"); return nil },
			startRetryDispatch: func(*runtimeOutput) error {
				calls = append(calls, "due")
				if fail {
					return wantErr
				}
				return nil
			},
			startCache:           func() { calls = append(calls, "cache") },
			startAIWorkflowRelay: func() error { calls = append(calls, "ai"); return nil },
			startSchedulers:      func(*runtimeOutput) { calls = append(calls, "other_schedulers") },
		}, &runtimeOutput{})
		want := []string{"owners", "due", "cache", "ai", "other_schedulers"}
		if fail {
			want = want[:2]
			if !errors.Is(err, wantErr) {
				t.Fatal("preflight failure lost", err)
			}
		} else if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(calls, want) {
			t.Fatalf("runtime order %v, want %v", calls, want)
		}
	}
}

func TestLegacyRuntimeDoesNotBuildTransactionDispatch(t *testing.T) {
	var s *server
	if s.buildRetryDispatchStart(containerOutput{}) != nil {
		t.Fatal("legacy runtime acquired transaction work")
	}
}

func TestRunRuntimeStageInvokesCacheEventsAndSchedulers(t *testing.T) {
	t.Parallel()

	var cacheCalled bool
	var eventsCalled bool
	var relayCalled bool
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
		startAIWorkflowRelay: func() error { relayCalled = true; return nil },
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
	if !eventsCalled || !relayCalled {
		t.Fatal("event subsystem was not started")
	}
	if !schedulersCalled {
		t.Fatal("startSchedulers was not called")
	}
	if got := output.lifecycle.Len(); got != 1 {
		t.Fatalf("shutdown hook count = %d, want 1", got)
	}

	if err := runPrepareRunShutdownHooks(output.lifecycle); err != nil {
		t.Fatal(err)
	}
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
