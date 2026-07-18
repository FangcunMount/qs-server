package messaging

import (
	"context"
	"fmt"
	"sync"

	basemessaging "github.com/FangcunMount/component-base/pkg/messaging"
	"github.com/nsqio/go-nsq"
)

type governedNSQSubscriber struct {
	lookupd   []string
	config    *nsq.Config
	recorder  DeadLetterRecorder
	mu        sync.Mutex
	consumers []*nsq.Consumer
	stopped   bool
}

func newGovernedNSQSubscriber(lookupd []string, config *nsq.Config, recorder DeadLetterRecorder) (basemessaging.Subscriber, error) {
	if len(lookupd) == 0 {
		return nil, fmt.Errorf("lookupd addresses cannot be empty")
	}
	return &governedNSQSubscriber{lookupd: lookupd, config: config, recorder: recorder}, nil
}

func (s *governedNSQSubscriber) Subscribe(topic, channel string, handler basemessaging.Handler) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return fmt.Errorf("subscriber is stopped")
	}
	consumer, err := nsq.NewConsumer(topic, channel, s.config)
	if err != nil {
		return err
	}
	concurrency := s.config.MaxInFlight
	if concurrency < 1 {
		concurrency = 1
	}
	consumer.AddConcurrentHandlers(nsq.HandlerFunc(func(raw *nsq.Message) error {
		message, ok, err := basemessaging.DecodeMessagePayload(raw.Body)
		if err != nil {
			message = &basemessaging.Message{UUID: string(raw.ID[:]), Payload: raw.Body, Metadata: map[string]string{}}
		} else if !ok {
			message = &basemessaging.Message{UUID: string(raw.ID[:]), Payload: raw.Body, Metadata: map[string]string{}}
		}
		if message.UUID == "" {
			message.UUID = string(raw.ID[:])
		}
		if message.Metadata == nil {
			message.Metadata = map[string]string{}
		}
		message.Attempts, message.Timestamp, message.Topic, message.Channel = raw.Attempts, raw.Timestamp, topic, channel
		message.SetAckFunc(func() error { raw.Finish(); return nil })
		message.SetNackFunc(func() error {
			if raw.Attempts >= s.config.MaxAttempts {
				if s.recorder == nil {
					raw.Requeue(transportRetryDelay(int(raw.Attempts), message.UUID))
					return fmt.Errorf("NSQ dead-letter recorder is not configured")
				}
				if err := s.recorder.RecordDeadLetter(context.Background(), deadLetterRecord("nsq", topic, channel, int(raw.Attempts), message.UUID, message.Payload, "transport delivery exhausted")); err != nil {
					raw.Requeue(transportRetryDelay(int(raw.Attempts), message.UUID))
					return err
				}
				raw.Finish()
				return nil
			}
			raw.Requeue(transportRetryDelay(int(raw.Attempts), message.UUID))
			return nil
		})
		handlerErr := handler(context.Background(), message)
		if message.IsSettled() {
			return nil
		}
		if handlerErr != nil {
			return message.Nack()
		}
		return message.Ack()
	}), concurrency)
	if err := consumer.ConnectToNSQLookupds(s.lookupd); err != nil {
		consumer.Stop()
		return err
	}
	s.consumers = append(s.consumers, consumer)
	return nil
}

func (s *governedNSQSubscriber) SubscribeWithMiddleware(topic, channel string, handler basemessaging.Handler, middlewares ...basemessaging.Middleware) error {
	for index := len(middlewares) - 1; index >= 0; index-- {
		handler = middlewares[index](handler)
	}
	return s.Subscribe(topic, channel, handler)
}

func (s *governedNSQSubscriber) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return
	}
	s.stopped = true
	for _, consumer := range s.consumers {
		consumer.Stop()
	}
}

func (s *governedNSQSubscriber) Close() error {
	s.Stop()
	for _, consumer := range s.consumers {
		<-consumer.StopChan
	}
	return nil
}

var _ basemessaging.Subscriber = (*governedNSQSubscriber)(nil)
