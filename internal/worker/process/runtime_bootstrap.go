package process

import (
	"context"
	"fmt"
	"time"

	eventtransport "github.com/FangcunMount/qs-server/internal/pkg/eventing/transport"
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
		output.observability.metricsServer = metrics
		if err := metrics.Start(); err != nil {
			return output, err
		}
	}

	if s.config != nil && s.config.Messaging.Provider == "nsq" {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		err := messagingintegration.EnsureChannels(ctx, s.config.Messaging, s.config.Worker.ServiceName, containerOutput.container)
		cancel()
		if err != nil {
			return output, fmt.Errorf("prepare Worker NSQ channels before consuming: %w", err)
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
		eventtransport.SDKFailedHandoffHandler(deadLetterRecorder, s.config.Messaging.Provider), eventtransport.SDKRocketMQFailureHandler(deadLetterRecorder),
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

	subscribeErr := messagingintegration.SubscribeSDKHandlersWithOptions(messagingintegration.SubscribeSDKHandlersOptions{
		ServiceName: s.config.Worker.ServiceName, Logger: s.logger,
		Runtime: containerOutput.container, Subscriber: subscriber,
		HoldRecorder:    holdStore,
		UnknownRecorder: eventtransport.NewDeliveryUnknownEventRecorder(s.config.Messaging.Provider, deadLetterRecorder),
	})
	if subscribeErr != nil {
		return output, subscribeErr
	}
	if (s.config.RetryGovernance == nil || s.config.RetryGovernance.AutomaticRetryEnabled) && (s.config.Messaging.Provider != "rocketmq" || s.config.Messaging.RocketMQ.PublishingEnabled()) {
		var holdReplayer *messagingintegration.RetryEventHoldReplayer
		wirePublisher, publishErr := messagingintegration.CreateSDKWirePublisher(s.config.Messaging)
		if publishErr == nil {
			output.messaging.publisher = wirePublisher
			holdReplayer, publishErr = messagingintegration.NewSDKRetryEventHoldReplayer(holdStore, wirePublisher)
		}
		if publishErr != nil {
			return output, publishErr
		}
		output.messaging.holdReplayer = holdReplayer
		output.messaging.holdReplayer.Start()
	}

	return output, nil
}

func (s *server) loadedSubscriberConfig() eventtransport.SubscriberConfig {
	return eventtransport.SubscriberConfig{
		RocketMQ: s.config.Messaging.RocketMQ, Provider: s.config.Messaging.Provider, NSQLookupdAddr: s.config.Messaging.NSQLookupdAddr,
		NSQMessageTimeout: s.config.Messaging.NSQMessageTimeout,
		NSQDHTTPEndpoints: s.config.Messaging.NSQDHTTPEndpoints,
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
