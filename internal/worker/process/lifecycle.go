package process

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/component-base/pkg/shutdown"
	"github.com/FangcunMount/qs-server/internal/pkg/processruntime"
)

type lifecycleDeps struct {
	stopSubscriber   func() error
	closeGRPCManager func() error
	closeDatabase    func() error
	shutdownMetrics  func() error
	cleanupContainer func() error
}

func (s *server) registerShutdownCallback(deps lifecycleDeps) {
	s.gs.AddShutdownCallback(shutdown.ShutdownFunc(func(string) error {
		runWorkerLifecycle(deps)
		log.Info("🏗️  Worker Server shutdown complete")
		return nil
	}))
}

func buildLifecycleDeps(resources resourceOutput, containerOutput containerOutput, integrationOutput integrationOutput, runtimeOutput runtimeOutput) lifecycleDeps {
	var deps lifecycleDeps
	if runtimeOutput.messaging.subscriber != nil {
		deps.stopSubscriber = func() error {
			runtimeOutput.messaging.subscriber.Stop()
			return runtimeOutput.messaging.subscriber.Close()
		}
	}
	if integrationOutput.grpcClients.grpcManager != nil {
		deps.closeGRPCManager = integrationOutput.grpcClients.grpcManager.Close
	}
	if resources.handles.dbManager != nil {
		deps.closeDatabase = resources.handles.dbManager.Close
	}
	if runtimeOutput.observability.metricsServer != nil {
		deps.shutdownMetrics = func() error {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			return runtimeOutput.observability.metricsServer.Shutdown(shutdownCtx)
		}
	}
	if containerOutput.container != nil {
		deps.cleanupContainer = func() error {
			containerOutput.container.Cleanup()
			return nil
		}
	}
	return deps
}

func runWorkerLifecycle(deps lifecycleDeps) {
	lifecycle := processruntime.Lifecycle{}
	lifecycle.AddShutdownHook("stop subscriber", deps.stopSubscriber)
	lifecycle.AddShutdownHook("close grpc manager", deps.closeGRPCManager)
	lifecycle.AddShutdownHook("close database", deps.closeDatabase)
	lifecycle.AddShutdownHook("shutdown metrics", deps.shutdownMetrics)
	lifecycle.AddShutdownHook("cleanup container", deps.cleanupContainer)
	lifecycle.Run(func(name string, err error) {
		log.Warnf("%s failed: %v", name, err)
	})
}

func (s preparedServer) Run() error {
	if s.startShutdown != nil {
		if err := s.startShutdown(); err != nil {
			log.Fatalf("start shutdown manager failed: %s", err.Error())
		}
	}
	log.Info("🚦 Shutdown manager started, worker coming online")
	log.Info("🚀 Worker started, waiting for events...")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Info("Shutdown signal received, stopping workers...")
	return nil
}
