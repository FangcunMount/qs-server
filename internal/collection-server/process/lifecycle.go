package process

import (
	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/component-base/pkg/shutdown"
	"github.com/FangcunMount/qs-server/internal/pkg/processruntime"
)

type lifecycleDeps struct {
	closeGRPCManager func() error
	closeDatabase    func() error
	stopAuthzSync    func() error
	closeIAM         func() error
	cleanupContainer func() error
	closeHTTP        func()
}

func (s *server) registerShutdownCallback(deps lifecycleDeps) {
	s.gs.AddShutdownCallback(shutdown.ShutdownFunc(func(string) error {
		runCollectionLifecycle(deps)
		log.Info("🏗️  Collection Server shutdown complete")
		return nil
	}))
}

func buildLifecycleDeps(resources resourceOutput, containerOutput containerOutput, integrationOutput integrationOutput, transportOutput transportOutput) lifecycleDeps {
	var deps lifecycleDeps
	if integrationOutput.grpcClients.grpcManager != nil {
		deps.closeGRPCManager = integrationOutput.grpcClients.grpcManager.Close
	}
	if resources.handles.dbManager != nil {
		deps.closeDatabase = resources.handles.dbManager.Close
	}
	if integrationOutput.iamSync.authzVersionSubscriber != nil {
		deps.stopAuthzSync = func() error {
			integrationOutput.iamSync.authzVersionSubscriber.Stop()
			return integrationOutput.iamSync.authzVersionSubscriber.Close()
		}
	}
	if containerOutput.container != nil && containerOutput.container.IAMModule != nil {
		deps.closeIAM = containerOutput.container.IAMModule.Close
		deps.cleanupContainer = func() error {
			containerOutput.container.Cleanup()
			return nil
		}
	}
	if transportOutput.httpServer != nil {
		deps.closeHTTP = transportOutput.httpServer.Close
	}
	return deps
}

func runCollectionLifecycle(deps lifecycleDeps) {
	lifecycle := processruntime.Lifecycle{}
	lifecycle.AddShutdownHook("close grpc clients", deps.closeGRPCManager)
	lifecycle.AddShutdownHook("close database", deps.closeDatabase)
	lifecycle.AddShutdownHook("stop authz sync", deps.stopAuthzSync)
	lifecycle.AddShutdownHook("close iam", deps.closeIAM)
	lifecycle.AddShutdownHook("cleanup container", deps.cleanupContainer)
	lifecycle.Run(func(name string, err error) {
		log.Errorf("Failed to %s: %v", name, err)
	})
	if deps.closeHTTP != nil {
		deps.closeHTTP()
	}
}

func (s preparedServer) Run() error {
	if s.startShutdown != nil {
		if err := s.startShutdown(); err != nil {
			log.Fatalf("start shutdown manager failed: %s", err.Error())
		}
	}
	log.Info("🚦 Shutdown manager started, servers coming online")
	log.Info("🚀 Starting Collection Server HTTP REST API server...")
	return processruntime.RunGroup{
		Services: []processruntime.ServiceRunner{
			{Name: "http", Run: s.httpServer.Run},
		},
	}.Run()
}
