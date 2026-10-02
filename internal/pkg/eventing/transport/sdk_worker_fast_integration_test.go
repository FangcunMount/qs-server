//go:build integration

package transport

import (
	"context"
	"time"

	rmtransport "github.com/FangcunMount/reliable-messaging/transport"
	rmnsq "github.com/FangcunMount/reliable-messaging/transport/nsq"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
)

// fastSDKWorkerSubscriber keeps the production SDK delivery and handoff
// semantics while shortening retry delay in disposable broker tests.
type fastSDKWorkerSubscriber struct {
	inner  *rmnsq.Subscriber
	failed func(context.Context, legacy.FailedHandoff) error
}

func newFastSDKWorkerSubscriber(config SubscriberConfig, attempts int, delay time.Duration, recorder DeadLetterRecorder) (*fastSDKWorkerSubscriber, error) {
	return newFastSDKWorkerSubscriberWithRetry(config, attempts, rmnsq.Backoff{BaseDelay: delay, MaxDelay: delay}, recorder)
}

func newFastSDKWorkerSubscriberWithRetry(config SubscriberConfig, attempts int, retry rmnsq.Backoff, recorder DeadLetterRecorder) (*fastSDKWorkerSubscriber, error) {
	driver, err := newNSQConfig(config.NSQMessageTimeout)
	if err != nil {
		return nil, err
	}
	inner, err := rmnsq.NewSubscriber(rmnsq.SubscriberConfig{
		LookupdAddresses: []string{config.NSQLookupdAddr}, Driver: driver,
		MaxInFlight: 1, MaxAttempts: uint16(attempts),
		Retry:              retry,
		FailedHandoffGroup: config.FailedHandoffGroup,
	})
	if err != nil {
		return nil, err
	}
	return &fastSDKWorkerSubscriber{inner: inner, failed: SDKFailedHandoffHandler(recorder)}, nil
}

func (s *fastSDKWorkerSubscriber) Subscribe(topic, channel string, handler rmtransport.Handler) error {
	return s.inner.Subscribe(context.Background(), topic, channel, handler, s.failed)
}

func (s *fastSDKWorkerSubscriber) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	return s.inner.Close(ctx)
}

func publishSDKTestEnvelope(ctx context.Context, publisher interface {
	PublishWire(context.Context, string, []byte) error
}, topic string, envelope legacy.Envelope) error {
	wire, err := legacy.Encode(envelope, legacy.Revision2)
	if err != nil {
		return err
	}
	return publisher.PublishWire(ctx, topic, wire)
}
