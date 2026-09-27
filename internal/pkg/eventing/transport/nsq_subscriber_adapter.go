package transport

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	basemessaging "github.com/FangcunMount/component-base/pkg/messaging"
	rmtransport "github.com/FangcunMount/reliable-messaging/transport"
	rmnsq "github.com/FangcunMount/reliable-messaging/transport/nsq"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
)

// nsqSubscriberAdapter keeps the existing QS business handler contract while
// the SDK owns NSQ receipt, retry and terminal handoff. The business handler,
// durable hold and unknown-event audit remain QS responsibilities.
type nsqSubscriberAdapter struct {
	subscriber *rmnsq.Subscriber
	failed     basemessaging.FailedMessageHandler
	stopOnce   sync.Once
	stopDone   chan struct{}
	stopErr    error
}

func newNSQSubscriber(config rmnsq.SubscriberConfig, failed basemessaging.FailedMessageHandler) (basemessaging.Subscriber, error) {
	if failed == nil {
		return nil, errors.New("durable failed-message handler required")
	}
	subscriber, err := rmnsq.NewSubscriber(config)
	if err != nil {
		return nil, err
	}
	return &nsqSubscriberAdapter{subscriber: subscriber, failed: failed, stopDone: make(chan struct{})}, nil
}

func (s *nsqSubscriberAdapter) Subscribe(topic, channel string, handler basemessaging.Handler) error {
	if handler == nil {
		return errors.New("business handler required")
	}
	return s.subscriber.Subscribe(context.Background(), topic, channel, func(ctx context.Context, delivery rmtransport.Delivery) error {
		received := delivery.Message()
		msg := basemessaging.NewMessage(received.ID, received.Payload)
		msg.Metadata = received.Metadata
		msg.TransportMessageID = received.TransportID
		msg.Topic, msg.Channel = received.Topic, received.Channel
		msg.Attempts, msg.Timestamp = received.Attempts, received.Timestamp
		msg.SetAckFunc(delivery.Ack)
		msg.SetNackFunc(func() error { return delivery.Nack(errors.New("business handler rejected delivery")) })
		return handler(ctx, msg)
	}, func(ctx context.Context, handoff legacy.FailedHandoff) error {
		msg := basemessaging.NewMessage(handoff.UUID, handoff.Payload)
		msg.Metadata = handoff.Metadata
		msg.TransportMessageID = handoff.TransportMessageID
		msg.Topic, msg.Channel = handoff.Topic, handoff.Channel
		msg.Attempts, msg.Timestamp = uint16(handoff.Attempts), handoff.Timestamp
		return s.failed(ctx, basemessaging.FailedMessage{
			Provider: "nsq", Topic: handoff.Topic, Channel: handoff.Channel,
			Message: msg, Attempts: handoff.Attempts, Cause: errors.New(handoff.Cause),
		})
	})
}

func (s *nsqSubscriberAdapter) SubscribeWithMiddleware(topic, channel string, handler basemessaging.Handler, middlewares ...basemessaging.Middleware) error {
	for i := len(middlewares) - 1; i >= 0; i-- {
		if middlewares[i] == nil {
			return fmt.Errorf("nil messaging middleware at index %d", i)
		}
		handler = middlewares[i](handler)
	}
	return s.Subscribe(topic, channel, handler)
}

func (s *nsqSubscriberAdapter) Stop() {
	s.stopOnce.Do(func() {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
			defer cancel()
			s.stopErr = s.subscriber.Close(ctx)
			close(s.stopDone)
		}()
	})
}

func (s *nsqSubscriberAdapter) Close() error {
	s.Stop()
	<-s.stopDone
	if s.stopErr == nil {
		return nil
	}
	// The SDK close is retryable after a timeout; keep the same stopped
	// subscriber and give in-flight handoff sends one more bounded drain.
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	return s.subscriber.Close(ctx)
}
