package transport

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	rmtransport "github.com/FangcunMount/reliable-messaging/transport"
	"github.com/FangcunMount/reliable-messaging/transport/rocketmq"
)

// SDKDeliverySubscriber keeps QS process lifecycle ownership while passing
// native RocketMQ deliveries to Worker handlers. The SDK owns visibility
// renewal, receipt settlement and terminal handoff; QS owns durable failure audit.
type SDKDeliverySubscriber struct {
	rocket       *rocketmq.Subscriber
	rocketFailed func(context.Context, rmtransport.Failure) error
	stopOnce     sync.Once
	stopDone     chan struct{}
	stopErr      error
}

func NewSDKDeliverySubscriber(
	config SubscriberConfig, maxInFlight, maxAttempts int,
	failed func(context.Context, rmtransport.Failure) error,
) (*SDKDeliverySubscriber, error) {

	if config.Provider != "rocketmq" {
		return nil, errors.New("QS delivery requires RocketMQ; NSQ is retired")
	}
	if failed == nil {
		return nil, errors.New("provider-aware RocketMQ durable failure handler required")
	}
	if maxAttempts < 1 || maxAttempts > retrygovernance.HardMaxDeliveryAttempts {
		return nil, fmt.Errorf("bounded attempts and durable failure handler required")
	}
	c, err := config.RocketMQ.Load(nil, max(1, maxInFlight), 5*time.Second)
	if err != nil {
		return nil, err
	}
	deadline := config.HandlerTimeout
	if deadline <= 0 {
		deadline = 9 * time.Minute
	}
	inner, err := rocketmq.NewSubscriber(rocketmq.SubscriberConfig{Config: c, MaxAttempts: uint16(maxAttempts), InvisibleDuration: 60 * time.Second, HandlerTimeout: deadline, RetryDelay: 30 * time.Second})
	if err != nil {
		return nil, err
	}
	return &SDKDeliverySubscriber{rocket: inner, rocketFailed: failed, stopDone: make(chan struct{})}, nil
}

func (s *SDKDeliverySubscriber) Subscribe(topic, channel string, handler rmtransport.Handler) error {
	if s == nil || s.rocket == nil || handler == nil {

		return errors.New("SDK delivery subscriber and handler are required")
	}
	return s.rocket.Subscribe(context.Background(), topic, channel, handler, s.rocketFailed)
}

func (s *SDKDeliverySubscriber) Stop() {
	if s == nil || s.rocket == nil {
		return
	}
	s.stopOnce.Do(func() {

		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
			defer cancel()
			s.stopErr = s.rocket.Close(ctx)
			close(s.stopDone)
		}()
	})
}

func (s *SDKDeliverySubscriber) Close() error {
	if s == nil || s.rocket == nil {
		return nil
	}
	s.Stop()
	<-s.stopDone
	if s.stopErr == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	return s.rocket.Close(ctx)
}

// RocketMQReady checks recent successful receives, including empty polls, on
// every registered physical subscription group. It is transport readiness,
// independent of business acceptance or transaction-witness completion.
func (s *SDKDeliverySubscriber) RocketMQReady(maxAge time.Duration) bool {
	return s != nil && s.rocket != nil && s.rocket.Ready(maxAge)
}

// RocketMQErrors is the existing bounded SDK supervision stream. Reading it
// creates no worker or retry and carries no business recovery authorization.
func (s *SDKDeliverySubscriber) RocketMQErrors() <-chan error {
	if s == nil || s.rocket == nil {
		return nil
	}
	return s.rocket.Errors()
}
