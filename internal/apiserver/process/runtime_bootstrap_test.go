package process

import (
	"context"
	"testing"
	"time"

	"github.com/FangcunMount/component-base/pkg/messaging"
	apiserverconfig "github.com/FangcunMount/qs-server/internal/apiserver/config"
	"github.com/FangcunMount/qs-server/internal/apiserver/container"
	surveymod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/survey"
	evalmod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/evaluation"
	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
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

func TestBuildRuntimeStageDepsDisablesDurableRelaysWithoutMQPublisher(t *testing.T) {
	t.Parallel()

	s := &server{}
	c := &container.Container{
		SurveyModule: &surveymod.Module{
			AnswerSheet: &surveymod.AnswerSheetSubModule{SubmittedEventRelay: fakeRuntimeRelay{}},
		},
		EvaluationModule: &evalmod.Module{AssessmentOutboxRelay: fakeRuntimeRelay{}},
	}

	deps := s.buildRuntimeStageDeps(resourceOutput{}, containerOutput{container: c})
	if len(deps.relays) != 0 {
		t.Fatalf("relay count = %d, want 0 without MQ publisher", len(deps.relays))
	}

	deps = s.buildRuntimeStageDeps(
		resourceOutput{messaging: messagingOutput{mqPublisher: fakeRuntimePublisher{}}},
		containerOutput{container: c},
	)
	if len(deps.relays) != 2 {
		t.Fatalf("relay count = %d, want 2 with MQ publisher", len(deps.relays))
	}
}

func TestBuildRuntimeStageDepsUsesConfiguredOutboxRelayIntervals(t *testing.T) {
	t.Parallel()

	opts := apiserveroptions.NewOptions()
	opts.OutboxRelay.Mongo.Interval = 250 * time.Millisecond
	opts.OutboxRelay.Assessment.Interval = 750 * time.Millisecond
	cfg, err := apiserverconfig.CreateConfigFromOptions(opts)
	if err != nil {
		t.Fatalf("CreateConfigFromOptions() error = %v", err)
	}
	s := &server{config: cfg}
	c := &container.Container{
		SurveyModule: &surveymod.Module{
			AnswerSheet: &surveymod.AnswerSheetSubModule{SubmittedEventRelay: fakeRuntimeRelay{}},
		},
		EvaluationModule: &evalmod.Module{AssessmentOutboxRelay: fakeRuntimeRelay{}},
	}

	deps := s.buildRuntimeStageDeps(
		resourceOutput{messaging: messagingOutput{mqPublisher: fakeRuntimePublisher{}}},
		containerOutput{container: c},
	)
	if len(deps.relays) != 2 {
		t.Fatalf("relay count = %d, want 2", len(deps.relays))
	}
	if deps.relays[0].interval != 250*time.Millisecond {
		t.Fatalf("mongo relay interval = %s, want 250ms", deps.relays[0].interval)
	}
	if deps.relays[1].interval != 750*time.Millisecond {
		t.Fatalf("assessment relay interval = %s, want 750ms", deps.relays[1].interval)
	}
}

type fakeRuntimeRelay struct{}

func (fakeRuntimeRelay) DispatchDue(context.Context) error { return nil }

type fakeRuntimePublisher struct{}

func (fakeRuntimePublisher) Publish(context.Context, string, []byte) error { return nil }
func (fakeRuntimePublisher) PublishMessage(context.Context, string, *messaging.Message) error {
	return nil
}
func (fakeRuntimePublisher) Close() error { return nil }
