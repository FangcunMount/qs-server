package messagingruntime

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

var (
	ErrNSQPublishUnknown  = errors.New("NSQ publish outcome unknown; retain original message identity")
	ErrNSQPublishRejected = errors.New("NSQ publish rejected before delivery")
)

// sdkNSQPublisher is a temporary QS business-port bridge. The SDK owns the
// NSQ producer, bounded sends and their confirmation classification.
type sdkNSQPublisher struct {
	publisher *rmnsq.ManagedPublisher
	closeOnce sync.Once
	closed    chan struct{}
	closeErr  error
}

func NewSDKNSQPublisher(address string) (basemessaging.Publisher, error) {
	publisher, err := rmnsq.NewManagedPublisher(rmnsq.ManagedPublisherConfig{Address: address, MaxInFlight: 64})
	if err != nil {
		return nil, err
	}
	return &sdkNSQPublisher{publisher: publisher, closed: make(chan struct{})}, nil
}

func (p *sdkNSQPublisher) Publish(ctx context.Context, topic string, body []byte) error {
	return p.PublishWire(ctx, topic, body)
}

// PublishWire sends the caller's complete NSQ wire envelope without building
// or mutating a component-base Message. Unknown retains the original identity.
func (p *sdkNSQPublisher) PublishWire(ctx context.Context, topic string, body []byte) error {
	return classifyNSQPublish(topic, p.publisher.PublishRaw(ctx, topic, body))
}

func (p *sdkNSQPublisher) PublishMessage(ctx context.Context, topic string, msg *basemessaging.Message) error {
	if msg == nil {
		return errors.New("message is nil")
	}
	body, err := legacy.Encode(legacy.Envelope{UUID: msg.UUID, Metadata: msg.Metadata, Payload: msg.Payload}, legacy.Revision2)
	if err != nil {
		return err
	}
	return p.Publish(ctx, topic, body)
}

func classifyNSQPublish(topic string, result rmtransport.Result) error {
	switch result.Outcome {
	case rmtransport.Confirmed:
		return nil
	case rmtransport.Rejected:
		return fmt.Errorf("%w: topic %s", ErrNSQPublishRejected, topic)
	default:
		// A timed-out or failed driver call may already have reached NSQ. The
		// caller's durable intent must retain its UUID and bytes for recovery.
		return fmt.Errorf("%w: topic %s", ErrNSQPublishUnknown, topic)
	}
}

func (p *sdkNSQPublisher) Close() error {
	p.closeOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		p.closeErr = p.publisher.Close(ctx)
		cancel()
		if p.closeErr != nil {
			p.publisher.Interrupt()
			ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
			p.closeErr = errors.Join(p.closeErr, p.publisher.Close(ctx))
			cancel()
		}
		close(p.closed)
	})
	<-p.closed
	return p.closeErr
}
