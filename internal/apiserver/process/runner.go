package process

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/container"
	"github.com/FangcunMount/qs-server/internal/pkg/processruntime"
)

type prepareState struct {
	resources   resourceOutput
	container   containerOutput
	integration integrationOutput
	transport   transportOutput
	runtime     runtimeOutput
}

type prepareRunner struct {
	server *server
	state  prepareState
	stages []processruntime.Stage[prepareState]
}

// PrepareRun 准备运行 API 服务器（六边形架构版本）
func (s *server) PrepareRun() preparedServer {
	prepared, failedStage, err := newPrepareRunner(s).run()
	if err != nil {
		s.fatalPrepareRun(failedStage, err)
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
			transportStage{server: server},
			runtimeStage{server: server},
			shutdownStage{server: server},
		},
	}
}

type resourceStage struct {
	server *server
}

func (resourceStage) Name() string { return "prepare resources" }

func (s resourceStage) Run(state *prepareState) error {
	resources, err := prepareResources(s.server.buildResourceStageDeps())
	if err != nil {
		return err
	}
	state.resources = resources
	return nil
}

type containerStage struct {
	server *server
}

func (containerStage) Name() string { return "initialize container" }

func (s containerStage) Run(state *prepareState) error {
	output, err := s.server.initializeContainer(state.resources)
	if err != nil {
		return err
	}
	state.container = output
	return nil
}

type integrationStage struct {
	server *server
}

func (integrationStage) Name() string { return "initialize integrations" }

func (s integrationStage) Run(state *prepareState) error {
	output, err := s.server.initializeIntegrations(state.container)
	if err != nil {
		return err
	}
	state.integration = output
	return nil
}

type transportStage struct {
	server *server
}

func (transportStage) Name() string { return "initialize transports" }

func (s transportStage) Run(state *prepareState) error {
	output, err := s.server.initializeTransports(state.container)
	if err != nil {
		return err
	}
	state.transport = output
	return nil
}

type runtimeStage struct {
	server *server
}

func (runtimeStage) Name() string { return "start background runtimes" }

func (s runtimeStage) Run(state *prepareState) error {
	runRuntimeStage(s.server.buildRuntimeStageDeps(state.resources, state.container), &state.runtime)
	return nil
}

type shutdownStage struct {
	server *server
}

func (shutdownStage) Name() string { return "register shutdown callback" }

func (s shutdownStage) Run(state *prepareState) error {
	s.server.registerShutdownCallback(buildLifecycleDeps(state.resources, state.container, state.integration, state.transport, state.runtime))
	return nil
}

func (r *prepareRunner) run() (preparedServer, string, error) {
	return processruntime.Runner[prepareState, preparedServer]{
		State:  &r.state,
		Stages: r.stages,
		BuildPrepared: func(state *prepareState) preparedServer {
			return preparedServer{
				startShutdown: r.server.gs.Start,
				httpServer:    state.transport.httpServer,
				grpcServer:    state.transport.grpcServer,
			}
		},
	}.Run()
}

func newContainerFromResourceStage(resources resourceOutput) *container.Container {
	return container.NewContainerWithOptions(
		resources.handles.mysqlDB,
		resources.handles.mongoDB,
		resources.handles.redisCache,
		resources.containerInput.containerOptions,
	)
}
