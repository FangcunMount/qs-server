package process

import (
	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/component-base/pkg/processruntime"
)

type prepareRunner struct {
	server *server
	state  prepareState
	stages []processruntime.Stage[prepareState]
}

func (s *server) PrepareRun() preparedServer {
	prepared, failedStage, err := newPrepareRunner(s).run()
	if err != nil {
		log.Fatalf("Failed to %s: %v", failedStage, err)
	}
	return prepared
}

func newPrepareRunner(server *server) *prepareRunner {
	return &prepareRunner{
		server: server,
		stages: []processruntime.Stage[prepareState]{
			resourceStage{server: server},
			containerStage{server: server},
			integrationStage{server: server},
			runtimeStage{server: server},
			shutdownStage{server: server},
		},
	}
}

func (r *prepareRunner) run() (preparedServer, string, error) {
	return processruntime.Runner[prepareState, preparedServer]{
		State:  &r.state,
		Stages: r.stages,
		BuildPrepared: func(*prepareState) preparedServer {
			return preparedServer{
				startShutdown: r.server.gs.Start,
			}
		},
	}.Run()
}

type resourceStage struct{ server *server }

func (resourceStage) Name() string { return "prepare resources" }

func (s resourceStage) Run(state *prepareState) error {
	output, err := s.server.prepareResources()
	if err != nil {
		return err
	}
	state.resources = output
	return nil
}

type containerStage struct{ server *server }

func (containerStage) Name() string { return "initialize container" }

func (s containerStage) Run(state *prepareState) error {
	output, err := s.server.initializeContainer(state.resources)
	if err != nil {
		return err
	}
	state.container = output
	return nil
}

type integrationStage struct{ server *server }

func (integrationStage) Name() string { return "initialize integrations" }

func (s integrationStage) Run(state *prepareState) error {
	output, err := s.server.initializeIntegrations(state.container)
	if err != nil {
		return err
	}
	state.integration = output
	return nil
}

type runtimeStage struct{ server *server }

func (runtimeStage) Name() string { return "initialize runtime" }

func (s runtimeStage) Run(state *prepareState) error {
	output, err := s.server.initializeRuntime(state.resources, state.container)
	if err != nil {
		return err
	}
	state.runtime = output
	return nil
}

type shutdownStage struct{ server *server }

func (shutdownStage) Name() string { return "register shutdown callback" }

func (s shutdownStage) Run(state *prepareState) error {
	s.server.registerShutdownCallback(buildLifecycleDeps(state.resources, state.container, state.integration, state.runtime))
	return nil
}
