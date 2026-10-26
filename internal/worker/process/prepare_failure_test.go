package process

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FangcunMount/component-base/pkg/processruntime"
	eventtransport "github.com/FangcunMount/qs-server/internal/pkg/eventing/transport"
	genericoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
	workerconfig "github.com/FangcunMount/qs-server/internal/worker/config"
	"github.com/FangcunMount/qs-server/internal/worker/container"
	workeroptions "github.com/FangcunMount/qs-server/internal/worker/options"
)

type failingWorkerStage struct{ fail func(*prepareState) error }

func (s failingWorkerStage) Name() string                  { return "owned failed fixture stage" }
func (s failingWorkerStage) Run(state *prepareState) error { return s.fail(state) }

type failingPreparationStore struct {
	calls int
	err   error
}

func (s *failingPreparationStore) Close() error { s.calls++; return s.err }

func TestFailedWorkerPreparationPreservesStartupAndDrainErrors(t *testing.T) {
	startup, drain := errors.New("owned fixture startup failed"), errors.New("owned store still active")
	store := &failingPreparationStore{err: drain}
	r := &prepareRunner{stages: []processruntime.Stage[prepareState]{
		failingWorkerStage{fail: func(s *prepareState) error { s.runtime.messaging.holdStore = store; return startup }},
		failingWorkerStage{fail: func(*prepareState) error { t.Error("later startup stage ran after failure"); return nil }},
	}}
	_, stage, err := r.run()
	if !errors.Is(err, startup) || !errors.Is(err, drain) || store.calls != 1 || stage != "owned failed fixture stage" {
		t.Fatal("partial ownership or errors lost", stage, err, store.calls)
	}
}

func TestActualWorkerRuntimeFailureRetainsAndClosesPreSubscriberPool(t *testing.T) {
	opts := workeroptions.NewOptions()
	opts.Metrics.Enable = false
	off := false
	opts.Messaging.Provider = "rocketmq"
	opts.Messaging.RocketMQ = &genericoptions.RocketMQOptions{PublishEnabled: &off}
	// sql.Open is inert. Missing frozen RocketMQ files fail before Subscribe or any database
	// operation. The later verification uses a cancelled context, so even a
	// regression that leaves the pool open cannot dial an unowned endpoint.
	opts.MySQL = &genericoptions.MySQLOptions{Host: "127.0.0.1:1", Database: "owned_constructor_only"}
	s := &server{config: &workerconfig.Config{Options: opts}}
	r := &prepareRunner{server: s, state: prepareState{container: containerOutput{container: &container.Container{}}}, stages: []processruntime.Stage[prepareState]{runtimeStage{server: s}}}
	_, stage, err := r.run()
	if err == nil || stage != "initialize runtime" {
		t.Fatal("invalid frozen binding unexpectedly started", stage, err)
	}
	if r.state.runtime.messaging.deadLetterRecorder == nil || r.state.runtime.messaging.subscriber != nil {
		t.Fatal("partial pre-subscriber ownership lost")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = r.state.runtime.messaging.deadLetterRecorder.RecordDeadLetter(ctx, eventtransport.DeadLetterRecord{MessageID: "fixture", Provider: "rocketmq", Topic: "fixture", Channel: "fixture", DeliveryAttempts: 1})
	if err == nil || !strings.Contains(err.Error(), "database is closed") {
		t.Fatal("pre-subscriber pool was not closed by failed preparation", err)
	}
}

func TestActualWorkerResourceFailureRetainsManagerBeforeCatalogFailure(t *testing.T) {
	opts := workeroptions.NewOptions()
	off := false
	opts.Messaging.Provider = "rocketmq"
	opts.Messaging.RocketMQ = &genericoptions.RocketMQOptions{PublishEnabled: &off}
	opts.MySQL, opts.Redis, opts.MongoDB = nil, nil, nil
	opts.RedisProfiles = nil
	opts.Worker.EventConfigPath = filepath.Join(t.TempDir(), "absent-events.yaml")
	s := &server{config: &workerconfig.Config{Options: opts}}
	r := &prepareRunner{server: s, stages: []processruntime.Stage[prepareState]{resourceStage{server: s}}}
	_, stage, err := r.run()
	if err == nil || stage != "prepare resources" || r.state.resources.handles.dbManager == nil {
		t.Fatal("actual partial resource stage lost its manager", stage, err)
	}
}
