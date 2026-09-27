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
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
)

// SDKDeliverySubscriber keeps QS process lifecycle ownership while passing
// native SDK deliveries to Worker handlers. The SDK owns NSQ retry, receipt
// settlement and terminal handoff; QS owns durable failure audit.
type SDKDeliverySubscriber struct {
	inner    *rmnsq.Subscriber
	failed   func(context.Context, legacy.FailedHandoff) error
	stopOnce sync.Once
	stopDone chan struct{}
	stopErr  error
}

func NewSDKDeliverySubscriber(
	config SubscriberConfig,
	maxInFlight, maxAttempts int,
	failed func(context.Context, legacy.FailedHandoff) error,
) (*SDKDeliverySubscriber, error) {
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
	if s == nil || s.inner == nil || handler == nil {
		return errors.New("SDK delivery subscriber and handler are required")
	}
	return s.inner.Subscribe(context.Background(), topic, channel, handler, s.failed)
}

func (s *SDKDeliverySubscriber) Stop() {
	if s == nil || s.inner == nil {
		return
	}
	s.stopOnce.Do(func() {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
			defer cancel()
			s.stopErr = s.inner.Close(ctx)
			close(s.stopDone)
		}()
	})
}

func (s *SDKDeliverySubscriber) Close() error {
	if s == nil || s.inner == nil {
		return nil
	}
	s.Stop()
	<-s.stopDone
	if s.stopErr == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	return s.inner.Close(ctx)
}
