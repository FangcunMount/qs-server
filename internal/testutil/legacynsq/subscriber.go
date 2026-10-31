package legacynsq

import (
	"context"
	"fmt"
	"time"

	rmtransport "github.com/FangcunMount/reliable-messaging/transport"
	rmnsq "github.com/FangcunMount/reliable-messaging/transport/nsq"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
	"github.com/nsqio/go-nsq"
)

// These fixtures exercise the SDK's retained NSQ contract. No QS host can
// construct this subscriber; current host acceptance uses RocketMQ.
type SubscriberConfig struct {
	Provider           string
	NSQLookupdAddr     string
	NSQMessageTimeout  time.Duration
	FailedHandoffGroup string
	NSQDHTTPEndpoints  []string
}

func newNSQConfig(timeout time.Duration) (*nsq.Config, error) {
	if timeout < 0 {
		return nil, fmt.Errorf("negative historical message timeout")
	}
	config := nsq.NewConfig()
	if timeout > 0 {
		config.MsgTimeout = timeout
	}
	return config, nil
}

type legacySDKDeliverySubscriber struct {
	inner  *rmnsq.Subscriber
	failed func(context.Context, legacy.FailedHandoff) error
}

func NewSubscriber(config SubscriberConfig, maxInFlight, maxAttempts int, failed func(context.Context, legacy.FailedHandoff) error) (*legacySDKDeliverySubscriber, error) {
	if config.Provider != "nsq" || failed == nil {
		return nil, fmt.Errorf("historical NSQ fixture requires its failure handler")
	}
	driver, err := newNSQConfig(config.NSQMessageTimeout)
	if err != nil {
		return nil, err
	}
	inner, err := rmnsq.NewSubscriber(rmnsq.SubscriberConfig{
		LookupdAddresses: []string{config.NSQLookupdAddr}, Driver: driver,
		MaxInFlight: maxInFlight, MaxAttempts: uint16(maxAttempts),
		FailedHandoffGroup: config.FailedHandoffGroup,
		Retry:              rmnsq.Backoff{BaseDelay: 30 * time.Second, MaxDelay: 5 * time.Minute, JitterFraction: 0.2},
	})
	if err != nil {
		return nil, err
	}
	return &legacySDKDeliverySubscriber{inner: inner, failed: failed}, nil
}

func (s *legacySDKDeliverySubscriber) Subscribe(topic, channel string, handler rmtransport.Handler) error {
	return s.inner.Subscribe(context.Background(), topic, channel, handler, s.failed)
}

func (s *legacySDKDeliverySubscriber) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	return s.inner.Close(ctx)
}

func (s *legacySDKDeliverySubscriber) Stop() { _ = s.Close() }
