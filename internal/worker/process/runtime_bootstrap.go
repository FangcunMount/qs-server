package process

import (
	"context"
	"log/slog"

	genericoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	messagingintegration "github.com/FangcunMount/qs-server/internal/worker/integration/messaging"
	observability "github.com/FangcunMount/qs-server/internal/worker/observability"
)

func (s *server) initializeRuntime(resources resourceOutput, containerOutput containerOutput) (runtimeOutput, error) {
	output := runtimeOutput{}
	if containerOutput.container == nil {
		return output, nil
	}

	if s.config != nil && s.config.Metrics != nil && s.config.Metrics.Enable {
		metrics := observability.NewMetricsServerWithGovernanceAndResilience(
			s.config.Metrics.BindAddress,
			s.config.Metrics.BindPort,
			"worker",
			resources.redisRuntime.familyStatus,
			containerOutput.container.ResilienceSnapshot,
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

	subscriber, err := messagingintegration.CreateSubscriberWithOptions(s.config.Messaging, s.logger,
		messagingintegration.NewSubscriberOptions(s.workerMaxInFlight(), s.workerMaxDeliveryAttempts(), mustDeadLetterRecorder(s.config.MySQL)))
	if err != nil {
		if output.observability.metricsServer != nil {
			_ = output.observability.metricsServer.Shutdown(context.Background())
		}
		return runtimeOutput{}, err
	}
	output.messaging.subscriber = subscriber
	holdStore, err := messagingintegration.NewMySQLRetryEventHoldStore(s.config.MySQL, s.config.Messaging.Provider, s.holdReplayPolicy())
	if err != nil {
		subscriber.Stop()
		_ = subscriber.Close()
		return runtimeOutput{}, err
	}

	if err := messagingintegration.SubscribeHandlersWithOptions(messagingintegration.SubscribeHandlersOptions{
		ServiceName:  s.config.Worker.ServiceName,
		Logger:       s.logger,
		Runtime:      containerOutput.container,
		Subscriber:   subscriber,
		HoldRecorder: holdStore,
	}); err != nil {
		subscriber.Stop()
		_ = subscriber.Close()
		if output.observability.metricsServer != nil {
			_ = output.observability.metricsServer.Shutdown(context.Background())
		}
		return runtimeOutput{}, err
	}
	if s.config.RetryGovernance == nil || s.config.RetryGovernance.AutomaticRetryEnabled {
		publisher, publishErr := messagingintegration.CreatePublisher(s.config.Messaging)
		if publishErr != nil {
			subscriber.Stop()
			_ = subscriber.Close()
			return runtimeOutput{}, publishErr
		}
		output.messaging.publisher = publisher
		output.messaging.holdReplayer = messagingintegration.NewRetryEventHoldReplayer(holdStore, publisher)
		output.messaging.holdReplayer.Start()
	}

	return output, nil
}

func (s *server) holdReplayPolicy() retrygovernance.Policy {
	policy := retrygovernance.DefaultOutboxPolicy
	policy.Version = "retry-hold-publish/v1"
	if s.config == nil || s.config.RetryGovernance == nil || s.config.RetryGovernance.HoldReplay == nil {
		return policy
	}
	configured := s.config.RetryGovernance.HoldReplay
	policy.MaxAutomaticAttempts = min(configured.MaxAttempts, retrygovernance.HardMaxOutboxAttempts)
	policy.BaseDelay = configured.BaseDelay
	policy.MaxDelay = configured.MaxDelay
	policy.JitterFraction = configured.JitterFraction
	return policy
}

func mustDeadLetterRecorder(options *genericoptions.MySQLOptions) messagingintegration.DeadLetterRecorder {
	recorder, err := messagingintegration.NewMySQLDeadLetterRecorder(options)
	if err != nil {
		panic(err)
	}
	return recorder
}

func (s *server) workerMaxDeliveryAttempts() int {
	if s.config != nil && s.config.Messaging != nil && s.config.Messaging.Delivery != nil &&
		(s.config.DeliveryConfigured() || s.config.Messaging.Delivery.MaxAttempts != 8) {
		if !s.config.Messaging.Delivery.Enable {
			return 1
		}
		if s.config.Messaging.Delivery.MaxAttempts > 0 {
			return min(s.config.Messaging.Delivery.MaxAttempts, 8)
		}
	}
	if s.config != nil && s.config.Worker != nil && s.config.Worker.MaxRetries > 0 {
		s.logger.Warn("worker.max-retries is deprecated; use messaging.delivery.max-attempts",
			slog.Int("configured", s.config.Worker.MaxRetries), slog.Int("effective_max", 8))
		return min(s.config.Worker.MaxRetries, 8)
	}
	return 8
}

func (s *server) workerMaxInFlight() int {
	if s.config != nil && s.config.Worker != nil && s.config.Worker.Concurrency > 0 {
		return s.config.Worker.Concurrency
	}
	return 1
}
