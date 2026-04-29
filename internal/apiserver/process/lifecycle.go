package process

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/component-base/pkg/processruntime"
	"github.com/FangcunMount/component-base/pkg/shutdown"
)

type resourceLifecycleDeps struct {
	closeDatabase func() error
}

type containerLifecycleDeps struct {
	containerCleanup func() error
	stopAuthzSync    func() error
}

type transportLifecycleDeps struct {
	closeHTTP func()
	closeGRPC func()
}

type runtimeLifecycleDeps struct {
	lifecycle processruntime.Lifecycle
}

type processLifecycleDeps struct {
	resource  resourceLifecycleDeps
	container containerLifecycleDeps
	transport transportLifecycleDeps
	runtime   runtimeLifecycleDeps
}

type preparedServerTransports struct {
	runHTTP func() error
	runGRPC func() error
}

type preparedServerRunDeps struct {
	startShutdown func() error
	transports    preparedServerTransports
}

func (s *server) registerShutdownCallback(deps processLifecycleDeps) {
	s.gs.AddShutdownCallback(shutdown.ShutdownFunc(func(string) error {
		runPrepareRunShutdownHooks(deps.runtime.lifecycle)
		runProcessLifecycleDeps(deps)
		log.Info("🏗️  Hexagonal Architecture server shutdown complete")
		return nil
	}))
}

func buildLifecycleDeps(resources resourceOutput, containerOutput containerOutput, integrationOutput integrationOutput, transportOutput transportOutput, runtimeOutput runtimeOutput) processLifecycleDeps {
	var deps processLifecycleDeps

	if resources.handles.dbManager != nil {
		deps.resource.closeDatabase = resources.handles.dbManager.Close
	}
	if containerOutput.container != nil {
		deps.container.containerCleanup = containerOutput.container.Cleanup
	}
	if integrationOutput.authzVersionSubscriber != nil {
		deps.container.stopAuthzSync = func() error {
			integrationOutput.authzVersionSubscriber.Stop()
			return integrationOutput.authzVersionSubscriber.Close()
		}
	}
	if transportOutput.httpServer != nil {
		deps.transport.closeHTTP = transportOutput.httpServer.Close
	}
	if transportOutput.grpcServer != nil {
		deps.transport.closeGRPC = transportOutput.grpcServer.Close
	}
	deps.runtime.lifecycle = runtimeOutput.lifecycle

	return deps
}

func runPrepareRunShutdownHooks(lifecycle processruntime.Lifecycle) {
	lifecycle.Run(func(name string, err error) {
		log.Errorf("Failed to run shutdown hook %q: %v", name, err)
	})
}

func runProcessLifecycleDeps(deps processLifecycleDeps) {
	if deps.container.containerCleanup != nil {
		if err := deps.container.containerCleanup(); err != nil {
			log.Errorf("Failed to cleanup container resources: %v", err)
		}
	}
	if deps.container.stopAuthzSync != nil {
		if err := deps.container.stopAuthzSync(); err != nil {
			log.Errorf("Failed to close IAM authz version subscriber: %v", err)
		}
	}
	if deps.resource.closeDatabase != nil {
		if err := deps.resource.closeDatabase(); err != nil {
			log.Errorf("Failed to close database connections: %v", err)
		}
	}
	if deps.transport.closeHTTP != nil {
		deps.transport.closeHTTP()
	}
	if deps.transport.closeGRPC != nil {
		deps.transport.closeGRPC()
	}
}

func (s *server) fatalPrepareRun(action string, err error) {
	logger.L(context.Background()).Errorw("Failed to prepare api server",
		"component", "apiserver",
		"action", action,
		"error", err.Error(),
	)
	log.Fatalf("Failed to %s: %v", action, err)
}

func (s preparedServer) buildPreparedServerRunDeps() preparedServerRunDeps {
	var deps preparedServerRunDeps
	if s.startShutdown != nil {
		deps.startShutdown = s.startShutdown
	}
	if s.httpServer != nil {
		deps.transports.runHTTP = s.httpServer.Run
	}
	if s.grpcServer != nil {
		deps.transports.runGRPC = s.grpcServer.Run
	}
	return deps
}

func runPreparedServer(deps preparedServerRunDeps) error {
	if deps.startShutdown != nil {
		if err := deps.startShutdown(); err != nil {
			log.Fatalf("start shutdown manager failed: %s", err.Error())
		}
	}
	if deps.transports.runHTTP != nil {
		log.Info("🚀 Starting Hexagonal Architecture HTTP REST API server...")
	}
	if deps.transports.runGRPC != nil {
		log.Info("🚀 Starting Hexagonal Architecture GRPC server...")
	}
	if err := (processruntime.RunGroup{
		Services: []processruntime.ServiceRunner{
			{
				Name: "http",
				Run:  deps.transports.runHTTP,
			},
			{
				Name: "grpc",
				Run:  deps.transports.runGRPC,
			},
		},
	}).Run(); err != nil {
		log.Errorf("Failed to run prepared server: %v", err)
		return err
	}
	return nil
}

// Run 运行 API 服务器
func (s preparedServer) Run() error {
	return runPreparedServer(s.buildPreparedServerRunDeps())
}
