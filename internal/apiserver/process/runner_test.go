package process

import (
	"errors"
	"reflect"
	"testing"

	"github.com/FangcunMount/component-base/pkg/processruntime"
	"github.com/FangcunMount/component-base/pkg/shutdown"
	grpcpkg "github.com/FangcunMount/qs-server/internal/pkg/grpc"
	genericapiserver "github.com/FangcunMount/qs-server/internal/pkg/server"
)

type fakePrepareRunStage struct {
	stageName string
	stageRun  func(*prepareState) error
}

func (s fakePrepareRunStage) Name() string { return s.stageName }

func (s fakePrepareRunStage) Run(state *prepareState) error {
	return s.stageRun(state)
}

func TestPrepareRunRunnerExecutesStagesInOrder(t *testing.T) {
	t.Parallel()

	var order []string
	runner := &prepareRunner{
		server: &server{gs: shutdown.New()},
		stages: []processruntime.Stage[prepareState]{
			fakePrepareRunStage{
				stageName: "prepare resources",
				stageRun: func(_ *prepareState) error {
					order = append(order, "prepare resources")
					return nil
				},
			},
			fakePrepareRunStage{
				stageName: "initialize container",
				stageRun: func(_ *prepareState) error {
					order = append(order, "initialize container")
					return nil
				},
			},
			fakePrepareRunStage{
				stageName: "initialize transports",
				stageRun: func(state *prepareState) error {
					order = append(order, "initialize transports")
					state.transport.httpServer = &genericapiserver.GenericAPIServer{}
					state.transport.grpcServer = &grpcpkg.Server{}
					return nil
				},
			},
		},
	}

	prepared, failedStage, err := runner.run()
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if failedStage != "" {
		t.Fatalf("failedStage = %q, want empty", failedStage)
	}
	if prepared.startShutdown == nil {
		t.Fatal("prepared.startShutdown = nil, want value")
	}
	if prepared.httpServer != runner.state.transport.httpServer {
		t.Fatalf("prepared httpServer = %#v, want %#v", prepared.httpServer, runner.state.transport.httpServer)
	}
	if prepared.grpcServer != runner.state.transport.grpcServer {
		t.Fatalf("prepared grpcServer = %#v, want %#v", prepared.grpcServer, runner.state.transport.grpcServer)
	}

	want := []string{"prepare resources", "initialize container", "initialize transports"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("stage order = %#v, want %#v", order, want)
	}
}

func TestPrepareRunRunnerStopsOnFirstError(t *testing.T) {
	t.Parallel()

	var order []string
	runner := &prepareRunner{
		server: &server{gs: shutdown.New()},
		stages: []processruntime.Stage[prepareState]{
			fakePrepareRunStage{
				stageName: "prepare resources",
				stageRun: func(_ *prepareState) error {
					order = append(order, "prepare resources")
					return nil
				},
			},
			fakePrepareRunStage{
				stageName: "initialize container",
				stageRun: func(_ *prepareState) error {
					order = append(order, "initialize container")
					return errors.New("boom")
				},
			},
			fakePrepareRunStage{
				stageName: "initialize transports",
				stageRun: func(_ *prepareState) error {
					order = append(order, "initialize transports")
					return nil
				},
			},
		},
	}

	_, failedStage, err := runner.run()
	if err == nil {
		t.Fatal("run() error = nil, want failure")
	}
	if failedStage != "initialize container" {
		t.Fatalf("failedStage = %q, want initialize container", failedStage)
	}

	want := []string{"prepare resources", "initialize container"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("stage order = %#v, want %#v", order, want)
	}
}
