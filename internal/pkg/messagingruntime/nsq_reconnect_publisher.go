package messagingruntime

import (
	"context"
	"errors"
	"time"

	basemessaging "github.com/FangcunMount/component-base/pkg/messaging"
	"github.com/FangcunMount/qs-server/internal/pkg/retryobservability"
	"github.com/nsqio/go-nsq"
)

var defaultNSQReconnectDelays = []time.Duration{
	25 * time.Millisecond,
	50 * time.Millisecond,
	100 * time.Millisecond,
}

// WrapNSQReconnectPublisher absorbs the short go-nsq reconnect window without
// hiding real transport failures. go-nsq resets a disconnected producer back
// to its reconnectable state asynchronously; concurrent publishes made during
// that transition otherwise all fail immediately with ErrNotConnected.
func WrapNSQReconnectPublisher(delegate basemessaging.Publisher) basemessaging.Publisher {
	if delegate == nil {
		return nil
	}
	return newNSQReconnectPublisher(delegate, defaultNSQReconnectDelays, waitForContext)
}

type nsqReconnectPublisher struct {
	delegate basemessaging.Publisher
	delays   []time.Duration
	wait     func(context.Context, time.Duration) error
}

func newNSQReconnectPublisher(
	delegate basemessaging.Publisher,
	delays []time.Duration,
	wait func(context.Context, time.Duration) error,
) *nsqReconnectPublisher {
	return &nsqReconnectPublisher{
		delegate: delegate,
		delays:   append([]time.Duration(nil), delays...),
		wait:     wait,
	}
}

func (p *nsqReconnectPublisher) Publish(ctx context.Context, topic string, body []byte) error {
	return p.publishWithReconnect(ctx, func() error {
		return p.delegate.Publish(ctx, topic, body)
	})
}

func (p *nsqReconnectPublisher) PublishMessage(ctx context.Context, topic string, msg *basemessaging.Message) error {
	return p.publishWithReconnect(ctx, func() error {
		return p.delegate.PublishMessage(ctx, topic, msg)
	})
}

func (p *nsqReconnectPublisher) Close() error {
	return p.delegate.Close()
}

func (p *nsqReconnectPublisher) publishWithReconnect(ctx context.Context, publish func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	attempt := 1
	err := publish()
	observeNSQPublishAttempt(attempt, err)
	for _, delay := range p.delays {
		if !errors.Is(err, nsq.ErrNotConnected) {
			return err
		}
		if waitErr := p.wait(ctx, delay); waitErr != nil {
			return waitErr
		}
		attempt++
		err = publish()
		observeNSQPublishAttempt(attempt, err)
	}
	return err
}

func observeNSQPublishAttempt(attempt int, err error) {
	outcome := retryobservability.OutcomeSuccess
	if err != nil {
		outcome = retryobservability.OutcomeFailure
	}
	retryobservability.Observe(
		retryobservability.LayerTransport,
		"transport",
		retryobservability.AttemptClassForAttempt(attempt),
		"na",
		outcome,
	)
}

func waitForContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
