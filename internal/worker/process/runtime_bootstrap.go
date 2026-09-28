package process

import (
	"context"
	"fmt"
	"io"
	"time"

	basemessaging "github.com/FangcunMount/component-base/pkg/messaging"
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
		if err := metrics.Start(); err != nil {
			return runtimeOutput{}, err
		}
		output.observability.metricsServer = metrics
	}

	if s.config != nil && s.config.Messaging.Provider == "nsq" {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		err := messagingintegration.EnsureChannels(ctx, s.config.Messaging, s.config.Worker.ServiceName, containerOutput.container)
		cancel()
		if err != nil {
			if output.observability.metricsServer != nil {
				_ = output.observability.metricsServer.Shutdown(context.Background())
			}
			return runtimeOutput{}, fmt.Errorf("prepare Worker NSQ channels before consuming: %w", err)
		}
	}

	deadLetterRecorder, err := eventtransport.OpenMySQLDeadLetterRecorder(s.config.MySQL)
	if err != nil {
		return runtimeOutput{}, err
	}
	subscriberConfig := eventtransport.SubscriberConfig{
		Provider: s.config.Messaging.Provider, NSQLookupdAddr: s.config.Messaging.NSQLookupdAddr,
		NSQMessageTimeout: s.config.Messaging.NSQMessageTimeout, RabbitMQURL: s.config.Messaging.RabbitMQURL,
	}
	var subscriber workerSubscriber
	var sdkSubscriber *eventtransport.SDKDeliverySubscriber
	var legacySubscriber basemessaging.Subscriber
	if subscriberConfig.Provider == "nsq" {
		sdkSubscriber, err = eventtransport.NewSDKDeliverySubscriber(
			subscriberConfig, s.workerMaxInFlight(), s.workerMaxDeliveryAttempts(),
			eventtransport.SDKFailedHandoffHandler(deadLetterRecorder),
		)
		subscriber = sdkSubscriber
	} else {
		var options basemessaging.SubscriberOptions
		options, err = eventtransport.NewSubscriberOptions(s.workerMaxInFlight(), s.workerMaxDeliveryAttempts(), eventtransport.FailedMessageHandler(deadLetterRecorder))
		if err == nil {
			legacySubscriber, err = eventtransport.NewSubscriber(subscriberConfig, options)
			subscriber = legacySubscriber
		}
	}
	if err != nil {
		_ = deadLetterRecorder.Close()
		if output.observability.metricsServer != nil {
			_ = output.observability.metricsServer.Shutdown(context.Background())
		}
		return runtimeOutput{}, err
	}
	output.messaging.subscriber = subscriber
	output.messaging.deadLetterRecorder = deadLetterRecorder
	holdStore, err := messagingintegration.NewMySQLRetryEventHoldStore(s.config.MySQL, s.config.Messaging.Provider, s.holdReplayPolicy())
	if err != nil {
		subscriber.Stop()
		_ = subscriber.Close()
		_ = deadLetterRecorder.Close()
		return runtimeOutput{}, err
	}
	output.messaging.holdStore = holdStore

	var subscribeErr error
	if sdkSubscriber != nil {
		subscribeErr = messagingintegration.SubscribeSDKHandlersWithOptions(messagingintegration.SubscribeSDKHandlersOptions{
			ServiceName: s.config.Worker.ServiceName, Logger: s.logger,
			Runtime: containerOutput.container, Subscriber: sdkSubscriber,
			HoldRecorder:    holdStore,
			UnknownRecorder: eventtransport.NewDeliveryUnknownEventRecorder("nsq", deadLetterRecorder),
		})
	} else {
		subscribeErr = messagingintegration.SubscribeHandlersWithOptions(messagingintegration.SubscribeHandlersOptions{
			ServiceName: s.config.Worker.ServiceName, Logger: s.logger,
			Runtime: containerOutput.container, Subscriber: legacySubscriber,
			HoldRecorder:    holdStore,
			UnknownRecorder: eventtransport.NewUnknownEventRecorder(s.config.Messaging.Provider, deadLetterRecorder),
		})
	}
	if subscribeErr != nil {
		subscriber.Stop()
		_ = subscriber.Close()
		_ = holdStore.Close()
		_ = deadLetterRecorder.Close()
		if output.observability.metricsServer != nil {
			_ = output.observability.metricsServer.Shutdown(context.Background())
		}
		return runtimeOutput{}, subscribeErr
	}
	if s.config.RetryGovernance == nil || s.config.RetryGovernance.AutomaticRetryEnabled {
		var publisher io.Closer
		var holdReplayer *messagingintegration.RetryEventHoldReplayer
		var publishErr error
		if s.config.Messaging.Provider == "nsq" {
			var wirePublisher messagingintegration.WirePublisherCloser
			wirePublisher, publishErr = messagingintegration.CreateSDKWirePublisher(s.config.Messaging)
			if publishErr == nil {
				publisher = wirePublisher
				holdReplayer, publishErr = messagingintegration.NewSDKRetryEventHoldReplayer(holdStore, wirePublisher)
			}
		} else {
			var legacyPublisher basemessaging.Publisher
			legacyPublisher, publishErr = messagingintegration.CreatePublisher(s.config.Messaging)
			if publishErr == nil {
				publisher = legacyPublisher
				holdReplayer = messagingintegration.NewRetryEventHoldReplayer(holdStore, legacyPublisher)
			}
		}
		if publishErr != nil {
			if publisher != nil {
				_ = publisher.Close()
			}
			subscriber.Stop()
			_ = subscriber.Close()
			_ = holdStore.Close()
			_ = deadLetterRecorder.Close()
			return runtimeOutput{}, publishErr
		}
		output.messaging.publisher = publisher
		output.messaging.holdReplayer = holdReplayer
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
