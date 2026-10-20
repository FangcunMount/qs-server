package messagingruntime

import (
	"context"
	"errors"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/options"
	"github.com/FangcunMount/reliable-messaging/transport"
	"github.com/FangcunMount/reliable-messaging/transport/rocketmq"
)

// NormalWirePublisher is the explicit best-effort side of a mixed owner host.
// It accepts complete wire bytes and cannot publish transaction branches.
// Construction is inert; the host starts and closes this client separately.
type NormalWirePublisher struct {
	publisher *rocketmq.Publisher
	paused    bool
}

func NewNormalWirePublisher(o *options.MessagingOptions, topics []string, concurrency int) (*NormalWirePublisher, error) {
	if o == nil || o.Provider != "rocketmq" || o.RocketMQ == nil || len(topics) == 0 || concurrency < 1 {
		return nil, errors.New("explicit RocketMQ NORMAL topics and budget required")
	}
	config, err := o.RocketMQ.Load(topics, concurrency, 5*time.Second)
	if err != nil {
		return nil, err
	}
	config.Routes, err = normalRoutes(config.Routes, topics)
	if err != nil {
		return nil, err
	}
	publisher, err := rocketmq.NewPublisher(config)
	if err != nil {
		return nil, err
	}
	return &NormalWirePublisher{publisher: publisher, paused: !o.RocketMQ.PublishingEnabled()}, nil
}

func normalRoutes(routes []transport.RouteBinding, topics []string) ([]transport.RouteBinding, error) {
	selected := make([]transport.RouteBinding, 0, len(routes))
	seen := make(map[string]bool)
	for _, route := range routes {
		if route.MessageType != transport.NormalMessageType || route.RouteRevision != "m3-1" {
			continue
		}
		for _, topic := range topics {
			if route.Topic == topic {
				selected = append(selected, route)
				seen[topic] = true
				break
			}
		}
	}
	for _, topic := range topics {
		if !seen[topic] {
			return nil, errors.New("required m3-1 NORMAL topic absent")
		}
	}
	return selected, nil
}

func (p *NormalWirePublisher) Start() error { return p.publisher.Start() }

func (p *NormalWirePublisher) PublishWire(ctx context.Context, topic string, wire []byte) error {
	if p.paused {
		return errors.New("NORMAL publication admission closed")
	}
	if p.publisher.PublishRaw(ctx, topic, wire).Outcome != transport.Confirmed {
		return errors.New("NORMAL publication unconfirmed")
	}
	return nil
}

func (p *NormalWirePublisher) Close(ctx context.Context) error { return p.publisher.Close(ctx) }
