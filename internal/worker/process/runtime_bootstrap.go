package process

import (
	"context"
	"log/slog"

	messagingintegration "github.com/FangcunMount/qs-server/internal/worker/integration/messaging"
	observability "github.com/FangcunMount/qs-server/internal/worker/observability"
)

func (s *server) initializeRuntime(resources resourceOutput, containerOutput containerOutput) (runtimeOutput, error) {
	output := runtimeOutput{}
	if containerOutput.container == nil {
		return output, nil
	}

	if s.config != nil && s.config.Metrics != nil && s.config.Metrics.Enable {
		metrics := observability.NewMetricsServerWithGovernance(
			s.config.Metrics.BindAddress,
			s.config.Metrics.BindPort,
			"worker",
			resources.redisRuntime.familyStatus,
		)
		if err := metrics.Start(); err != nil {
			return runtimeOutput{}, err
		}
		output.observability.metricsServer = metrics
	}

	if s.config != nil && s.config.Messaging.Provider == "nsq" {
		if err := messagingintegration.EnsureTopics(s.config.Messaging, s.logger, containerOutput.container); err != nil {
			s.logger.Warn("topic creation failed (non-fatal)", slog.String("error", err.Error()))
		}
	}

	subscriber, err := messagingintegration.CreateSubscriber(s.config.Messaging, s.logger, s.workerMaxInFlight())
	if err != nil {
		if output.observability.metricsServer != nil {
			_ = output.observability.metricsServer.Shutdown(context.Background())
		}
		return runtimeOutput{}, err
	}
	output.messaging.subscriber = subscriber

	if err := messagingintegration.SubscribeHandlers(s.config.Worker.ServiceName, s.logger, containerOutput.container, subscriber); err != nil {
		subscriber.Stop()
		_ = subscriber.Close()
		if output.observability.metricsServer != nil {
			_ = output.observability.metricsServer.Shutdown(context.Background())
		}
		return runtimeOutput{}, err
	}

	return output, nil
}

func (s *server) workerMaxInFlight() int {
	if s.config != nil && s.config.Worker != nil && s.config.Worker.Concurrency > 0 {
		return s.config.Worker.Concurrency
	}
	return 1
}
