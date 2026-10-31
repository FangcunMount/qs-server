package process

import (
	"context"
	"time"

	eventtransport "github.com/FangcunMount/qs-server/internal/pkg/eventing/transport"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	messagingintegration "github.com/FangcunMount/qs-server/internal/worker/integration/messaging"
	observability "github.com/FangcunMount/qs-server/internal/worker/observability"
)

func (s *server) initializeRuntime(resources resourceOutput, containerOutput containerOutput) (runtimeOutput, error) {
	output := runtimeOutput{}
	if err := s.validateFinalMessaging(); err != nil {
		return output, err
	}
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
		output.observability.metricsServer = metrics
		if err := metrics.Start(); err != nil {
			return output, err
		}
	}

	deadLetterRecorder, err := eventtransport.OpenMySQLDeadLetterRecorder(s.config.MySQL)
	if err != nil {
		return output, err
	}
	output.messaging.deadLetterRecorder = deadLetterRecorder
	subscriberConfig := s.loadedSubscriberConfig()
	subscriber, err := eventtransport.NewSDKDeliverySubscriber(
		subscriberConfig, s.workerMaxInFlight(), s.workerMaxDeliveryAttempts(),
		eventtransport.SDKRocketMQFailureHandler(deadLetterRecorder),
	)
	if err != nil {
		return output, err
	}
	output.messaging.subscriber = subscriber
	holdStore, err := messagingintegration.NewMySQLRetryEventHoldStore(s.config.MySQL, s.config.Messaging.Provider, s.holdReplayPolicy())
	if err != nil {
		return output, err
	}
	output.messaging.holdStore = holdStore
	var holdRecorder messagingintegration.DeliveryRetryEventHoldRecorder = holdStore
	if s.consumeOnlyRocketMQ() {
		// Load the same fixed host route file; message properties cannot select
		// stores, connections, topics or business retry authorization.
		config, loadErr := s.config.Messaging.RocketMQ.Load(nil, s.workerMaxInFlight(), 5*time.Second)
		if loadErr != nil {
			return output, loadErr
		}
		deferredStore, storeErr := messagingintegration.NewRocketMQDeferredStoreWithHistory(holdStore, s.config.Worker.ServiceName, s.workerMaxDeliveryAttempts(), config.Routes, config.HistoricalRoutes)
		if storeErr != nil {
			return output, storeErr
		}
		if storeErr = deferredStore.Validate(context.Background()); storeErr != nil {
			return output, storeErr
		}
		holdRecorder = deferredStore
		timeout := s.config.Messaging.NSQMessageTimeout
		if timeout <= 0 {
			timeout = 9 * time.Minute
		}
		consumer, consumerErr := messagingintegration.NewDeferredConsumer(deferredStore, containerOutput.container, messagingintegration.DefaultDeferredConsumerOptions(timeout))
		if consumerErr != nil {
			return output, consumerErr
		}
		output.messaging.deferredConsumer = consumer
	}

	subscribeErr := messagingintegration.SubscribeSDKHandlersWithOptions(messagingintegration.SubscribeSDKHandlersOptions{
		ServiceName: s.config.Worker.ServiceName, Logger: s.logger,
		Runtime: containerOutput.container, Subscriber: subscriber,
		HoldRecorder:    holdRecorder,
		UnknownRecorder: eventtransport.NewDeliveryUnknownEventRecorder(s.config.Messaging.Provider, deadLetterRecorder),
	})
	if subscribeErr != nil {
		return output, subscribeErr
	}
	automatic := s.config.RetryGovernance == nil || s.config.RetryGovernance.AutomaticRetryEnabled
	if automatic && output.messaging.deferredConsumer != nil {
		if startErr := output.messaging.deferredConsumer.Start(context.Background()); startErr != nil {
			return output, startErr
		}
	}

	return output, nil
}

func (s *server) consumeOnlyRocketMQ() bool {
	return s != nil && s.config != nil && s.config.Messaging != nil && s.config.Messaging.Provider == "rocketmq" && s.config.Messaging.RocketMQ != nil && !s.config.Messaging.RocketMQ.PublishingEnabled()
}

func (s *server) loadedSubscriberConfig() eventtransport.SubscriberConfig {
	return eventtransport.SubscriberConfig{
		RocketMQ: s.config.Messaging.RocketMQ, Provider: "rocketmq",
		HandlerTimeout: s.config.Messaging.NSQMessageTimeout,
	}
}

func (s *server) holdReplayPolicy() retrygovernance.Policy {
	// Hold replay has its own budget and scheduling contract. Do not inherit
	// changes to the API Outbox's recovery scheduling ceiling.
	policy := retrygovernance.Policy{
		Version:              "retry-hold-publish/v1",
		MaxAutomaticAttempts: retrygovernance.HardMaxOutboxAttempts,
		BaseDelay:            10 * time.Second,
		MaxDelay:             time.Hour,
		JitterFraction:       0.20,
	}
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
	return 8
}

func (s *server) workerMaxInFlight() int {
	if s.config != nil && s.config.Worker != nil && s.config.Worker.Concurrency > 0 {
		return s.config.Worker.Concurrency
	}
	return 1
}
