package process

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/component-base/pkg/shutdown"
)

type lifecycleDeps struct {
	stopSubscriber   func() error
	closeGRPCManager func() error
	closeDatabase    func() error
	shutdownMetrics  func() error
	cleanupContainer func() error
}

func (s *server) registerShutdownCallback(deps lifecycleDeps) {
	s.shutdown = newWorkerShutdown(deps)
	s.gs.SetErrorHandler(shutdown.ErrorFunc(func(error) {
		log.Error("worker shutdown incomplete or failed")
		log.Flush()
		os.Exit(1)
	}))
	s.gs.AddShutdownCallback(shutdown.ShutdownFunc(func(string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := s.shutdown.Close(ctx); err != nil {
			return err
		}
		log.Info("🏗️  Worker Server shutdown complete")
		return nil
	}))
}

func buildLifecycleDeps(resources resourceOutput, containerOutput containerOutput, integrationOutput integrationOutput, runtimeOutput runtimeOutput) lifecycleDeps {
	var deps lifecycleDeps
	if runtimeOutput.messaging.subscriber != nil || runtimeOutput.messaging.holdReplayer != nil || runtimeOutput.messaging.publisher != nil || runtimeOutput.messaging.deadLetterRecorder != nil || runtimeOutput.messaging.holdStore != nil {
		deps.stopSubscriber = func() error {
			if runtimeOutput.messaging.subscriber != nil {
				runtimeOutput.messaging.subscriber.Stop()
			}
			if runtimeOutput.messaging.holdReplayer != nil {
				runtimeOutput.messaging.holdReplayer.Stop()
			}
			if runtimeOutput.messaging.subscriber != nil {
				if err := runtimeOutput.messaging.subscriber.Close(); err != nil {
					return err
				}
			}
			if runtimeOutput.messaging.publisher != nil {
				if err := runtimeOutput.messaging.publisher.Close(); err != nil {
					return err
				}
			}
			if runtimeOutput.messaging.deadLetterRecorder != nil {
				if err := runtimeOutput.messaging.deadLetterRecorder.Close(); err != nil {
					return err
				}
			}
			if runtimeOutput.messaging.holdStore != nil {
				if err := runtimeOutput.messaging.holdStore.Close(); err != nil {
					return err
				}
			}
			return nil
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

func runWorkerLifecycle(deps lifecycleDeps) error {
	log.Info("Draining worker subscribers before closing gRPC and database dependencies...")
	for _, step := range []struct {
		name string
		run  func() error
	}{
		{"stop subscriber", deps.stopSubscriber},
		{"cleanup container", deps.cleanupContainer},
		{"close grpc manager", deps.closeGRPCManager},
		{"close database", deps.closeDatabase},
		{"shutdown metrics", deps.shutdownMetrics},
	} {
		if step.run != nil {
			if err := step.run(); err != nil {
				return fmt.Errorf("worker shutdown %s: %w", step.name, err)
			}
		}
	}
	return nil
}

func (s preparedServer) Run() error {
	if s.shutdown == nil || s.startShutdown == nil {
		return errors.New("worker requires its owned shutdown coordinator")
	}

	if s.startShutdown != nil {
		if err := s.startShutdown(); err != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			return errors.Join(err, s.shutdown.Close(ctx))
		}
	}

	log.Info("🚦 Shutdown manager started, worker coming online")
	log.Info("🚀 Worker started, waiting for events...")

	// The existing POSIX manager owns signal handling. Main joins the same drain
	// that its callback starts; it cannot return when a handler is still running.
	<-s.shutdown.requested
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return s.shutdown.Wait(ctx)
}
