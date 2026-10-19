package transport

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	rmtransport "github.com/FangcunMount/reliable-messaging/transport"
	rmnsq "github.com/FangcunMount/reliable-messaging/transport/nsq"
	"github.com/FangcunMount/reliable-messaging/transport/rocketmq"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
)

// SDKDeliverySubscriber keeps QS process lifecycle ownership while passing
// native SDK deliveries to Worker handlers. The SDK owns NSQ retry, receipt
// settlement and terminal handoff; QS owns durable failure audit.
type SDKDeliverySubscriber struct {
	rocket       *rocketmq.Subscriber
	rocketFailed func(context.Context, rmtransport.Failure) error
	inner        sdkDeliverySource
	failed       func(context.Context, legacy.FailedHandoff) error
	stopOnce     sync.Once
	stopDone     chan struct{}
	stopErr      error
}

type sdkDeliverySource interface {
	Subscribe(context.Context, string, string, rmtransport.Handler, func(context.Context, legacy.FailedHandoff) error) error
	Close(context.Context) error
}

func NewSDKDeliverySubscriber(
	config SubscriberConfig, maxInFlight, maxAttempts int,
	failed func(context.Context, legacy.FailedHandoff) error,
	rocketFailed ...func(context.Context, rmtransport.Failure) error,
) (result *SDKDeliverySubscriber, resultErr error) {

	if config.Provider == "rocketmq" {
		if len(rocketFailed) != 1 || rocketFailed[0] == nil {
			return nil, errors.New("provider-aware RocketMQ durable failure handler required")
		}
		if maxAttempts < 1 || maxAttempts > retrygovernance.HardMaxDeliveryAttempts || failed == nil {
			return nil, fmt.Errorf("bounded attempts and durable failure handler required")
		}
		c, err := config.RocketMQ.Load(nil, max(1, maxInFlight), 5*time.Second)
		if err != nil {
			return nil, err
		}
		deadline := config.NSQMessageTimeout
		if deadline <= 0 {
			deadline = 9 * time.Minute
		}
		inner, err := rocketmq.NewSubscriber(rocketmq.SubscriberConfig{Config: c, MaxAttempts: uint16(maxAttempts), InvisibleDuration: 60 * time.Second, HandlerTimeout: deadline, RetryDelay: 30 * time.Second})
		if err != nil {
			return nil, err
		}
		return &SDKDeliverySubscriber{rocket: inner, rocketFailed: rocketFailed[0], failed: failed, stopDone: make(chan struct{})}, nil
	}
	if config.Provider != "nsq" || config.NSQLookupdAddr == "" {
		return nil, errors.New("NSQ lookupd is required for SDK delivery subscriber")
	}
	if maxAttempts < 1 || maxAttempts > retrygovernance.HardMaxDeliveryAttempts || failed == nil {
		return nil, fmt.Errorf("bounded attempts and durable failed-message handler are required")
	}
	driverConfig, err := newNSQConfig(config.NSQMessageTimeout)
	if err != nil {
		return nil, err
	}

	inner, err := rmnsq.NewSubscriber(rmnsq.SubscriberConfig{
		LookupdAddresses: []string{config.NSQLookupdAddr}, Driver: driverConfig,
		MaxInFlight: maxInFlight, MaxAttempts: uint16(maxAttempts),
		FailedHandoffGroup: config.FailedHandoffGroup,
		Retry: rmnsq.Backoff{
			BaseDelay: DefaultRetryBaseDelay, MaxDelay: DefaultRetryMaxDelay,
			JitterFraction: DefaultRetryJitter,
		},
	})
	if err != nil {

		return nil, err
	}
	return &SDKDeliverySubscriber{inner: inner, failed: failed, stopDone: make(chan struct{})}, nil
}

func (s *SDKDeliverySubscriber) Subscribe(topic, channel string, handler rmtransport.Handler) error {
	if s == nil || (s.inner == nil && s.rocket == nil) || handler == nil {

		return errors.New("SDK delivery subscriber and handler are required")
	}
	if s.rocket != nil {
		return s.rocket.Subscribe(context.Background(), topic, channel, handler, s.rocketFailed)
	}
	err := s.inner.Subscribe(context.Background(), topic, channel, handler, s.failed)

	return err
}

func (s *SDKDeliverySubscriber) Stop() {
	if s == nil || (s.inner == nil && s.rocket == nil) {
		return
	}
	s.stopOnce.Do(func() {

		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
			defer cancel()
			s.stopErr = s.closeInner(ctx)
			close(s.stopDone)
		}()
	})
}

func (s *SDKDeliverySubscriber) Close() error {
	if s == nil || (s.inner == nil && s.rocket == nil) {
		return nil
	}
	s.Stop()
	<-s.stopDone
	if s.stopErr == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	return s.closeInner(ctx)
}

func (s *SDKDeliverySubscriber) closeInner(ctx context.Context) error {
	if s.rocket != nil {
		return s.rocket.Close(ctx)
	}
	return s.inner.Close(ctx)
}
