package messagingruntime

import (
	"context"
	"errors"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/options"
	"github.com/FangcunMount/reliable-messaging/message"
	"github.com/FangcunMount/reliable-messaging/transport"
	nsqtransport "github.com/FangcunMount/reliable-messaging/transport/nsq"
	"github.com/FangcunMount/reliable-messaging/transport/rocketmq"
	"github.com/nsqio/go-nsq"
)

type Publisher interface {
	transport.Publisher
	PublishRaw(context.Context, string, []byte) transport.Result
	Close(context.Context) error
	Interrupt()
}

func NewPublisher(o *options.MessagingOptions, routes map[string]string, concurrency int) (Publisher, error) {
	if o == nil {
		return nil, errors.New("explicit messaging configuration required")
	}
	if o.Provider == "rocketmq" {
		topics := []string{}
		for _, topic := range routes {
			topics = append(topics, topic)
		}
		if len(topics) == 0 {
			topics = []string{"qs.survey.lifecycle", "qs.evaluation.lifecycle", "qs.plan.task"}
		}
		config, err := o.RocketMQ.Load(topics, concurrency, 10*time.Second)
		if err != nil {
			return nil, err
		}
		p, err := rocketmq.NewPublisher(config)
		if err != nil {
			return nil, err
		}
		if err = p.Start(); err != nil {
			return nil, err
		}
		return &publicationGate{Publisher: p, paused: !o.RocketMQ.PublishingEnabled()}, nil
	}
	if o.Provider != "nsq" {
		return nil, errors.New("unsupported messaging provider")
	}
	driver := nsq.NewConfig()
	driver.DialTimeout = 5 * time.Second
	driver.HeartbeatInterval = 5 * time.Second
	driver.ReadTimeout = 15 * time.Second
	driver.WriteTimeout = 10 * time.Second
	return nsqtransport.NewManagedPublisher(nsqtransport.ManagedPublisherConfig{Address: o.NSQAddr, Driver: driver, Routes: routes, MaxInFlight: concurrency})
}

type publicationGate struct {
	Publisher
	paused bool
}

func (p *publicationGate) Publish(ctx context.Context, m message.Message) transport.Result {
	if p.paused {
		return transport.Result{Outcome: transport.Unknown}
	}
	return p.Publisher.Publish(ctx, m)
}
func (p *publicationGate) PublishRaw(ctx context.Context, topic string, body []byte) transport.Result {
	if p.paused {
		return transport.Result{Outcome: transport.Unknown}
	}
	return p.Publisher.PublishRaw(ctx, topic, body)
}

// NewSDKWirePublisher explicitly creates the selected transport at host bootstrap.
func NewSDKWirePublisher(o *options.MessagingOptions) (*sdkNSQWirePublisher, error) {
	if o == nil {
		return nil, errors.New("messaging options required")
	}
	if o.RocketMQ != nil && len(o.RocketMQ.Transactions) > 0 {
		return nil, errors.New("transaction owners cannot use the legacy Outbox wire publisher")
	}
	if o.Provider == "nsq" {
		return NewSDKNSQWirePublisher(o.NSQAddr)
	}
	p, err := NewPublisher(o, nil, 64)
	if err != nil {
		return nil, err
	}
	return &sdkNSQWirePublisher{publisher: p, provider: o.Provider, closed: make(chan struct{})}, nil
}
